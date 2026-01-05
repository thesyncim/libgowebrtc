package packetizer

import (
	"testing"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

func TestPacketizerErrors(t *testing.T) {
	errors := []error{
		ErrPacketizerClosed,
		ErrBufferTooSmall,
		ErrInvalidData,
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

func TestConfigDefaults(t *testing.T) {
	configs := []struct {
		name      string
		codec     codec.Type
		clockRate uint32
	}{
		{"H264", codec.H264, 90000},
		{"VP8", codec.VP8, 90000},
		{"VP9", codec.VP9, 90000},
		{"AV1", codec.AV1, 90000},
		{"Opus", codec.Opus, 48000},
	}

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{
				Codec:       tc.codec,
				SSRC:        1,
				PayloadType: 96,
			}

			// ClockRate defaults to codec's clock rate when 0
			if cfg.ClockRate == 0 {
				cfg.ClockRate = tc.codec.ClockRate()
			}

			if cfg.ClockRate != tc.clockRate {
				t.Errorf("ClockRate = %v, want %v", cfg.ClockRate, tc.clockRate)
			}
		})
	}
}

func TestMaxPacketsCalculation(t *testing.T) {
	// Create a mock packetizer with known config
	p := &packetizer{
		config: Config{
			MTU: 1200,
		},
	}

	tests := []struct {
		frameSize   int
		minExpected int
	}{
		{1000, 1},     // Small frame, single packet
		{5000, 4},     // Medium frame
		{50000, 40},   // Large frame (keyframe)
		{200000, 180}, // Very large frame (4K keyframe)
	}

	for _, tt := range tests {
		maxPkts := p.MaxPackets(tt.frameSize)
		if maxPkts < tt.minExpected {
			t.Errorf("MaxPackets(%d) = %d, want at least %d", tt.frameSize, maxPkts, tt.minExpected)
		}
	}
}

func TestMaxPacketSize(t *testing.T) {
	p := &packetizer{
		config: Config{
			MTU: 1200,
		},
	}

	if p.MaxPacketSize() != 1200 {
		t.Errorf("MaxPacketSize() = %d, want 1200", p.MaxPacketSize())
	}

	p.config.MTU = 1400
	if p.MaxPacketSize() != 1400 {
		t.Errorf("MaxPacketSize() = %d, want 1400", p.MaxPacketSize())
	}
}

func BenchmarkMaxPackets(b *testing.B) {
	p := &packetizer{
		config: Config{
			MTU: 1200,
		},
	}

	for i := 0; i < b.N; i++ {
		_ = p.MaxPackets(50000)
	}
}
