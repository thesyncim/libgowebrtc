package ffi

import "testing"

// TestFFIOutputParameters validates that FFI calls correctly write to output parameters.
// This is critical for detecting pointer-passing issues across the Go/C boundary.
func TestFFIOutputParameters(t *testing.T) {
	// Test that video decoder correctly writes output dimensions for each codec
	t.Run("VideoDecoderOutput", func(t *testing.T) {
		codecs := []struct {
			name  string
			codec CodecType
		}{
			{"VP8", CodecVP8},
			{"VP9", CodecVP9},
			{"AV1", CodecAV1},
		}

		for _, tc := range codecs {
			t.Run(tc.name, func(t *testing.T) {
				// Create encoder to produce valid bitstream
				encCfg := &VideoEncoderConfig{
					Width:            320,
					Height:           240,
					BitrateBps:       500000,
					Framerate:        30.0,
					KeyframeInterval: 30,
					PreferHW:         0,
				}

				encoder, err := CreateVideoEncoder(tc.codec, encCfg)
				if err != nil {
					t.Fatalf("CreateVideoEncoder: %v", err)
				}
				defer VideoEncoderDestroy(encoder)

				decoder, err := CreateVideoDecoder(tc.codec)
				if err != nil {
					t.Fatalf("CreateVideoDecoder: %v", err)
				}
				defer VideoDecoderDestroy(decoder)

				// Create test frame and encode
				yPlane := make([]byte, 320*240)
				uPlane := make([]byte, 160*120)
				vPlane := make([]byte, 160*120)
				encBuf := make([]byte, 320*240)

				// Try to encode multiple frames to get output
				var encoded []byte
				for i := 0; i < 5; i++ {
					n, _, err := VideoEncoderEncodeInto(
						encoder, yPlane, uPlane, vPlane,
						320, 160, 160, uint32(i*3000), i == 0, encBuf,
					)
					if err == nil && n > 0 {
						encoded = make([]byte, n)
						copy(encoded, encBuf[:n])
						break
					}
				}

				if len(encoded) == 0 {
					t.Skip("encoder did not produce output")
				}

				// Decode
				decY := make([]byte, 320*240)
				decU := make([]byte, 160*120)
				decV := make([]byte, 160*120)

				// Try multiple decode attempts as decoder may need warmup
				var w, h int
				for i := 0; i < 5; i++ {
					var err error
					w, h, _, _, _, err = VideoDecoderDecodeInto(decoder, encoded, 0, true, decY, decU, decV)
					if err == nil && w > 0 && h > 0 {
						break
					}
				}

				// Validate output dimensions were written correctly
				if w != 320 || h != 240 {
					t.Errorf("%s decoded dimensions %dx%d, want 320x240 (FFI output parameter issue)", tc.name, w, h)
				}
			})
		}
	})

	// Test that video encoder correctly writes output size
	t.Run("VideoEncoderOutput", func(t *testing.T) {
		cfg := &VideoEncoderConfig{
			Width:            320,
			Height:           240,
			BitrateBps:       500000,
			Framerate:        30.0,
			KeyframeInterval: 30,
			PreferHW:         0,
		}

		encoder, err := CreateVideoEncoder(CodecVP8, cfg)
		if err != nil {
			t.Fatalf("CreateVideoEncoder: %v", err)
		}
		defer VideoEncoderDestroy(encoder)

		// Create test frame
		yPlane := make([]byte, 320*240)
		uPlane := make([]byte, 160*120)
		vPlane := make([]byte, 160*120)
		encBuf := make([]byte, 320*240)

		// Encode - output size should be > 0 for keyframe
		n, isKeyframe, err := VideoEncoderEncodeInto(
			encoder, yPlane, uPlane, vPlane,
			320, 160, 160, 0, true, encBuf,
		)
		// Some encoders may need warmup frames
		if err == nil {
			if n <= 0 {
				t.Error("encoded size should be > 0 when no error")
			}
			if !isKeyframe {
				t.Error("first frame with forceKeyframe should be keyframe")
			}
		} else if err != ErrNeedMoreData {
			t.Errorf("unexpected error: %v", err)
		}
	})

	// Test that audio encoder correctly writes output size
	t.Run("AudioEncoderOutput", func(t *testing.T) {
		cfg := &AudioEncoderConfig{
			SampleRate: 48000,
			Channels:   2,
			BitrateBps: 64000,
		}

		encoder, err := CreateAudioEncoder(cfg)
		if err != nil {
			t.Fatalf("CreateAudioEncoder: %v", err)
		}
		defer AudioEncoderDestroy(encoder)

		// 20ms at 48kHz stereo = 960 samples * 2 channels * 2 bytes
		samples := make([]byte, 960*2*2)
		encBuf := make([]byte, 4000)

		n, err := AudioEncoderEncodeInto(encoder, samples, 960, encBuf)
		if err != nil {
			t.Fatalf("AudioEncoderEncodeInto: %v", err)
		}
		if n <= 0 {
			t.Error("encoded size should be > 0")
		}
	})

	// Test that packetizer correctly writes packet count and offsets
	t.Run("PacketizerOutput", func(t *testing.T) {
		cfg := &PacketizerConfig{
			Codec:       int32(CodecVP8),
			SSRC:        12345,
			PayloadType: 96,
			MTU:         1200,
			ClockRate:   90000,
		}

		packetizer := CreatePacketizer(cfg)
		if packetizer == 0 {
			t.Fatal("CreatePacketizer returned 0")
		}
		defer PacketizerDestroy(packetizer)

		testData := make([]byte, 100)
		dstBuf := make([]byte, 1500)
		offsets := make([]int32, 10)
		sizes := make([]int32, 10)

		count, err := PacketizerPacketizeInto(packetizer, testData, 0, true, dstBuf, offsets, sizes, 10)
		if err != nil {
			t.Fatalf("PacketizerPacketizeInto: %v", err)
		}
		if count <= 0 {
			t.Error("packet count should be > 0")
		}
		if sizes[0] <= 0 {
			t.Error("first packet size should be > 0")
		}
	})
}
