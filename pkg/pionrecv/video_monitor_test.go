package pionrecv

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	dd "github.com/thesyncim/libgowebrtc/internal/dependencydescriptor"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

func TestVideoSubscriberMonitorTracksCodecSwitchAndContinuity(t *testing.T) {
	vp8Params := mustCodecParams(t, codec.VP8, 96)
	h264Params := mustCodecParams(t, codec.H264, 102)
	track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, vp8Params, 0x99887766)

	track.enqueuePackets(mustEncodeVideoPackets(t, codec.VP8, 96, track.SSRC(), []uint32{0, 3000}), vp8Params, 96)
	track.enqueuePackets(mustEncodeVideoPackets(t, codec.H264, 102, track.SSRC(), []uint32{6000, 9000}), h264Params, 102)
	track.close()

	monitor := NewVideoSubscriberMonitor(VideoSubscriberMonitorConfig{})
	decoded, err := newDecodedTrack(track, nil, nil, WithVideoSubscriberMonitor(monitor))
	if err != nil {
		t.Fatalf("newDecodedTrack: %v", err)
	}
	if err := decoded.SetOnVideoFrame(func(*frame.VideoFrame) {}); err != nil {
		t.Fatalf("SetOnVideoFrame: %v", err)
	}

	if err := decoded.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snapshot := monitor.Snapshot()
	if snapshot.FrameCount == 0 {
		t.Fatal("FrameCount = 0, want decoded frames")
	}
	if snapshot.CurrentCodec != codec.H264 {
		t.Fatalf("CurrentCodec = %v, want %v", snapshot.CurrentCodec, codec.H264)
	}
	if len(snapshot.CodecSwitches) != 1 {
		t.Fatalf("len(CodecSwitches) = %d, want 1", len(snapshot.CodecSwitches))
	}
	if snapshot.CodecSwitches[0].Change.PreviousType != codec.VP8 || snapshot.CodecSwitches[0].Change.CurrentType != codec.H264 {
		t.Fatalf("codec switch = %+v, want VP8 -> H264", snapshot.CodecSwitches[0].Change)
	}
	if snapshot.PacketCount == 0 {
		t.Fatal("PacketCount = 0, want observed RTP")
	}
	if snapshot.LastFrameAt.Before(snapshot.CodecSwitches[0].At) {
		t.Fatalf("LastFrameAt = %v, want decoded frames after codec switch at %v", snapshot.LastFrameAt, snapshot.CodecSwitches[0].At)
	}
}

func TestVideoSubscriberMonitorObservesRIDSwitchesAndSequenceGaps(t *testing.T) {
	params := mustCodecParams(t, codec.VP8, 96)
	track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, params, 0x12345678)

	monitor := NewVideoSubscriberMonitor(VideoSubscriberMonitorConfig{
		HeaderExtensions: []webrtc.RTPHeaderExtensionParameter{
			{ID: 1, URI: rtpStreamIDHeaderExtensionURI},
		},
	})
	monitor.bind(track, nil, codec.VP8, params)

	pkt1 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    uint8(params.PayloadType),
			SequenceNumber: 10,
			Timestamp:      3000,
			SSRC:           uint32(track.SSRC()),
		},
		Payload: []byte{0x00},
	}
	if err := pkt1.SetExtension(1, []byte("q")); err != nil {
		t.Fatalf("pkt1.SetExtension: %v", err)
	}

	pkt2 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    uint8(params.PayloadType),
			SequenceNumber: 13,
			Timestamp:      6000,
			SSRC:           uint32(track.SSRC()),
		},
		Payload: []byte{0x00},
	}
	if err := pkt2.SetExtension(1, []byte("h")); err != nil {
		t.Fatalf("pkt2.SetExtension: %v", err)
	}

	monitor.observePacket(pkt1)
	monitor.observePacket(pkt2)

	snapshot := monitor.Snapshot()
	if snapshot.SequenceGapCount != 1 {
		t.Fatalf("SequenceGapCount = %d, want 1", snapshot.SequenceGapCount)
	}
	if snapshot.MissingPackets != 2 {
		t.Fatalf("MissingPackets = %d, want 2", snapshot.MissingPackets)
	}
	if snapshot.CurrentRID != "h" {
		t.Fatalf("CurrentRID = %q, want %q", snapshot.CurrentRID, "h")
	}
	if len(snapshot.RIDSwitches) != 1 {
		t.Fatalf("len(RIDSwitches) = %d, want 1", len(snapshot.RIDSwitches))
	}
	if snapshot.RIDSwitches[0].Previous != "q" || snapshot.RIDSwitches[0].Current != "h" {
		t.Fatalf("RID switch = %+v, want q -> h", snapshot.RIDSwitches[0])
	}
}

func TestVideoSubscriberMonitorObservesDependencyDescriptorLayerSwitch(t *testing.T) {
	params := mustCodecParams(t, codec.VP9, 98)
	track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, params, 0x12341234)

	monitor := NewVideoSubscriberMonitor(VideoSubscriberMonitorConfig{
		HeaderExtensions: []webrtc.RTPHeaderExtensionParameter{
			{ID: 3, URI: dd.ExtensionURI},
		},
	})
	monitor.bind(track, nil, codec.VP9, params)

	pkt1 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    uint8(params.PayloadType),
			SequenceNumber: 1,
			Timestamp:      3000,
			SSRC:           uint32(track.SSRC()),
		},
		Payload: []byte{0x00},
	}
	if err := pkt1.SetExtension(3, makeTestDependencyDescriptorPayload(1, 0, true, nil)); err != nil {
		t.Fatalf("pkt1.SetExtension: %v", err)
	}

	activeMask := uint32(0b10)
	pkt2 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    uint8(params.PayloadType),
			SequenceNumber: 2,
			Timestamp:      6000,
			SSRC:           uint32(track.SSRC()),
		},
		Payload: []byte{0x00},
	}
	if err := pkt2.SetExtension(3, makeTestDependencyDescriptorPayload(2, 1, false, &activeMask)); err != nil {
		t.Fatalf("pkt2.SetExtension: %v", err)
	}

	monitor.observePacket(pkt1)
	first := monitor.Snapshot()
	if !first.DependencyDescriptorSeen || !first.DependencyStructureSeen {
		t.Fatalf("DD visibility = seen:%v structure:%v, want both true", first.DependencyDescriptorSeen, first.DependencyStructureSeen)
	}
	if !first.HasCurrentLayer || first.CurrentLayer != (VideoLayer{Spatial: 0, Temporal: 0}) {
		t.Fatalf("CurrentLayer after first packet = %+v, want {0 0}", first.CurrentLayer)
	}

	monitor.observePacket(pkt2)
	second := monitor.Snapshot()
	if !second.HasCurrentLayer || second.CurrentLayer != (VideoLayer{Spatial: 1, Temporal: 0}) {
		t.Fatalf("CurrentLayer after switch = %+v, want {1 0}", second.CurrentLayer)
	}
	if !second.HasMaxActiveLayer || second.MaxActiveLayer != (VideoLayer{Spatial: 1, Temporal: 0}) {
		t.Fatalf("MaxActiveLayer = %+v (has=%v), want {1 0}", second.MaxActiveLayer, second.HasMaxActiveLayer)
	}
	if len(second.LayerSwitches) != 1 {
		t.Fatalf("len(LayerSwitches) = %d, want 1", len(second.LayerSwitches))
	}
	if second.LayerSwitches[0].Width != 640 || second.LayerSwitches[0].Height != 360 {
		t.Fatalf("layer switch resolution = %dx%d, want 640x360", second.LayerSwitches[0].Width, second.LayerSwitches[0].Height)
	}
}

type testBitWriter struct {
	bytes []byte
	bits  int
}

func (w *testBitWriter) WriteBool(value bool) {
	bit := uint64(0)
	if value {
		bit = 1
	}
	w.WriteBits(bit, 1)
}

func (w *testBitWriter) WriteBits(value uint64, count int) {
	for i := count - 1; i >= 0; i-- {
		bit := byte((value >> i) & 1)
		if w.bits%8 == 0 {
			w.bytes = append(w.bytes, 0)
		}
		w.bytes[len(w.bytes)-1] |= bit << (7 - (w.bits % 8))
		w.bits++
	}
}

func (w *testBitWriter) Bytes() []byte {
	return append([]byte(nil), w.bytes...)
}

func makeTestDependencyDescriptorPayload(frameNumber uint16, templateID int, includeStructure bool, activeMask *uint32) []byte {
	var writer testBitWriter
	writer.WriteBool(true)
	writer.WriteBool(true)
	writer.WriteBits(uint64(templateID), 6)
	writer.WriteBits(uint64(frameNumber), 16)

	if includeStructure || activeMask != nil {
		writer.WriteBool(includeStructure)
		writer.WriteBool(activeMask != nil)
		writer.WriteBool(false)
		writer.WriteBool(false)
		writer.WriteBool(false)

		if includeStructure {
			writer.WriteBits(0, 6) // structure ID
			writer.WriteBits(1, 5) // two decode targets

			writer.WriteBits(2, 2) // template 0 -> next spatial layer
			writer.WriteBits(3, 2) // template 1 -> stop

			writer.WriteBits(uint64(dd.DecodeTargetRequired), 2)
			writer.WriteBits(uint64(dd.DecodeTargetNotPresent), 2)
			writer.WriteBits(uint64(dd.DecodeTargetRequired), 2)
			writer.WriteBits(uint64(dd.DecodeTargetRequired), 2)

			writer.WriteBool(false)
			writer.WriteBool(false)

			writer.WriteBits(0, 1) // num chains = 0 in non-symmetric coding for range [0,2]

			writer.WriteBool(true)
			writer.WriteBits(319, 16)
			writer.WriteBits(179, 16)
			writer.WriteBits(639, 16)
			writer.WriteBits(359, 16)
		}
		if activeMask != nil {
			writer.WriteBits(uint64(*activeMask), 2)
		}
	}

	return writer.Bytes()
}

func TestVideoSubscriberMonitorDetectsFrameFreeze(t *testing.T) {
	monitor := NewVideoSubscriberMonitor(VideoSubscriberMonitorConfig{
		FreezeThreshold: 5 * time.Millisecond,
	})

	track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, mustCodecParams(t, codec.VP8, 96), 0x45674567)
	monitor.bind(track, nil, codec.VP8, track.Codec())

	first := frame.NewI420Frame(160, 90)
	first.PTS = 0
	second := frame.NewI420Frame(160, 90)
	second.PTS = 3000

	monitor.observeFrame(first)
	time.Sleep(10 * time.Millisecond)
	monitor.observeFrame(second)

	snapshot := monitor.Snapshot()
	if snapshot.FrameGapCount != 1 {
		t.Fatalf("FrameGapCount = %d, want 1", snapshot.FrameGapCount)
	}
	if !snapshot.HasFreeze() {
		t.Fatal("HasFreeze() = false, want true")
	}
	if len(snapshot.FreezeEvents) == 0 || snapshot.FreezeEvents[0].Kind != "frame" {
		t.Fatalf("FreezeEvents = %+v, want frame freeze event", snapshot.FreezeEvents)
	}
}

func TestVideoSubscriberMonitorDeduplicatesVisibleFreezeCount(t *testing.T) {
	params := mustCodecParams(t, codec.VP8, 96)
	track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, params, 0x45674568)

	monitor := NewVideoSubscriberMonitor(VideoSubscriberMonitorConfig{
		FreezeThreshold:    5 * time.Millisecond,
		PacketGapThreshold: 5 * time.Millisecond,
	})
	monitor.bind(track, nil, codec.VP8, params)

	pkt1 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    uint8(params.PayloadType),
			SequenceNumber: 10,
			Timestamp:      3000,
			SSRC:           uint32(track.SSRC()),
		},
		Payload: []byte{0x01},
	}
	pkt2 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    uint8(params.PayloadType),
			SequenceNumber: 13,
			Timestamp:      6000,
			SSRC:           uint32(track.SSRC()),
		},
		Payload: []byte{0x02},
	}

	first := frame.NewI420Frame(160, 90)
	first.PTS = 0
	second := frame.NewI420Frame(160, 90)
	second.PTS = 3000
	monitor.observePacket(pkt1)
	monitor.observeFrame(first)
	time.Sleep(10 * time.Millisecond)
	monitor.observePacket(pkt2)
	monitor.observeFrame(second)

	snapshot := monitor.Snapshot()
	if snapshot.PacketGapCount != 1 {
		t.Fatalf("PacketGapCount = %d, want 1", snapshot.PacketGapCount)
	}
	if snapshot.FrameGapCount != 1 {
		t.Fatalf("FrameGapCount = %d, want 1", snapshot.FrameGapCount)
	}
	if snapshot.FreezeCount != 1 {
		t.Fatalf("FreezeCount = %d, want 1 visible freeze", snapshot.FreezeCount)
	}
	if len(snapshot.FreezeEvents) != 2 {
		t.Fatalf("len(FreezeEvents) = %d, want 2 raw freeze events", len(snapshot.FreezeEvents))
	}
}

func TestVideoSubscriberMonitorWaitForCodecRequiresDecodedFrames(t *testing.T) {
	params := mustCodecParams(t, codec.VP8, 96)
	track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, params, 0x45674569)

	monitor := NewVideoSubscriberMonitor(VideoSubscriberMonitorConfig{})
	monitor.bind(track, nil, codec.VP8, params)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := monitor.WaitForCodec(ctx, codec.VP8); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForCodec before frame error = %v, want %v", err, context.DeadlineExceeded)
	}

	video := frame.NewI420Frame(160, 90)
	video.PTS = 0
	monitor.observeFrame(video)

	readyCtx, readyCancel := context.WithTimeout(context.Background(), testTimeout)
	defer readyCancel()
	if err := monitor.WaitForCodec(readyCtx, codec.VP8); err != nil {
		t.Fatalf("WaitForCodec after frame: %v", err)
	}
}
