// Package ffi provides FFI bindings to the libwebrtc shim library.
// It supports both purego (default) and CGO backends via build tags.
package ffi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrLibraryNotLoaded is returned when the shim library hasn't been loaded.
	ErrLibraryNotLoaded = errors.New("libwebrtc_shim library not loaded")

	// ErrLibraryNotFound is returned when the shim library cannot be found.
	ErrLibraryNotFound = errors.New("libwebrtc_shim library not found")

	// FFI error sentinels - these match shim error codes and support errors.Is().
	ErrInvalidParam        = errors.New("invalid parameter")
	ErrInitFailed          = errors.New("initialization failed")
	ErrEncodeFailed        = errors.New("encode failed")
	ErrDecodeFailed        = errors.New("decode failed")
	ErrOutOfMemory         = errors.New("out of memory")
	ErrNotSupported        = errors.New("not supported")
	ErrNeedMoreData        = errors.New("need more data")
	ErrBufferTooSmall      = errors.New("buffer too small")
	ErrNotFound            = errors.New("not found")
	ErrRenegotiationNeeded = errors.New("renegotiation needed")
)

// Error codes from shim (int32 to match C int)
const (
	ShimOK                     int32 = 0
	ShimErrInvalidParam        int32 = -1
	ShimErrInitFailed          int32 = -2
	ShimErrEncodeFailed        int32 = -3
	ShimErrDecodeFailed        int32 = -4
	ShimErrOutOfMemory         int32 = -5
	ShimErrNotSupported        int32 = -6
	ShimErrNeedMoreData        int32 = -7
	ShimErrBufferTooSmall      int32 = -8
	ShimErrNotFound            int32 = -9
	ShimErrRenegotiationNeeded int32 = -10
)

// CodecType matches ShimCodecType in shim.h (int32 to match C int)
type CodecType int32

const (
	CodecH264 CodecType = 0
	CodecVP8  CodecType = 1
	CodecVP9  CodecType = 2
	CodecAV1  CodecType = 3
	CodecOpus CodecType = 10
)

var (
	libHandle uintptr
	libLoaded atomic.Bool // Use atomic for lock-free reads
	libMu     sync.Mutex  // Still used for load/unload operations
)

var (
	resolveLibraryFunc            = resolveLibrary
	downloadShimFunc              = downloadShim
	dlopenLibraryFunc             = dlopenLibrary
	dlcloseLibraryFunc            = dlcloseLibrary
	retryLoadLibraryFunc          = retryLoadLibrary
	registerFunctionsFunc         = registerFunctions
	checkLoadedLibraryVersionFunc = checkLoadedLibraryVersion
)

// Function pointers are defined in func_vars.go and populated by registerFunctions() in
// either func_bind_purego.go or func_bind_cgo.go depending on build tags.

// LoadLibrary loads the libwebrtc_shim shared library.
// It resolves the library in the following order:
// 1. Exact path specified by LIBWEBRTC_SHIM_PATH (authoritative if set)
// 2. ./lib/{os}_{arch}/ (module-relative)
// 3. Auto-download from GitHub Releases (unless disabled)
// 4. System library paths
func LoadLibrary() error {
	libMu.Lock()
	defer libMu.Unlock()

	if libLoaded.Load() {
		return nil
	}

	if shouldPreferSoftwareCodecs() {
		if err := ensureOpenH264(true); err != nil {
			return err
		}
	} else {
		if err := preloadOpenH264Optional(); err != nil {
			return fmt.Errorf("preload openh264: %w", err)
		}
	}

	// On Linux, preload system libraries that the shim depends on
	if runtime.GOOS == "linux" {
		preloadLinuxDeps()
	}

	libPath, downloadErr, err := resolveLibraryFunc()
	if err != nil {
		return err
	}

	handle, resolvedPath, err := openLibraryHandle(libPath)
	if err != nil {
		if fallbackHandle, fallbackPath, fallbackErr := tryFallbackToDownloadedShim(libPath, err); fallbackErr == nil {
			handle = fallbackHandle
			libPath = fallbackPath
			err = nil
		} else {
			libPath = fallbackPath
			err = fallbackErr
		}
	} else {
		libPath = resolvedPath
	}
	if err != nil {
		return wrapLibraryLoadError(libPath, err, downloadErr)
	}

	libHandle = handle
	if err := initializeLoadedLibrary(); err != nil {
		_ = dlcloseLibraryFunc(handle)
		libHandle = 0
		if fallbackHandle, fallbackPath, fallbackErr := tryFallbackToDownloadedShim(libPath, err); fallbackErr == nil {
			libHandle = fallbackHandle
			handle = fallbackHandle
			libPath = fallbackPath
		} else {
			return fallbackErr
		}
	}

	libLoaded.Store(true)
	return nil
}

// MustLoadLibrary loads the library and panics on failure.
func MustLoadLibrary() {
	if err := LoadLibrary(); err != nil {
		panic(fmt.Sprintf("libgowebrtc: %v", err))
	}
}

// IsLoaded returns true if the shim library is loaded.
// Thread-safe due to atomic.Bool.
func IsLoaded() bool {
	return libLoaded.Load()
}

// Close unloads the shim library.
func Close() error {
	libMu.Lock()
	defer libMu.Unlock()

	if !libLoaded.Load() {
		return nil
	}

	if err := dlcloseLibraryFunc(libHandle); err != nil {
		return err
	}

	libLoaded.Store(false)
	libHandle = 0
	return nil
}

// ExpectedLibWebRTCVersion is the libwebrtc version this Go code expects.
// Must match kLibWebRTCVersion in shim/shim_common.cc.
const ExpectedLibWebRTCVersion = "M141"

// ExpectedShimVersion is the shim API version this Go code expects.
// Must match kShimVersion in shim/shim_common.cc.
const ExpectedShimVersion = "0.5.1"

// ErrVersionMismatch is returned when the shim version doesn't match.
var ErrVersionMismatch = errors.New("shim version mismatch")

// ShimVersion returns the shim library version.
// Returns empty string if library is not loaded.
func ShimVersion() string {
	if !libLoaded.Load() {
		return ""
	}
	return currentShimVersion()
}

func currentShimVersion() string {
	ptr := shimVersion()
	if ptr == 0 {
		return ""
	}
	return GoStringFromC(ptr)
}

// LibWebRTCVersion returns the libwebrtc version the shim was built with.
// Returns empty string if library is not loaded.
func LibWebRTCVersion() string {
	if !libLoaded.Load() {
		return ""
	}
	return currentLibWebRTCVersion()
}

func currentLibWebRTCVersion() string {
	ptr := shimLibwebrtcVersion()
	if ptr == 0 {
		return ""
	}
	return GoStringFromC(ptr)
}

// CheckVersion verifies the shim version matches what this Go code expects.
// Returns nil if versions match, ErrVersionMismatch otherwise.
func CheckVersion() error {
	if !libLoaded.Load() {
		return ErrLibraryNotLoaded
	}

	return checkLoadedLibraryVersion()
}

func checkLoadedLibraryVersion() error {
	shimVer := currentShimVersion()
	webrtcVer := currentLibWebRTCVersion()

	if shimVer != ExpectedShimVersion {
		return fmt.Errorf("%w: shim version %q, expected %q", ErrVersionMismatch, shimVer, ExpectedShimVersion)
	}
	if webrtcVer != ExpectedLibWebRTCVersion {
		return fmt.Errorf("%w: libwebrtc version %q, expected %q", ErrVersionMismatch, webrtcVer, ExpectedLibWebRTCVersion)
	}
	return nil
}

func findLocalLibrary() (string, bool, error) {
	// An explicit shim path is authoritative. If it is configured but invalid,
	// fail fast instead of silently falling back to a different library.
	if path := os.Getenv("LIBWEBRTC_SHIM_PATH"); path != "" {
		resolvedPath := path
		if absPath, err := filepath.Abs(path); err == nil {
			resolvedPath = absPath
		}

		info, err := os.Stat(resolvedPath)
		switch {
		case err == nil && !info.IsDir():
			return resolvedPath, true, nil
		case err == nil && info.IsDir():
			return resolvedPath, false, fmt.Errorf("%w: LIBWEBRTC_SHIM_PATH must point to a file, got directory %s", ErrLibraryNotFound, resolvedPath)
		case err != nil:
			return resolvedPath, false, fmt.Errorf("%w: inspect LIBWEBRTC_SHIM_PATH %s: %w", ErrLibraryNotFound, resolvedPath, err)
		}
	}

	libName := getLibraryName()
	platformDir := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)

	// Build search paths
	var searchPaths []string

	// Check relative to executable
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		searchPaths = append(searchPaths, filepath.Join(execDir, "lib", platformDir, libName))
	}

	// Check working directory
	if wd, err := os.Getwd(); err == nil {
		searchPaths = append(searchPaths,
			filepath.Join(wd, "lib", platformDir, libName),
			filepath.Join(wd, "..", "lib", platformDir, libName),
			filepath.Join(wd, "..", "..", "lib", platformDir, libName),
		)
	}

	// Check relative to this source file (for development/testing)
	// This finds lib/ relative to the Go module root
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		// thisFile is .../internal/ffi/lib.go, go up to module root
		moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
		searchPaths = append(searchPaths, filepath.Join(moduleRoot, "lib", platformDir, libName))
	}

	// Standard paths
	searchPaths = append(searchPaths,
		filepath.Join(".", "lib", platformDir, libName),
		filepath.Join("..", "lib", platformDir, libName),
	)

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath, true, nil
		}
	}

	return "", false, nil
}

func getLibraryName() string {
	return getLibraryNameFor(runtime.GOOS)
}

func getLibraryNameFor(goos string) string {
	switch goos {
	case "darwin":
		return "libwebrtc_shim.dylib"
	case "windows":
		return "libwebrtc_shim.dll"
	default:
		return "libwebrtc_shim.so"
	}
}

// registerFunctions is implemented in func_bind_purego.go or func_bind_cgo.go

// ShimError converts a shim error code to a Go error.
// Returns sentinel errors that support errors.Is() comparisons.
func ShimError(code int32) error {
	switch code {
	case ShimOK:
		return nil
	case ShimErrInvalidParam:
		return ErrInvalidParam
	case ShimErrInitFailed:
		return ErrInitFailed
	case ShimErrEncodeFailed:
		return ErrEncodeFailed
	case ShimErrDecodeFailed:
		return ErrDecodeFailed
	case ShimErrOutOfMemory:
		return ErrOutOfMemory
	case ShimErrNotSupported:
		return ErrNotSupported
	case ShimErrNeedMoreData:
		return ErrNeedMoreData
	case ShimErrBufferTooSmall:
		return ErrBufferTooSmall
	case ShimErrNotFound:
		return ErrNotFound
	case ShimErrRenegotiationNeeded:
		return ErrRenegotiationNeeded
	default:
		return fmt.Errorf("unknown shim error: %d", code)
	}
}

// preloadLinuxDeps preloads system libraries required by the shim on Linux.
// This ensures dependencies like libgbm are available with RTLD_GLOBAL
// before the shim is loaded.
func preloadLinuxDeps() {
	// Libraries that the shim may depend on for GBM/DRM support
	libs := []string{
		"libgbm.so.1",
		"libdrm.so.2",
	}
	for _, lib := range libs {
		// Best effort - ignore errors as these may not be needed on all systems
		_, _ = dlopenLibrary(lib, RTLD_NOW|RTLD_GLOBAL)
	}
}

func wrapLibraryLoadError(libPath string, loadErr, downloadErr error) error {
	if hint := linuxLoadFailureHintFor(runtime.GOOS, loadErr); hint != "" {
		loadErr = fmt.Errorf("%w (%s)", loadErr, hint)
	}
	if downloadErr != nil {
		return fmt.Errorf("failed to load %s: %w (auto-download failed: %w)", libPath, loadErr, downloadErr)
	}
	return fmt.Errorf("failed to load %s: %w", libPath, loadErr)
}

func linuxLoadFailureHintFor(goos string, err error) string {
	if goos != "linux" || err == nil {
		return ""
	}

	msg := err.Error()
	if strings.Contains(msg, "GLIBC_") && strings.Contains(msg, "not found") {
		return "downloaded linux shim requires a newer glibc than this system; publish the shim from an older distro baseline such as Debian bullseye"
	}

	return ""
}

func openLibraryHandle(libPath string) (uintptr, string, error) {
	handle, err := dlopenLibraryFunc(libPath, RTLD_NOW|RTLD_GLOBAL)
	if err != nil {
		if retryHandle, retryPath, retryErr := retryLoadLibraryFunc(libPath); retryErr == nil {
			return retryHandle, retryPath, nil
		}
		return 0, libPath, err
	}
	return handle, libPath, nil
}

func initializeLoadedLibrary() (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			switch value := recovered.(type) {
			case error:
				err = fmt.Errorf("register shim functions: %w", value)
			default:
				err = fmt.Errorf("register shim functions: %v", value)
			}
		}
	}()

	if err := registerFunctionsFunc(); err != nil {
		return fmt.Errorf("register shim functions: %w", err)
	}
	if err := checkLoadedLibraryVersionFunc(); err != nil {
		return fmt.Errorf("verify shim compatibility: %w", err)
	}
	return nil
}

func tryFallbackToDownloadedShim(currentPath string, currentErr error) (uintptr, string, error) {
	if !shouldFallbackToDownloadedShim(currentPath) {
		return 0, currentPath, currentErr
	}

	downloadedPath, err := downloadShimFunc()
	if err != nil {
		return 0, currentPath, fmt.Errorf("%w (fallback download failed: %v)", currentErr, err)
	}
	if downloadedPath == "" || sameLibraryPath(downloadedPath, currentPath) {
		return 0, currentPath, currentErr
	}

	handle, resolvedPath, err := openLibraryHandle(downloadedPath)
	if err != nil {
		return 0, resolvedPath, fmt.Errorf("%w (failed to load downloaded shim %s: %v)", currentErr, resolvedPath, err)
	}

	libHandle = handle
	if err := initializeLoadedLibrary(); err != nil {
		_ = dlcloseLibraryFunc(handle)
		libHandle = 0
		return 0, resolvedPath, fmt.Errorf("%w (downloaded shim %s is incompatible: %v)", currentErr, resolvedPath, err)
	}

	return handle, resolvedPath, nil
}

func shouldFallbackToDownloadedShim(path string) bool {
	if path == "" || strings.TrimSpace(os.Getenv("LIBWEBRTC_SHIM_PATH")) != "" || isDownloadDisabled() {
		return false
	}

	cacheRoot, err := shimCacheRoot()
	if err != nil {
		return true
	}
	return !isPathWithin(path, filepath.Join(cacheRoot, "shim"))
}

func sameLibraryPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func retryLoadLibrary(libPath string) (uintptr, string, error) {
	if libPath == "" || !filepath.IsAbs(libPath) {
		return 0, libPath, ErrLibraryNotFound
	}
	if _, statErr := os.Stat(libPath); statErr != nil {
		return 0, libPath, statErr
	}

	var lastErr error

	// Freshly downloaded shared libraries on overlay-backed temp storage can
	// transiently fail their first dlopen even though the bytes are correct.
	for _, delay := range []time.Duration{250 * time.Millisecond, time.Second} {
		time.Sleep(delay)
		if handle, err := dlopenLibraryFunc(libPath, RTLD_NOW|RTLD_GLOBAL); err == nil {
			return handle, libPath, nil
		} else {
			lastErr = err
		}
	}

	retryPath := libPath + ".retry"
	if err := copyFile(libPath, retryPath); err != nil {
		return 0, libPath, err
	}
	_ = os.Chmod(retryPath, 0o755)

	for _, delay := range []time.Duration{250 * time.Millisecond, time.Second} {
		time.Sleep(delay)
		handle, err := dlopenLibraryFunc(retryPath, RTLD_NOW|RTLD_GLOBAL)
		if err == nil {
			return handle, retryPath, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrLibraryNotLoaded
	}
	return 0, libPath, lastErr
}
