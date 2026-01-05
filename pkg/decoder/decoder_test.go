package decoder

import (
	"testing"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

func TestDecoderErrors(t *testing.T) {
	errors := []error{
		ErrDecoderClosed,
		ErrInvalidData,
		ErrDecodeFailed,
		ErrUnsupportedCodec,
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

func TestVideoDecoderInterface(t *testing.T) {
	// Compile-time check
	var _ VideoDecoder = (VideoDecoder)(nil)
}

func TestAudioDecoderInterface(t *testing.T) {
	// Compile-time check
	var _ AudioDecoder = (AudioDecoder)(nil)
}

func TestNewVideoDecoderUnsupportedCodec(t *testing.T) {
	_, err := NewVideoDecoder(codec.Type(999))
	if err != ErrUnsupportedCodec {
		t.Errorf("Expected ErrUnsupportedCodec, got %v", err)
	}
}

func TestNewAudioDecoderUnsupportedCodec(t *testing.T) {
	_, err := NewAudioDecoder(codec.Type(999), 48000, 2)
	if err != ErrUnsupportedCodec {
		t.Errorf("Expected ErrUnsupportedCodec, got %v", err)
	}
}

func TestNewVideoDecoderCodecTypes(t *testing.T) {
	codecs := []struct {
		codec codec.Type
		name  string
	}{
		{codec.H264, "H264"},
		{codec.VP8, "VP8"},
		{codec.VP9, "VP9"},
		{codec.AV1, "AV1"},
	}

	for _, tc := range codecs {
		t.Run(tc.name, func(t *testing.T) {
			// These will fail without the shim library, but tests compile-time check
			_, _ = NewVideoDecoder(tc.codec)
		})
	}
}

func TestNewAudioDecoderCodecType(t *testing.T) {
	// This will fail without the shim library, but tests compile-time check
	_, _ = NewAudioDecoder(codec.Opus, 48000, 2)
}

func BenchmarkErrorCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ErrDecoderClosed
		_ = ErrInvalidData
		_ = ErrDecodeFailed
	}
}
