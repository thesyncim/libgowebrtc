package packetizer

import (
	"errors"
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/testutil"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/depacketizer"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
)

func encodeH264Frame(t *testing.T) ([]byte, bool) {
	t.Helper()
	testutil.SkipIfNoShim(t)

	cfg := codec.DefaultH264Config(320, 240)
	cfg.PreferHW = false

	enc, err := encoder.NewH264Encoder(cfg)
	if err != nil {
		t.Fatalf("NewH264Encoder: %v", err)
	}
	defer enc.Close()

	dst := make([]byte, enc.MaxEncodedSize())
	result, err := enc.EncodeInto(testutil.CreateTestVideoFrame(320, 240), dst, true)
	if err != nil {
		t.Fatalf("EncodeInto: %v", err)
	}

	return append([]byte(nil), dst[:result.N]...), result.IsKeyframe
}

func TestPacketizeIntoRoundTripVideo(t *testing.T) {
	encoded, isKeyframe := encodeH264Frame(t)
	p, err := New(Config{
		Codec:       codec.H264,
		SSRC:        1234,
		PayloadType: 96,
		MTU:         1200,
		ClockRate:   codec.H264.ClockRate(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	dp, err := depacketizer.New(codec.H264)
	if err != nil {
		t.Fatalf("depacketizer.New: %v", err)
	}
	defer dp.Close()

	beforeSeq := p.SequenceNumber()
	packetCount, packetBuf, packets, err := packetizeWithHeadroom(p, encoded, 90_000, isKeyframe)
	if err != nil {
		t.Fatalf("packetizeWithHeadroom: %v", err)
	}
	if packetCount == 0 {
		t.Fatal("PacketizeInto returned zero packets")
	}

	afterSeq := p.SequenceNumber()
	if afterSeq <= beforeSeq {
		t.Fatalf("SequenceNumber did not advance: before=%d after=%d", beforeSeq, afterSeq)
	}

	for i := 0; i < packetCount; i++ {
		info := packets[i]
		if err := dp.Push(packetBuf[info.Offset : info.Offset+info.Size]); err != nil {
			t.Fatalf("Push packet %d: %v", i, err)
		}
	}

	dst := make([]byte, len(encoded)+1024)
	frameInfo, err := dp.PopInto(dst)
	if err != nil {
		t.Fatalf("PopInto: %v", err)
	}
	if frameInfo.Timestamp != 90_000 {
		t.Fatalf("Timestamp = %d, want 90000", frameInfo.Timestamp)
	}
	if frameInfo.Size == 0 {
		t.Fatal("PopInto returned an empty frame")
	}
	if _, err := dp.PopInto(dst); err != depacketizer.ErrNeedMoreData {
		t.Fatalf("second PopInto error = %v, want %v", err, depacketizer.ErrNeedMoreData)
	}
}

func TestPacketizerErrorPaths(t *testing.T) {
	testutil.SkipIfNoShim(t)

	p, err := New(Config{
		Codec:       codec.H264,
		SSRC:        1234,
		PayloadType: 96,
		MTU:         1200,
		ClockRate:   codec.H264.ClockRate(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := p.PacketizeInto(nil, 0, false, make([]byte, 1024), make([]PacketInfo, 1)); err != ErrInvalidData {
		t.Fatalf("PacketizeInto(nil) error = %v, want %v", err, ErrInvalidData)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := p.PacketizeInto([]byte{1}, 0, false, make([]byte, 1024), make([]PacketInfo, 1)); err != ErrPacketizerClosed {
		t.Fatalf("PacketizeInto after Close error = %v, want %v", err, ErrPacketizerClosed)
	}
	if got := p.SequenceNumber(); got != 0 {
		t.Fatalf("SequenceNumber after Close = %d, want 0", got)
	}
}

func packetizeWithHeadroom(p Packetizer, encoded []byte, timestamp uint32, isKeyframe bool) (int, []byte, []PacketInfo, error) {
	basePackets := p.MaxPackets(len(encoded))
	packetCounts := []int{basePackets, basePackets + 8, basePackets * 2}
	for _, packetCount := range packetCounts {
		if packetCount <= 0 {
			continue
		}
		packetBuf := make([]byte, packetCount*p.MaxPacketSize())
		packets := make([]PacketInfo, packetCount)
		n, err := p.PacketizeInto(encoded, timestamp, isKeyframe, packetBuf, packets)
		if err == nil {
			return n, packetBuf, packets, nil
		}
		if !errors.Is(err, ErrBufferTooSmall) {
			return 0, nil, nil, err
		}
	}

	return 0, nil, nil, ErrBufferTooSmall
}
