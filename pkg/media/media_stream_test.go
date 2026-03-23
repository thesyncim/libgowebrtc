package media

import (
	"testing"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/testutil"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

type fakeMediaTrack struct {
	id         string
	kind       string
	label      string
	enabled    bool
	muted      bool
	readyState string
}

func (f *fakeMediaTrack) ID() string            { return f.id }
func (f *fakeMediaTrack) Kind() string          { return f.kind }
func (f *fakeMediaTrack) Label() string         { return f.label }
func (f *fakeMediaTrack) Enabled() bool         { return f.enabled }
func (f *fakeMediaTrack) SetEnabled(v bool)     { f.enabled = v }
func (f *fakeMediaTrack) Muted() bool           { return f.muted }
func (f *fakeMediaTrack) ReadyState() string    { return f.readyState }
func (f *fakeMediaTrack) Stop()                 { f.readyState = "ended" }
func (f *fakeMediaTrack) Clone() MediaStreamTrack { return &fakeMediaTrack{
	id:         f.id + "-clone",
	kind:       f.kind,
	label:      f.label,
	enabled:    f.enabled,
	muted:      f.muted,
	readyState: f.readyState,
} }

func TestMediaStreamTrackCollectionOperations(t *testing.T) {
	stream := NewMediaStream()

	video, err := CreateVideoTrack(VideoConstraints{
		Width:     640,
		Height:    480,
		FrameRate: 30,
		Codec:     codec.VP8,
		Bitrate:   500_000,
	})
	if err != nil {
		t.Fatalf("CreateVideoTrack: %v", err)
	}

	audio, err := CreateAudioTrack(AudioConstraints{
		SampleRate:   48_000,
		ChannelCount: 2,
		Bitrate:      64_000,
	})
	if err != nil {
		t.Fatalf("CreateAudioTrack: %v", err)
	}

	stream.AddTrack(video)
	stream.AddTrack(audio)

	if got := len(stream.GetVideoTracks()); got != 1 {
		t.Fatalf("GetVideoTracks len = %d, want 1", got)
	}
	if got := len(stream.GetAudioTracks()); got != 1 {
		t.Fatalf("GetAudioTracks len = %d, want 1", got)
	}
	if got := len(stream.GetTracks()); got != 2 {
		t.Fatalf("GetTracks len = %d, want 2", got)
	}
	if got := stream.GetTrackByID(video.ID()); got == nil {
		t.Fatal("GetTrackByID(video) returned nil")
	}
	if !stream.Active() {
		t.Fatal("stream should be active while tracks are live")
	}

	stream.RemoveTrack(video)
	if got := len(stream.GetVideoTracks()); got != 0 {
		t.Fatalf("GetVideoTracks len after remove = %d, want 0", got)
	}
	if got := len(stream.GetTracks()); got != 1 {
		t.Fatalf("GetTracks len after remove = %d, want 1", got)
	}

	stream.RemoveTrack(video)
	if got := len(stream.GetTracks()); got != 1 {
		t.Fatalf("GetTracks len after removing missing track = %d, want 1", got)
	}
}

func TestMediaStreamClonePreservesTopology(t *testing.T) {
	stream, err := GetUserMedia(Constraints{
		Video: &VideoConstraints{Width: 640, Height: 480, Codec: codec.H264},
		Audio: &AudioConstraints{SampleRate: 48_000, ChannelCount: 2},
	})
	if err != nil {
		t.Fatalf("GetUserMedia: %v", err)
	}

	clone := stream.Clone()
	if clone == nil {
		t.Fatal("Clone returned nil")
	}
	if clone.ID() == stream.ID() {
		t.Fatal("clone stream should have a distinct ID")
	}
	if got := len(clone.GetTracks()); got != 2 {
		t.Fatalf("clone GetTracks len = %d, want 2", got)
	}
	if clone.GetVideoTracks()[0].ID() == stream.GetVideoTracks()[0].ID() {
		t.Fatal("clone video track should have a distinct ID")
	}
	if clone.GetAudioTracks()[0].ID() == stream.GetAudioTracks()[0].ID() {
		t.Fatal("clone audio track should have a distinct ID")
	}

	stream.GetVideoTracks()[0].Stop()
	stream.GetAudioTracks()[0].Stop()
	if stream.Active() {
		t.Fatal("stream should be inactive after all tracks stop")
	}
	if !clone.Active() {
		t.Fatal("clone should remain active when original stops")
	}
}

func TestVideoStreamTrackConstraintsAndGuards(t *testing.T) {
	rawTrack, err := CreateVideoTrack(VideoConstraints{
		Width:     640,
		Height:    480,
		FrameRate: 30,
		Codec:     codec.VP8,
		Bitrate:   400_000,
	})
	if err != nil {
		t.Fatalf("CreateVideoTrack: %v", err)
	}

	vt, ok := rawTrack.(VideoStreamTrack)
	if !ok {
		t.Fatal("expected VideoStreamTrack")
	}

	if err := vt.ApplyConstraints(VideoConstraints{Bitrate: 900_000, FrameRate: 15}); err != nil {
		t.Fatalf("ApplyConstraints: %v", err)
	}

	if got := vt.GetConstraints().Bitrate; got != 900_000 {
		t.Fatalf("Bitrate after ApplyConstraints = %d, want 900000", got)
	}
	if got := vt.GetSettings().FrameRate; got != 15 {
		t.Fatalf("FrameRate after ApplyConstraints = %.0f, want 15", got)
	}

	if _, ok := AsVideoTrack(rawTrack); !ok {
		t.Fatal("AsVideoTrack should succeed for video track")
	}
	if _, ok := AsAudioTrack(rawTrack); ok {
		t.Fatal("AsAudioTrack should fail for video track")
	}

	vt.SetEnabled(false)
	if err := vt.WriteFrame(testutil.CreateTestVideoFrame(640, 480), false); err != nil {
		t.Fatalf("WriteFrame when disabled should be ignored, got %v", err)
	}

	vt.SetEnabled(true)
	vt.Stop()
	if got := vt.ReadyState(); got != "ended" {
		t.Fatalf("ReadyState after Stop = %q, want ended", got)
	}
	if err := vt.WriteFrame(testutil.CreateTestVideoFrame(640, 480), false); err != nil {
		t.Fatalf("WriteFrame after Stop should be ignored, got %v", err)
	}
}

func TestAudioStreamTrackConstraintsAndGuards(t *testing.T) {
	rawTrack, err := CreateAudioTrack(AudioConstraints{
		SampleRate:   48_000,
		ChannelCount: 2,
		Bitrate:      64_000,
	})
	if err != nil {
		t.Fatalf("CreateAudioTrack: %v", err)
	}

	at, ok := rawTrack.(AudioStreamTrack)
	if !ok {
		t.Fatal("expected AudioStreamTrack")
	}

	if err := at.ApplyConstraints(AudioConstraints{Bitrate: 96_000}); err != nil {
		t.Fatalf("ApplyConstraints: %v", err)
	}
	if got := at.GetConstraints().Bitrate; got != 96_000 {
		t.Fatalf("Bitrate after ApplyConstraints = %d, want 96000", got)
	}

	if _, ok := AsAudioTrack(rawTrack); !ok {
		t.Fatal("AsAudioTrack should succeed for audio track")
	}
	if _, ok := AsVideoTrack(rawTrack); ok {
		t.Fatal("AsVideoTrack should fail for audio track")
	}

	at.SetEnabled(false)
	if err := at.WriteFrame(testutil.CreateSilentAudioFrame(48_000, 2, 480)); err != nil {
		t.Fatalf("WriteFrame when disabled should be ignored, got %v", err)
	}

	at.SetEnabled(true)
	at.Stop()
	if got := at.ReadyState(); got != "ended" {
		t.Fatalf("ReadyState after Stop = %q, want ended", got)
	}
	if err := at.WriteFrame(testutil.CreateSilentAudioFrame(48_000, 2, 480)); err != nil {
		t.Fatalf("WriteFrame after Stop should be ignored, got %v", err)
	}
}

func TestPionTrackLocalAndAddTracksToPC(t *testing.T) {
	stream, err := GetUserMedia(Constraints{
		Video: &VideoConstraints{Width: 640, Height: 480, Codec: codec.H264},
		Audio: &AudioConstraints{SampleRate: 48_000, ChannelCount: 2},
	})
	if err != nil {
		t.Fatalf("GetUserMedia: %v", err)
	}

	stream.AddTrack(&fakeMediaTrack{
		id:         "fake-video",
		kind:       "video",
		label:      "fake",
		enabled:    true,
		readyState: "live",
	})

	if pionTrack, ok := PionTrackLocal(stream.GetVideoTracks()[0]); !ok || pionTrack == nil {
		t.Fatal("PionTrackLocal should succeed for real media track")
	}
	if pionTrack, ok := PionTrackLocal(stream.GetTrackByID("fake-video")); ok || pionTrack != nil {
		t.Fatal("PionTrackLocal should fail for fake media track")
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	senders, err := AddTracksToPC(pc, stream)
	if err != nil {
		t.Fatalf("AddTracksToPC: %v", err)
	}
	if got := len(senders); got != 2 {
		t.Fatalf("AddTracksToPC senders len = %d, want 2", got)
	}
}
