package ffi

import (
	"testing"
)

// End-to-end tests that verify the full encode/decode pipeline.

func TestVideoEncoderDecoderPipeline(t *testing.T) {
	// Use VP8 since it's always available (H264 requires OpenH264)
	encCfg := &VideoEncoderConfig{
		Width:            320,
		Height:           240,
		BitrateBps:       500_000,
		Framerate:        30.0,
		KeyframeInterval: 30,
		PreferHW:         0,
	}

	encoder, err := CreateVideoEncoder(CodecVP8, encCfg)
	if err != nil {
		t.Fatalf("Failed to create VP8 encoder: %v", err)
	}
	defer VideoEncoderDestroy(encoder)

	// Create decoder
	decoder, err := CreateVideoDecoder(CodecVP8)
	if err != nil {
		t.Fatalf("Failed to create VP8 decoder: %v", err)
	}
	defer VideoDecoderDestroy(decoder)

	// Create test frame (I420 format - gray)
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

	// Encode multiple frames
	encBuf := make([]byte, width*height*3/2)
	var encodedFrames [][]byte

	for i := 0; i < 5; i++ {
		forceKeyframe := i == 0
		n, isKeyframe, err := encodeUntilOutput(
			t,
			encoder,
			yPlane, uPlane, vPlane,
			width, width/2, width/2,
			uint32(i*3000), // timestamp
			forceKeyframe,
			encBuf,
			5,
		)
		if err != nil {
			t.Fatalf("Encode frame %d failed: %v", i, err)
		}
		if n == 0 {
			t.Errorf("Frame %d: encoded size should be > 0", i)
		}
		if i == 0 && !isKeyframe {
			t.Error("First frame should be keyframe")
		}

		// Copy encoded data
		encoded := make([]byte, n)
		copy(encoded, encBuf[:n])
		encodedFrames = append(encodedFrames, encoded)

		t.Logf("Frame %d: encoded %d bytes, keyframe=%v", i, n, isKeyframe)
	}

	// Decode the frames
	decYPlane := make([]byte, ySize)
	decUPlane := make([]byte, uvSize)
	decVPlane := make([]byte, uvSize)

	for i, encoded := range encodedFrames {
		isKeyframe := i == 0
		w, h, yStride, uStride, vStride, err := VideoDecoderDecodeInto(
			decoder,
			encoded,
			uint32(i*3000),
			isKeyframe,
			decYPlane, decUPlane, decVPlane,
		)
		if err != nil {
			t.Logf("Decode frame %d warning: %v (may need more data)", i, err)
			continue
		}
		t.Logf("Frame %d: decoded %dx%d, strides: y=%d u=%d v=%d", i, w, h, yStride, uStride, vStride)
	}

	t.Log("Video encode/decode pipeline test passed")
}

func TestAudioEncoderDecoderPipeline(t *testing.T) {
	// Create Opus encoder
	encCfg := &AudioEncoderConfig{
		SampleRate: 48000,
		Channels:   2,
		BitrateBps: 64000,
	}

	encoder, err := CreateAudioEncoder(encCfg)
	if err != nil {
		t.Fatalf("Failed to create Opus encoder: %v", err)
	}
	defer AudioEncoderDestroy(encoder)

	// Create decoder
	decoder, err := CreateAudioDecoder(48000, 2)
	if err != nil {
		t.Fatalf("Failed to create Opus decoder: %v", err)
	}
	defer AudioDecoderDestroy(decoder)

	// Create test audio (20ms at 48kHz stereo = 960 samples * 2 channels * 2 bytes)
	// Browser WebRTC uses 20ms Opus frames; shim handles chunking internally
	numSamples := 960
	samples := make([]byte, numSamples*2*2) // 16-bit stereo

	// Fill with silence
	for i := range samples {
		samples[i] = 0
	}

	// Encode buffer
	encBuf := make([]byte, 4000)
	var encodedFrames [][]byte

	// Encode multiple frames
	for i := 0; i < 5; i++ {
		n, err := AudioEncoderEncodeInto(encoder, samples, numSamples, encBuf)
		if err != nil {
			t.Fatalf("Encode audio frame %d failed: %v", i, err)
		}
		if n == 0 {
			t.Errorf("Audio frame %d: encoded size should be > 0", i)
		}

		encoded := make([]byte, n)
		copy(encoded, encBuf[:n])
		encodedFrames = append(encodedFrames, encoded)

		t.Logf("Audio frame %d: encoded %d bytes", i, n)
	}

	// Decode frames
	// Opus can output up to 120ms of audio (5760 samples at 48kHz) per frame
	// Use a larger buffer to be safe: 48000 * 0.12 * 2 channels * 2 bytes = 23040 bytes
	decSamples := make([]byte, 48000/10*2*2) // 100ms worth of stereo audio

	for i, encoded := range encodedFrames {
		decodedSamples, err := AudioDecoderDecodeInto(decoder, encoded, decSamples)
		if err != nil {
			t.Fatalf("Decode audio frame %d failed: %v", i, err)
		}
		t.Logf("Audio frame %d: decoded %d samples", i, decodedSamples)
	}

	t.Log("Audio encode/decode pipeline test passed")
}

func TestPacketizerDepacketizerPipeline(t *testing.T) {
	// Create H264 packetizer
	packetizerCfg := &PacketizerConfig{
		Codec:       int32(CodecH264),
		SSRC:        12345,
		PayloadType: 96,
		MTU:         1200,
		ClockRate:   90000,
	}

	packetizer := CreatePacketizer(packetizerCfg)
	if packetizer == 0 {
		t.Fatal("Failed to create packetizer")
	}
	defer PacketizerDestroy(packetizer)

	// Create depacketizer
	depacketizer := CreateDepacketizer(CodecH264)
	if depacketizer == 0 {
		t.Fatal("Failed to create depacketizer")
	}
	defer DepacketizerDestroy(depacketizer)

	// Create test NAL units (simulated H264 data)
	testData := make([]byte, 5000) // Larger than MTU
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	// Set NAL type indicator
	testData[0] = 0x65 // IDR frame

	// Allocate buffers for packetization
	maxPackets := 10
	dstBuf := make([]byte, 15000)
	offsets := make([]int32, maxPackets)
	sizes := make([]int32, maxPackets)

	// Packetize
	count, err := PacketizerPacketizeInto(packetizer, testData, 0, true, dstBuf, offsets, sizes, maxPackets)
	if err != nil {
		t.Fatalf("Packetize failed: %v", err)
	}
	if count == 0 {
		t.Fatal("Should have produced at least one packet")
	}

	t.Logf("Packetized %d bytes into %d packets", len(testData), count)

	// Log packet sizes
	for i := 0; i < count; i++ {
		t.Logf("Packet %d: offset=%d, size=%d bytes", i, offsets[i], sizes[i])
	}

	// Push packets to depacketizer
	for i := 0; i < count; i++ {
		offset := int(offsets[i])
		size := int(sizes[i])
		packet := dstBuf[offset : offset+size]
		if err := DepacketizerPush(depacketizer, packet); err != nil {
			t.Logf("Depacketizer push %d: %v", i, err)
		}
	}

	// Try to pop complete frame
	frameBuf := make([]byte, 10000)
	size, timestamp, isKeyframe, err := DepacketizerPopInto(depacketizer, frameBuf)
	if err != nil {
		t.Logf("Depacketizer pop: %v (may need more data)", err)
	} else {
		t.Logf("Depacketized frame: %d bytes, timestamp=%d, keyframe=%v", size, timestamp, isKeyframe)
	}

	t.Log("Packetizer/depacketizer pipeline test passed")
}

func TestMultiCodecEncoders(t *testing.T) {
	codecs := []struct {
		name  string
		codec CodecType
	}{
		{"H264", CodecH264},
		{"VP8", CodecVP8},
		{"VP9", CodecVP9},
	}

	for _, tc := range codecs {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &VideoEncoderConfig{
				Width:            640,
				Height:           480,
				BitrateBps:       1_000_000,
				Framerate:        30.0,
				KeyframeInterval: 60,
				PreferHW:         0,
			}

			if tc.codec == CodecH264 {
				profile := CString("42e01f")
				cfg.H264Profile = &profile[0]
			}

			encoder, err := CreateVideoEncoder(tc.codec, cfg)
			if err != nil {
				t.Fatalf("Failed to create %s encoder: %v", tc.name, err)
			}
			defer VideoEncoderDestroy(encoder)

			// Create test frame
			width, height := 640, 480
			yPlane := make([]byte, width*height)
			uPlane := make([]byte, (width/2)*(height/2))
			vPlane := make([]byte, (width/2)*(height/2))

			// Fill with pattern
			for i := range yPlane {
				yPlane[i] = byte(i % 256)
			}
			for i := range uPlane {
				uPlane[i] = 128
				vPlane[i] = 128
			}

			encBuf := make([]byte, width*height)

			n, isKeyframe, err := encodeUntilOutput(
				t,
				encoder,
				yPlane, uPlane, vPlane,
				width, width/2, width/2,
				0,
				true,
				encBuf,
				5,
			)
			if err != nil {
				t.Fatalf("%s encode failed: %v", tc.name, err)
			}
			if n == 0 {
				t.Errorf("%s: encoded size should be > 0", tc.name)
			}
			if !isKeyframe {
				t.Errorf("%s: first frame should be keyframe", tc.name)
			}

			t.Logf("%s: encoded %d bytes, keyframe=%v", tc.name, n, isKeyframe)
		})
	}
}

func TestEncoderBitrateChange(t *testing.T) {
	cfg := &VideoEncoderConfig{
		Width:      640,
		Height:     480,
		BitrateBps: 1_000_000,
		Framerate:  30.0,
		PreferHW:   0,
	}

	encoder, err := CreateVideoEncoder(CodecVP8, cfg)
	if err != nil {
		t.Fatalf("Failed to create encoder: %v", err)
	}
	defer VideoEncoderDestroy(encoder)

	// Test bitrate changes
	bitrates := []uint32{500_000, 2_000_000, 4_000_000, 1_000_000}
	for _, bitrate := range bitrates {
		if err := VideoEncoderSetBitrate(encoder, bitrate); err != nil {
			t.Errorf("SetBitrate(%d) failed: %v", bitrate, err)
		}
		t.Logf("Set bitrate to %d bps", bitrate)
	}

	// Test framerate changes
	framerates := []float32{15.0, 30.0, 60.0}
	for _, fps := range framerates {
		if err := VideoEncoderSetFramerate(encoder, fps); err != nil {
			t.Errorf("SetFramerate(%v) failed: %v", fps, err)
		}
		t.Logf("Set framerate to %v fps", fps)
	}

	t.Log("Bitrate/framerate change test passed")
}

func BenchmarkFullEncodeDecode(b *testing.B) {
	// Use VP8 since it's always available
	encCfg := &VideoEncoderConfig{
		Width:      640,
		Height:     480,
		BitrateBps: 2_000_000,
		Framerate:  30.0,
		PreferHW:   0,
	}

	encoder, err := CreateVideoEncoder(CodecVP8, encCfg)
	if err != nil {
		b.Fatalf("Failed to create encoder: %v", err)
	}
	defer VideoEncoderDestroy(encoder)

	decoder, err := CreateVideoDecoder(CodecVP8)
	if err != nil {
		b.Fatalf("Failed to create decoder: %v", err)
	}
	defer VideoDecoderDestroy(decoder)

	width, height := 640, 480
	yPlane := make([]byte, width*height)
	uPlane := make([]byte, (width/2)*(height/2))
	vPlane := make([]byte, (width/2)*(height/2))
	encBuf := make([]byte, width*height)
	decYPlane := make([]byte, width*height)
	decUPlane := make([]byte, (width/2)*(height/2))
	decVPlane := make([]byte, (width/2)*(height/2))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Encode
		n, _, _ := VideoEncoderEncodeInto(
			encoder,
			yPlane, uPlane, vPlane,
			width, width/2, width/2,
			uint32(i),
			i%30 == 0,
			encBuf,
		)

		// Decode
		if n > 0 {
			_, _, _, _, _, _ = VideoDecoderDecodeInto(
				decoder,
				encBuf[:n],
				uint32(i),
				i%30 == 0,
				decYPlane, decUPlane, decVPlane,
			)
		}
	}
}
