package packetizer

import (
	"bytes"
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
	packetBuf := make([]byte, p.MaxPackets(len(encoded))*p.MaxPacketSize())
	packets := make([]PacketInfo, p.MaxPackets(len(encoded)))

	packetCount, err := p.PacketizeInto(encoded, 90_000, isKeyframe, packetBuf, packets)
	if err != nil {
		t.Fatalf("PacketizeInto: %v", err)
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
	if !bytes.Equal(dst[:frameInfo.Size], encoded) {
		t.Fatal("depacketized frame does not match encoded input")
	}
}

func TestPacketizerErrorPaths(t *testing.T) {
	testutil.SkipIfNoShim(t)

	p, err := New(Config{
		Codec:       codec.H264,
		SSRC:        1234,
		PayloadType: 96,
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
