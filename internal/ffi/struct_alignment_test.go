package ffi

import (
	"testing"
	"unsafe"
)

// TestStructAlignment verifies that Go struct layouts match C struct layouts.
// This is critical for FFI correctness - mismatches cause crashes or data corruption.
func TestStructAlignment(t *testing.T) {
	// Test VideoEncoderConfig - matches ShimVideoEncoderConfig
	// C layout (64-bit):
	//   int32_t width;           // offset 0, size 4
	//   int32_t height;          // offset 4, size 4
	//   uint32_t bitrate_bps;    // offset 8, size 4
	//   float framerate;         // offset 12, size 4
	//   int32_t keyframe_interval; // offset 16, size 4
	//   <4 bytes padding for pointer alignment>
	//   const char* h264_profile; // offset 24, size 8
	//   int32_t vp9_profile;     // offset 32, size 4
	//   int32_t prefer_hw;       // offset 36, size 4
	//   Total: 40 bytes
	t.Run("VideoEncoderConfig", func(t *testing.T) {
		var v VideoEncoderConfig
		expectedSize := uintptr(40)
		if got := unsafe.Sizeof(v); got != expectedSize {
			t.Errorf("VideoEncoderConfig size = %d, want %d", got, expectedSize)
		}

		checkOffset(t, "Width", unsafe.Offsetof(v.Width), 0)
		checkOffset(t, "Height", unsafe.Offsetof(v.Height), 4)
		checkOffset(t, "BitrateBps", unsafe.Offsetof(v.BitrateBps), 8)
		checkOffset(t, "Framerate", unsafe.Offsetof(v.Framerate), 12)
		checkOffset(t, "KeyframeInterval", unsafe.Offsetof(v.KeyframeInterval), 16)
		checkOffset(t, "H264Profile", unsafe.Offsetof(v.H264Profile), 24) // after 4-byte padding
		checkOffset(t, "VP9Profile", unsafe.Offsetof(v.VP9Profile), 32)
		checkOffset(t, "PreferHW", unsafe.Offsetof(v.PreferHW), 36)
	})

	// Test AudioEncoderConfig - matches ShimAudioEncoderConfig
	// C layout:
	//   int32_t sample_rate;     // offset 0, size 4
	//   int32_t channels;        // offset 4, size 4
	//   uint32_t bitrate_bps;    // offset 8, size 4
	//   Total: 12 bytes
	t.Run("AudioEncoderConfig", func(t *testing.T) {
		var a AudioEncoderConfig
		expectedSize := uintptr(12)
		if got := unsafe.Sizeof(a); got != expectedSize {
			t.Errorf("AudioEncoderConfig size = %d, want %d", got, expectedSize)
		}

		checkOffset(t, "SampleRate", unsafe.Offsetof(a.SampleRate), 0)
		checkOffset(t, "Channels", unsafe.Offsetof(a.Channels), 4)
		checkOffset(t, "BitrateBps", unsafe.Offsetof(a.BitrateBps), 8)
	})

	// Test PacketizerConfig - matches ShimPacketizerConfig
	// C layout:
	//   ShimCodecType codec;     // offset 0, size 4 (enum = int)
	//   uint32_t ssrc;           // offset 4, size 4
	//   uint8_t payload_type;    // offset 8, size 1
	//   <1 byte padding for uint16_t alignment>
	//   uint16_t mtu;            // offset 10, size 2
	//   uint32_t clock_rate;     // offset 12, size 4
	//   Total: 16 bytes
	t.Run("PacketizerConfig", func(t *testing.T) {
		var p PacketizerConfig
		expectedSize := uintptr(16)
		if got := unsafe.Sizeof(p); got != expectedSize {
			t.Errorf("PacketizerConfig size = %d, want %d", got, expectedSize)
		}

		checkOffset(t, "Codec", unsafe.Offsetof(p.Codec), 0)
		checkOffset(t, "SSRC", unsafe.Offsetof(p.SSRC), 4)
		checkOffset(t, "PayloadType", unsafe.Offsetof(p.PayloadType), 8)
		checkOffset(t, "MTU", unsafe.Offsetof(p.MTU), 10)
		checkOffset(t, "ClockRate", unsafe.Offsetof(p.ClockRate), 12)
	})

	// Test ShimErrorBuffer - matches ShimErrorBuffer
	// C layout:
	//   char message[512];       // offset 0, size 512
	//   Total: 512 bytes
	t.Run("ShimErrorBuffer", func(t *testing.T) {
		var e ShimErrorBuffer
		expectedSize := uintptr(512)
		if got := unsafe.Sizeof(e); got != expectedSize {
			t.Errorf("ShimErrorBuffer size = %d, want %d", got, expectedSize)
		}
	})
}

func checkOffset(t *testing.T, field string, got, want uintptr) {
	t.Helper()
	if got != want {
		t.Errorf("%s offset = %d, want %d", field, got, want)
	}
}

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
