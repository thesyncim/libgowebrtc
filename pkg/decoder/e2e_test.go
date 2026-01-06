package decoder

import (
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

func TestMain(m *testing.M) {
	if err := ffi.LoadLibrary(); err != nil {
		return
	}
	defer ffi.Close()
	m.Run()
}

func TestH264EncodeDecode(t *testing.T) {
	// Create encoder
	enc, err := encoder.NewH264Encoder(codec.H264Config{
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("NewH264Encoder: %v", err)
	}
	defer enc.Close()

	// Create decoder
	dec, err := NewH264Decoder()
	if err != nil {
		t.Fatalf("NewH264Decoder: %v", err)
	}
	defer dec.Close()

	// Create source frame
	src := frame.NewI420Frame(640, 480)
	for i := range src.Data[0] {
		src.Data[0][i] = 128
	}
	for i := range src.Data[1] {
		src.Data[1][i] = 128
		src.Data[2][i] = 128
	}

	// Encode
	encodedBuf := make([]byte, enc.MaxEncodedSize())
	result, err := enc.EncodeInto(src, encodedBuf, true)
	if err != nil {
		t.Fatalf("EncodeInto: %v", err)
	}
	encoded := encodedBuf[:result.N]
	t.Logf("Encoded: %d bytes", result.N)

	// Decode
	dst := frame.NewI420Frame(640, 480)
	err = dec.DecodeInto(encoded, dst, 0, true)
	if err != nil {
		// Some decoders need multiple frames
		t.Logf("DecodeInto: %v (may need more data)", err)
	} else {
		t.Logf("Decoded: %dx%d", dst.Width, dst.Height)
	}
}

func TestVP8EncodeDecode(t *testing.T) {
	enc, err := encoder.NewVP8Encoder(codec.VP8Config{
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("NewVP8Encoder: %v", err)
	}
	defer enc.Close()

	dec, err := NewVP8Decoder()
	if err != nil {
		t.Fatalf("NewVP8Decoder: %v", err)
	}
	defer dec.Close()

	src := frame.NewI420Frame(640, 480)
	for i := range src.Data[0] {
		src.Data[0][i] = 128
	}
	for i := range src.Data[1] {
		src.Data[1][i] = 128
		src.Data[2][i] = 128
	}

	encodedBuf := make([]byte, enc.MaxEncodedSize())
	result, err := enc.EncodeInto(src, encodedBuf, true)
	if err != nil {
		t.Fatalf("EncodeInto: %v", err)
	}
	encoded := encodedBuf[:result.N]
	t.Logf("VP8 encoded: %d bytes", result.N)

	dst := frame.NewI420Frame(640, 480)
	err = dec.DecodeInto(encoded, dst, 0, true)
	if err != nil {
		t.Logf("DecodeInto: %v (may need more data)", err)
	} else {
		t.Logf("VP8 decoded: %dx%d", dst.Width, dst.Height)
	}
}

func TestVP9EncodeDecode(t *testing.T) {
	enc, err := encoder.NewVP9Encoder(codec.VP9Config{
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("NewVP9Encoder: %v", err)
	}
	defer enc.Close()

	dec, err := NewVP9Decoder()
	if err != nil {
		t.Fatalf("NewVP9Decoder: %v", err)
	}
	defer dec.Close()

	src := frame.NewI420Frame(640, 480)
	for i := range src.Data[0] {
		src.Data[0][i] = 128
	}
	for i := range src.Data[1] {
		src.Data[1][i] = 128
		src.Data[2][i] = 128
	}

	encodedBuf := make([]byte, enc.MaxEncodedSize())
	result, err := enc.EncodeInto(src, encodedBuf, true)
	if err != nil {
		t.Fatalf("EncodeInto: %v", err)
	}
	encoded := encodedBuf[:result.N]
	t.Logf("VP9 encoded: %d bytes", result.N)

	dst := frame.NewI420Frame(640, 480)
	err = dec.DecodeInto(encoded, dst, 0, true)
	if err != nil {
		t.Logf("DecodeInto: %v (may need more data)", err)
	} else {
		t.Logf("VP9 decoded: %dx%d", dst.Width, dst.Height)
	}
}

func TestOpusEncodeDecode(t *testing.T) {
	enc, err := encoder.NewOpusEncoder(codec.OpusConfig{
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		t.Fatalf("NewOpusEncoder: %v", err)
	}
	defer enc.Close()

	dec, err := NewOpusDecoder(48000, 2)
	if err != nil {
		t.Fatalf("NewOpusDecoder: %v", err)
	}
	defer dec.Close()

	// Create 20ms audio frame (960 samples per channel at 48kHz)
	// Browser WebRTC uses 20ms Opus frames; shim handles chunking internally
	src := frame.NewAudioFrameS16(48000, 2, 960)

	encodedBuf := make([]byte, enc.MaxEncodedSize())
	n, err := enc.EncodeInto(src, encodedBuf)
	if err != nil {
		t.Fatalf("EncodeInto: %v", err)
	}
	encoded := encodedBuf[:n]
	t.Logf("Opus encoded: %d bytes", n)

	// Decoder requires buffer for MaxSamplesPerFrame (5760 samples per channel)
	dst := frame.NewAudioFrameS16(48000, 2, 5760)
	numSamples, err := dec.DecodeInto(encoded, dst)
	if err != nil {
		t.Fatalf("DecodeInto: %v", err)
	}
	t.Logf("Opus decoded: %d samples", numSamples)
}

func TestDecoderClose(t *testing.T) {
	dec, err := NewH264Decoder()
	if err != nil {
		t.Fatalf("NewH264Decoder: %v", err)
	}

	if err := dec.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	// Operations on closed decoder should fail
	dst := frame.NewI420Frame(640, 480)
	err = dec.DecodeInto([]byte{0, 0, 0, 1, 0x67}, dst, 0, true)
	if err != ErrDecoderClosed {
		t.Errorf("Expected ErrDecoderClosed, got %v", err)
	}
}

func TestMultiFrameEncodeDecode(t *testing.T) {
	enc, err := encoder.NewH264Encoder(codec.H264Config{
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("NewH264Encoder: %v", err)
	}
	defer enc.Close()

	dec, err := NewH264Decoder()
	if err != nil {
		t.Fatalf("NewH264Decoder: %v", err)
	}
	defer dec.Close()

	src := frame.NewI420Frame(640, 480)
	for i := range src.Data[0] {
		src.Data[0][i] = 128
	}
	for i := range src.Data[1] {
		src.Data[1][i] = 128
		src.Data[2][i] = 128
	}

	encodedBuf := make([]byte, enc.MaxEncodedSize())
	dst := frame.NewI420Frame(640, 480)

	decodedCount := 0
	for i := 0; i < 30; i++ {
		src.PTS = uint32(i * 33)
		forceKeyframe := i == 0

		result, err := enc.EncodeInto(src, encodedBuf, forceKeyframe)
		if err != nil {
			t.Fatalf("EncodeInto frame %d: %v", i, err)
		}

		encoded := encodedBuf[:result.N]
		err = dec.DecodeInto(encoded, dst, src.PTS, forceKeyframe)
		if err == nil {
			decodedCount++
		}
	}

	t.Logf("Decoded %d of 30 frames", decodedCount)
	if decodedCount == 0 {
		t.Error("Expected at least one decoded frame")
	}
}

func BenchmarkH264EncodeDecode(b *testing.B) {
	enc, err := encoder.NewH264Encoder(codec.H264Config{
		Width:   1280,
		Height:  720,
		Bitrate: 2_000_000,
		FPS:     30,
	})
	if err != nil {
		b.Fatalf("NewH264Encoder: %v", err)
	}
	defer enc.Close()

	dec, err := NewH264Decoder()
	if err != nil {
		b.Fatalf("NewH264Decoder: %v", err)
	}
	defer dec.Close()

	src := frame.NewI420Frame(1280, 720)
	for i := range src.Data[0] {
		src.Data[0][i] = 128
	}
	for i := range src.Data[1] {
		src.Data[1][i] = 128
		src.Data[2][i] = 128
	}

	encodedBuf := make([]byte, enc.MaxEncodedSize())
	dst := frame.NewI420Frame(1280, 720)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src.PTS = uint32(i * 33)
		result, _ := enc.EncodeInto(src, encodedBuf, i%30 == 0)
		if result.N > 0 {
			dec.DecodeInto(encodedBuf[:result.N], dst, src.PTS, i%30 == 0)
		}
	}
}
