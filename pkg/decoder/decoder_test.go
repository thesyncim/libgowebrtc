package decoder

import (
	"errors"
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

func TestApplyVideoDecodeOutput(t *testing.T) {
	dst := frame.NewI420Frame(320, 240)

	if err := applyVideoDecodeOutput(dst, 320, 240, 320, 160, 160, 1234); err != nil {
		t.Fatalf("applyVideoDecodeOutput() error = %v", err)
	}

	if dst.Width != 320 || dst.Height != 240 {
		t.Fatalf("decoded size = %dx%d, want 320x240", dst.Width, dst.Height)
	}
	if dst.Stride[0] != 320 || dst.Stride[1] != 160 || dst.Stride[2] != 160 {
		t.Fatalf("decoded strides = %v, want [320 160 160]", dst.Stride)
	}
	if dst.PTS != 1234 {
		t.Fatalf("decoded pts = %d, want 1234", dst.PTS)
	}
	if dst.Format != frame.PixelFormatI420 {
		t.Fatalf("decoded format = %v, want I420", dst.Format)
	}
}

func TestApplyVideoDecodeOutputNeedMoreData(t *testing.T) {
	dst := frame.NewI420Frame(320, 240)

	if err := applyVideoDecodeOutput(dst, 0, 240, 320, 160, 160, 1234); err != ErrNeedMoreData {
		t.Fatalf("applyVideoDecodeOutput() error = %v, want ErrNeedMoreData", err)
	}
}

func TestApplyAudioDecodeOutput(t *testing.T) {
	dst := frame.NewAudioFrameS16(48000, 2, 960)

	numSamples, err := applyAudioDecodeOutput(dst, 48000, 2, 960)
	if err != nil {
		t.Fatalf("applyAudioDecodeOutput() error = %v", err)
	}
	if numSamples != 960 {
		t.Fatalf("numSamples = %d, want 960", numSamples)
	}
	if dst.SampleRate != 48000 || dst.Channels != 2 {
		t.Fatalf("decoded audio settings = %dHz/%dch, want 48000Hz/2ch", dst.SampleRate, dst.Channels)
	}
	if dst.NumSamples != 960 {
		t.Fatalf("dst.NumSamples = %d, want 960", dst.NumSamples)
	}
	if dst.Format != frame.AudioFormatS16 {
		t.Fatalf("decoded format = %v, want S16", dst.Format)
	}
}

func TestApplyAudioDecodeOutputNeedMoreData(t *testing.T) {
	dst := frame.NewAudioFrameS16(48000, 2, 960)

	numSamples, err := applyAudioDecodeOutput(dst, 48000, 2, 0)
	if err != ErrNeedMoreData {
		t.Fatalf("applyAudioDecodeOutput() error = %v, want ErrNeedMoreData", err)
	}
	if numSamples != 0 {
		t.Fatalf("numSamples = %d, want 0", numSamples)
	}
}

func TestNormalizeVideoDecodeError(t *testing.T) {
	if err := normalizeVideoDecodeError(nil, true); err != nil {
		t.Fatalf("normalizeVideoDecodeError(nil) = %v, want nil", err)
	}
	if err := normalizeVideoDecodeError(ffi.ErrNeedMoreData, true); err != ErrNeedMoreData {
		t.Fatalf("normalizeVideoDecodeError(ErrNeedMoreData) = %v, want ErrNeedMoreData", err)
	}
	if err := normalizeVideoDecodeError(ffi.ErrDecodeFailed, false); err != ErrNeedMoreData {
		t.Fatalf("normalizeVideoDecodeError(ErrDecodeFailed, false) = %v, want ErrNeedMoreData", err)
	}

	err := normalizeVideoDecodeError(ffi.ErrDecodeFailed, true)
	if !errors.Is(err, ErrDecodeFailed) {
		t.Fatalf("normalizeVideoDecodeError(ErrDecodeFailed, true) = %v, want ErrDecodeFailed wrapper", err)
	}

	want := errors.New("boom")
	if err := normalizeVideoDecodeError(want, true); !errors.Is(err, want) {
		t.Fatalf("normalizeVideoDecodeError(other) = %v, want %v", err, want)
	}
}
