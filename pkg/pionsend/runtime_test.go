package pionsend

import (
	"errors"
	"testing"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pioncodec"
	"github.com/thesyncim/libgowebrtc/pkg/track"
)

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

func TestPublishAudioDefaultsAndLifecycle(t *testing.T) {
	pc := newTestPeerConnection(t)

	published, err := PublishAudio(pc, AudioPublishConfig{TrackID: "audio-track"})
	if err != nil {
		t.Fatalf("PublishAudio: %v", err)
	}

	audio := published.(*publishedAudio)
	if audio.cfg.StreamID != "audio-track" {
		t.Fatalf("StreamID = %q, want track ID", audio.cfg.StreamID)
	}
	if audio.cfg.Browser != pioncodec.BrowserChrome {
		t.Fatalf("Browser = %q, want %q", audio.cfg.Browser, pioncodec.BrowserChrome)
	}
	if audio.cfg.SampleRate != 48000 || audio.cfg.Channels != 2 {
		t.Fatalf("audio defaults = %d Hz/%d ch, want 48000/2", audio.cfg.SampleRate, audio.cfg.Channels)
	}
	if audio.cfg.Bitrate != 64000 || audio.cfg.MTU != 1200 || audio.cfg.PTime != defaultAudioPTime {
		t.Fatalf("audio defaults bitrate/mtu/ptime = %d/%d/%v, want 64000/1200/%v", audio.cfg.Bitrate, audio.cfg.MTU, audio.cfg.PTime, defaultAudioPTime)
	}
	if len(audio.cfg.CodecPreferences) == 0 {
		t.Fatal("CodecPreferences = 0, want browser defaults")
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

	audio := &publishedAudio{track: audioTrack}
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

func TestPublishVideoDefaultsAndLifecycle(t *testing.T) {
	pc := newTestPeerConnection(t)

	published, err := PublishVideo(pc, VideoPublishConfig{
		TrackID: "video-track",
		Width:   1280,
		Height:  720,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("PublishVideo: %v", err)
	}

	video := published.(*publishedVideo)
	if video.cfg.StreamID != "video-track" {
		t.Fatalf("StreamID = %q, want track ID", video.cfg.StreamID)
	}
	if video.cfg.Browser != pioncodec.BrowserChrome {
		t.Fatalf("Browser = %q, want %q", video.cfg.Browser, pioncodec.BrowserChrome)
	}
	if video.cfg.Bitrate == 0 {
		t.Fatal("Bitrate = 0, want derived default")
	}
	if len(video.cfg.CodecPreferences) == 0 {
		t.Fatal("CodecPreferences = 0, want browser defaults")
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

func TestPublishVideoSimulcastControls(t *testing.T) {
	pc := newTestPeerConnection(t)

	published, err := PublishVideo(pc, VideoPublishConfig{
		TrackID: "video-track",
		Width:   1280,
		Height:  720,
		FPS:     30,
		SVC:     &codec.SVCConfig{Mode: codec.SVCModeS3T3},
	})
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

	if got := codecFromPreferences([]webrtc.RTPCodecParameters{{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeAV1}}}); got != codec.AV1 {
		t.Fatalf("codecFromPreferences() = %v, want %v", got, codec.AV1)
	}
	if got := codecFromPreferences(nil); got != codec.VP8 {
		t.Fatalf("codecFromPreferences(nil) = %v, want %v", got, codec.VP8)
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
		count       int
		wantWeights []int
		wantRIDs    []string
	}{
		{count: 2, wantWeights: []int{1, 2}, wantRIDs: []string{"h", "f"}},
		{count: 3, wantWeights: []int{1, 2, 4}, wantRIDs: []string{"q", "h", "f"}},
		{count: 4, wantWeights: []int{1, 1, 1, 1}, wantRIDs: []string{""}},
	} {
		gotWeights := defaultLayerWeights(tc.count)
		if len(gotWeights) != len(tc.wantWeights) {
			t.Fatalf("defaultLayerWeights(%d) len = %d, want %d", tc.count, len(gotWeights), len(tc.wantWeights))
		}
		for i, want := range tc.wantWeights {
			if gotWeights[i] != want {
				t.Fatalf("defaultLayerWeights(%d)[%d] = %d, want %d", tc.count, i, gotWeights[i], want)
			}
		}

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

	if got := weightedBitrate(700_000, 2, []int{1, 2, 4}); got == 0 {
		t.Fatal("weightedBitrate() = 0, want positive value")
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
