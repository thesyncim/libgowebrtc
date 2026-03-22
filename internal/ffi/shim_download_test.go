package ffi

import (
	"net/http"
	"net/http/httptest"
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
