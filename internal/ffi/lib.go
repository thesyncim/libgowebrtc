// Package ffi provides FFI bindings to the libwebrtc shim library.
// It supports both purego (default) and CGO backends via build tags.
package ffi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
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

// Function pointers are defined in func_vars.go and populated by registerFunctions() in
// either func_bind_purego.go or func_bind_cgo.go depending on build tags.

// LoadLibrary loads the libwebrtc_shim shared library.
// It searches in the following locations:
// 1. Path specified by LIBWEBRTC_SHIM_PATH environment variable
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
	}

	// On Linux, preload system libraries that the shim depends on
	if runtime.GOOS == "linux" {
		preloadLinuxDeps()
	}

	libPath, downloadErr, err := resolveLibrary()
	if err != nil {
		return err
	}

	handle, err := dlopenLibrary(libPath, RTLD_NOW|RTLD_GLOBAL)
	if err != nil {
		if downloadErr != nil {
			return fmt.Errorf("failed to load %s: %w (auto-download failed: %w)", libPath, err, downloadErr)
		}
		return fmt.Errorf("failed to load %s: %w", libPath, err)
	}

	libHandle = handle
	if err := registerFunctions(); err != nil {
		_ = dlcloseLibrary(handle)
		return err
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

	if err := dlcloseLibrary(libHandle); err != nil {
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
const ExpectedShimVersion = "0.2.0"

// ErrVersionMismatch is returned when the shim version doesn't match.
var ErrVersionMismatch = errors.New("shim version mismatch")

// ShimVersion returns the shim library version.
// Returns empty string if library is not loaded.
func ShimVersion() string {
	if !libLoaded.Load() {
		return ""
	}
	ptr := shimVersion()
	if ptr == 0 {
		return ""
	}
	return GoString(unsafe.Pointer(ptr))
}

// LibWebRTCVersion returns the libwebrtc version the shim was built with.
// Returns empty string if library is not loaded.
func LibWebRTCVersion() string {
	if !libLoaded.Load() {
		return ""
	}
	ptr := shimLibwebrtcVersion()
	if ptr == 0 {
		return ""
	}
	return GoString(unsafe.Pointer(ptr))
}

// CheckVersion verifies the shim version matches what this Go code expects.
// Returns nil if versions match, ErrVersionMismatch otherwise.
func CheckVersion() error {
	if !libLoaded.Load() {
		return ErrLibraryNotLoaded
	}

	shimVer := ShimVersion()
	webrtcVer := LibWebRTCVersion()

	if shimVer != ExpectedShimVersion {
		return fmt.Errorf("%w: shim version %q, expected %q", ErrVersionMismatch, shimVer, ExpectedShimVersion)
	}
	if webrtcVer != ExpectedLibWebRTCVersion {
		return fmt.Errorf("%w: libwebrtc version %q, expected %q", ErrVersionMismatch, webrtcVer, ExpectedLibWebRTCVersion)
	}
	return nil
}

func findLocalLibrary() (string, bool) {
	// Check environment variable first
	if path := os.Getenv("LIBWEBRTC_SHIM_PATH"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path, true
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
			return absPath, true
		}
	}

	return "", false
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
