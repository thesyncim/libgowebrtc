package media

import (
	"testing"

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

func (f *fakeMediaTrack) ID() string         { return f.id }
func (f *fakeMediaTrack) Kind() string       { return f.kind }
func (f *fakeMediaTrack) Label() string      { return f.label }
func (f *fakeMediaTrack) Enabled() bool      { return f.enabled }
func (f *fakeMediaTrack) SetEnabled(v bool)  { f.enabled = v }
func (f *fakeMediaTrack) Muted() bool        { return f.muted }
func (f *fakeMediaTrack) ReadyState() string { return f.readyState }
func (f *fakeMediaTrack) Stop()              { f.readyState = "ended" }
func (f *fakeMediaTrack) Clone() MediaStreamTrack {
	return &fakeMediaTrack{
		id:         f.id + "-clone",
		kind:       f.kind,
		label:      f.label,
		enabled:    f.enabled,
		muted:      f.muted,
		readyState: f.readyState,
	}
}

func newTestVideoTrack(t *testing.T) *videoStreamTrack {
	t.Helper()

	track, err := newVideoStreamTrack(
		VideoConstraints{
			Width:     ExactInt(640),
			Height:    ExactInt(480),
			FrameRate: ExactFloat(30),
			Codec:     codec.VP8,
			Bitrate:   500_000,
		},
		VideoTrackSettings{
			Width:     640,
			Height:    480,
			FrameRate: 30,
		},
		"camera",
	)
	if err != nil {
		t.Fatalf("newVideoStreamTrack() error = %v", err)
	}
	t.Cleanup(func() { track.Stop() })
	return track
}

func newTestAudioTrack(t *testing.T) *audioStreamTrack {
	t.Helper()

	track, err := newAudioStreamTrack(
		AudioConstraints{
			SampleRate:       ExactInt(48_000),
			ChannelCount:     ExactInt(2),
			EchoCancellation: ExactBool(false),
			NoiseSuppression: ExactBool(false),
			AutoGainControl:  ExactBool(false),
			Bitrate:          64_000,
		},
		AudioTrackSettings{
			SampleRate:   48_000,
			ChannelCount: 2,
		},
		"microphone",
	)
	if err != nil {
		t.Fatalf("newAudioStreamTrack() error = %v", err)
	}
	t.Cleanup(func() { track.Stop() })
	return track
}

func TestMediaStreamTrackCollectionOperations(t *testing.T) {
	stream := NewMediaStream()
	video := newTestVideoTrack(t)
	audio := newTestAudioTrack(t)

	stream.AddTrack(video)
	stream.AddTrack(audio)

	if got := len(stream.GetVideoTracks()); got != 1 {
		t.Fatalf("GetVideoTracks() len = %d, want 1", got)
	}
	if got := len(stream.GetAudioTracks()); got != 1 {
		t.Fatalf("GetAudioTracks() len = %d, want 1", got)
	}
	if got := len(stream.GetTracks()); got != 2 {
		t.Fatalf("GetTracks() len = %d, want 2", got)
	}
	if got := stream.GetTrackByID(video.ID()); got == nil {
		t.Fatal("GetTrackByID(video) returned nil")
	}
	if !stream.Active() {
		t.Fatal("stream should be active while tracks are live")
	}

	stream.RemoveTrack(video)
	if got := len(stream.GetVideoTracks()); got != 0 {
		t.Fatalf("GetVideoTracks() len after remove = %d, want 0", got)
	}
	if got := len(stream.GetTracks()); got != 1 {
		t.Fatalf("GetTracks() len after remove = %d, want 1", got)
	}

	stream.RemoveTrack(video)
	if got := len(stream.GetTracks()); got != 1 {
		t.Fatalf("GetTracks() len after removing missing track = %d, want 1", got)
	}
}

func TestMediaStreamTrackCollectionIgnoresNilAndDuplicates(t *testing.T) {
	stream := NewMediaStream()
	video := newTestVideoTrack(t)

	stream.AddTrack(nil)
	stream.AddTrack(video)
	stream.AddTrack(video)

	if got := len(stream.GetTracks()); got != 1 {
		t.Fatalf("GetTracks() len after nil/duplicate add = %d, want 1", got)
	}

	stream.RemoveTrack(nil)
	if got := len(stream.GetTracks()); got != 1 {
		t.Fatalf("GetTracks() len after nil remove = %d, want 1", got)
	}
}

func TestMediaStreamClonePreservesTopology(t *testing.T) {
	stream := NewMediaStream()
	video := newTestVideoTrack(t)
	audio := newTestAudioTrack(t)
	stream.AddTrack(video)
	stream.AddTrack(audio)

	clone := stream.Clone()
	if clone == nil {
		t.Fatal("Clone() returned nil")
	}
	if clone.ID() == stream.ID() {
		t.Fatal("clone stream should have a distinct ID")
	}
	if got := len(clone.GetTracks()); got != 2 {
		t.Fatalf("clone GetTracks() len = %d, want 2", got)
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
		t.Fatal("stream should be inactive after all original tracks stop")
	}
	if !clone.Active() {
		t.Fatal("clone should remain active when original stops")
	}
}

func TestMediaStreamClonePreservesTrackState(t *testing.T) {
	stream := NewMediaStream()
	video := newTestVideoTrack(t)
	audio := newTestAudioTrack(t)
	stream.AddTrack(video)
	stream.AddTrack(audio)

	video.SetEnabled(false)
	audio.SetEnabled(false)
	video.Stop()

	clone := stream.Clone()
	if clone == nil {
		t.Fatal("Clone() returned nil")
	}

	clonedVideo := clone.GetVideoTracks()[0]
	clonedAudio := clone.GetAudioTracks()[0]

	if clonedVideo.Enabled() {
		t.Fatal("cloned video track should preserve disabled state")
	}
	if clonedAudio.Enabled() {
		t.Fatal("cloned audio track should preserve disabled state")
	}
	if got := clonedVideo.ReadyState(); got != "ended" {
		t.Fatalf("cloned video ReadyState() = %q, want ended", got)
	}
	if got := clonedAudio.ReadyState(); got != "live" {
		t.Fatalf("cloned audio ReadyState() = %q, want live", got)
	}
}

func TestVideoStreamTrackApplyConstraintsAndLifecycle(t *testing.T) {
	video := newTestVideoTrack(t)

	if err := video.ApplyConstraints(VideoConstraints{
		Bitrate:   900_000,
		FrameRate: ExactFloat(15),
	}); err != nil {
		t.Fatalf("ApplyConstraints() error = %v", err)
	}

	if got := video.GetConstraints().Bitrate; got != 900_000 {
		t.Fatalf("Bitrate after ApplyConstraints() = %d, want 900000", got)
	}
	if got := video.GetSettings().FrameRate; got != 15 {
		t.Fatalf("FrameRate after ApplyConstraints() = %.0f, want 15", got)
	}

	if err := video.ApplyConstraints(VideoConstraints{FrameRate: IdealFloat(60)}); err != nil {
		t.Fatalf("ApplyConstraints() with ideal frame rate above supported max = %v", err)
	}
	if got := video.GetSettings().FrameRate; got != 30 {
		t.Fatalf("FrameRate after ideal clamp = %.0f, want 30", got)
	}

	if err := video.ApplyConstraints(VideoConstraints{FrameRate: ExactFloat(60)}); err == nil {
		t.Fatal("ApplyConstraints() with unsupported exact frame rate = nil, want error")
	}
	if err := video.ApplyConstraints(VideoConstraints{Width: ExactInt(800)}); err == nil {
		t.Fatal("ApplyConstraints() with incompatible width = nil, want error")
	}

	video.SetEnabled(false)
	if video.Enabled() {
		t.Fatal("Enabled() = true, want false")
	}

	video.Stop()
	if got := video.ReadyState(); got != "ended" {
		t.Fatalf("ReadyState() after Stop() = %q, want %q", got, "ended")
	}
}

func TestAudioStreamTrackApplyConstraintsAndLifecycle(t *testing.T) {
	audio := newTestAudioTrack(t)

	if err := audio.ApplyConstraints(AudioConstraints{
		Bitrate:          96_000,
		EchoCancellation: ExactBool(true),
		NoiseSuppression: ExactBool(true),
		AutoGainControl:  ExactBool(true),
	}); err != nil {
		t.Fatalf("ApplyConstraints() error = %v", err)
	}

	if got := audio.GetConstraints().Bitrate; got != 96_000 {
		t.Fatalf("Bitrate after ApplyConstraints() = %d, want 96000", got)
	}
	if !audio.GetSettings().EchoCancellation {
		t.Fatal("EchoCancellation = false, want true")
	}
	if !audio.GetSettings().NoiseSuppression {
		t.Fatal("NoiseSuppression = false, want true")
	}
	if !audio.GetSettings().AutoGainControl {
		t.Fatal("AutoGainControl = false, want true")
	}

	if err := audio.ApplyConstraints(AudioConstraints{SampleRate: ExactInt(44_100)}); err == nil {
		t.Fatal("ApplyConstraints() with incompatible sample rate = nil, want error")
	}

	audio.SetEnabled(false)
	if audio.Enabled() {
		t.Fatal("Enabled() = true, want false")
	}

	audio.Stop()
	if got := audio.ReadyState(); got != "ended" {
		t.Fatalf("ReadyState() after Stop() = %q, want %q", got, "ended")
	}
}

func TestTrackLocalExtractsUnderlyingPionTrack(t *testing.T) {
	stream := NewMediaStream()
	video := newTestVideoTrack(t)
	stream.AddTrack(video)
	stream.AddTrack(&fakeMediaTrack{
		id:         "fake-video",
		kind:       "video",
		label:      "fake",
		enabled:    true,
		readyState: "live",
	})

	if pionTrack, ok := TrackLocal(video); !ok || pionTrack == nil {
		t.Fatal("TrackLocal() should succeed for real media track")
	}
	if pionTrack, ok := TrackLocal(stream.GetTrackByID("fake-video")); ok || pionTrack != nil {
		t.Fatal("TrackLocal() should fail for fake media track")
	}
}
