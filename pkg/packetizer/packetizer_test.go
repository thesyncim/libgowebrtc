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

func TestConfig(t *testing.T) {
	cfg := Config{
		Codec:       codec.H264,
		SSRC:        12345,
		PayloadType: 96,
		MTU:         1200,
		ClockRate:   90000,
	}

	if cfg.Codec != codec.H264 {
		t.Errorf("Codec = %v, want H264", cfg.Codec)
	}
	if cfg.SSRC != 12345 {
		t.Errorf("SSRC = %v, want 12345", cfg.SSRC)
	}
	if cfg.PayloadType != 96 {
		t.Errorf("PayloadType = %v, want 96", cfg.PayloadType)
	}
	if cfg.MTU != 1200 {
		t.Errorf("MTU = %v, want 1200", cfg.MTU)
	}
	if cfg.ClockRate != 90000 {
		t.Errorf("ClockRate = %v, want 90000", cfg.ClockRate)
	}
}

func TestPacketInfo(t *testing.T) {
	info := PacketInfo{
		Offset: 0,
		Size:   1200,
	}

	if info.Offset != 0 {
		t.Errorf("Offset = %v, want 0", info.Offset)
	}
	if info.Size != 1200 {
		t.Errorf("Size = %v, want 1200", info.Size)
	}
}

func TestPacketizerInterface(t *testing.T) {
	// Compile-time check that packetizer implements Packetizer
	var _ Packetizer = (*packetizer)(nil)
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

func TestPacketizerConfigCodecs(t *testing.T) {
	codecs := []struct {
		codec codec.Type
		name  string
	}{
		{codec.H264, "H264"},
		{codec.VP8, "VP8"},
		{codec.VP9, "VP9"},
		{codec.AV1, "AV1"},
		{codec.Opus, "Opus"},
	}

	for _, tc := range codecs {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{
				Codec:       tc.codec,
				SSRC:        1,
				PayloadType: 96,
				MTU:         1200,
			}

			if cfg.Codec != tc.codec {
				t.Errorf("Codec = %v, want %v", cfg.Codec, tc.codec)
			}
		})
	}
}

func TestPacketInfoSlice(t *testing.T) {
	// Test allocation of PacketInfo slice
	packets := make([]PacketInfo, 100)

	// Simulate filling packet info
	for i := 0; i < 10; i++ {
		packets[i] = PacketInfo{
			Offset: i * 1200,
			Size:   1200,
		}
	}

	// Verify
	for i := 0; i < 10; i++ {
		if packets[i].Offset != i*1200 {
			t.Errorf("packets[%d].Offset = %d, want %d", i, packets[i].Offset, i*1200)
		}
		if packets[i].Size != 1200 {
			t.Errorf("packets[%d].Size = %d, want 1200", i, packets[i].Size)
		}
	}
}

func BenchmarkPacketInfoAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		packets := make([]PacketInfo, 100)
		_ = packets
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

func BenchmarkConfigCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cfg := Config{
			Codec:       codec.H264,
			SSRC:        uint32(i),
			PayloadType: 96,
			MTU:         1200,
			ClockRate:   90000,
		}
		_ = cfg
	}
}
