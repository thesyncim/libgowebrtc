package pionsend

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/track"
)

func explicitVideoCodecPreferences(mimeTypes ...string) []webrtc.RTPCodecParameters {
	if len(mimeTypes) == 0 {
		mimeTypes = []string{webrtc.MimeTypeVP8}
	}
	out := make([]webrtc.RTPCodecParameters, 0, len(mimeTypes))
	for _, mimeType := range mimeTypes {
		out = append(out, webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  mimeType,
				ClockRate: 90000,
			},
		})
	}
	return out
}

func explicitAudioPublishConfig() AudioPublishConfig {
	return AudioPublishConfig{
		TrackID:          "audio-track",
		StreamID:         "stream-audio",
		SampleRate:       48000,
		Channels:         2,
		Bitrate:          64000,
		PTime:            20 * time.Millisecond,
		CodecPreferences: []webrtc.RTPCodecParameters{{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}}},
	}
}

func explicitVideoPublishConfig() VideoPublishConfig {
	return VideoPublishConfig{
		TrackID:          "video-track",
		StreamID:         "stream-video",
		Width:            1280,
		Height:           720,
		Bitrate:          700000,
		FPS:              30,
		CodecPreferences: explicitVideoCodecPreferences(),
	}
}

func newTestPeerConnection(t *testing.T) *webrtc.PeerConnection {
	t.Helper()

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	t.Cleanup(func() {
		_ = pc.Close()
	})
	return pc
}

func TestPublishAudioLifecycle(t *testing.T) {
	pc := newTestPeerConnection(t)

	published, err := PublishAudio(pc, explicitAudioPublishConfig())
	if err != nil {
		t.Fatalf("PublishAudio: %v", err)
	}

	audio := published.(*publishedAudio)
	if audio.cfg.SampleRate != 48000 || audio.cfg.Channels != 2 {
		t.Fatalf("audio config = %d Hz/%d ch, want 48000/2", audio.cfg.SampleRate, audio.cfg.Channels)
	}
	if audio.cfg.StreamID != "stream-audio" {
		t.Fatalf("StreamID = %q, want %q", audio.cfg.StreamID, "stream-audio")
	}
	if audio.cfg.Bitrate != 64000 || audio.cfg.PTime != 20*time.Millisecond {
		t.Fatalf("audio config bitrate/ptime = %d/%v, want 64000/20ms", audio.cfg.Bitrate, audio.cfg.PTime)
	}
	if len(audio.cfg.CodecPreferences) != 1 || audio.cfg.CodecPreferences[0].MimeType != webrtc.MimeTypeOpus {
		t.Fatalf("CodecPreferences = %+v, want explicit Opus prefs", audio.cfg.CodecPreferences)
	}
	if audio.Sender() == nil {
		t.Fatal("Sender() = nil, want sender")
	}

	if err := audio.WriteFrame(nil); err != ErrNilAudioFrame {
		t.Fatalf("WriteFrame(nil) error = %v, want %v", err, ErrNilAudioFrame)
	}
	if err := audio.SetBitrate(96_000); err != nil {
		t.Fatalf("SetBitrate: %v", err)
	}
	if audio.cfg.Bitrate != 96_000 {
		t.Fatalf("cfg.Bitrate = %d, want 96000", audio.cfg.Bitrate)
	}

	if err := audio.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := audio.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	silence := frame.NewAudioFrameS16(48_000, 2, 960)
	if err := audio.WriteFrame(silence); err != nil {
		t.Fatalf("WriteFrame after close error = %v, want nil", err)
	}
	if err := audio.SetBitrate(128_000); err != nil {
		t.Fatalf("SetBitrate after close: %v", err)
	}
}

func TestPublishedAudioDelegatesToTrack(t *testing.T) {
	audioTrack, err := track.NewAudioTrack(track.AudioTrackConfig{
		ID:         "audio-track",
		StreamID:   "stream-audio",
		SampleRate: 48_000,
		Channels:   2,
		Bitrate:    64_000,
	})
	if err != nil {
		t.Fatalf("NewAudioTrack: %v", err)
	}

	audio := &publishedAudio{
		cfg: AudioPublishConfig{
			TrackID:    "audio-track",
			StreamID:   "stream-audio",
			SampleRate: 48_000,
			Channels:   2,
			PTime:      20 * time.Millisecond,
		},
		track:           audioTrack,
		samplesPerFrame: 960,
	}
	silence := frame.NewAudioFrameS16(48_000, 2, 960)
	if err := audio.WriteFrame(silence); !errors.Is(err, track.ErrNotBound) {
		t.Fatalf("WriteFrame(unbound) error = %v, want %v", err, track.ErrNotBound)
	}
	if err := audio.SetBitrate(80_000); err != nil {
		t.Fatalf("SetBitrate: %v", err)
	}
	if got := audio.Sender(); got != nil {
		t.Fatalf("Sender() = %v, want nil", got)
	}
	if err := audio.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestPublishAudioRejectsInvalidPTime(t *testing.T) {
	pc := newTestPeerConnection(t)

	cfg := explicitAudioPublishConfig()
	cfg.PTime = 333 * time.Microsecond
	_, err := PublishAudio(pc, cfg)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("PublishAudio(invalid ptime) error = %v, want %v", err, ErrInvalidConfig)
	}
}

func TestPublishAudioRequiresExplicitCodecPreferences(t *testing.T) {
	pc := newTestPeerConnection(t)

	cfg := explicitAudioPublishConfig()
	cfg.CodecPreferences = nil
	_, err := PublishAudio(pc, cfg)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("PublishAudio(empty codec preferences) error = %v, want %v", err, ErrInvalidConfig)
	}
}

func TestPublishedAudioRejectsMismatchedFrameShape(t *testing.T) {
	audioTrack, err := track.NewAudioTrack(track.AudioTrackConfig{
		ID:         "audio-track",
		StreamID:   "stream-audio",
		SampleRate: 48_000,
		Channels:   2,
		Bitrate:    64_000,
	})
	if err != nil {
		t.Fatalf("NewAudioTrack: %v", err)
	}

	audio := &publishedAudio{
		cfg: AudioPublishConfig{
			TrackID:    "audio-track",
			StreamID:   "stream-audio",
			SampleRate: 48_000,
			Channels:   2,
			PTime:      10 * time.Millisecond,
		},
		track:           audioTrack,
		samplesPerFrame: 480,
	}

	for _, tc := range []struct {
		name string
		src  *frame.AudioFrame
	}{
		{name: "sample rate", src: frame.NewAudioFrameS16(44_100, 2, 480)},
		{name: "channels", src: frame.NewAudioFrameS16(48_000, 1, 480)},
		{name: "ptime", src: frame.NewAudioFrameS16(48_000, 2, 960)},
	} {
		if err := audio.WriteFrame(tc.src); err == nil {
			t.Fatalf("WriteFrame(%s mismatch) error = nil, want error", tc.name)
		}
	}

	matching := frame.NewAudioFrameS16(48_000, 2, 480)
	if err := audio.WriteFrame(matching); !errors.Is(err, track.ErrNotBound) {
		t.Fatalf("WriteFrame(valid frame, unbound track) error = %v, want %v", err, track.ErrNotBound)
	}
}

func TestPublishVideoLifecycle(t *testing.T) {
	pc := newTestPeerConnection(t)

	published, err := PublishVideo(pc, explicitVideoPublishConfig())
	if err != nil {
		t.Fatalf("PublishVideo: %v", err)
	}

	video := published.(*publishedVideo)
	if video.cfg.StreamID != "stream-video" {
		t.Fatalf("StreamID = %q, want %q", video.cfg.StreamID, "stream-video")
	}
	if video.cfg.Bitrate != 700000 {
		t.Fatalf("Bitrate = %d, want 700000", video.cfg.Bitrate)
	}
	if len(video.cfg.CodecPreferences) != 1 || video.cfg.CodecPreferences[0].MimeType != webrtc.MimeTypeVP8 {
		t.Fatalf("CodecPreferences = %+v, want explicit VP8 prefs", video.cfg.CodecPreferences)
	}
	if video.Sender() == nil {
		t.Fatal("Sender() = nil, want sender")
	}

	encodings := video.Encodings()
	if len(encodings) != 1 {
		t.Fatalf("len(Encodings()) = %d, want 1", len(encodings))
	}
	if encodings[0].Width != 1280 || encodings[0].Height != 720 {
		t.Fatalf("encoding size = %dx%d, want 1280x720", encodings[0].Width, encodings[0].Height)
	}
	if encodings[0].Track == nil {
		t.Fatal("encoding.Track = nil, want track")
	}

	if err := video.WriteFrame(nil, false); err != ErrNilVideoFrame {
		t.Fatalf("WriteFrame(nil) error = %v, want %v", err, ErrNilVideoFrame)
	}
	if err := video.SetLayerActive(-1, true); err != ErrInvalidLayerIndex {
		t.Fatalf("SetLayerActive(-1) error = %v, want %v", err, ErrInvalidLayerIndex)
	}
	if err := video.SetLayerBitrate(-1, 1234); err != ErrInvalidLayerIndex {
		t.Fatalf("SetLayerBitrate(-1) error = %v, want %v", err, ErrInvalidLayerIndex)
	}
	if err := video.validateNegotiatedLocked(); err != nil {
		t.Fatalf("validateNegotiatedLocked: %v", err)
	}

	if err := video.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := video.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	src := frame.NewI420Frame(1280, 720)
	if err := video.WriteFrame(src, true); err != nil {
		t.Fatalf("WriteFrame after close error = %v, want nil", err)
	}
}

func TestPublishVideoOptsInToTrackAdaptation(t *testing.T) {
	pc := newTestPeerConnection(t)

	published, err := PublishVideo(pc, explicitVideoPublishConfig())
	if err != nil {
		t.Fatalf("PublishVideo: %v", err)
	}

	video := published.(*publishedVideo)
	defer video.Close()

	var calls int32
	video.encodings[0].videoTrack.SetBWESource(func() *track.BandwidthEstimate {
		atomic.AddInt32(&calls, 1)
		return &track.BandwidthEstimate{TargetBitrateBps: 1_000_000}
	})
	t.Cleanup(func() {
		video.encodings[0].videoTrack.SetBWESource(nil)
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for atomic.LoadInt32(&calls) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&calls) == 0 {
		t.Fatal("SetBWESource did not start adaptation loop, want helper opt-in")
	}
}

func TestPublishVideoSimulcastControls(t *testing.T) {
	pc := newTestPeerConnection(t)

	cfg := explicitVideoPublishConfig()
	cfg.SVC = &codec.SVCConfig{
		Mode: codec.SVCModeS3T3,
		Layers: []codec.SVCLayerConfig{
			{Bitrate: 120_000, Active: true},
			{Bitrate: 240_000, Active: true},
			{Bitrate: 480_000, Active: true},
		},
	}
	published, err := PublishVideo(pc, cfg)
	if err != nil {
		t.Fatalf("PublishVideo(simulcast): %v", err)
	}

	video := published.(*publishedVideo)
	encodings := video.Encodings()
	if len(encodings) != 3 {
		t.Fatalf("len(Encodings()) = %d, want 3", len(encodings))
	}
	for i, wantRID := range []string{"q", "h", "f"} {
		if encodings[i].RID != wantRID {
			t.Fatalf("encodings[%d].RID = %q, want %q", i, encodings[i].RID, wantRID)
		}
	}

	if err := video.SetLayerActive(1, false); err != nil {
		t.Fatalf("SetLayerActive: %v", err)
	}
	if video.encodings[1].active {
		t.Fatal("encodings[1].active = true, want false")
	}

	if err := video.SetLayerBitrate(2, 456_789); err != nil {
		t.Fatalf("SetLayerBitrate: %v", err)
	}
	if got := video.Encodings()[2].Bitrate; got != 456_789 {
		t.Fatalf("Encodings()[2].Bitrate = %d, want 456789", got)
	}

	copied := video.Encodings()
	copied[0].RID = "mutated"
	if got := video.Encodings()[0].RID; got != "q" {
		t.Fatalf("Encodings() did not return a copy, got RID %q", got)
	}

	video.RequestKeyFrame()
	if err := video.validateNegotiatedLocked(); err != nil {
		t.Fatalf("validateNegotiatedLocked: %v", err)
	}
}

func TestPublishedVideoHelpers(t *testing.T) {
	firstTrack, err := track.NewVideoTrack(track.VideoTrackConfig{
		ID:       "video-1",
		StreamID: "stream-video",
		Codec:    codec.VP8,
		Width:    640,
		Height:   360,
		Bitrate:  300_000,
		FPS:      30,
	})
	if err != nil {
		t.Fatalf("NewVideoTrack(first): %v", err)
	}
	secondTrack, err := track.NewVideoTrack(track.VideoTrackConfig{
		ID:       "video-2",
		StreamID: "stream-video",
		Codec:    codec.VP8,
		Width:    320,
		Height:   180,
		Bitrate:  120_000,
		FPS:      30,
	})
	if err != nil {
		t.Fatalf("NewVideoTrack(second): %v", err)
	}

	published := &publishedVideo{
		cfg: VideoPublishConfig{
			TrackID: "video-track",
			Width:   640,
			Height:  360,
			FPS:     30,
			SVC:     &codec.SVCConfig{Mode: codec.SVCModeL3T3_KEY},
		},
		encodings: []*encodingRuntime{
			{
				PublishedEncoding: PublishedEncoding{Index: 0, RID: "f", Width: 640, Height: 360, Bitrate: 300_000, Track: firstTrack},
				videoTrack:        firstTrack,
				active:            false,
			},
			{
				PublishedEncoding: PublishedEncoding{Index: 1, RID: "h", Width: 320, Height: 180, Bitrate: 120_000, Track: secondTrack},
				videoTrack:        secondTrack,
				active:            true,
				scale:             2.0,
				scaled:            frame.NewI420Frame(320, 180),
			},
		},
	}

	src := frame.NewI420Frame(640, 360)
	src.PTS = 9000
	src.Timestamp = 33
	src.IsKeyframe = true

	if err := published.WriteFrame(src, false); !errors.Is(err, track.ErrNotBound) {
		t.Fatalf("WriteFrame(unbound) error = %v, want %v", err, track.ErrNotBound)
	}
	if published.encodings[1].scaled.PTS != src.PTS || published.encodings[1].scaled.Timestamp != src.Timestamp || !published.encodings[1].scaled.IsKeyframe {
		t.Fatalf("scaled frame metadata = %+v, want copied PTS/timestamp/keyframe", published.encodings[1].scaled)
	}
	if err := published.validateNegotiatedLocked(); err != nil {
		t.Fatalf("validateNegotiatedLocked(unbound): %v", err)
	}

	if got, ok := codecFromPreferences([]webrtc.RTPCodecParameters{{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeAV1}}}); !ok || got != codec.AV1 {
		t.Fatalf("codecFromPreferences() = (%v, %v), want (%v, true)", got, ok, codec.AV1)
	}
	if _, ok := codecFromPreferences(nil); ok {
		t.Fatal("codecFromPreferences(nil) = ok, want no match")
	}

	original := &codec.SVCConfig{
		Mode: codec.SVCModeS2T3,
		Layers: []codec.SVCLayerConfig{
			{Width: 320, Height: 180, Bitrate: 100_000, Active: true},
		},
	}
	cloned := cloneSVCConfig(original)
	cloned.Layers[0].Width = 999
	if cloned.Mode != codec.SVCModeS2T3 {
		t.Fatalf("cloneSVCConfig().Mode = %v, want %v", cloned.Mode, codec.SVCModeS2T3)
	}
	if original.Layers[0].Width != 320 {
		t.Fatalf("cloneSVCConfig() should deep-copy layers, got width %d", original.Layers[0].Width)
	}
	if cloneSVCConfig(nil) != nil {
		t.Fatal("cloneSVCConfig(nil) != nil")
	}

	if frame := (encodingConfig{Scale: 2.0, Width: 320, Height: 180}).AllocScaledFrame(); frame == nil {
		t.Fatal("AllocScaledFrame() = nil, want frame")
	}
	if frame := (encodingConfig{Scale: 1.0, Width: 320, Height: 180}).AllocScaledFrame(); frame != nil {
		t.Fatalf("AllocScaledFrame(scale=1) = %v, want nil", frame)
	}

	if err := closePublishedTracks(published.encodings); err != nil {
		t.Fatalf("closePublishedTracks: %v", err)
	}
	if err := firstTrack.WriteFrame(nil, false); err != track.ErrTrackClosed {
		t.Fatalf("firstTrack.WriteFrame after close error = %v, want %v", err, track.ErrTrackClosed)
	}
	if err := secondTrack.WriteFrame(nil, false); err != track.ErrTrackClosed {
		t.Fatalf("secondTrack.WriteFrame after close error = %v, want %v", err, track.ErrTrackClosed)
	}

	for _, tc := range []struct {
		count    int
		wantRIDs []string
	}{
		{count: 2, wantRIDs: []string{"h", "f"}},
		{count: 3, wantRIDs: []string{"q", "h", "f"}},
		{count: 4, wantRIDs: []string{""}},
	} {
		gotRIDs := defaultRIDs(tc.count)
		if len(gotRIDs) != len(tc.wantRIDs) {
			t.Fatalf("defaultRIDs(%d) len = %d, want %d", tc.count, len(gotRIDs), len(tc.wantRIDs))
		}
		for i, want := range tc.wantRIDs {
			if gotRIDs[i] != want {
				t.Fatalf("defaultRIDs(%d)[%d] = %q, want %q", tc.count, i, gotRIDs[i], want)
			}
		}
	}

	if got := clampPositive(319, 640); got != 318 {
		t.Fatalf("clampPositive(319, 640) = %d, want 318", got)
	}
	if got := clampPositive(0, 641); got != 640 {
		t.Fatalf("clampPositive(0, 641) = %d, want 640", got)
	}
	if got := clampPositiveUint32(0, 42); got != 42 {
		t.Fatalf("clampPositiveUint32(0, 42) = %d, want 42", got)
	}
	if got := evenDimension(3); got != 2 {
		t.Fatalf("evenDimension(3) = %d, want 2", got)
	}
	if got := maxInt(3, 4); got != 4 {
		t.Fatalf("maxInt(3, 4) = %d, want 4", got)
	}
}

func TestPublishedVideoRejectsUnsupportedScaledInput(t *testing.T) {
	videoTrack, err := track.NewVideoTrack(track.VideoTrackConfig{
		ID:       "video-invalid-scaled",
		StreamID: "stream-video",
		Codec:    codec.VP8,
		Width:    640,
		Height:   360,
		Bitrate:  300_000,
		FPS:      30,
	})
	if err != nil {
		t.Fatalf("NewVideoTrack: %v", err)
	}
	defer videoTrack.Close()

	published := &publishedVideo{
		encodings: []*encodingRuntime{{
			PublishedEncoding: PublishedEncoding{Index: 0, Width: 320, Height: 180, Track: videoTrack},
			videoTrack:        videoTrack,
			active:            true,
			scale:             2.0,
			scaled:            frame.NewI420Frame(320, 180),
		}},
	}

	src := frame.NewNV12Frame(640, 360)
	if err := published.WriteFrame(src, false); !errors.Is(err, encoder.ErrUnsupportedPixelFormat) {
		t.Fatalf("WriteFrame(NV12) error = %v, want %v", err, encoder.ErrUnsupportedPixelFormat)
	}
}
