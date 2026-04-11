package depacketizer

import (
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/testutil"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/packetizer"
)

func encodeVP8FrameForDepacketizer(t *testing.T) ([]byte, bool) {
	t.Helper()
	testutil.SkipIfNoShim(t)

	cfg := codec.DefaultVP8Config(320, 240)
	enc, err := encoder.NewVP8Encoder(cfg)
	if err != nil {
		t.Fatalf("NewVP8Encoder: %v", err)
	}
	defer enc.Close()

	dst := make([]byte, enc.MaxEncodedSize())
	result, err := enc.EncodeInto(testutil.CreateTestVideoFrame(320, 240), dst, true)
	if err != nil {
		t.Fatalf("EncodeInto: %v", err)
	}

	return append([]byte(nil), dst[:result.N]...), result.IsKeyframe
}

func TestDepacketizerNeedMoreDataAndClose(t *testing.T) {
	testutil.SkipIfNoShim(t)

	d, err := New(codec.H264)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := d.PopInto(make([]byte, 1024)); err != ErrNeedMoreData {
		t.Fatalf("PopInto without packets error = %v, want %v", err, ErrNeedMoreData)
	}
	if err := d.Push(nil); err != nil {
		t.Fatalf("Push(nil): %v", err)
	}

	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := d.Push([]byte{1, 2, 3}); err != ErrDepacketizerClosed {
		t.Fatalf("Push after Close error = %v, want %v", err, ErrDepacketizerClosed)
	}
	if _, err := d.PopInto(make([]byte, 32)); err != ErrDepacketizerClosed {
		t.Fatalf("PopInto after Close error = %v, want %v", err, ErrDepacketizerClosed)
	}
}

func TestDepacketizerRoundTripVP8(t *testing.T) {
	encoded, isKeyframe := encodeVP8FrameForDepacketizer(t)

	p, err := packetizer.New(packetizer.Config{
		Codec:       codec.VP8,
		SSRC:        3333,
		PayloadType: 96,
		ClockRate:   codec.VP8.ClockRate(),
	})
	if err != nil {
		t.Fatalf("packetizer.New: %v", err)
	}
	defer p.Close()

	d, err := New(codec.VP8)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer d.Close()

	packetBuf := make([]byte, p.MaxPackets(len(encoded))*p.MaxPacketSize())
	packets := make([]packetizer.PacketInfo, p.MaxPackets(len(encoded)))

	packetCount, err := p.PacketizeInto(encoded, 777, isKeyframe, packetBuf, packets)
	if err != nil {
		t.Fatalf("PacketizeInto: %v", err)
	}

	for i := 0; i < packetCount; i++ {
		info := packets[i]
		if err := d.Push(packetBuf[info.Offset : info.Offset+info.Size]); err != nil {
			t.Fatalf("Push packet %d: %v", i, err)
		}
	}

	dst := make([]byte, len(encoded)+256)
	frameInfo, err := d.PopInto(dst)
	if err != nil {
		t.Fatalf("PopInto: %v", err)
	}
	if frameInfo.Timestamp != 777 {
		t.Fatalf("Timestamp = %d, want 777", frameInfo.Timestamp)
	}
	if frameInfo.Size == 0 {
		t.Fatal("PopInto returned an empty frame")
	}
	if _, err := d.PopInto(dst); err != ErrNeedMoreData {
		t.Fatalf("second PopInto error = %v, want %v", err, ErrNeedMoreData)
	}
}
