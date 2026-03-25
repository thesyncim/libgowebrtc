package ffi

import (
	"errors"
	"testing"
)

func TestLoadLibraryFallsBackToDownloadedShimWhenImplicitLocalShimIsIncompatible(t *testing.T) {
	release := withFFITestSerialExecution(t)
	defer release()

	restore := stubLibraryLoadingForTest(t)
	defer restore()

	t.Setenv(envOpenH264DisableDownload, "1")
	t.Setenv(envShimCacheDir, t.TempDir())
	t.Setenv("LIBWEBRTC_SHIM_PATH", "")

	const (
		localHandle      = uintptr(11)
		downloadedHandle = uintptr(22)
		localPath        = "/tmp/local/libwebrtc_shim.dylib"
		downloadedPath   = "/tmp/downloaded/libwebrtc_shim.dylib"
	)

	var (
		openedPaths []string
		closed      []uintptr
	)

	resolveLibraryFunc = func() (string, error, error) {
		return localPath, nil, nil
	}
	downloadShimFunc = func() (string, error) {
		return downloadedPath, nil
	}
	dlopenLibraryFunc = func(path string, flags int) (uintptr, error) {
		openedPaths = append(openedPaths, path)
		switch path {
		case localPath:
			return localHandle, nil
		case downloadedPath:
			return downloadedHandle, nil
		default:
			return 0, errors.New("unexpected library path")
		}
	}
	retryLoadLibraryFunc = func(path string) (uintptr, string, error) {
		return 0, path, errors.New("retry should not be used")
	}
	registerFunctionsFunc = func() error {
		if libHandle == localHandle {
			return errors.New("undefined symbol: shim_peer_connection_get_stats_json")
		}
		return nil
	}
	checkLoadedLibraryVersionFunc = func() error {
		return nil
	}
	dlcloseLibraryFunc = func(handle uintptr) error {
		closed = append(closed, handle)
		return nil
	}

	if err := LoadLibrary(); err != nil {
		t.Fatalf("LoadLibrary() returned error: %v", err)
	}
	if !libLoaded.Load() {
		t.Fatal("expected library to be marked loaded")
	}
	if libHandle != downloadedHandle {
		t.Fatalf("libHandle = %d, want %d", libHandle, downloadedHandle)
	}
	if len(openedPaths) != 2 || openedPaths[0] != localPath || openedPaths[1] != downloadedPath {
		t.Fatalf("opened paths = %v, want [%s %s]", openedPaths, localPath, downloadedPath)
	}
	if len(closed) != 1 || closed[0] != localHandle {
		t.Fatalf("closed handles = %v, want [%d]", closed, localHandle)
	}
}

func TestLoadLibraryDoesNotFallbackForExplicitShimPath(t *testing.T) {
	release := withFFITestSerialExecution(t)
	defer release()

	restore := stubLibraryLoadingForTest(t)
	defer restore()

	t.Setenv(envOpenH264DisableDownload, "1")
	t.Setenv(envShimDisableDownload, "0")
	t.Setenv("LIBWEBRTC_SHIM_PATH", "/tmp/explicit/libwebrtc_shim.dylib")

	const explicitPath = "/tmp/explicit/libwebrtc_shim.dylib"

	var downloadCalled bool

	resolveLibraryFunc = func() (string, error, error) {
		return explicitPath, nil, nil
	}
	downloadShimFunc = func() (string, error) {
		downloadCalled = true
		return "/tmp/downloaded/libwebrtc_shim.dylib", nil
	}
	dlopenLibraryFunc = func(path string, flags int) (uintptr, error) {
		if path != explicitPath {
			t.Fatalf("unexpected library path: %s", path)
		}
		return 33, nil
	}
	retryLoadLibraryFunc = func(path string) (uintptr, string, error) {
		return 0, path, errors.New("retry should not be used")
	}
	registerFunctionsFunc = func() error {
		return errors.New("undefined symbol: shim_peer_connection_get_stats_json")
	}
	checkLoadedLibraryVersionFunc = func() error {
		return nil
	}
	dlcloseLibraryFunc = func(handle uintptr) error {
		return nil
	}

	err := LoadLibrary()
	if err == nil {
		t.Fatal("expected LoadLibrary() to fail")
	}
	if downloadCalled {
		t.Fatal("expected explicit shim path failure to avoid download fallback")
	}
}

func stubLibraryLoadingForTest(t *testing.T) func() {
	t.Helper()

	libMu.Lock()
	defer libMu.Unlock()

	originalResolveLibraryFunc := resolveLibraryFunc
	originalDownloadShimFunc := downloadShimFunc
	originalDlopenLibraryFunc := dlopenLibraryFunc
	originalDlcloseLibraryFunc := dlcloseLibraryFunc
	originalRetryLoadLibraryFunc := retryLoadLibraryFunc
	originalRegisterFunctionsFunc := registerFunctionsFunc
	originalCheckLoadedLibraryVersionFunc := checkLoadedLibraryVersionFunc
	originalLibHandle := libHandle
	originalLoaded := libLoaded.Load()

	libHandle = 0
	libLoaded.Store(false)

	return func() {
		libMu.Lock()
		defer libMu.Unlock()

		resolveLibraryFunc = originalResolveLibraryFunc
		downloadShimFunc = originalDownloadShimFunc
		dlopenLibraryFunc = originalDlopenLibraryFunc
		dlcloseLibraryFunc = originalDlcloseLibraryFunc
		retryLoadLibraryFunc = originalRetryLoadLibraryFunc
		registerFunctionsFunc = originalRegisterFunctionsFunc
		checkLoadedLibraryVersionFunc = originalCheckLoadedLibraryVersionFunc
		libHandle = originalLibHandle
		libLoaded.Store(originalLoaded)
	}
}
