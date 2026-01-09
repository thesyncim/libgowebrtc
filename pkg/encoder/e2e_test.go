package encoder

import (
	"os"
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

func TestMain(m *testing.M) {
	if err := ffi.LoadLibrary(); err != nil {
		os.Exit(0) // Skip all tests if shim unavailable
	}
	os.Exit(m.Run())
}

func TestH264EncoderEncode(t *testing.T) {
	enc, err := NewH264Encoder(codec.H264Config{
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("NewH264Encoder: %v", err)
	}
	defer enc.Close()

	// Create gray test frame
	src := frame.NewI420Frame(640, 480)
	for i := range src.Data[0] {
		src.Data[0][i] = 128
	}
	for i := range src.Data[1] {
		src.Data[1][i] = 128
		src.Data[2][i] = 128
	}

	dst := make([]byte, enc.MaxEncodedSize())

	// Encode first frame (should be keyframe)
	result, err := enc.EncodeInto(src, dst, true)
	if err != nil {
		t.Fatalf("EncodeInto: %v", err)
	}
	if result.N == 0 {
		t.Error("Expected encoded data, got 0 bytes")
	}
	if !result.IsKeyframe {
		t.Error("First frame with forceKeyframe=true should be keyframe")
	}
	t.Logf("Encoded keyframe: %d bytes", result.N)

	// Encode more frames
	for i := 1; i < 10; i++ {
		src.PTS = uint32(i * 33)
		result, err := enc.EncodeInto(src, dst, false)
		if err != nil {
			t.Fatalf("EncodeInto frame %d: %v", i, err)
		}
		t.Logf("Frame %d: %d bytes, keyframe=%v", i, result.N, result.IsKeyframe)
	}
}

func TestVP8EncoderEncode(t *testing.T) {
	enc, err := NewVP8Encoder(codec.VP8Config{
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("NewVP8Encoder: %v", err)
	}
	defer enc.Close()

	src := frame.NewI420Frame(640, 480)
	for i := range src.Data[0] {
		src.Data[0][i] = 128
	}
	for i := range src.Data[1] {
		src.Data[1][i] = 128
		src.Data[2][i] = 128
	}

	dst := make([]byte, enc.MaxEncodedSize())

	result, err := enc.EncodeInto(src, dst, true)
	if err != nil {
		t.Fatalf("EncodeInto: %v", err)
	}
	if result.N == 0 {
		t.Error("Expected encoded data")
	}
	t.Logf("VP8 encoded: %d bytes", result.N)
}

func TestVP9EncoderEncode(t *testing.T) {
	enc, err := NewVP9Encoder(codec.VP9Config{
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("NewVP9Encoder: %v", err)
	}
	defer enc.Close()

	src := frame.NewI420Frame(640, 480)
	for i := range src.Data[0] {
		src.Data[0][i] = 128
	}
	for i := range src.Data[1] {
		src.Data[1][i] = 128
		src.Data[2][i] = 128
	}

	dst := make([]byte, enc.MaxEncodedSize())

	result, err := enc.EncodeInto(src, dst, true)
	if err != nil {
		t.Fatalf("EncodeInto: %v", err)
	}
	if result.N == 0 {
		t.Error("Expected encoded data")
	}
	t.Logf("VP9 encoded: %d bytes", result.N)
}

func TestEncoderSetBitrate(t *testing.T) {
	enc, err := NewH264Encoder(codec.H264Config{
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("NewH264Encoder: %v", err)
	}
	defer enc.Close()

	// Change bitrate
	if err := enc.SetBitrate(2_000_000); err != nil {
		t.Errorf("SetBitrate: %v", err)
	}
	if err := enc.SetBitrate(500_000); err != nil {
		t.Errorf("SetBitrate: %v", err)
	}
}

func TestEncoderSetFramerate(t *testing.T) {
	enc, err := NewH264Encoder(codec.H264Config{
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("NewH264Encoder: %v", err)
	}
	defer enc.Close()

	if err := enc.SetFramerate(60); err != nil {
		t.Errorf("SetFramerate: %v", err)
	}
	if err := enc.SetFramerate(15); err != nil {
		t.Errorf("SetFramerate: %v", err)
	}
}

func TestEncoderRequestKeyFrame(t *testing.T) {
	enc, err := NewH264Encoder(codec.H264Config{
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("NewH264Encoder: %v", err)
	}
	defer enc.Close()

	src := frame.NewI420Frame(640, 480)
	for i := range src.Data[0] {
		src.Data[0][i] = 128
	}
	for i := range src.Data[1] {
		src.Data[1][i] = 128
		src.Data[2][i] = 128
	}

	dst := make([]byte, enc.MaxEncodedSize())

	// Encode a few P-frames
	enc.EncodeInto(src, dst, true) // First keyframe
	for i := 0; i < 5; i++ {
		src.PTS = uint32((i + 1) * 33)
		enc.EncodeInto(src, dst, false)
	}

	// Request keyframe
	enc.RequestKeyFrame()

	// Next encode should be keyframe
	src.PTS = uint32(6 * 33)
	result, err := enc.EncodeInto(src, dst, false)
	if err != nil {
		t.Fatalf("EncodeInto: %v", err)
	}
	if !result.IsKeyframe {
		t.Error("Expected keyframe after RequestKeyFrame()")
	}
}

func TestEncoderClose(t *testing.T) {
	enc, err := NewH264Encoder(codec.H264Config{
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("NewH264Encoder: %v", err)
	}

	if err := enc.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	// Operations on closed encoder should fail
	src := frame.NewI420Frame(640, 480)
	dst := make([]byte, 1000000)
	_, err = enc.EncodeInto(src, dst, true)
	if err != ErrEncoderClosed {
		t.Errorf("Expected ErrEncoderClosed, got %v", err)
	}
}

func TestOpusEncoderEncode(t *testing.T) {
	enc, err := NewOpusEncoder(codec.OpusConfig{
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		t.Fatalf("NewOpusEncoder: %v", err)
	}
	defer enc.Close()

	// Create 20ms of audio (960 samples per channel at 48kHz)
	// Browser WebRTC uses 20ms Opus frames; shim handles chunking internally
	src := frame.NewAudioFrameS16(48000, 2, 960)

	dst := make([]byte, enc.MaxEncodedSize())

	n, err := enc.EncodeInto(src, dst)
	if err != nil {
		t.Fatalf("EncodeInto: %v", err)
	}
	if n == 0 {
		t.Error("Expected encoded data")
	}
	t.Logf("Opus encoded: %d bytes", n)
}

func BenchmarkH264Encode(b *testing.B) {
	enc, err := NewH264Encoder(codec.H264Config{
		Width:   1280,
		Height:  720,
		Bitrate: 2_000_000,
		FPS:     30,
	})
	if err != nil {
		b.Fatalf("NewH264Encoder: %v", err)
	}
	defer enc.Close()

	src := frame.NewI420Frame(1280, 720)
	for i := range src.Data[0] {
		src.Data[0][i] = 128
	}
	for i := range src.Data[1] {
		src.Data[1][i] = 128
		src.Data[2][i] = 128
	}

	dst := make([]byte, enc.MaxEncodedSize())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src.PTS = uint32(i * 33)
		enc.EncodeInto(src, dst, i%30 == 0)
	}
}
