package pionrecv

import (
	"context"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

func TestAudioSubscriberMonitorTracksCodecConfigAndActivity(t *testing.T) {
	opusParams := mustCodecParams(t, codec.Opus, 111)
	pcmuParams := mustCodecParams(t, codec.PCMU, 0)
	track := newFakeTrackReader(webrtc.RTPCodecTypeAudio, opusParams, 0xabcdef01)

	monitor := NewAudioSubscriberMonitor(AudioSubscriberMonitorConfig{})
	monitor.bind(track, nil, codec.Opus, opusParams)

	active := newTestAudioFrame(testAudioClockRate * testAudioFrameMs / 1000)
	active.PTS = 0
	silent := frame.NewAudioFrameS16(8000, 1, 160)
	silent.PTS = 160

	monitor.observeFrame(active)
	monitor.observeCodecChange(CodecChange{
		PreviousType:  codec.Opus,
		CurrentType:   codec.PCMU,
		PreviousCodec: opusParams,
		CurrentCodec:  pcmuParams,
	})
	monitor.observeFrame(silent)

	snapshot := monitor.Snapshot()
	if snapshot.FrameCount != 2 {
		t.Fatalf("FrameCount = %d, want 2", snapshot.FrameCount)
	}
	if snapshot.ActiveFrames != 1 {
		t.Fatalf("ActiveFrames = %d, want 1", snapshot.ActiveFrames)
	}
	if snapshot.SilentFrames != 1 {
		t.Fatalf("SilentFrames = %d, want 1", snapshot.SilentFrames)
	}
	if snapshot.LastFrameSilent != true {
		t.Fatalf("LastFrameSilent = %v, want true", snapshot.LastFrameSilent)
	}
	if snapshot.CurrentCodec != codec.PCMU {
		t.Fatalf("CurrentCodec = %v, want %v", snapshot.CurrentCodec, codec.PCMU)
	}
	if snapshot.CurrentSampleRate != 8000 || snapshot.CurrentChannels != 1 {
		t.Fatalf("current audio config = %d Hz/%d ch, want 8000/1", snapshot.CurrentSampleRate, snapshot.CurrentChannels)
	}
	if len(snapshot.CodecSwitches) != 1 {
		t.Fatalf("len(CodecSwitches) = %d, want 1", len(snapshot.CodecSwitches))
	}
	if len(snapshot.ConfigSwitches) != 2 {
		t.Fatalf("len(ConfigSwitches) = %d, want 2", len(snapshot.ConfigSwitches))
	}
	if snapshot.ConfigSwitches[len(snapshot.ConfigSwitches)-1].CurrentPTime != 20*time.Millisecond {
		t.Fatalf("CurrentPTime = %v, want 20ms", snapshot.ConfigSwitches[len(snapshot.ConfigSwitches)-1].CurrentPTime)
	}
}

func TestAudioSubscriberMonitorObservesPacketAndFrameGaps(t *testing.T) {
	params := mustCodecParams(t, codec.Opus, 111)
	track := newFakeTrackReader(webrtc.RTPCodecTypeAudio, params, 0xabcdef02)

	monitor := NewAudioSubscriberMonitor(AudioSubscriberMonitorConfig{
		FreezeThreshold:    5 * time.Millisecond,
		PacketGapThreshold: 5 * time.Millisecond,
	})
	monitor.bind(track, nil, codec.Opus, params)

	pkt1 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    uint8(params.PayloadType),
			SequenceNumber: 10,
			Timestamp:      0,
			SSRC:           uint32(track.SSRC()),
		},
		Payload: []byte{0x01, 0x02},
	}
	pkt2 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    uint8(params.PayloadType),
			SequenceNumber: 13,
			Timestamp:      960,
			SSRC:           uint32(track.SSRC()),
		},
		Payload: []byte{0x03, 0x04},
	}

	monitor.observePacket(pkt1)
	time.Sleep(10 * time.Millisecond)
	monitor.observePacket(pkt2)

	first := newTestAudioFrame(testAudioClockRate * testAudioFrameMs / 1000)
	first.PTS = 0
	second := newTestAudioFrame(testAudioClockRate * testAudioFrameMs / 1000)
	second.PTS = 960
	monitor.observeFrame(first)
	time.Sleep(10 * time.Millisecond)
	monitor.observeFrame(second)

	snapshot := monitor.Snapshot()
	if snapshot.SequenceGapCount != 1 {
		t.Fatalf("SequenceGapCount = %d, want 1", snapshot.SequenceGapCount)
	}
	if snapshot.MissingPackets != 2 {
		t.Fatalf("MissingPackets = %d, want 2", snapshot.MissingPackets)
	}
	if snapshot.PacketGapCount != 1 {
		t.Fatalf("PacketGapCount = %d, want 1", snapshot.PacketGapCount)
	}
	if snapshot.FrameGapCount != 1 {
		t.Fatalf("FrameGapCount = %d, want 1", snapshot.FrameGapCount)
	}
	if !snapshot.HasFreeze() {
		t.Fatal("HasFreeze() = false, want true")
	}
	if len(snapshot.FreezeEvents) < 2 {
		t.Fatalf("len(FreezeEvents) = %d, want at least 2", len(snapshot.FreezeEvents))
	}
}

func TestAudioSubscriberMonitorWaitersAndDecodedTrackWiring(t *testing.T) {
	params := mustCodecParams(t, codec.Opus, 111)
	track := newFakeTrackReader(webrtc.RTPCodecTypeAudio, params, 0xabcdef03)
	track.enqueuePackets(mustEncodeAudioPackets(t, 111, track.SSRC(), []uint32{0, 960}), params, 111)
	track.close()

	monitor := NewAudioSubscriberMonitor(AudioSubscriberMonitorConfig{})
	decoded, err := newDecodedTrack(track, nil, nil, WithAudioSubscriberMonitor(monitor))
	if err != nil {
		t.Fatalf("newDecodedTrack: %v", err)
	}
	if err := decoded.SetOnAudioFrame(func(*frame.AudioFrame) {}); err != nil {
		t.Fatalf("SetOnAudioFrame: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- decoded.Run()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := monitor.WaitForFrames(ctx, 1); err != nil {
		t.Fatalf("WaitForFrames: %v", err)
	}
	if err := monitor.WaitForConfig(ctx, testAudioClockRate, testAudioChannels); err != nil {
		t.Fatalf("WaitForConfig: %v", err)
	}
	if err := monitor.WaitForCodec(ctx, codec.Opus); err != nil {
		t.Fatalf("WaitForCodec: %v", err)
	}
	if err := monitor.WaitForActivity(ctx); err != nil {
		t.Fatalf("WaitForActivity: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for decoded track to finish")
	}

	snapshot := monitor.Snapshot()
	if snapshot.FrameCount == 0 {
		t.Fatal("FrameCount = 0, want decoded audio frames")
	}
	if !snapshot.Continuous() {
		t.Fatalf("Continuous() = false, snapshot = %+v", snapshot)
	}

	silentMonitor := NewAudioSubscriberMonitor(AudioSubscriberMonitorConfig{})
	silentTrack := newFakeTrackReader(webrtc.RTPCodecTypeAudio, params, 0xabcdef04)
	silentMonitor.bind(silentTrack, nil, codec.Opus, params)
	go silentMonitor.observeFrame(frame.NewAudioFrameS16(testAudioClockRate, testAudioChannels, 960))

	silenceCtx, silenceCancel := context.WithTimeout(context.Background(), testTimeout)
	defer silenceCancel()
	if err := silentMonitor.WaitForSilence(silenceCtx); err != nil {
		t.Fatalf("WaitForSilence: %v", err)
	}
}
