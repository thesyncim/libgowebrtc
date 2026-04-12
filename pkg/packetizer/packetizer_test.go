package packetizer

import (
	"bytes"
	"testing"

	"github.com/pion/rtp"
	pioncodecs "github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

func TestNewRequiresExplicitClockRate(t *testing.T) {
	p, err := New(Config{
		Codec:       codec.H264,
		SSRC:        1,
		PayloadType: 96,
		MTU:         1200,
	})
	if err == nil {
		t.Fatal("New() error = nil, want error")
	}
	if p != nil {
		t.Fatalf("New() = %v, want nil packetizer", p)
	}
}

func TestNewRequiresExplicitMTU(t *testing.T) {
	p, err := New(Config{
		Codec:       codec.H264,
		SSRC:        1,
		PayloadType: 96,
		ClockRate:   codec.H264.ClockRate(),
	})
	if err == nil {
		t.Fatal("New() error = nil, want error")
	}
	if p != nil {
		t.Fatalf("New() = %v, want nil packetizer", p)
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

func TestPacketizeIntoInteropsWithPionDepacketizers(t *testing.T) {
	testCases := []struct {
		name        string
		codecType   codec.Type
		payloadType uint8
		data        []byte
	}{
		{
			name:        "VP8",
			codecType:   codec.VP8,
			payloadType: 96,
			data:        bytes.Repeat([]byte{0x9d, 0x01, 0x2a, 0x33, 0x44, 0x55}, 300),
		},
		{
			name:        "H264",
			codecType:   codec.H264,
			payloadType: 102,
			data: append(
				append(
					append([]byte{}, []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0xe0, 0x1f, 0x8c, 0x68, 0x2c, 0x40}...),
					[]byte{0x00, 0x00, 0x00, 0x01, 0x68, 0xce, 0x06, 0xe2}...,
				),
				append([]byte{0x00, 0x00, 0x00, 0x01, 0x65}, bytes.Repeat([]byte{0x88}, 2200)...)...,
			),
		},
		{
			name:        "VP9",
			codecType:   codec.VP9,
			payloadType: 98,
			data:        bytes.Repeat([]byte{0x82, 0x49, 0x83, 0x42, 0x11}, 500),
		},
		{
			name:        "Opus",
			codecType:   codec.Opus,
			payloadType: 111,
			data:        bytes.Repeat([]byte{0xf8, 0xff, 0xfe}, 200),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			packetizer, err := New(Config{
				Codec:       tc.codecType,
				SSRC:        0x01020304,
				PayloadType: tc.payloadType,
				MTU:         1200,
				ClockRate:   tc.codecType.ClockRate(),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			maxPackets := packetizer.MaxPackets(len(tc.data))
			dst := make([]byte, maxPackets*packetizer.MaxPacketSize())
			packets := make([]PacketInfo, maxPackets)

			count, err := packetizer.PacketizeInto(tc.data, 0x11223344, true, dst, packets)
			if err != nil {
				t.Fatalf("PacketizeInto: %v", err)
			}
			if count == 0 {
				t.Fatal("expected at least one RTP packet")
			}

			builder := samplebuilder.New(50, mustDepacketizer(t, tc.codecType), tc.codecType.ClockRate())
			for i := 0; i < count; i++ {
				var pkt rtp.Packet
				if err := pkt.Unmarshal(dst[packets[i].Offset : packets[i].Offset+packets[i].Size]); err != nil {
					t.Fatalf("Unmarshal(packet %d): %v", i, err)
				}
				builder.Push(&pkt)
			}
			builder.Flush()

			sample := builder.Pop()
			if sample == nil {
				t.Fatal("expected reassembled sample")
			}
			if !bytes.Equal(sample.Data, tc.data) {
				t.Fatalf("reassembled sample differs: got %d bytes want %d", len(sample.Data), len(tc.data))
			}
		})
	}
}

func mustDepacketizer(t testing.TB, codecType codec.Type) rtp.Depacketizer {
	t.Helper()

	switch codecType {
	case codec.H264:
		return &pioncodecs.H264Packet{}
	case codec.VP8:
		return &pioncodecs.VP8Packet{}
	case codec.VP9:
		return &pioncodecs.VP9Packet{}
	case codec.Opus:
		return &pioncodecs.OpusPacket{}
	default:
		t.Fatalf("unsupported depacketizer codec %s", codecType)
		return nil
	}
}
