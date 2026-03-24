package diagnostics

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckReportsExplicitLocalPaths(t *testing.T) {
	tempDir := t.TempDir()
	shimPath := filepath.Join(tempDir, "local", shimLibraryName())
	openh264Path := filepath.Join(tempDir, "local", openh264LibraryName())
	writeTestBinary(t, shimPath)
	writeTestBinary(t, openh264Path)

	t.Setenv("LIBWEBRTC_SHIM_PATH", shimPath)
	t.Setenv("LIBWEBRTC_SHIM_CACHE_DIR", filepath.Join(tempDir, "shim-cache"))
	t.Setenv("LIBWEBRTC_OPENH264_PATH", openh264Path)
	t.Setenv("LIBWEBRTC_OPENH264_CACHE_DIR", filepath.Join(tempDir, "openh264-cache"))

	report, err := Check()
	if err != nil {
		t.Fatalf("Check(): %v", err)
	}

	if !report.Shim.Available {
		t.Fatal("expected shim to be available")
	}
	if report.Shim.Source != "local" {
		t.Fatalf("shim source = %q, want %q", report.Shim.Source, "local")
	}
	if report.Shim.Path != shimPath {
		t.Fatalf("shim path = %q, want %q", report.Shim.Path, shimPath)
	}
	if report.Shim.ChecksumStatus != StatusUnverified {
		t.Fatalf("shim checksum status = %q, want %q", report.Shim.ChecksumStatus, StatusUnverified)
	}

	if !report.OpenH264.Available {
		t.Fatal("expected OpenH264 to be available")
	}
	if report.OpenH264.Source != "local" {
		t.Fatalf("openh264 source = %q, want %q", report.OpenH264.Source, "local")
	}
	if report.OpenH264.Path != openh264Path {
		t.Fatalf("openh264 path = %q, want %q", report.OpenH264.Path, openh264Path)
	}
}

func TestCheckReportsDownloadedShimChecksumMismatch(t *testing.T) {
	tempDir := t.TempDir()
	cacheRoot := filepath.Join(tempDir, "cache")
	shimPath := filepath.Join(cacheRoot, "shim", "fake-release", platformKeyForTest(), shimLibraryName())
	writeTestBinary(t, shimPath)

	t.Setenv("LIBWEBRTC_SHIM_PATH", shimPath)
	t.Setenv("LIBWEBRTC_SHIM_CACHE_DIR", cacheRoot)
	t.Setenv("LIBWEBRTC_OPENH264_DISABLE_DOWNLOAD", "1")
	t.Setenv("LIBWEBRTC_OPENH264_CACHE_DIR", filepath.Join(tempDir, "openh264-cache"))

	report, err := Check()
	if err != nil {
		t.Fatalf("Check(): %v", err)
	}

	if report.Shim.Source != "downloaded" {
		t.Fatalf("shim source = %q, want %q", report.Shim.Source, "downloaded")
	}
	if !report.Shim.Available {
		t.Fatal("expected downloaded shim candidate to be available")
	}
	if report.Shim.ExpectedSHA256 == "" {
		t.Skip("shim manifest checksum is unavailable on this platform")
	}
	if report.Shim.ChecksumStatus != StatusMismatch {
		t.Fatalf("shim checksum status = %q, want %q", report.Shim.ChecksumStatus, StatusMismatch)
	}
	if len(report.Shim.BlockingIssues) == 0 {
		t.Fatal("expected checksum mismatch to produce a blocking issue")
	}
}

func TestCheckReportsDisabledDownloadMode(t *testing.T) {
	tempDir := t.TempDir()
	shimPath := filepath.Join(tempDir, "local", shimLibraryName())
	writeTestBinary(t, shimPath)

	t.Setenv("LIBWEBRTC_SHIM_PATH", shimPath)
	t.Setenv("LIBWEBRTC_SHIM_DISABLE_DOWNLOAD", "1")
	t.Setenv("LIBWEBRTC_SHIM_CACHE_DIR", filepath.Join(tempDir, "shim-cache"))
	t.Setenv("LIBWEBRTC_OPENH264_DISABLE_DOWNLOAD", "1")
	t.Setenv("LIBWEBRTC_OPENH264_CACHE_DIR", filepath.Join(tempDir, "openh264-cache"))

	report, err := Check()
	if err != nil {
		t.Fatalf("Check(): %v", err)
	}

	if !report.Shim.DownloadDisabled {
		t.Fatal("expected shim download to be marked disabled")
	}
	if !report.OpenH264.DownloadDisabled {
		t.Fatal("expected OpenH264 download to be marked disabled")
	}
	if report.OpenH264.Available {
		t.Fatal("expected OpenH264 to be unavailable without an explicit path")
	}
	if len(report.OpenH264.Warnings) == 0 {
		t.Fatal("expected disabled OpenH264 download mode to produce a warning")
	}
	if !containsSubstring(report.OpenH264.Warnings, "auto-download is disabled") {
		t.Fatalf("expected disabled download warning, got %v", report.OpenH264.Warnings)
	}
}

func TestCheckRejectsMissingExplicitShimPathWithoutFallback(t *testing.T) {
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

	fallbackPath := filepath.Join(tempDir, "lib", platformKeyForTest(), shimLibraryName())
	writeTestBinary(t, fallbackPath)

	missingPath := filepath.Join(tempDir, "missing", shimLibraryName())
	expectedPath, err := filepath.Abs(missingPath)
	if err != nil {
		t.Fatalf("Abs(%q): %v", missingPath, err)
	}

	t.Setenv("LIBWEBRTC_SHIM_PATH", missingPath)
	t.Setenv("LIBWEBRTC_SHIM_DISABLE_DOWNLOAD", "1")
	t.Setenv("LIBWEBRTC_SHIM_CACHE_DIR", filepath.Join(tempDir, "shim-cache"))
	t.Setenv("LIBWEBRTC_OPENH264_DISABLE_DOWNLOAD", "1")
	t.Setenv("LIBWEBRTC_OPENH264_CACHE_DIR", filepath.Join(tempDir, "openh264-cache"))

	report, err := Check()
	if err != nil {
		t.Fatalf("Check(): %v", err)
	}

	if report.Shim.Available {
		t.Fatal("expected shim to be unavailable when LIBWEBRTC_SHIM_PATH is missing")
	}
	if report.Shim.Path != expectedPath {
		t.Fatalf("shim path = %q, want %q", report.Shim.Path, expectedPath)
	}
	if report.Shim.Source != "local" {
		t.Fatalf("shim source = %q, want %q", report.Shim.Source, "local")
	}
	if report.Shim.ChecksumStatus != StatusMissing {
		t.Fatalf("shim checksum status = %q, want %q", report.Shim.ChecksumStatus, StatusMissing)
	}
	if !containsSubstring(report.Shim.BlockingIssues, "LIBWEBRTC_SHIM_PATH") {
		t.Fatalf("expected blocking issue to mention LIBWEBRTC_SHIM_PATH, got %v", report.Shim.BlockingIssues)
	}
}

func writeTestBinary(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("diagnostic-test-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func containsSubstring(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func platformKeyForTest() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin_" + runtime.GOARCH
	case "linux":
		return "linux_" + runtime.GOARCH
	case "windows":
		return "windows_" + runtime.GOARCH
	default:
		return runtime.GOOS + "_" + runtime.GOARCH
	}
}

func shimLibraryName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libwebrtc_shim.dylib"
	case "windows":
		return "libwebrtc_shim.dll"
	default:
		return "libwebrtc_shim.so"
	}
}

func openh264LibraryName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libopenh264.dylib"
	case "windows":
		return "openh264.dll"
	default:
		return "libopenh264.so.7"
	}
}
