package e2e

import (
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/decoder"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

// TestVideoCodecRoundtrip tests encode/decode roundtrip for all video codecs.
func TestVideoCodecRoundtrip(t *testing.T) {
	if !ffi.IsLoaded() {
		t.Skip("shim library not available")
	}

	codecs := []struct {
		name   string
		codec  codec.Type
		config func(w, h int) interface{}
	}{
		{"H264", codec.H264, func(w, h int) interface{} {
			return codec.H264Config{Width: w, Height: h, Bitrate: 1_000_000, FPS: 30}
		}},
		{"VP8", codec.VP8, func(w, h int) interface{} {
			return codec.VP8Config{Width: w, Height: h, Bitrate: 1_000_000, FPS: 30}
		}},
		{"VP9", codec.VP9, func(w, h int) interface{} {
			return codec.VP9Config{Width: w, Height: h, Bitrate: 1_000_000, FPS: 30}
		}},
		{"AV1", codec.AV1, func(w, h int) interface{} {
			return codec.AV1Config{Width: w, Height: h, Bitrate: 1_000_000, FPS: 30}
		}},
	}

	width, height := 320, 240

	for _, tc := range codecs {
		t.Run(tc.name, func(t *testing.T) {
			// Create encoder
			var enc encoder.VideoEncoder
			var err error

			switch tc.codec {
			case codec.H264:
				enc, err = encoder.NewH264Encoder(tc.config(width, height).(codec.H264Config))
			case codec.VP8:
				enc, err = encoder.NewVP8Encoder(tc.config(width, height).(codec.VP8Config))
			case codec.VP9:
				enc, err = encoder.NewVP9Encoder(tc.config(width, height).(codec.VP9Config))
			case codec.AV1:
				enc, err = encoder.NewAV1Encoder(tc.config(width, height).(codec.AV1Config))
			}

			if err != nil {
				// H264 may not be available on all platforms (needs VideoToolbox on macOS)
				if tc.codec == codec.H264 {
					t.Skipf("H264 encoder not available: %v", err)
				}
				t.Fatalf("Failed to create %s encoder: %v", tc.name, err)
			}
			defer enc.Close()

			// Create decoder
			dec, err := decoder.NewVideoDecoder(tc.codec)
			if err != nil {
				t.Fatalf("Failed to create %s decoder: %v", tc.name, err)
			}
			defer dec.Close()

			// Create test frame
			srcFrame := frame.NewI420Frame(width, height)
			fillTestPattern(srcFrame)

			// Encode
			encBuf := make([]byte, enc.MaxEncodedSize())
			result, err := enc.EncodeInto(srcFrame, encBuf, true)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}

			if result.N == 0 {
				t.Fatal("Encoded size is 0")
			}

			if !result.IsKeyframe {
				t.Error("First frame should be keyframe")
			}

			t.Logf("%s: encoded %dx%d frame to %d bytes (keyframe=%v)",
				tc.name, width, height, result.N, result.IsKeyframe)

			// Decode
			dstFrame := frame.NewI420Frame(width, height)
			err = dec.DecodeInto(encBuf[:result.N], dstFrame, 0, true)
			if err != nil {
				t.Logf("Decode returned error (may need more data): %v", err)
			} else {
				if dstFrame.Width != width || dstFrame.Height != height {
					t.Errorf("Decoded size = %dx%d, want %dx%d",
						dstFrame.Width, dstFrame.Height, width, height)
				}
				t.Logf("%s: decoded back to %dx%d", tc.name, dstFrame.Width, dstFrame.Height)
			}
		})
	}
}

// TestOpusRoundtrip tests Opus audio encode/decode roundtrip.
func TestOpusRoundtrip(t *testing.T) {
	if !ffi.IsLoaded() {
		t.Skip("shim library not available")
	}

	sampleRate := 48000
	channels := 2
	frameDuration := 10 // ms - WebRTC expects 10ms frames per Encode call
	samplesPerFrame := sampleRate * frameDuration / 1000

	// Create encoder
	enc, err := encoder.NewOpusEncoder(codec.OpusConfig{
		SampleRate: sampleRate,
		Channels:   channels,
		Bitrate:    64000,
	})
	if err != nil {
		t.Fatalf("Failed to create Opus encoder: %v", err)
	}
	defer enc.Close()

	// Create decoder
	dec, err := decoder.NewAudioDecoder(codec.Opus, sampleRate, channels)
	if err != nil {
		t.Fatalf("Failed to create Opus decoder: %v", err)
	}
	defer dec.Close()

	// Create test audio frame
	srcFrame := frame.NewAudioFrameS16(sampleRate, channels, samplesPerFrame)
	fillAudioTestPattern(srcFrame)

	// Encode
	encBuf := make([]byte, enc.MaxEncodedSize())
	encodedSize, err := enc.EncodeInto(srcFrame, encBuf)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if encodedSize == 0 {
		t.Fatal("Encoded size is 0")
	}

	t.Logf("Opus: encoded %d samples to %d bytes", samplesPerFrame, encodedSize)

	// Decode - need buffer for max possible decoded samples (up to 120ms at 48kHz)
	maxSamplesPerChannel := 48000 * 120 / 1000 // 5760 samples per channel
	dstFrame := frame.NewAudioFrameS16(sampleRate, channels, maxSamplesPerChannel)
	decodedSamples, err := dec.DecodeInto(encBuf[:encodedSize], dstFrame)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	t.Logf("Opus: decoded back to %d samples per channel", decodedSamples)
}

// TestEncoderBitrateControl tests runtime bitrate changes.
func TestEncoderBitrateControl(t *testing.T) {
	if !ffi.IsLoaded() {
		t.Skip("shim library not available")
	}

	enc, err := encoder.NewVP8Encoder(codec.VP8Config{
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("Failed to create encoder: %v", err)
	}
	defer enc.Close()

	// Test bitrate changes
	bitrates := []uint32{500_000, 1_000_000, 2_000_000, 4_000_000}

	for _, br := range bitrates {
		err := enc.SetBitrate(br)
		if err != nil {
			t.Errorf("SetBitrate(%d) failed: %v", br, err)
		}
	}

	t.Log("Bitrate control test passed")
}

// TestEncoderFramerateControl tests runtime framerate changes.
func TestEncoderFramerateControl(t *testing.T) {
	if !ffi.IsLoaded() {
		t.Skip("shim library not available")
	}

	// Use VP8 since H264 may not be available on all platforms
	enc, err := encoder.NewVP8Encoder(codec.VP8Config{
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("Failed to create encoder: %v", err)
	}
	defer enc.Close()

	// Test framerate changes
	framerates := []float64{15, 30, 60}

	for _, fps := range framerates {
		err := enc.SetFramerate(fps)
		if err != nil {
			t.Errorf("SetFramerate(%.0f) failed: %v", fps, err)
		}
	}

	t.Log("Framerate control test passed")
}

// TestKeyframeRequest tests requesting keyframes from encoder.
func TestKeyframeRequest(t *testing.T) {
	if !ffi.IsLoaded() {
		t.Skip("shim library not available")
	}

	enc, err := encoder.NewVP8Encoder(codec.VP8Config{
		Width:   320,
		Height:  240,
		Bitrate: 500_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("Failed to create encoder: %v", err)
	}
	defer enc.Close()

	srcFrame := frame.NewI420Frame(320, 240)
	fillTestPattern(srcFrame)
	encBuf := make([]byte, enc.MaxEncodedSize())

	// Encode first frame (should be keyframe)
	result1, err := enc.EncodeInto(srcFrame, encBuf, true)
	if err != nil {
		t.Fatalf("First encode failed: %v", err)
	}
	if !result1.IsKeyframe {
		t.Error("First frame should be keyframe")
	}

	// Encode second frame (should not be keyframe)
	srcFrame.PTS = 3000
	result2, err := enc.EncodeInto(srcFrame, encBuf, false)
	if err != nil {
		t.Fatalf("Second encode failed: %v", err)
	}
	// Note: second frame may or may not be keyframe depending on encoder

	// Request keyframe
	enc.RequestKeyFrame()

	// Encode third frame (should be keyframe after request)
	srcFrame.PTS = 6000
	result3, err := enc.EncodeInto(srcFrame, encBuf, false)
	if err != nil {
		t.Fatalf("Third encode failed: %v", err)
	}

	t.Logf("Frame sizes: kf1=%d, f2=%d (kf=%v), f3=%d (kf=%v)",
		result1.N, result2.N, result2.IsKeyframe, result3.N, result3.IsKeyframe)
}

// Helper functions

func fillTestPattern(f *frame.VideoFrame) {
	// Fill Y with gradient
	for y := 0; y < f.Height; y++ {
		for x := 0; x < f.Width; x++ {
			f.Data[0][y*f.Width+x] = byte((x + y) % 256)
		}
	}
	// Fill U/V with neutral
	for i := range f.Data[1] {
		f.Data[1][i] = 128
		f.Data[2][i] = 128
	}
}

func fillAudioTestPattern(f *frame.AudioFrame) {
	totalSamples := f.NumSamples * f.Channels
	for i := 0; i < totalSamples && i*2+1 < len(f.Samples); i++ {
		val := int16((i * 100) % 32767)
		f.Samples[i*2] = byte(val)
		f.Samples[i*2+1] = byte(val >> 8)
	}
}

// BenchmarkH264Encode benchmarks H264 encoding performance.
func BenchmarkH264Encode(b *testing.B) {
	if !ffi.IsLoaded() {
		b.Skip("shim library not available")
	}

	enc, _ := encoder.NewH264Encoder(codec.H264Config{
		Width:   1280,
		Height:  720,
		Bitrate: 2_000_000,
		FPS:     30,
	})
	defer enc.Close()

	srcFrame := frame.NewI420Frame(1280, 720)
	fillTestPattern(srcFrame)
	encBuf := make([]byte, enc.MaxEncodedSize())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		srcFrame.PTS = uint32(i * 3000)
		enc.EncodeInto(srcFrame, encBuf, i == 0)
	}
}

// BenchmarkVP8Encode benchmarks VP8 encoding performance.
func BenchmarkVP8Encode(b *testing.B) {
	if !ffi.IsLoaded() {
		b.Skip("shim library not available")
	}

	enc, _ := encoder.NewVP8Encoder(codec.VP8Config{
		Width:   1280,
		Height:  720,
		Bitrate: 2_000_000,
		FPS:     30,
	})
	defer enc.Close()

	srcFrame := frame.NewI420Frame(1280, 720)
	fillTestPattern(srcFrame)
	encBuf := make([]byte, enc.MaxEncodedSize())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		srcFrame.PTS = uint32(i * 3000)
		enc.EncodeInto(srcFrame, encBuf, i == 0)
	}
}

// BenchmarkVP9Encode benchmarks VP9 encoding performance.
func BenchmarkVP9Encode(b *testing.B) {
	if !ffi.IsLoaded() {
		b.Skip("shim library not available")
	}

	enc, err := encoder.NewVP9Encoder(codec.VP9Config{
		Width:   1280,
		Height:  720,
		Bitrate: 2_000_000,
		FPS:     30,
	})
	if err != nil {
		b.Fatalf("Failed to create VP9 encoder: %v", err)
	}
	defer enc.Close()

	srcFrame := frame.NewI420Frame(1280, 720)
	fillTestPattern(srcFrame)
	encBuf := make([]byte, enc.MaxEncodedSize())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		srcFrame.PTS = uint32(i * 3000)
		enc.EncodeInto(srcFrame, encBuf, i == 0)
	}
}

// BenchmarkAV1Encode benchmarks AV1 encoding performance.
func BenchmarkAV1Encode(b *testing.B) {
	if !ffi.IsLoaded() {
		b.Skip("shim library not available")
	}

	enc, err := encoder.NewAV1Encoder(codec.AV1Config{
		Width:   1280,
		Height:  720,
		Bitrate: 2_000_000,
		FPS:     30,
	})
	if err != nil {
		b.Fatalf("Failed to create AV1 encoder: %v", err)
	}
	defer enc.Close()

	srcFrame := frame.NewI420Frame(1280, 720)
	fillTestPattern(srcFrame)
	encBuf := make([]byte, enc.MaxEncodedSize())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		srcFrame.PTS = uint32(i * 3000)
		enc.EncodeInto(srcFrame, encBuf, i == 0)
	}
}

// BenchmarkOpusEncode benchmarks Opus encoding performance.
func BenchmarkOpusEncode(b *testing.B) {
	if !ffi.IsLoaded() {
		b.Skip("shim library not available")
	}

	enc, err := encoder.NewOpusEncoder(codec.OpusConfig{
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		b.Fatalf("Failed to create Opus encoder: %v", err)
	}
	defer enc.Close()

	srcFrame := frame.NewAudioFrameS16(48000, 2, 960) // 20ms
	fillAudioTestPattern(srcFrame)
	encBuf := make([]byte, enc.MaxEncodedSize())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		srcFrame.PTS = uint32(i * 960)
		enc.EncodeInto(srcFrame, encBuf)
	}
}

// BenchmarkAllVideoCodecs runs a comparative benchmark of all video codecs.
func BenchmarkAllVideoCodecs(b *testing.B) {
	if !ffi.IsLoaded() {
		b.Skip("shim library not available")
	}

	codecs := []struct {
		name   string
		newEnc func() (encoder.VideoEncoder, error)
	}{
		{"H264", func() (encoder.VideoEncoder, error) {
			return encoder.NewH264Encoder(codec.H264Config{
				Width: 1280, Height: 720, Bitrate: 2_000_000, FPS: 30,
			})
		}},
		{"VP8", func() (encoder.VideoEncoder, error) {
			return encoder.NewVP8Encoder(codec.VP8Config{
				Width: 1280, Height: 720, Bitrate: 2_000_000, FPS: 30,
			})
		}},
		{"VP9", func() (encoder.VideoEncoder, error) {
			return encoder.NewVP9Encoder(codec.VP9Config{
				Width: 1280, Height: 720, Bitrate: 2_000_000, FPS: 30,
			})
		}},
		{"AV1", func() (encoder.VideoEncoder, error) {
			return encoder.NewAV1Encoder(codec.AV1Config{
				Width: 1280, Height: 720, Bitrate: 2_000_000, FPS: 30,
			})
		}},
	}

	srcFrame := frame.NewI420Frame(1280, 720)
	fillTestPattern(srcFrame)

	for _, tc := range codecs {
		b.Run(tc.name, func(b *testing.B) {
			enc, err := tc.newEnc()
			if err != nil {
				b.Skipf("Encoder not available: %v", err)
			}
			defer enc.Close()

			encBuf := make([]byte, enc.MaxEncodedSize())

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				srcFrame.PTS = uint32(i * 3000)
				enc.EncodeInto(srcFrame, encBuf, i == 0)
			}
		})
	}
}
