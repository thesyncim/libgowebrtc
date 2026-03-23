//go:build extended

package ffi

import (
	"errors"
	"os"
	"runtime"
	"testing"
)

func TestOpenH264AutoDownload(t *testing.T) {
	if isOpenH264DownloadDisabled() {
		t.Fatal("OpenH264 auto-download must be enabled for zero-config tests")
	}
	if os.Getenv(envOpenH264Path) == "" && os.Getenv(envOpenH264URL) == "" {
		if _, err := openh264ArchiveName(defaultOpenH264Version); errors.Is(err, errOpenH264Unsupported) {
			t.Skipf("OpenH264 binary not published for %s/%s; set %s or %s to enable", runtime.GOOS, runtime.GOARCH, envOpenH264Path, envOpenH264URL)
		}
	}

	if err := LoadLibrary(); err != nil {
		t.Fatalf("load shim: %v", err)
	}

	profile := CString("42e01f")
	cfg := &VideoEncoderConfig{
		Width:            320,
		Height:           240,
		BitrateBps:       200_000,
		Framerate:        30,
		KeyframeInterval: 60,
		H264Profile:      &profile[0],
		PreferHW:         0,
	}

	enc, err := CreateVideoEncoder(CodecH264, cfg)
	if err != nil {
		t.Fatalf("create H264 encoder: %v", err)
	}
	VideoEncoderDestroy(enc)
}
