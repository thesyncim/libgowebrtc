package decoder

import (
	"sync"
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/testutil"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

func TestOpusDecoder_DecodeEncodedFrame(t *testing.T) {
	testutil.SkipIfNoShim(t)

	enc, err := encoder.NewOpusEncoder(codec.OpusConfig{
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		t.Fatalf("new encoder: %v", err)
	}
	defer enc.Close()

	dec, err := NewOpusDecoder(48000, 2)
	if err != nil {
		t.Fatalf("new decoder: %v", err)
	}
	defer dec.Close()

	srcFrame := testutil.CreateSilentAudioFrame(48000, 2, 960)
	encBuf := make([]byte, enc.MaxEncodedSize())

	n, err := enc.EncodeInto(srcFrame, encBuf)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	dstFrame := frame.NewAudioFrameS16(48000, 2, dec.MaxSamplesPerFrame())
	samples, err := dec.DecodeInto(encBuf[:n], dstFrame)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if samples == 0 {
		t.Error("decoded 0 samples")
	}
}

func TestOpusDecoder_DecodeSequence(t *testing.T) {
	testutil.SkipIfNoShim(t)

	enc, err := encoder.NewOpusEncoder(codec.OpusConfig{
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		t.Fatalf("new encoder: %v", err)
	}
	defer enc.Close()

	dec, err := NewOpusDecoder(48000, 2)
	if err != nil {
		t.Fatalf("new decoder: %v", err)
	}
	defer dec.Close()

	srcFrame := testutil.CreateSilentAudioFrame(48000, 2, 960)
	encBuf := make([]byte, enc.MaxEncodedSize())
	dstFrame := frame.NewAudioFrameS16(48000, 2, dec.MaxSamplesPerFrame())

	for i := 0; i < 50; i++ {
		n, err := enc.EncodeInto(srcFrame, encBuf)
		if err != nil {
			t.Fatalf("encode frame %d: %v", i, err)
		}

		samples, err := dec.DecodeInto(encBuf[:n], dstFrame)
		if err != nil {
			t.Fatalf("decode frame %d: %v", i, err)
		}
		if samples == 0 {
			t.Fatalf("frame %d: decoded 0 samples", i)
		}
	}
}

func TestOpusDecoder_DecodeAfterClose(t *testing.T) {
	testutil.SkipIfNoShim(t)

	dec, err := NewOpusDecoder(48000, 2)
	if err != nil {
		t.Fatalf("new decoder: %v", err)
	}
	dec.Close()

	dstFrame := frame.NewAudioFrameS16(48000, 2, 960)
	_, err = dec.DecodeInto([]byte{1, 2, 3}, dstFrame)
	if err != ErrDecoderClosed {
		t.Errorf("expected ErrDecoderClosed, got %v", err)
	}
}

func TestOpusDecoder_DoubleClose(t *testing.T) {
	testutil.SkipIfNoShim(t)

	dec, err := NewOpusDecoder(48000, 2)
	if err != nil {
		t.Fatalf("new decoder: %v", err)
	}
	dec.Close()
	dec.Close() // should not panic
}

func TestOpusDecoder_EmptyInput(t *testing.T) {
	testutil.SkipIfNoShim(t)

	dec, err := NewOpusDecoder(48000, 2)
	if err != nil {
		t.Fatalf("new decoder: %v", err)
	}
	defer dec.Close()

	dstFrame := frame.NewAudioFrameS16(48000, 2, 960)
	_, err = dec.DecodeInto([]byte{}, dstFrame)
	if err != ErrInvalidData {
		t.Errorf("expected ErrInvalidData, got %v", err)
	}
}

func TestOpusDecoder_NilFrame(t *testing.T) {
	testutil.SkipIfNoShim(t)

	dec, err := NewOpusDecoder(48000, 2)
	if err != nil {
		t.Fatalf("new decoder: %v", err)
	}
	defer dec.Close()

	_, err = dec.DecodeInto([]byte{1, 2, 3}, nil)
	if err != ErrBufferTooSmall {
		t.Errorf("expected ErrBufferTooSmall, got %v", err)
	}
}

func TestOpusDecoder_DifferentConfigs(t *testing.T) {
	testutil.SkipIfNoShim(t)

	configs := []struct {
		name       string
		sampleRate int
		channels   int
	}{
		{"48kHz Stereo", 48000, 2},
		{"48kHz Mono", 48000, 1},
		{"16kHz Mono", 16000, 1},
	}

	for _, cfg := range configs {
		t.Run(cfg.name, func(t *testing.T) {
			enc, err := encoder.NewOpusEncoder(codec.OpusConfig{
				SampleRate: cfg.sampleRate,
				Channels:   cfg.channels,
				Bitrate:    64000,
			})
			if err != nil {
				t.Fatalf("new encoder: %v", err)
			}
			defer enc.Close()

			dec, err := NewOpusDecoder(cfg.sampleRate, cfg.channels)
			if err != nil {
				t.Fatalf("new decoder: %v", err)
			}
			defer dec.Close()

			// 20ms of audio
			samplesPerFrame := cfg.sampleRate / 50
			srcFrame := testutil.CreateSilentAudioFrame(cfg.sampleRate, cfg.channels, samplesPerFrame)
			encBuf := make([]byte, enc.MaxEncodedSize())

			n, err := enc.EncodeInto(srcFrame, encBuf)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}

			dstFrame := frame.NewAudioFrameS16(cfg.sampleRate, cfg.channels, dec.MaxSamplesPerFrame())
			samples, err := dec.DecodeInto(encBuf[:n], dstFrame)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if samples == 0 {
				t.Error("decoded 0 samples")
			}
		})
	}
}

func TestOpusDecoder_ConcurrentDecode(t *testing.T) {
	testutil.SkipIfNoShim(t)

	// First encode a frame
	enc, err := encoder.NewOpusEncoder(codec.OpusConfig{
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		t.Fatalf("new encoder: %v", err)
	}

	srcFrame := testutil.CreateSilentAudioFrame(48000, 2, 960)
	encBuf := make([]byte, enc.MaxEncodedSize())
	n, _ := enc.EncodeInto(srcFrame, encBuf)
	encodedData := make([]byte, n)
	copy(encodedData, encBuf[:n])
	enc.Close()

	// Test concurrent decoding
	dec, err := NewOpusDecoder(48000, 2)
	if err != nil {
		t.Fatalf("new decoder: %v", err)
	}
	defer dec.Close()

	const numGoroutines = 4

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dstFrame := frame.NewAudioFrameS16(48000, 2, dec.MaxSamplesPerFrame())
			_, err := dec.DecodeInto(encodedData, dstFrame)
			if err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent decode error: %v", err)
	}
}

func TestNewAudioDecoder_Factory(t *testing.T) {
	testutil.SkipIfNoShim(t)

	dec, err := NewAudioDecoder(codec.Opus, 48000, 2)
	if err != nil {
		t.Fatalf("NewAudioDecoder: %v", err)
	}
	defer dec.Close()

	if dec.Codec() != codec.Opus {
		t.Errorf("Codec() = %v, want Opus", dec.Codec())
	}
}

func TestNewAudioDecoder_UnsupportedCodec(t *testing.T) {
	testutil.SkipIfNoShim(t)

	_, err := NewAudioDecoder(codec.H264, 48000, 2) // video codec
	if err != ErrUnsupportedCodec {
		t.Errorf("expected ErrUnsupportedCodec, got %v", err)
	}
}
