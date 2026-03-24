package ffi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveShimSHA256UsesManifestValue(t *testing.T) {
	want := strings.Repeat("ab", 32)

	got, err := resolveShimSHA256("https://example.invalid", "shim-vtest", shimAssetRef{
		File:   "libwebrtc_shim_darwin_arm64.tar.gz",
		SHA256: strings.ToUpper(want),
	})
	if err != nil {
		t.Fatalf("resolveShimSHA256 returned error: %v", err)
	}
	if got != want {
		t.Fatalf("resolveShimSHA256 = %q, want %q", got, want)
	}
}

func TestResolveShimSHA256FallsBackToSidecar(t *testing.T) {
	want := strings.Repeat("cd", 32)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shim-vtest/libwebrtc_shim_darwin_arm64.tar.gz.sha256" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(want + "  libwebrtc_shim_darwin_arm64.tar.gz\n"))
	}))
	defer server.Close()

	got, err := resolveShimSHA256(server.URL, "shim-vtest", shimAssetRef{
		File: "libwebrtc_shim_darwin_arm64.tar.gz",
	})
	if err != nil {
		t.Fatalf("resolveShimSHA256 returned error: %v", err)
	}
	if got != want {
		t.Fatalf("resolveShimSHA256 = %q, want %q", got, want)
	}
}

func TestResolveLibraryRejectsMissingExplicitShimPath(t *testing.T) {
	release := withFFITestSerialExecution(t)
	defer release()

	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd(): %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir(%q): %v", tempDir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})

	fallbackPath := filepath.Join(tempDir, "lib", runtime.GOOS+"_"+runtime.GOARCH, getLibraryName())
	writeShimFixture(t, fallbackPath)

	missingPath := filepath.Join(tempDir, "missing", getLibraryName())
	expectedPath, err := filepath.Abs(missingPath)
	if err != nil {
		t.Fatalf("Abs(%q): %v", missingPath, err)
	}

	t.Setenv("LIBWEBRTC_SHIM_PATH", missingPath)
	t.Setenv(envShimDisableDownload, "1")

	path, _, err := resolveLibrary()
	if err == nil {
		t.Fatal("expected resolveLibrary to fail for a missing explicit shim path")
	}
	if !errors.Is(err, ErrLibraryNotFound) {
		t.Fatalf("resolveLibrary error = %v, want ErrLibraryNotFound", err)
	}
	if path != expectedPath {
		t.Fatalf("resolveLibrary path = %q, want %q", path, expectedPath)
	}
	if !strings.Contains(err.Error(), "LIBWEBRTC_SHIM_PATH") {
		t.Fatalf("expected error to mention LIBWEBRTC_SHIM_PATH, got %v", err)
	}
}

func writeShimFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("shim-fixture"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
