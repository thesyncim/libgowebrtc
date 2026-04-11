package media

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pc"
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
	startErr  error

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

func (f *fakeDecodedRemoteTrack) ID() string        { return f.id }
func (f *fakeDecodedRemoteTrack) StreamID() string  { return f.streamID }
func (f *fakeDecodedRemoteTrack) RID() string       { return f.rid }
func (f *fakeDecodedRemoteTrack) Kind() string      { return f.kind.String() }
func (f *fakeDecodedRemoteTrack) Label() string     { return f.id }
func (f *fakeDecodedRemoteTrack) Codec() codec.Type { return f.codecType }
func (f *fakeDecodedRemoteTrack) CodecParameters() webrtc.RTPCodecParameters {
	return f.codecParams
}
func (f *fakeDecodedRemoteTrack) PayloadType() webrtc.PayloadType { return f.payloadType }
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
func (f *fakeDecodedRemoteTrack) Start(onEnd func()) error {
	if f.startErr != nil {
		return f.startErr
	}
	go func() {
		<-f.runDone
		onEnd()
	}()
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

type fakePCRemoteTrack struct {
	id       string
	label    string
	kind     string
	streamID string
	libTrack *pc.Track
	receiver *pc.RTPReceiver

	onVideo func(*frame.VideoFrame)
	onAudio func(*frame.AudioFrame)

	closeOnce sync.Once
	closed    atomic.Bool
}

func newFakePCRemoteTrack(id, kind, streamID string) *fakePCRemoteTrack {
	return &fakePCRemoteTrack{
		id:       id,
		label:    "remote-" + id,
		kind:     kind,
		streamID: streamID,
		libTrack: &pc.Track{},
		receiver: &pc.RTPReceiver{},
	}
}

func (f *fakePCRemoteTrack) ID() string       { return f.id }
func (f *fakePCRemoteTrack) StreamID() string { return f.streamID }
func (f *fakePCRemoteTrack) RID() string      { return "" }
func (f *fakePCRemoteTrack) Kind() string     { return f.kind }
func (f *fakePCRemoteTrack) Label() string    { return f.label }
func (f *fakePCRemoteTrack) SetOnVideoFrame(handler func(*frame.VideoFrame)) error {
	f.onVideo = handler
	return nil
}
func (f *fakePCRemoteTrack) SetOnAudioFrame(handler func(*frame.AudioFrame)) error {
	f.onAudio = handler
	return nil
}
func (f *fakePCRemoteTrack) Start(func()) error { return nil }
func (f *fakePCRemoteTrack) Close() error {
	f.closeOnce.Do(func() {
		f.closed.Store(true)
		f.onVideo = nil
		f.onAudio = nil
	})
	return nil
}
func (f *fakePCRemoteTrack) RawPCTrack() *pc.Track          { return f.libTrack }
func (f *fakePCRemoteTrack) RawPCReceiver() *pc.RTPReceiver { return f.receiver }
func (f *fakePCRemoteTrack) emitVideoFrame(v *frame.VideoFrame) {
	if f.onVideo != nil {
		f.onVideo(v)
	}
}
func (f *fakePCRemoteTrack) emitAudioFrame(a *frame.AudioFrame) {
	if f.onAudio != nil {
		f.onAudio(a)
	}
}

func TestBoundRemoteTrackPreservesStreamID(t *testing.T) {
	videoDecoded := newFakeDecodedRemoteTrack("remote-video", "stream-1", webrtc.RTPCodecTypeVideo, codec.VP8, 96)
	audioDecoded := newFakeDecodedRemoteTrack("remote-audio", "stream-1", webrtc.RTPCodecTypeAudio, codec.Opus, 111)

	videoTrack := bindTestRemoteTrack(t, videoDecoded)
	audioTrack := bindTestRemoteTrack(t, audioDecoded)
	t.Cleanup(func() {
		videoTrack.Stop()
		audioTrack.Stop()
	})

	if got := videoTrack.StreamID(); got != "stream-1" {
		t.Fatalf("videoTrack.StreamID() = %q, want %q", got, "stream-1")
	}
	if got := audioTrack.StreamID(); got != "stream-1" {
		t.Fatalf("audioTrack.StreamID() = %q, want %q", got, "stream-1")
	}
}

func TestBindRemoteTrackSourceStartFailure(t *testing.T) {
	startErr := errors.New("boom")
	videoDecoded := newFakeDecodedRemoteTrack("remote-video", "stream-1", webrtc.RTPCodecTypeVideo, codec.VP8, 96)
	videoDecoded.startErr = startErr

	track, err := bindRemoteTrackSource(videoDecoded)
	if !errors.Is(err, startErr) {
		t.Fatalf("bindRemoteTrackSource() error = %v, want %v", err, startErr)
	}
	if track != nil {
		t.Fatalf("bindRemoteTrackSource() track = %#v, want nil", track)
	}
	waitForCondition(t, 2*time.Second, func() bool { return videoDecoded.closed.Load() })
}

func TestPionRemoteTrackCloneFanoutAndStop(t *testing.T) {
	videoDecoded := newFakeDecodedRemoteTrack("remote-video", "", webrtc.RTPCodecTypeVideo, codec.VP8, 96)

	videoTrack := bindTestRemoteTrack(t, videoDecoded).(PionRemoteVideoTrack)
	clone := videoTrack.Clone().(PionRemoteVideoTrack)

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

func TestPionRemoteTrackClonePreservesState(t *testing.T) {
	videoDecoded := newFakeDecodedRemoteTrack("remote-video", "", webrtc.RTPCodecTypeVideo, codec.VP8, 96)

	videoTrack := bindTestRemoteTrack(t, videoDecoded).(PionRemoteVideoTrack)
	videoTrack.SetEnabled(false)
	videoTrack.Stop()

	clone := videoTrack.Clone().(PionRemoteVideoTrack)
	if clone.Enabled() {
		t.Fatal("clone should preserve disabled state")
	}
	if got := clone.ReadyState(); got != "ended" {
		t.Fatalf("clone ReadyState() = %q, want ended", got)
	}
}

func TestPionRemoteTrackLateCloneAfterLastStopStartsEnded(t *testing.T) {
	videoDecoded := newFakeDecodedRemoteTrack("remote-video", "", webrtc.RTPCodecTypeVideo, codec.VP8, 96)

	videoTrack := bindTestRemoteTrack(t, videoDecoded).(PionRemoteVideoTrack)
	view := remoteTrackViewFor(videoTrack)
	if view == nil {
		t.Fatal("remoteTrackViewFor(videoTrack) = nil")
	}

	videoTrack.Stop()
	waitForCondition(t, 2*time.Second, func() bool { return videoDecoded.closed.Load() })

	lateClone := view.source.newTrack("late-clone")
	if got := lateClone.ReadyState(); got != "ended" {
		t.Fatalf("late clone ReadyState() = %q, want ended", got)
	}
}

func TestPionRemoteTrackCodecChangeAndNaturalEnd(t *testing.T) {
	videoDecoded := newFakeDecodedRemoteTrack("remote-video", "stream-2", webrtc.RTPCodecTypeVideo, codec.VP8, 96)

	videoTrack := bindTestRemoteTrack(t, videoDecoded).(PionRemoteVideoTrack)

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

func TestPCRemoteTrackSingleStreamFanoutAndClose(t *testing.T) {
	videoSource := newFakePCRemoteTrack("remote-video", "video", "stream-a")

	videoTrack := bindTestRemoteTrack(t, videoSource).(PCRemoteVideoTrack)
	clone := videoTrack.Clone().(PCRemoteVideoTrack)
	t.Cleanup(func() {
		videoTrack.Stop()
		clone.Stop()
	})

	if got := videoTrack.StreamID(); got != "stream-a" {
		t.Fatalf("StreamID() = %q, want %q", got, "stream-a")
	}
	if videoTrack.PCTrack() != videoSource.libTrack {
		t.Fatal("PCTrack() did not expose underlying lib track")
	}
	if videoTrack.PCReceiver() != videoSource.receiver {
		t.Fatal("PCReceiver() did not expose underlying receiver")
	}

	frames := 0
	cloneFrames := 0
	if err := videoTrack.SetOnVideoFrame(func(*frame.VideoFrame) { frames++ }); err != nil {
		t.Fatalf("SetOnVideoFrame(original) error = %v", err)
	}
	if err := clone.SetOnVideoFrame(func(*frame.VideoFrame) { cloneFrames++ }); err != nil {
		t.Fatalf("SetOnVideoFrame(clone) error = %v", err)
	}

	videoSource.emitVideoFrame(frame.NewI420Frame(32, 24))
	if frames != 1 || cloneFrames != 1 {
		t.Fatalf("frame fanout counts = (%d, %d), want (1, 1)", frames, cloneFrames)
	}

	videoTrack.Stop()
	videoSource.emitVideoFrame(frame.NewI420Frame(32, 24))
	if frames != 1 {
		t.Fatalf("stopped original should not receive more frames, got %d", frames)
	}
	if cloneFrames != 2 {
		t.Fatalf("live clone should keep receiving frames, got %d", cloneFrames)
	}

	clone.Stop()
	waitForCondition(t, 2*time.Second, func() bool { return videoSource.closed.Load() })
}

func TestBindDecodedRemoteTrackRejectsNil(t *testing.T) {
	if _, err := BindDecodedRemoteTrack(nil); !errors.Is(err, ErrTrackNotFound) {
		t.Fatalf("BindDecodedRemoteTrack(nil) error = %v, want %v", err, ErrTrackNotFound)
	}
}

func TestRemoteFrameSourceIgnoresNilFrames(t *testing.T) {
	videoDecoded := newFakeDecodedRemoteTrack("remote-video", "stream-1", webrtc.RTPCodecTypeVideo, codec.VP8, 96)
	audioDecoded := newFakeDecodedRemoteTrack("remote-audio", "stream-1", webrtc.RTPCodecTypeAudio, codec.Opus, 111)

	videoTrack := bindTestRemoteTrack(t, videoDecoded)
	audioTrack := bindTestRemoteTrack(t, audioDecoded)

	var videoFrames, audioFrames int
	if err := videoTrack.(RemoteVideoTrack).SetOnVideoFrame(func(*frame.VideoFrame) { videoFrames++ }); err != nil {
		t.Fatalf("SetOnVideoFrame: %v", err)
	}
	if err := audioTrack.(RemoteAudioTrack).SetOnAudioFrame(func(*frame.AudioFrame) { audioFrames++ }); err != nil {
		t.Fatalf("SetOnAudioFrame: %v", err)
	}

	videoDecoded.emitVideoFrame(nil)
	audioDecoded.emitAudioFrame(nil)

	if videoFrames != 0 || audioFrames != 0 {
		t.Fatalf("nil frame dispatch counts = (%d, %d), want (0, 0)", videoFrames, audioFrames)
	}
}

func bindTestRemoteTrack(t *testing.T, source remoteFrameTrack) RemoteTrack {
	t.Helper()

	track, err := bindRemoteTrackSource(source)
	if err != nil {
		t.Fatalf("bindRemoteTrackSource() error = %v", err)
	}
	return track
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
