package ffi

import (
	"runtime"
	"testing"
)

func TestH264PreferHWMac(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only")
	}

	t.Setenv(envPreferSoftwareCodecs, "0")

	if err := LoadLibrary(); err != nil {
		t.Fatalf("load library: %v", err)
	}

	codecs, err := GetSupportedVideoCodecs()
	if err != nil {
		t.Fatalf("GetSupportedVideoCodecs failed: %v", err)
	}

	h264Available := false
	for _, c := range codecs {
		if ByteArrayToString(c.MimeType[:]) == "video/H264" {
			h264Available = true
			break
		}
	}
	if !h264Available {
		t.Fatal("H264 should be available in codec list")
	}

	profile := CString("42e01f") // Constrained Baseline
	cfg := &VideoEncoderConfig{
		Width:            1280,
		Height:           720,
		BitrateBps:       2_000_000,
		Framerate:        30.0,
		KeyframeInterval: 60,
		H264Profile:      &profile[0],
		PreferHW:         1,
	}

	handle, err := CreateVideoEncoder(CodecH264, cfg)
	if err != nil {
		t.Fatalf("create H264 encoder with PreferHW=1 should fall back automatically: %v", err)
	}
	VideoEncoderDestroy(handle)
}
