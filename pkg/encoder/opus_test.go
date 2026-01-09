package encoder

import (
	"fmt"
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/testutil"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/decoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

func TestOpus_RoundTrip(t *testing.T) {
	testutil.SkipIfNoShim(t)

	enc, err := NewOpusEncoder(codec.OpusConfig{
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		t.Fatalf("NewOpusEncoder: %v", err)
	}
	defer enc.Close()

	dec, err := decoder.NewOpusDecoder(48000, 2)
	if err != nil {
		t.Fatalf("NewOpusDecoder: %v", err)
	}
	defer dec.Close()

	// 20ms at 48kHz stereo = 960 samples
	srcFrame := testutil.CreateSilentAudioFrame(48000, 2, 960)
	encBuf := make([]byte, enc.MaxEncodedSize())

	n, err := enc.EncodeInto(srcFrame, encBuf)
	if err != nil {
		t.Fatalf("EncodeInto: %v", err)
	}
	if n == 0 {
		t.Fatal("encoded size is 0")
	}

	dstFrame := frame.NewAudioFrameS16(48000, 2, dec.MaxSamplesPerFrame())
	samples, err := dec.DecodeInto(encBuf[:n], dstFrame)
	if err != nil {
		t.Fatalf("DecodeInto: %v", err)
	}
	if samples == 0 {
		t.Error("decoded 0 samples")
	}
}

func TestOpus_MultiFrameSequence(t *testing.T) {
	testutil.SkipIfNoShim(t)

	enc, err := NewOpusEncoder(codec.OpusConfig{
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		t.Fatalf("NewOpusEncoder: %v", err)
	}
	defer enc.Close()

	dec, err := decoder.NewOpusDecoder(48000, 2)
	if err != nil {
		t.Fatalf("NewOpusDecoder: %v", err)
	}
	defer dec.Close()

	srcFrame := testutil.CreateSilentAudioFrame(48000, 2, 960)
	encBuf := make([]byte, enc.MaxEncodedSize())
	dstFrame := frame.NewAudioFrameS16(48000, 2, dec.MaxSamplesPerFrame())

	for i := 0; i < 50; i++ {
		n, err := enc.EncodeInto(srcFrame, encBuf)
		if err != nil {
			t.Fatalf("frame %d: EncodeInto: %v", i, err)
		}
		if n == 0 {
			t.Fatalf("frame %d: encoded size is 0", i)
		}

		samples, err := dec.DecodeInto(encBuf[:n], dstFrame)
		if err != nil {
			t.Fatalf("frame %d: DecodeInto: %v", i, err)
		}
		if samples == 0 {
			t.Fatalf("frame %d: decoded 0 samples", i)
		}
	}
}

func TestOpus_InvalidConfig(t *testing.T) {
	testutil.SkipIfNoShim(t)

	tests := []struct {
		name   string
		config codec.OpusConfig
	}{
		{"invalid sample rate", codec.OpusConfig{SampleRate: 44100, Channels: 2, Bitrate: 64000}},
		{"zero channels", codec.OpusConfig{SampleRate: 48000, Channels: 0, Bitrate: 64000}},
		{"too many channels", codec.OpusConfig{SampleRate: 48000, Channels: 8, Bitrate: 64000}},
		{"zero bitrate", codec.OpusConfig{SampleRate: 48000, Channels: 2, Bitrate: 0}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enc, err := NewOpusEncoder(tc.config)
			if err == nil {
				enc.Close()
				t.Error("expected error for invalid config")
			}
		})
	}
}

func TestOpus_ValidSampleRates(t *testing.T) {
	testutil.SkipIfNoShim(t)

	// Only 16000 and 48000 are supported by the underlying encoder
	validRates := []int{16000, 48000}

	for _, rate := range validRates {
		t.Run(fmt.Sprintf("%d_Hz", rate), func(t *testing.T) {
			enc, err := NewOpusEncoder(codec.OpusConfig{
				SampleRate: rate,
				Channels:   1,
				Bitrate:    32000,
			})
			if err != nil {
				t.Errorf("sample rate %d should be valid: %v", rate, err)
				return
			}
			enc.Close()
		})
	}
}

func TestOpus_EncodeAfterClose(t *testing.T) {
	testutil.SkipIfNoShim(t)

	enc, err := NewOpusEncoder(codec.OpusConfig{
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		t.Fatalf("NewOpusEncoder: %v", err)
	}
	enc.Close()

	srcFrame := testutil.CreateSilentAudioFrame(48000, 2, 960)
	encBuf := make([]byte, 4000)

	_, err = enc.EncodeInto(srcFrame, encBuf)
	if err != ErrEncoderClosed {
		t.Errorf("expected ErrEncoderClosed, got %v", err)
	}
}

func TestOpus_NilFrame(t *testing.T) {
	testutil.SkipIfNoShim(t)

	enc, err := NewOpusEncoder(codec.OpusConfig{
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		t.Fatalf("NewOpusEncoder: %v", err)
	}
	defer enc.Close()

	encBuf := make([]byte, 4000)
	_, err = enc.EncodeInto(nil, encBuf)
	if err != ErrInvalidFrame {
		t.Errorf("expected ErrInvalidFrame, got %v", err)
	}
}

func BenchmarkOpus_Encode_48kHz_Stereo(b *testing.B) {
	testutil.RequireShim(b)

	enc, err := NewOpusEncoder(codec.OpusConfig{
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		b.Fatalf("NewOpusEncoder: %v", err)
	}
	defer enc.Close()

	srcFrame := testutil.CreateSilentAudioFrame(48000, 2, 960)
	encBuf := make([]byte, enc.MaxEncodedSize())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc.EncodeInto(srcFrame, encBuf)
	}
}
