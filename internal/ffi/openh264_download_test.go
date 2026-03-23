package ffi

import "testing"

func TestOpenH264ArchiveNames(t *testing.T) {
	// Test all expected archive names for version 2.5.1
	// These should match exactly what Cisco publishes at https://github.com/cisco/openh264/releases
	testCases := []struct {
		goos     string
		goarch   string
		expected string
	}{
		// macOS
		{"darwin", "amd64", "libopenh264-2.5.1-mac-x64.dylib.bz2"},
		{"darwin", "arm64", "libopenh264-2.5.1-mac-arm64.dylib.bz2"},
		// Linux
		{"linux", "amd64", "libopenh264-2.5.1-linux64.7.so.bz2"},
		{"linux", "386", "libopenh264-2.5.1-linux32.7.so.bz2"},
		{"linux", "arm", "libopenh264-2.5.1-linux-arm.7.so.bz2"},
		{"linux", "arm64", "libopenh264-2.5.1-linux-arm64.7.so.bz2"},
		// Windows
		{"windows", "amd64", "openh264-2.5.1-win64.dll.bz2"},
		{"windows", "386", "openh264-2.5.1-win32.dll.bz2"},
		{"windows", "arm64", "openh264-2.5.1-win-arm64.dll.bz2"},
	}

	for _, tc := range testCases {
		t.Run(tc.goos+"_"+tc.goarch, func(t *testing.T) {
			archive, err := openh264ArchiveNameFor(tc.goos, tc.goarch, "2.5.1", defaultOpenH264SOVersion)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if archive != tc.expected {
				t.Errorf("archive name mismatch: got %q, want %q", archive, tc.expected)
			}
		})
	}
}

func TestOpenH264LibraryNames(t *testing.T) {
	testCases := []struct {
		goos     string
		expected string
	}{
		{"darwin", "libopenh264.dylib"},
		{"linux", "libopenh264.so.7"},
		{"windows", "openh264.dll"},
	}

	for _, tc := range testCases {
		t.Run(tc.goos, func(t *testing.T) {
			libName, err := openh264LibraryNameFor(tc.goos, defaultOpenH264SOVersion)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if libName != tc.expected {
				t.Errorf("library name mismatch: got %q, want %q", libName, tc.expected)
			}
		})
	}
}

func TestOpenH264PlatformKeys(t *testing.T) {
	testCases := []struct {
		goos     string
		goarch   string
		expected string
	}{
		{"darwin", "amd64", "darwin_amd64"},
		{"darwin", "arm64", "darwin_arm64"},
		{"linux", "amd64", "linux_amd64"},
		{"linux", "386", "linux_386"},
		{"linux", "arm", "linux_arm"},
		{"linux", "arm64", "linux_arm64"},
		{"windows", "amd64", "windows_amd64"},
		{"windows", "386", "windows_386"},
		{"windows", "arm64", "windows_arm64"},
	}

	for _, tc := range testCases {
		t.Run(tc.goos+"_"+tc.goarch, func(t *testing.T) {
			key, err := openh264PlatformKeyFor(tc.goos, tc.goarch)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if key != tc.expected {
				t.Errorf("platform key mismatch: got %q, want %q", key, tc.expected)
			}
		})
	}

	unsupported := []struct {
		goos   string
		goarch string
	}{
		{"freebsd", "amd64"},
		{"linux", "ppc64le"},
	}

	for _, tc := range unsupported {
		t.Run(tc.goos+"_"+tc.goarch, func(t *testing.T) {
			if _, err := openh264PlatformKeyFor(tc.goos, tc.goarch); err == nil {
				t.Fatalf("expected error for %s/%s", tc.goos, tc.goarch)
			}
		})
	}
}
