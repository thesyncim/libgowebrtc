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

	config := VideoCaptureConfig{
		Width:     640,
		Height:    480,
		FrameRate: 30,
		Codec:     codec.VP8,
		Bitrate:   500_000,
	}
	settings := VideoTrackSettings{
		Width:     640,
		Height:    480,
		FrameRate: 30,
	}
	cfg, resolved := buildVideoTrackConfig(config, settings)
	track, err := newVideoStreamTrack(
		cfg,
		resolved,
		settings,
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

	config := AudioCaptureConfig{SampleRate: 48_000, ChannelCount: 2, Bitrate: 64_000}
	settings := AudioTrackSettings{
		SampleRate:   48_000,
		ChannelCount: 2,
	}
	cfg, resolved := buildAudioTrackConfig(config, settings)
	track, err := newAudioStreamTrack(
		cfg,
		resolved,
		settings,
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

func TestVideoStreamTrackReconfigureAndLifecycle(t *testing.T) {
	video := newTestVideoTrack(t)

	cfg := video.Config()
	cfg.Bitrate = 900_000
	cfg.FrameRate = 15
	if err := video.Reconfigure(cfg); err != nil {
		t.Fatalf("Reconfigure() error = %v", err)
	}

	if got := video.Config().Bitrate; got != 900_000 {
		t.Fatalf("Bitrate after Reconfigure() = %d, want 900000", got)
	}
	if got := video.Settings().FrameRate; got != 15 {
		t.Fatalf("FrameRate after Reconfigure() = %.0f, want 15", got)
	}

	cfg = video.Config()
	cfg.FrameRate = 60
	if err := video.Reconfigure(cfg); err == nil {
		t.Fatal("Reconfigure() with unsupported frame rate = nil, want error")
	}
	cfg = video.Config()
	cfg.Width = 800
	if err := video.Reconfigure(cfg); err == nil {
		t.Fatal("Reconfigure() with incompatible width = nil, want error")
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

func TestAudioStreamTrackReconfigureAndLifecycle(t *testing.T) {
	audio := newTestAudioTrack(t)

	cfg := audio.Config()
	cfg.Bitrate = 96_000
	cfg.EchoCancellation = true
	cfg.NoiseSuppression = true
	cfg.AutoGainControl = true
	if err := audio.Reconfigure(cfg); err != nil {
		t.Fatalf("Reconfigure() error = %v", err)
	}

	if got := audio.Config().Bitrate; got != 96_000 {
		t.Fatalf("Bitrate after Reconfigure() = %d, want 96000", got)
	}
	if !audio.Settings().EchoCancellation {
		t.Fatal("EchoCancellation = false, want true")
	}
	if !audio.Settings().NoiseSuppression {
		t.Fatal("NoiseSuppression = false, want true")
	}
	if !audio.Settings().AutoGainControl {
		t.Fatal("AutoGainControl = false, want true")
	}

	cfg = audio.Config()
	cfg.SampleRate = 44_100
	if err := audio.Reconfigure(cfg); err == nil {
		t.Fatal("Reconfigure() with incompatible sample rate = nil, want error")
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

func TestPionTrackLocalExtractsUnderlyingPionTrack(t *testing.T) {
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

	if pionTrack, ok := PionTrackLocal(video); !ok || pionTrack == nil {
		t.Fatal("PionTrackLocal() should succeed for real media track")
	}
	if pionTrack, ok := PionTrackLocal(stream.GetTrackByID("fake-video")); ok || pionTrack != nil {
		t.Fatal("PionTrackLocal() should fail for fake media track")
	}
}
