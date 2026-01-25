package decoder

import (
	"sync"
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/testutil"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

type videoDecoderFactory struct {
	name       string
	newDecoder func() (VideoDecoder, error)
	newEncoder func() (encoder.VideoEncoder, error)
}

func videoDecoderFactories() []videoDecoderFactory {
	return []videoDecoderFactory{
		{
			name:       "H264",
			newDecoder: NewH264Decoder,
			newEncoder: func() (encoder.VideoEncoder, error) {
				return encoder.NewVideoEncoder(codec.VideoEncoderConfig{
					Codec: codec.H264, Width: 320, Height: 240, Bitrate: 500_000, FPS: 30, KeyInterval: 30,
					Profile: codec.H264ProfileConstrainedBase,
				})
			},
		},
		{
			name:       "VP8",
			newDecoder: func() (VideoDecoder, error) { return NewVideoDecoder(codec.VP8) },
			newEncoder: func() (encoder.VideoEncoder, error) {
				return encoder.NewVideoEncoder(codec.VideoEncoderConfig{
					Codec: codec.VP8, Width: 320, Height: 240, Bitrate: 500_000, FPS: 30, KeyInterval: 30,
				})
			},
		},
		{
			name:       "VP9",
			newDecoder: func() (VideoDecoder, error) { return NewVideoDecoder(codec.VP9) },
			newEncoder: func() (encoder.VideoEncoder, error) {
				return encoder.NewVideoEncoder(codec.VideoEncoderConfig{
					Codec: codec.VP9, Width: 320, Height: 240, Bitrate: 500_000, FPS: 30, KeyInterval: 30,
				})
			},
		},
		{
			name:       "AV1",
			newDecoder: func() (VideoDecoder, error) { return NewVideoDecoder(codec.AV1) },
			newEncoder: func() (encoder.VideoEncoder, error) {
				return encoder.NewVideoEncoder(codec.VideoEncoderConfig{
					Codec: codec.AV1, Width: 320, Height: 240, Bitrate: 500_000, FPS: 30, KeyInterval: 30,
				})
			},
		},
	}
}

func TestVideoDecoder_DecodeEncodedFrame(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoDecoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			enc, err := f.newEncoder()
			if err != nil {
				t.Fatalf("new encoder: %v", err)
			}
			defer enc.Close()

			dec, err := f.newDecoder()
			if err != nil {
				t.Fatalf("new decoder: %v", err)
			}
			defer dec.Close()

			// Encode a frame
			srcFrame := testutil.CreateTestVideoFrame(320, 240)
			encBuf := make([]byte, enc.MaxEncodedSize())
			result, err := encodeUntilOutput(t, enc, srcFrame, encBuf, true)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}

			// Decode it
			dstFrame := frame.NewI420Frame(320, 240)
			err = decodeUntilOutput(t, dec, encBuf[:result.N], dstFrame, 0, true)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}

			if dstFrame.Width != 320 || dstFrame.Height != 240 {
				t.Errorf("decoded dimensions %dx%d, want 320x240", dstFrame.Width, dstFrame.Height)
			}
			if dstFrame.Format != frame.PixelFormatI420 {
				t.Errorf("format = %v, want I420", dstFrame.Format)
			}
		})
	}
}

func TestVideoDecoder_DecodeSequence(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoDecoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			enc, err := f.newEncoder()
			if err != nil {
				t.Fatalf("new encoder: %v", err)
			}
			defer enc.Close()

			dec, err := f.newDecoder()
			if err != nil {
				t.Fatalf("new decoder: %v", err)
			}
			defer dec.Close()

			srcFrame := testutil.CreateGrayVideoFrame(320, 240)
			encBuf := make([]byte, enc.MaxEncodedSize())
			dstFrame := frame.NewI420Frame(320, 240)

			decodedFrames := 0
			for i := 0; i < 20; i++ {
				srcFrame.PTS = uint32(i * 3000)

				result, err := encodeUntilOutput(t, enc, srcFrame, encBuf, i == 0)
				if err != nil {
					t.Fatalf("encode frame %d: %v", i, err)
				}

				err = dec.DecodeInto(encBuf[:result.N], dstFrame, srcFrame.PTS, result.IsKeyframe)
				if err == nil {
					decodedFrames++
				} else if err != ErrNeedMoreData {
					t.Fatalf("decode frame %d: %v", i, err)
				}
			}

			if decodedFrames == 0 {
				t.Error("expected at least one decoded frame")
			}
		})
	}
}

func TestVideoDecoder_DecodeAfterClose(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoDecoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			dec, err := f.newDecoder()
			if err != nil {
				t.Fatalf("new decoder: %v", err)
			}
			dec.Close()

			dstFrame := frame.NewI420Frame(320, 240)
			err = dec.DecodeInto([]byte{1, 2, 3}, dstFrame, 0, true)
			if err != ErrDecoderClosed {
				t.Errorf("expected ErrDecoderClosed, got %v", err)
			}
		})
	}
}

func TestVideoDecoder_DoubleClose(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoDecoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			dec, err := f.newDecoder()
			if err != nil {
				t.Fatalf("new decoder: %v", err)
			}
			dec.Close()
			dec.Close() // should not panic
		})
	}
}

func TestVideoDecoder_EmptyInput(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoDecoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			dec, err := f.newDecoder()
			if err != nil {
				t.Fatalf("new decoder: %v", err)
			}
			defer dec.Close()

			dstFrame := frame.NewI420Frame(320, 240)
			err = dec.DecodeInto([]byte{}, dstFrame, 0, true)
			if err != ErrInvalidData {
				t.Errorf("expected ErrInvalidData, got %v", err)
			}
		})
	}
}

func TestVideoDecoder_NilInput(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoDecoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			dec, err := f.newDecoder()
			if err != nil {
				t.Fatalf("new decoder: %v", err)
			}
			defer dec.Close()

			dstFrame := frame.NewI420Frame(320, 240)
			err = dec.DecodeInto(nil, dstFrame, 0, true)
			if err != ErrInvalidData {
				t.Errorf("expected ErrInvalidData, got %v", err)
			}
		})
	}
}

func TestVideoDecoder_NilFrame(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoDecoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			dec, err := f.newDecoder()
			if err != nil {
				t.Fatalf("new decoder: %v", err)
			}
			defer dec.Close()

			err = dec.DecodeInto([]byte{1, 2, 3}, nil, 0, true)
			if err != ErrBufferTooSmall {
				t.Errorf("expected ErrBufferTooSmall, got %v", err)
			}
		})
	}
}

func TestVideoDecoder_ConcurrentDecode(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoDecoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			// First encode some frames
			enc, err := f.newEncoder()
			if err != nil {
				t.Fatalf("new encoder: %v", err)
			}

			srcFrame := testutil.CreateGrayVideoFrame(320, 240)
			encBuf := make([]byte, enc.MaxEncodedSize())
			result, err := encodeUntilOutput(t, enc, srcFrame, encBuf, true)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			encodedData := make([]byte, result.N)
			copy(encodedData, encBuf[:result.N])
			enc.Close()

			// Now test concurrent decoding
			dec, err := f.newDecoder()
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
					dstFrame := frame.NewI420Frame(320, 240)
					err := dec.DecodeInto(encodedData, dstFrame, 0, true)
					if err != nil && err != ErrNeedMoreData {
						errCh <- err
					}
				}()
			}

			wg.Wait()
			close(errCh)

			for err := range errCh {
				t.Errorf("concurrent decode error: %v", err)
			}
		})
	}
}

func TestVideoDecoder_MultipleInstances(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoDecoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			const numDecoders = 3
			decoders := make([]VideoDecoder, numDecoders)

			for i := 0; i < numDecoders; i++ {
				dec, err := f.newDecoder()
				if err != nil {
					t.Fatalf("new decoder %d: %v", i, err)
				}
				decoders[i] = dec
			}

			for _, dec := range decoders {
				dec.Close()
			}
		})
	}
}

func TestNewVideoDecoder_Factory(t *testing.T) {
	testutil.SkipIfNoShim(t)

	codecs := []codec.Type{codec.H264, codec.VP8, codec.VP9, codec.AV1}

	for _, c := range codecs {
		t.Run(c.String(), func(t *testing.T) {
			dec, err := NewVideoDecoder(c)
			if err != nil {
				t.Fatalf("NewVideoDecoder(%s): %v", c, err)
			}
			defer dec.Close()

			if dec.Codec() != c {
				t.Errorf("Codec() = %v, want %v", dec.Codec(), c)
			}
		})
	}
}

func TestNewVideoDecoder_UnsupportedCodec(t *testing.T) {
	testutil.SkipIfNoShim(t)

	_, err := NewVideoDecoder(codec.Opus) // audio codec
	if err != ErrUnsupportedCodec {
		t.Errorf("expected ErrUnsupportedCodec, got %v", err)
	}
}
