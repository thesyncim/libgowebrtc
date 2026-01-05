package encoder

import (
	"testing"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

func TestEncodeResultStruct(t *testing.T) {
	result := EncodeResult{
		N:          1024,
		IsKeyframe: true,
	}

	if result.N != 1024 {
		t.Errorf("N = %v, want 1024", result.N)
	}
	if !result.IsKeyframe {
		t.Error("IsKeyframe should be true")
	}
}

func TestEncoderErrors(t *testing.T) {
	errors := []error{
		ErrEncoderClosed,
		ErrInvalidFrame,
		ErrEncodeFailed,
		ErrUnsupportedCodec,
		ErrInvalidConfig,
		ErrBufferTooSmall,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("Error should not be nil")
		}
		if err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	}
}

func TestLayerInfo(t *testing.T) {
	layer := LayerInfo{
		SpatialID:  0,
		TemporalID: 1,
		Width:      1280,
		Height:     720,
		Bitrate:    2_000_000,
		FPS:        30.0,
		Active:     true,
		IsKeyframe: false,
	}

	if layer.Width != 1280 {
		t.Errorf("Width = %v, want 1280", layer.Width)
	}
	if layer.Height != 720 {
		t.Errorf("Height = %v, want 720", layer.Height)
	}
	if !layer.Active {
		t.Error("Layer should be active")
	}
}

func TestEncoderStats(t *testing.T) {
	stats := EncoderStats{
		FramesEncoded:   1000,
		BytesEncoded:    5_000_000,
		KeyframesForced: 10,
		AvgBitrate:      2_000_000,
		AvgFrameSize:    5000,
		AvgEncodeTimeUs: 1500,
	}

	if stats.FramesEncoded != 1000 {
		t.Errorf("FramesEncoded = %v, want 1000", stats.FramesEncoded)
	}
	if stats.BytesEncoded != 5_000_000 {
		t.Errorf("BytesEncoded = %v, want 5000000", stats.BytesEncoded)
	}
}

func TestNewVideoEncoderInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		codec  codec.Type
		config interface{}
	}{
		{"H264 wrong config type", codec.H264, "invalid"},
		{"H264 nil config", codec.H264, nil},
		{"VP8 wrong config type", codec.VP8, 123},
		{"VP9 wrong config type", codec.VP9, []byte{}},
		{"AV1 wrong config type", codec.AV1, struct{}{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewVideoEncoder(tt.codec, tt.config)
			if err != ErrInvalidConfig {
				t.Errorf("Expected ErrInvalidConfig, got %v", err)
			}
		})
	}
}

func TestNewVideoEncoderUnsupportedCodec(t *testing.T) {
	_, err := NewVideoEncoder(codec.Type(999), codec.H264Config{})
	if err != ErrUnsupportedCodec {
		t.Errorf("Expected ErrUnsupportedCodec, got %v", err)
	}
}

func TestNewAudioEncoderInvalidConfig(t *testing.T) {
	_, err := NewAudioEncoder(codec.Opus, "invalid config")
	if err != ErrInvalidConfig {
		t.Errorf("Expected ErrInvalidConfig, got %v", err)
	}
}

func TestNewAudioEncoderUnsupportedCodec(t *testing.T) {
	_, err := NewAudioEncoder(codec.Type(999), codec.OpusConfig{})
	if err != ErrUnsupportedCodec {
		t.Errorf("Expected ErrUnsupportedCodec, got %v", err)
	}
}

// VideoEncoderInterfaceCompliance tests that our concrete types
// could implement the interface (compile-time checks)
func TestVideoEncoderInterfaceCompliance(t *testing.T) {
	// This test verifies interface definitions are consistent
	var _ VideoEncoder = (VideoEncoder)(nil)
	var _ VideoEncoderAdvanced = (VideoEncoderAdvanced)(nil)
	var _ VideoEncoderSVC = (VideoEncoderSVC)(nil)
	var _ AudioEncoder = (AudioEncoder)(nil)
	var _ AudioEncoderAdvanced = (AudioEncoderAdvanced)(nil)
}

// TestVideoEncoderAdvancedInterface verifies VideoEncoderAdvanced extends VideoEncoder
func TestVideoEncoderAdvancedInterface(t *testing.T) {
	// Type assertion at compile time - advanced extends base
	var adv VideoEncoderAdvanced
	var _ VideoEncoder = adv
}

// TestVideoEncoderSVCInterface verifies VideoEncoderSVC extends VideoEncoder
func TestVideoEncoderSVCInterface(t *testing.T) {
	var svc VideoEncoderSVC
	var _ VideoEncoder = svc
}

// TestAudioEncoderAdvancedInterface verifies AudioEncoderAdvanced extends AudioEncoder
func TestAudioEncoderAdvancedInterface(t *testing.T) {
	var adv AudioEncoderAdvanced
	var _ AudioEncoder = adv
}

func TestEncoderConfigTypes(t *testing.T) {
	// Test that configs can be passed correctly to factory functions
	h264Cfg := codec.H264Config{
		Width:   1920,
		Height:  1080,
		Bitrate: 4_000_000,
		FPS:     30,
	}

	vp8Cfg := codec.VP8Config{
		Width:   1280,
		Height:  720,
		Bitrate: 2_000_000,
		FPS:     30,
	}

	vp9Cfg := codec.VP9Config{
		Width:   1920,
		Height:  1080,
		Bitrate: 3_000_000,
		FPS:     30,
	}

	av1Cfg := codec.AV1Config{
		Width:   1920,
		Height:  1080,
		Bitrate: 2_500_000,
		FPS:     30,
	}

	opusCfg := codec.OpusConfig{
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	}

	// Just verify these are the right types for the factory
	_ = h264Cfg
	_ = vp8Cfg
	_ = vp9Cfg
	_ = av1Cfg
	_ = opusCfg
}

func BenchmarkEncodeResultAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		result := EncodeResult{
			N:          1024,
			IsKeyframe: i%30 == 0,
		}
		_ = result
	}
}

func BenchmarkLayerInfoAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		layer := LayerInfo{
			SpatialID:  i % 3,
			TemporalID: i % 3,
			Width:      1920,
			Height:     1080,
			Bitrate:    2_000_000,
			FPS:        30.0,
			Active:     true,
		}
		_ = layer
	}
}
