package ffi

import "testing"

func TestShimPlatformKeys(t *testing.T) {
	testCases := []struct {
		goos     string
		goarch   string
		expected string
	}{
		{"darwin", "amd64", "darwin_amd64"},
		{"darwin", "arm64", "darwin_arm64"},
		{"linux", "amd64", "linux_amd64"},
		{"linux", "arm64", "linux_arm64"},
	}

	for _, tc := range testCases {
		t.Run(tc.goos+"_"+tc.goarch, func(t *testing.T) {
			key, err := shimPlatformKeyFor(tc.goos, tc.goarch)
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
		{"windows", "amd64"},
		{"linux", "386"},
		{"darwin", "386"},
	}

	for _, tc := range unsupported {
		t.Run(tc.goos+"_"+tc.goarch, func(t *testing.T) {
			if _, err := shimPlatformKeyFor(tc.goos, tc.goarch); err == nil {
				t.Fatalf("expected error for %s/%s", tc.goos, tc.goarch)
			}
		})
	}
}

func TestShimLibraryNames(t *testing.T) {
	testCases := []struct {
		goos     string
		expected string
	}{
		{"darwin", "libwebrtc_shim.dylib"},
		{"windows", "webrtc_shim.dll"},
		{"linux", "libwebrtc_shim.so"},
		{"freebsd", "libwebrtc_shim.so"},
	}

	for _, tc := range testCases {
		t.Run(tc.goos, func(t *testing.T) {
			libName := getLibraryNameFor(tc.goos)
			if libName != tc.expected {
				t.Errorf("library name mismatch: got %q, want %q", libName, tc.expected)
			}
		})
	}
}
