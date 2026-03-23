package media

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pionrecv"
)

type fakeDecodedRemoteTrack struct {
	id          string
	streamID    string
	rid         string
	kind        webrtc.RTPCodecType
	codecType   codec.Type
	codecParams webrtc.RTPCodecParameters
	payloadType webrtc.PayloadType

	onVideo       func(*frame.VideoFrame)
	onAudio       func(*frame.AudioFrame)
	onCodecChange func(pionrecv.CodecChange)

	closeOnce sync.Once
	closed    atomic.Bool
	runDone   chan struct{}

	keyframeRequests atomic.Int32
}

func newFakeDecodedRemoteTrack(id, streamID string, kind webrtc.RTPCodecType, codecType codec.Type, payloadType webrtc.PayloadType) *fakeDecodedRemoteTrack {
	return &fakeDecodedRemoteTrack{
		id:        id,
		streamID:  streamID,
		kind:      kind,
		codecType: codecType,
		codecParams: webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  codecType.MimeType(),
				ClockRate: codecType.ClockRate(),
			},
			PayloadType: payloadType,
		},
		payloadType: payloadType,
		runDone:     make(chan struct{}),
	}
}

func (f *fakeDecodedRemoteTrack) ID() string                                 { return f.id }
func (f *fakeDecodedRemoteTrack) StreamID() string                           { return f.streamID }
func (f *fakeDecodedRemoteTrack) RID() string                                { return f.rid }
func (f *fakeDecodedRemoteTrack) Kind() webrtc.RTPCodecType                  { return f.kind }
func (f *fakeDecodedRemoteTrack) Codec() codec.Type                          { return f.codecType }
func (f *fakeDecodedRemoteTrack) CodecParameters() webrtc.RTPCodecParameters { return f.codecParams }
func (f *fakeDecodedRemoteTrack) PayloadType() webrtc.PayloadType            { return f.payloadType }
func (f *fakeDecodedRemoteTrack) RequestKeyframe() error {
	f.keyframeRequests.Add(1)
	return nil
}
func (f *fakeDecodedRemoteTrack) SetOnVideoFrame(handler func(*frame.VideoFrame)) error {
	f.onVideo = handler
	return nil
}
func (f *fakeDecodedRemoteTrack) SetOnAudioFrame(handler func(*frame.AudioFrame)) error {
	f.onAudio = handler
	return nil
}
func (f *fakeDecodedRemoteTrack) SetOnCodecChange(handler func(pionrecv.CodecChange)) {
	f.onCodecChange = handler
}
func (f *fakeDecodedRemoteTrack) Run() error {
	<-f.runDone
	return nil
}
func (f *fakeDecodedRemoteTrack) Close() error {
	f.closeOnce.Do(func() {
		f.closed.Store(true)
		close(f.runDone)
	})
	return nil
}
func (f *fakeDecodedRemoteTrack) RawDecodedTrack() *pionrecv.DecodedTrack { return nil }

func (f *fakeDecodedRemoteTrack) emitVideoFrame(v *frame.VideoFrame) {
	if f.onVideo != nil {
		f.onVideo(v)
	}
}

func (f *fakeDecodedRemoteTrack) emitAudioFrame(a *frame.AudioFrame) {
	if f.onAudio != nil {
		f.onAudio(a)
	}
}

func (f *fakeDecodedRemoteTrack) emitCodecChange(change pionrecv.CodecChange) {
	if f.onCodecChange != nil {
		f.onCodecChange(change)
	}
}

func TestRemoteStreamRegistryReusesMediaStreamForMatchingStreamID(t *testing.T) {
	registry := NewRemoteStreamRegistry()
	videoDecoded := newFakeDecodedRemoteTrack("remote-video", "stream-1", webrtc.RTPCodecTypeVideo, codec.VP8, 96)
	audioDecoded := newFakeDecodedRemoteTrack("remote-audio", "stream-1", webrtc.RTPCodecTypeAudio, codec.Opus, 111)

	videoTrack, videoStreams, err := registry.bindDecodedTrack(videoDecoded)
	if err != nil {
		t.Fatalf("bindDecodedTrack(video) error = %v", err)
	}
	audioTrack, audioStreams, err := registry.bindDecodedTrack(audioDecoded)
	if err != nil {
		t.Fatalf("bindDecodedTrack(audio) error = %v", err)
	}
	t.Cleanup(func() {
		videoTrack.Stop()
		audioTrack.Stop()
	})

	if len(videoStreams) != 1 || len(audioStreams) != 1 {
		t.Fatalf("stream counts = (%d, %d), want (1, 1)", len(videoStreams), len(audioStreams))
	}
	if videoStreams[0] != audioStreams[0] {
		t.Fatal("expected matching stream IDs to reuse the same MediaStream instance")
	}
	if got := videoStreams[0].ID(); got != "stream-1" {
		t.Fatalf("MediaStream ID = %q, want %q", got, "stream-1")
	}
	if got := len(videoStreams[0].GetTracks()); got != 2 {
		t.Fatalf("GetTracks() len = %d, want 2", got)
	}
	if got := videoTrack.StreamID(); got != "stream-1" {
		t.Fatalf("videoTrack.StreamID() = %q, want %q", got, "stream-1")
	}
}

func TestRemoteTrackCloneFanoutAndStop(t *testing.T) {
	registry := NewRemoteStreamRegistry()
	videoDecoded := newFakeDecodedRemoteTrack("remote-video", "", webrtc.RTPCodecTypeVideo, codec.VP8, 96)

	track, streams, err := registry.bindDecodedTrack(videoDecoded)
	if err != nil {
		t.Fatalf("bindDecodedTrack() error = %v", err)
	}
	if len(streams) != 0 {
		t.Fatalf("streamless remote track should not create streams, got %d", len(streams))
	}
	videoTrack := track.(RemoteVideoTrack)
	clone := videoTrack.Clone().(RemoteVideoTrack)

	var (
		originalFrames []*frame.VideoFrame
		cloneFrames    []*frame.VideoFrame
	)
	if err := videoTrack.SetOnVideoFrame(func(f *frame.VideoFrame) {
		f.Data[0][0] = 99
		originalFrames = append(originalFrames, f)
	}); err != nil {
		t.Fatalf("SetOnVideoFrame(original) error = %v", err)
	}
	if err := clone.SetOnVideoFrame(func(f *frame.VideoFrame) {
		cloneFrames = append(cloneFrames, f)
	}); err != nil {
		t.Fatalf("SetOnVideoFrame(clone) error = %v", err)
	}

	videoDecoded.emitVideoFrame(frame.NewI420Frame(64, 48))
	if len(originalFrames) != 1 || len(cloneFrames) != 1 {
		t.Fatalf("frame fanout counts = (%d, %d), want (1, 1)", len(originalFrames), len(cloneFrames))
	}
	if cloneFrames[0].Data[0][0] != 0 {
		t.Fatalf("clone frame should be isolated from original mutation, got %d", cloneFrames[0].Data[0][0])
	}

	videoTrack.Stop()
	if got := videoTrack.ReadyState(); got != "ended" {
		t.Fatalf("original ReadyState() after Stop = %q, want ended", got)
	}
	videoDecoded.emitVideoFrame(frame.NewI420Frame(64, 48))
	if len(originalFrames) != 1 {
		t.Fatalf("stopped original should not receive more frames, got %d", len(originalFrames))
	}
	if len(cloneFrames) != 2 {
		t.Fatalf("live clone should keep receiving frames, got %d", len(cloneFrames))
	}

	if err := clone.RequestKeyframe(); err != nil {
		t.Fatalf("RequestKeyframe() error = %v", err)
	}
	if got := videoDecoded.keyframeRequests.Load(); got != 1 {
		t.Fatalf("RequestKeyframe count = %d, want 1", got)
	}

	clone.Stop()
	waitForCondition(t, 2*time.Second, func() bool { return videoDecoded.closed.Load() })
}

func TestRemoteTrackCodecChangeAndNaturalEnd(t *testing.T) {
	registry := NewRemoteStreamRegistry()
	videoDecoded := newFakeDecodedRemoteTrack("remote-video", "stream-2", webrtc.RTPCodecTypeVideo, codec.VP8, 96)

	track, _, err := registry.bindDecodedTrack(videoDecoded)
	if err != nil {
		t.Fatalf("bindDecodedTrack() error = %v", err)
	}
	videoTrack := track.(RemoteVideoTrack)

	changeCh := make(chan pionrecv.CodecChange, 1)
	videoTrack.SetOnCodecChange(func(change pionrecv.CodecChange) {
		changeCh <- change
	})

	videoDecoded.emitCodecChange(pionrecv.CodecChange{
		PreviousType: codec.VP8,
		CurrentType:  codec.H264,
		PreviousCodec: webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
			PayloadType:        96,
		},
		CurrentCodec: webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000},
			PayloadType:        102,
		},
		PreviousPayloadType: 96,
		CurrentPayloadType:  102,
	})

	select {
	case change := <-changeCh:
		if change.CurrentType != codec.H264 {
			t.Fatalf("codec change CurrentType = %v, want H264", change.CurrentType)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for codec change")
	}

	_ = videoDecoded.Close()
	waitForCondition(t, 2*time.Second, func() bool { return videoTrack.ReadyState() == "ended" })
}

func waitForCondition(t *testing.T, timeout time.Duration, predicate func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
