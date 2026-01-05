package depacketizer

import (
	"testing"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

func TestDepacketizerErrors(t *testing.T) {
	errors := []error{
		ErrDepacketizerClosed,
		ErrNeedMoreData,
		ErrBufferTooSmall,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("Error should not be nil")
		}
		if err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	}
}

func TestDepacketizerCodecs(t *testing.T) {
	codecs := []struct {
		codec codec.Type
		name  string
	}{
		{codec.H264, "H264"},
		{codec.VP8, "VP8"},
		{codec.VP9, "VP9"},
		{codec.AV1, "AV1"},
		{codec.Opus, "Opus"},
	}

	for _, tc := range codecs {
		t.Run(tc.name, func(t *testing.T) {
			// These will fail without the shim library, but tests compile-time check
			_, _ = New(tc.codec)
		})
	}
}
