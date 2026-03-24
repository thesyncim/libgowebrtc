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

func (f *fakeDecodedRemoteTrack) ID() string          { return f.id }
func (f *fakeDecodedRemoteTrack) StreamIDs() []string { return singleStreamIDs(f.streamID) }
func (f *fakeDecodedRemoteTrack) RID() string         { return f.rid }
func (f *fakeDecodedRemoteTrack) Kind() string        { return f.kind.String() }
func (f *fakeDecodedRemoteTrack) Label() string       { return f.id }
func (f *fakeDecodedRemoteTrack) Codec() codec.Type   { return f.codecType }
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
	streams  []string
	libTrack *pc.Track
	receiver *pc.RTPReceiver

	onVideo func(*frame.VideoFrame)
	onAudio func(*frame.AudioFrame)

	closeOnce sync.Once
	closed    atomic.Bool
}

func newFakePCRemoteTrack(id, kind string, streams []string) *fakePCRemoteTrack {
	return &fakePCRemoteTrack{
		id:       id,
		label:    "remote-" + id,
		kind:     kind,
		streams:  append([]string(nil), streams...),
		libTrack: &pc.Track{},
		receiver: &pc.RTPReceiver{},
	}
}

func (f *fakePCRemoteTrack) ID() string          { return f.id }
func (f *fakePCRemoteTrack) StreamIDs() []string { return append([]string(nil), f.streams...) }
func (f *fakePCRemoteTrack) RID() string         { return "" }
func (f *fakePCRemoteTrack) Kind() string        { return f.kind }
func (f *fakePCRemoteTrack) Label() string       { return f.label }
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

func TestRemoteStreamRegistryReusesMediaStreamForMatchingStreamID(t *testing.T) {
	registry := NewRemoteStreamRegistry()
	videoDecoded := newFakeDecodedRemoteTrack("remote-video", "stream-1", webrtc.RTPCodecTypeVideo, codec.VP8, 96)
	audioDecoded := newFakeDecodedRemoteTrack("remote-audio", "stream-1", webrtc.RTPCodecTypeAudio, codec.Opus, 111)

	videoTrack, videoStreams, err := registry.bindSource(videoDecoded)
	if err != nil {
		t.Fatalf("bindSource(video) error = %v", err)
	}
	audioTrack, audioStreams, err := registry.bindSource(audioDecoded)
	if err != nil {
		t.Fatalf("bindSource(audio) error = %v", err)
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
	if got := videoTrack.StreamIDs(); len(got) != 1 || got[0] != "stream-1" {
		t.Fatalf("videoTrack.StreamIDs() = %v, want [stream-1]", got)
	}
}

func TestPionRemoteTrackCloneFanoutAndStop(t *testing.T) {
	registry := NewRemoteStreamRegistry()
	videoDecoded := newFakeDecodedRemoteTrack("remote-video", "", webrtc.RTPCodecTypeVideo, codec.VP8, 96)

	track, streams, err := registry.bindSource(videoDecoded)
	if err != nil {
		t.Fatalf("bindSource() error = %v", err)
	}
	if len(streams) != 0 {
		t.Fatalf("streamless remote track should not create streams, got %d", len(streams))
	}
	videoTrack := track.(PionRemoteVideoTrack)
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

func TestPionRemoteTrackCodecChangeAndNaturalEnd(t *testing.T) {
	registry := NewRemoteStreamRegistry()
	videoDecoded := newFakeDecodedRemoteTrack("remote-video", "stream-2", webrtc.RTPCodecTypeVideo, codec.VP8, 96)

	track, _, err := registry.bindSource(videoDecoded)
	if err != nil {
		t.Fatalf("bindSource() error = %v", err)
	}
	videoTrack := track.(PionRemoteVideoTrack)

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

func TestPCRemoteTrackMultiStreamFanoutAndClose(t *testing.T) {
	registry := NewRemoteStreamRegistry()
	videoSource := newFakePCRemoteTrack("remote-video", "video", []string{"stream-a", "stream-b", "stream-a"})

	track, streams, err := registry.bindSource(videoSource)
	if err != nil {
		t.Fatalf("bindSource() error = %v", err)
	}
	videoTrack := track.(PCRemoteVideoTrack)
	clone := videoTrack.Clone().(PCRemoteVideoTrack)
	t.Cleanup(func() {
		registry.Close()
	})

	if got := videoTrack.StreamID(); got != "stream-a" {
		t.Fatalf("StreamID() = %q, want %q", got, "stream-a")
	}
	if got := videoTrack.StreamIDs(); len(got) != 2 || got[0] != "stream-a" || got[1] != "stream-b" {
		t.Fatalf("StreamIDs() = %v, want [stream-a stream-b]", got)
	}
	if len(streams) != 2 {
		t.Fatalf("streams len = %d, want 2", len(streams))
	}
	for _, stream := range streams {
		if got := stream.GetTrackByID(videoTrack.ID()); got == nil {
			t.Fatalf("expected track %q in stream %q", videoTrack.ID(), stream.ID())
		}
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

func TestRemoteStreamRegistryCloseStopsTracks(t *testing.T) {
	registry := NewRemoteStreamRegistry()
	videoSource := newFakePCRemoteTrack("remote-video", "video", []string{"stream-a"})

	track, _, err := registry.bindSource(videoSource)
	if err != nil {
		t.Fatalf("bindSource() error = %v", err)
	}
	videoTrack := track.(PCRemoteVideoTrack)

	registry.Close()

	waitForCondition(t, 2*time.Second, func() bool { return videoSource.closed.Load() })
	if got := videoTrack.ReadyState(); got != "ended" {
		t.Fatalf("ReadyState() after registry.Close = %q, want ended", got)
	}
}

func TestRemoteStreamRegistryBindDecodedTrackRejectsNil(t *testing.T) {
	registry := NewRemoteStreamRegistry()
	if _, _, err := registry.BindDecodedTrack(nil); !errors.Is(err, ErrTrackNotFound) {
		t.Fatalf("BindDecodedTrack(nil) error = %v, want %v", err, ErrTrackNotFound)
	}
}

func TestRemoteStreamRegistryPCOnTrackErrorPath(t *testing.T) {
	registry := NewRemoteStreamRegistry()

	var handlerCalled bool
	errCh := make(chan error, 1)
	callback := registry.PCOnTrack(func(PCTrackEvent) {
		handlerCalled = true
	}, func(err error) {
		errCh <- err
	})

	callback(nil, nil, nil)

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrTrackNotFound) {
			t.Fatalf("PCOnTrack() error = %v, want %v", err, ErrTrackNotFound)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for PCOnTrack error callback")
	}
	if handlerCalled {
		t.Fatal("PCOnTrack success handler should not run on bind error")
	}
}

func TestRemoteStreamRegistryPionOnTrackErrorPath(t *testing.T) {
	registry := NewRemoteStreamRegistry()

	var handlerCalled bool
	errCh := make(chan error, 1)
	callback := registry.PionOnTrack(func(PionTrackEvent) {
		handlerCalled = true
	}, func(err error) {
		errCh <- err
	})

	callback(nil, nil)

	select {
	case err := <-errCh:
		if !errors.Is(err, pionrecv.ErrNilTrack) {
			t.Fatalf("PionOnTrack() error = %v, want %v", err, pionrecv.ErrNilTrack)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for PionOnTrack error callback")
	}
	if handlerCalled {
		t.Fatal("PionOnTrack success handler should not run on bind error")
	}
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
