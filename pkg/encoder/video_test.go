package encoder

import (
	"errors"
	"sync"
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/internal/testutil"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/decoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

type videoEncoderFactory struct {
	name       string
	newEncoder func() (VideoEncoder, error)
	newDecoder func() (decoder.VideoDecoder, error)
}

func videoEncoderFactories() []videoEncoderFactory {
	return []videoEncoderFactory{
		{
			name: "H264",
			newEncoder: func() (VideoEncoder, error) {
				return NewH264Encoder(codec.H264Config{
					Width: 320, Height: 240, Bitrate: 500_000, FPS: 30, KeyInterval: 30,
					Profile: codec.H264ProfileConstrainedBase,
				})
			},
			newDecoder: decoder.NewH264Decoder,
		},
		{
			name: "VP8",
			newEncoder: func() (VideoEncoder, error) {
				return NewVP8Encoder(codec.VP8Config{
					Width: 320, Height: 240, Bitrate: 500_000, FPS: 30, KeyInterval: 30,
				})
			},
			newDecoder: decoder.NewVP8Decoder,
		},
		{
			name: "VP9",
			newEncoder: func() (VideoEncoder, error) {
				return NewVP9Encoder(codec.VP9Config{
					Width: 320, Height: 240, Bitrate: 500_000, FPS: 30, KeyInterval: 30,
				})
			},
			newDecoder: decoder.NewVP9Decoder,
		},
		{
			name: "AV1",
			newEncoder: func() (VideoEncoder, error) {
				return NewAV1Encoder(codec.AV1Config{
					Width: 320, Height: 240, Bitrate: 500_000, FPS: 30, KeyInterval: 30,
				})
			},
			newDecoder: decoder.NewAV1Decoder,
		},
	}
}

func TestVideoEncoder_RoundTrip(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoEncoderFactories() {
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

			srcFrame := testutil.CreateTestVideoFrame(320, 240)
			encBuf := make([]byte, enc.MaxEncodedSize())

			result, err := encodeUntilOutput(t, enc, srcFrame, encBuf, true)
			if err != nil {
				t.Fatalf("EncodeInto: %v", err)
			}
			if result.N == 0 {
				t.Fatal("encoded size is 0")
			}
			if !result.IsKeyframe {
				t.Error("first frame should be keyframe")
			}

			dstFrame := frame.NewI420Frame(320, 240)
			err = decodeUntilOutput(t, dec, encBuf[:result.N], dstFrame, 0, true)
			if err != nil {
				t.Fatalf("DecodeInto: %v", err)
			}

			if dstFrame.Width != 320 || dstFrame.Height != 240 {
				t.Errorf("decoded dimensions %dx%d, want 320x240", dstFrame.Width, dstFrame.Height)
			}
		})
	}
}

func TestVideoEncoder_MultiFrameSequence(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoEncoderFactories() {
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

			for i := 0; i < 20; i++ {
				srcFrame.PTS = uint32(i * 3000)

				result, err := encodeUntilOutput(t, enc, srcFrame, encBuf, i == 0)
				if err != nil {
					t.Fatalf("frame %d: EncodeInto: %v", i, err)
				}
				if result.N == 0 {
					t.Fatalf("frame %d: encoded size is 0", i)
				}

				err = dec.DecodeInto(encBuf[:result.N], dstFrame, srcFrame.PTS, result.IsKeyframe)
				if err != nil && err != decoder.ErrNeedMoreData {
					t.Fatalf("frame %d: DecodeInto: %v", i, err)
				}
			}
		})
	}
}

func TestVideoEncoder_EncodeAfterClose(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoEncoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			enc, err := f.newEncoder()
			if err != nil {
				t.Fatalf("new encoder: %v", err)
			}
			enc.Close()

			srcFrame := testutil.CreateGrayVideoFrame(320, 240)
			encBuf := make([]byte, 320*240*2)

			_, err = enc.EncodeInto(srcFrame, encBuf, false)
			if err != ErrEncoderClosed {
				t.Errorf("expected ErrEncoderClosed, got %v", err)
			}
		})
	}
}

func TestVideoEncoder_DoubleClose(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoEncoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			enc, err := f.newEncoder()
			if err != nil {
				t.Fatalf("new encoder: %v", err)
			}
			enc.Close()
			enc.Close() // should not panic
		})
	}
}

func TestVideoEncoder_NilFrame(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoEncoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			enc, err := f.newEncoder()
			if err != nil {
				t.Fatalf("new encoder: %v", err)
			}
			defer enc.Close()

			encBuf := make([]byte, enc.MaxEncodedSize())
			_, err = enc.EncodeInto(nil, encBuf, false)
			if err != ErrInvalidFrame {
				t.Errorf("expected ErrInvalidFrame, got %v", err)
			}
		})
	}
}

func TestVideoEncoder_BufferTooSmall(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoEncoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			enc, err := f.newEncoder()
			if err != nil {
				t.Fatalf("new encoder: %v", err)
			}
			defer enc.Close()

			srcFrame := testutil.CreateGrayVideoFrame(320, 240)
			tinyBuf := make([]byte, 10)

			_, err = enc.EncodeInto(srcFrame, tinyBuf, false)
			if err != ErrBufferTooSmall {
				t.Errorf("expected ErrBufferTooSmall, got %v", err)
			}
		})
	}
}

func TestVideoEncoder_SetBitrateAfterClose(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoEncoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			enc, err := f.newEncoder()
			if err != nil {
				t.Fatalf("new encoder: %v", err)
			}
			enc.Close()

			err = enc.SetBitrate(1_000_000)
			if err != ErrEncoderClosed {
				t.Errorf("expected ErrEncoderClosed, got %v", err)
			}
		})
	}
}

func TestVideoEncoder_RequestKeyFrame(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoEncoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			enc, err := f.newEncoder()
			if err != nil {
				t.Fatalf("new encoder: %v", err)
			}
			defer enc.Close()

			srcFrame := testutil.CreateGrayVideoFrame(320, 240)
			encBuf := make([]byte, enc.MaxEncodedSize())

			// First frame (keyframe)
			result, err := encodeUntilOutput(t, enc, srcFrame, encBuf, true)
			if err != nil {
				t.Fatalf("EncodeInto: %v", err)
			}
			if !result.IsKeyframe {
				t.Error("first frame should be keyframe")
			}

			// A few P-frames
			for i := 0; i < 5; i++ {
				srcFrame.PTS = uint32((i + 1) * 3000)
				_, err := enc.EncodeInto(srcFrame, encBuf, false)
				if err != nil && !errors.Is(err, ffi.ErrNeedMoreData) {
					t.Fatalf("EncodeInto: %v", err)
				}
			}

			// Request keyframe
			enc.RequestKeyFrame()

			// Next frame should be keyframe
			srcFrame.PTS = 6 * 3000
			result, err = encodeUntilOutput(t, enc, srcFrame, encBuf, false)
			if err != nil {
				t.Fatalf("EncodeInto: %v", err)
			}
			if !result.IsKeyframe {
				t.Error("frame after RequestKeyFrame should be keyframe")
			}
		})
	}
}

func TestVideoEncoder_ConcurrentEncode(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoEncoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			enc, err := f.newEncoder()
			if err != nil {
				t.Fatalf("new encoder: %v", err)
			}
			defer enc.Close()

			const numGoroutines = 4
			const framesPerGoroutine = 10

			var wg sync.WaitGroup
			errCh := make(chan error, numGoroutines*framesPerGoroutine)

			for g := 0; g < numGoroutines; g++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					srcFrame := testutil.CreateGrayVideoFrame(320, 240)
					encBuf := make([]byte, enc.MaxEncodedSize())

					for i := 0; i < framesPerGoroutine; i++ {
						srcFrame.PTS = uint32(id*1000 + i*100)
						_, err := enc.EncodeInto(srcFrame, encBuf, i == 0)
						if err != nil && !errors.Is(err, ffi.ErrNeedMoreData) {
							errCh <- err
						}
					}
				}(g)
			}

			wg.Wait()
			close(errCh)

			for err := range errCh {
				t.Errorf("concurrent encode error: %v", err)
			}
		})
	}
}

func TestVideoEncoder_MultipleInstances(t *testing.T) {
	testutil.SkipIfNoShim(t)

	for _, f := range videoEncoderFactories() {
		t.Run(f.name, func(t *testing.T) {
			const numEncoders = 3
			encoders := make([]VideoEncoder, numEncoders)

			for i := 0; i < numEncoders; i++ {
				enc, err := f.newEncoder()
				if err != nil {
					t.Fatalf("new encoder %d: %v", i, err)
				}
				encoders[i] = enc
			}

			srcFrame := testutil.CreateGrayVideoFrame(320, 240)
			for i, enc := range encoders {
				encBuf := make([]byte, enc.MaxEncodedSize())
				result, err := encodeUntilOutput(t, enc, srcFrame, encBuf, true)
				if err != nil {
					t.Errorf("encoder %d: EncodeInto: %v", i, err)
				}
				if result.N == 0 {
					t.Errorf("encoder %d: encoded size is 0", i)
				}
			}

			for _, enc := range encoders {
				enc.Close()
			}
		})
	}
}

// Benchmarks

func BenchmarkVideoEncoder_H264_320x240(b *testing.B) {
	benchmarkVideoEncoder(b, func() (VideoEncoder, error) {
		return NewH264Encoder(codec.H264Config{
			Width: 320, Height: 240, Bitrate: 500_000, FPS: 30,
		})
	}, 320, 240)
}

func BenchmarkVideoEncoder_VP8_320x240(b *testing.B) {
	benchmarkVideoEncoder(b, func() (VideoEncoder, error) {
		return NewVP8Encoder(codec.VP8Config{
			Width: 320, Height: 240, Bitrate: 500_000, FPS: 30,
		})
	}, 320, 240)
}

func BenchmarkVideoEncoder_VP9_320x240(b *testing.B) {
	benchmarkVideoEncoder(b, func() (VideoEncoder, error) {
		return NewVP9Encoder(codec.VP9Config{
			Width: 320, Height: 240, Bitrate: 500_000, FPS: 30,
		})
	}, 320, 240)
}

func BenchmarkVideoEncoder_AV1_320x240(b *testing.B) {
	benchmarkVideoEncoder(b, func() (VideoEncoder, error) {
		return NewAV1Encoder(codec.AV1Config{
			Width: 320, Height: 240, Bitrate: 500_000, FPS: 30,
		})
	}, 320, 240)
}

func BenchmarkVideoEncoder_H264_720p(b *testing.B) {
	benchmarkVideoEncoder(b, func() (VideoEncoder, error) {
		return NewH264Encoder(codec.H264Config{
			Width: 1280, Height: 720, Bitrate: 2_000_000, FPS: 30,
		})
	}, 1280, 720)
}

func benchmarkVideoEncoder(b *testing.B, newEncoder func() (VideoEncoder, error), w, h int) {
	testutil.RequireShim(b)

	enc, err := newEncoder()
	if err != nil {
		b.Fatalf("new encoder: %v", err)
	}
	defer enc.Close()

	srcFrame := testutil.CreateGrayVideoFrame(w, h)
	encBuf := make([]byte, enc.MaxEncodedSize())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		srcFrame.PTS = uint32(i)
		enc.EncodeInto(srcFrame, encBuf, i%30 == 0)
	}
}
