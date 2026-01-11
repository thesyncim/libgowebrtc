package ffi

import (
	"fmt"
	"os"
	"testing"
)

// Integration tests require the shim library to be present.
// Set LIBWEBRTC_SHIM_PATH environment variable to the path of the shim library.

func TestMain(m *testing.M) {
	// Attempt to load library
	if err := LoadLibrary(); err != nil {
		if os.Getenv("LIBWEBRTC_TEST_REQUIRE_SHIM") != "" {
			fmt.Fprintf(os.Stderr, "shim library required: %v\n", err)
			os.Exit(1)
		}
		// Skip all integration tests if library not available
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestLoadLibrary(t *testing.T) {
	if !IsLoaded() {
		t.Fatal("Library should be loaded")
	}
}

func TestListSupportedCodecs(t *testing.T) {
	codecs, err := GetSupportedVideoCodecs()
	if err != nil {
		t.Fatalf("GetSupportedVideoCodecs failed: %v", err)
	}

	t.Logf("Supported video codecs from factory (%d):", len(codecs))
	for _, c := range codecs {
		mimeType := ByteArrayToString(c.MimeType[:])
		fmtpLine := ByteArrayToString(c.SDPFmtpLine[:])
		t.Logf("  %s (PT=%d): %s", mimeType, c.PayloadType, fmtpLine)
	}

	// Check if H264 is available
	h264Available := false
	for _, c := range codecs {
		mimeType := ByteArrayToString(c.MimeType[:])
		if mimeType == "video/H264" {
			h264Available = true
			break
		}
	}
	if !h264Available {
		t.Log("WARNING: H264 is not available in this libwebrtc build")
		t.Log("H264 requires either OpenH264 library or VideoToolbox (macOS)")
	}
}

func TestCreateVideoEncoderH264(t *testing.T) {
	profile := CString("42e01f") // Constrained Baseline
	cfg := &VideoEncoderConfig{
		Width:            1280,
		Height:           720,
		BitrateBps:       2_000_000,
		Framerate:        30.0,
		KeyframeInterval: 60,
		H264Profile:      &profile[0],
		PreferHW:         0,
	}

	handle, err := CreateVideoEncoder(CodecH264, cfg)
	if err != nil {
		t.Fatalf("Failed to create H264 encoder: %v", err)
	}
	defer VideoEncoderDestroy(handle)

	t.Logf("Created H264 encoder: handle=%d", handle)
}

func TestCreateVideoEncoderVP8(t *testing.T) {
	cfg := &VideoEncoderConfig{
		Width:            1280,
		Height:           720,
		BitrateBps:       2_000_000,
		Framerate:        30.0,
		KeyframeInterval: 60,
		PreferHW:         0,
	}

	handle, err := CreateVideoEncoder(CodecVP8, cfg)
	if err != nil {
		t.Fatalf("Failed to create VP8 encoder: %v", err)
	}
	defer VideoEncoderDestroy(handle)

	t.Logf("Created VP8 encoder: handle=%d", handle)
}

func TestCreateVideoEncoderVP9(t *testing.T) {
	cfg := &VideoEncoderConfig{
		Width:            1280,
		Height:           720,
		BitrateBps:       2_000_000,
		Framerate:        30.0,
		KeyframeInterval: 60,
		VP9Profile:       0,
		PreferHW:         0,
	}

	handle, err := CreateVideoEncoder(CodecVP9, cfg)
	if err != nil {
		t.Fatalf("Failed to create VP9 encoder: %v", err)
	}
	defer VideoEncoderDestroy(handle)

	t.Logf("Created VP9 encoder: handle=%d", handle)
}

func TestCreateVideoDecoderH264(t *testing.T) {
	handle, err := CreateVideoDecoder(CodecH264)
	if err != nil {
		t.Fatalf("Failed to create H264 decoder: %v", err)
	}
	defer VideoDecoderDestroy(handle)

	t.Logf("Created H264 decoder: handle=%d", handle)
}

func TestCreateVideoDecoderVP8(t *testing.T) {
	handle, err := CreateVideoDecoder(CodecVP8)
	if err != nil {
		t.Fatalf("Failed to create VP8 decoder: %v", err)
	}
	defer VideoDecoderDestroy(handle)

	t.Logf("Created VP8 decoder: handle=%d", handle)
}

func TestCreateVideoDecoderVP9(t *testing.T) {
	handle, err := CreateVideoDecoder(CodecVP9)
	if err != nil {
		t.Fatalf("Failed to create VP9 decoder: %v", err)
	}
	defer VideoDecoderDestroy(handle)

	t.Logf("Created VP9 decoder: handle=%d", handle)
}

func TestCreateAudioEncoderOpus(t *testing.T) {
	cfg := &AudioEncoderConfig{
		SampleRate: 48000,
		Channels:   2,
		BitrateBps: 64000,
	}

	handle, err := CreateAudioEncoder(cfg)
	if err != nil {
		t.Fatalf("Failed to create Opus encoder: %v", err)
	}
	defer AudioEncoderDestroy(handle)

	t.Logf("Created Opus encoder: handle=%d", handle)
}

func TestCreateAudioDecoderOpus(t *testing.T) {
	handle, err := CreateAudioDecoder(48000, 2)
	if err != nil {
		t.Fatalf("Failed to create Opus decoder: %v", err)
	}
	defer AudioDecoderDestroy(handle)

	t.Logf("Created Opus decoder: handle=%d", handle)
}

func TestCreatePacketizerH264(t *testing.T) {
	cfg := &PacketizerConfig{
		Codec:       int32(CodecH264),
		SSRC:        12345,
		PayloadType: 96,
		MTU:         1200,
		ClockRate:   90000,
	}

	handle := CreatePacketizer(cfg)
	if handle == 0 {
		t.Fatal("Failed to create H264 packetizer")
	}
	defer PacketizerDestroy(handle)

	t.Logf("Created H264 packetizer: handle=%d", handle)
}

func TestCreateDepacketizerH264(t *testing.T) {
	handle := CreateDepacketizer(CodecH264)
	if handle == 0 {
		t.Fatal("Failed to create H264 depacketizer")
	}
	defer DepacketizerDestroy(handle)

	t.Logf("Created H264 depacketizer: handle=%d", handle)
}

func TestVideoEncoderSetBitrate(t *testing.T) {
	cfg := &VideoEncoderConfig{
		Width:      1280,
		Height:     720,
		BitrateBps: 2_000_000,
		Framerate:  30.0,
		PreferHW:   0,
	}

	handle, err := CreateVideoEncoder(CodecVP8, cfg)
	if err != nil {
		t.Fatalf("Failed to create encoder: %v", err)
	}
	defer VideoEncoderDestroy(handle)

	// Change bitrate
	err = VideoEncoderSetBitrate(handle, 4_000_000)
	if err != nil {
		t.Errorf("SetBitrate failed: %v", err)
	}
}

func TestVideoEncoderSetFramerate(t *testing.T) {
	cfg := &VideoEncoderConfig{
		Width:      1280,
		Height:     720,
		BitrateBps: 2_000_000,
		Framerate:  30.0,
		PreferHW:   0,
	}

	handle, err := CreateVideoEncoder(CodecVP8, cfg)
	if err != nil {
		t.Fatalf("Failed to create encoder: %v", err)
	}
	defer VideoEncoderDestroy(handle)

	// Change framerate
	err = VideoEncoderSetFramerate(handle, 60.0)
	if err != nil {
		t.Errorf("SetFramerate failed: %v", err)
	}
}

func TestVideoEncoderEncodeFrame(t *testing.T) {
	profile := CString("42e01f")
	cfg := &VideoEncoderConfig{
		Width:            320,
		Height:           240,
		BitrateBps:       500_000,
		Framerate:        30.0,
		KeyframeInterval: 30,
		H264Profile:      &profile[0],
		PreferHW:         0,
	}

	handle, err := CreateVideoEncoder(CodecH264, cfg)
	if err != nil {
		t.Fatalf("Failed to create encoder: %v", err)
	}
	defer VideoEncoderDestroy(handle)

	// Create test frame (I420 format)
	width, height := 320, 240
	ySize := width * height
	uvSize := (width / 2) * (height / 2)

	yPlane := make([]byte, ySize)
	uPlane := make([]byte, uvSize)
	vPlane := make([]byte, uvSize)

	// Fill with gray (Y=128, U=128, V=128)
	for i := range yPlane {
		yPlane[i] = 128
	}
	for i := range uPlane {
		uPlane[i] = 128
		vPlane[i] = 128
	}

	// Allocate output buffer
	dst := make([]byte, width*height*3/2)

	// Encode frame
	n, isKeyframe, err := encodeUntilOutput(
		t,
		handle,
		yPlane, uPlane, vPlane,
		width, width/2, width/2,
		0,    // timestamp
		true, // force keyframe
		dst,
		5,
	)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if n == 0 {
		t.Error("Encoded size should be > 0")
	}
	if !isKeyframe {
		t.Error("First frame should be keyframe")
	}

	t.Logf("Encoded frame: size=%d, keyframe=%v", n, isKeyframe)
}

func TestAudioEncoderEncodeFrame(t *testing.T) {
	cfg := &AudioEncoderConfig{
		SampleRate: 48000,
		Channels:   2,
		BitrateBps: 64000,
	}

	handle, err := CreateAudioEncoder(cfg)
	if err != nil {
		t.Fatalf("Failed to create encoder: %v", err)
	}
	defer AudioEncoderDestroy(handle)

	// Create test samples (20ms at 48kHz stereo = 960 samples * 2 channels * 2 bytes)
	// Browser WebRTC uses 20ms Opus frames; shim handles chunking internally
	numSamples := 960
	samples := make([]byte, numSamples*2*2) // 16-bit stereo

	// Fill with silence
	for i := range samples {
		samples[i] = 0
	}

	// Allocate output buffer
	dst := make([]byte, 4000) // Opus max frame size

	// Encode frame
	n, err := AudioEncoderEncodeInto(handle, samples, numSamples, dst)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if n == 0 {
		t.Error("Encoded size should be > 0")
	}

	t.Logf("Encoded audio frame: size=%d", n)
}

// Benchmarks that run with the shim

func BenchmarkVideoEncoderCreate(b *testing.B) {
	cfg := &VideoEncoderConfig{
		Width:      1280,
		Height:     720,
		BitrateBps: 2_000_000,
		Framerate:  30.0,
		PreferHW:   0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handle, err := CreateVideoEncoder(CodecVP8, cfg)
		if err != nil {
			b.Fatalf("Failed to create encoder: %v", err)
		}
		VideoEncoderDestroy(handle)
	}
}

func BenchmarkVideoEncoderEncode(b *testing.B) {
	profile := CString("42e01f")
	cfg := &VideoEncoderConfig{
		Width:       320,
		Height:      240,
		BitrateBps:  500_000,
		Framerate:   30.0,
		H264Profile: &profile[0],
		PreferHW:    0,
	}

	handle, err := CreateVideoEncoder(CodecH264, cfg)
	if err != nil {
		b.Fatalf("Failed to create encoder: %v", err)
	}
	defer VideoEncoderDestroy(handle)

	width, height := 320, 240
	ySize := width * height
	uvSize := (width / 2) * (height / 2)

	yPlane := make([]byte, ySize)
	uPlane := make([]byte, uvSize)
	vPlane := make([]byte, uvSize)
	dst := make([]byte, width*height*3/2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = VideoEncoderEncodeInto(
			handle,
			yPlane, uPlane, vPlane,
			width, width/2, width/2,
			uint32(i),
			i%30 == 0, // keyframe every 30 frames
			dst,
		)
	}
}
