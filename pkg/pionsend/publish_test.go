package pionsend

import (
	"testing"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/track"
)

func TestPublishAudioValidation(t *testing.T) {
	audioTrack, err := track.NewAudioTrack(track.AudioTrackConfig{
		ID:         "audio-track",
		StreamID:   "stream-audio",
		SampleRate: 48_000,
		Channels:   2,
		Bitrate:    64_000,
		MTU:        1200,
	})
	if err != nil {
		t.Fatalf("NewAudioTrack: %v", err)
	}

	if _, err := PublishAudio(nil, AudioPublishConfig{Track: audioTrack}); err != ErrNilPeerConnection {
		t.Fatalf("PublishAudio(nil) error = %v, want %v", err, ErrNilPeerConnection)
	}
	if _, err := PublishAudio(&webrtc.PeerConnection{}, AudioPublishConfig{}); err != ErrInvalidConfig {
		t.Fatalf("PublishAudio(empty cfg) error = %v, want %v", err, ErrInvalidConfig)
	}
}

func TestPublishVideoValidation(t *testing.T) {
	videoTrack, err := track.NewVideoTrack(track.VideoTrackConfig{
		ID:       "video-track",
		StreamID: "stream-video",
		Codec:    codec.VP8,
		Width:    1280,
		Height:   720,
		Bitrate:  700_000,
		FPS:      30,
		MTU:      1200,
	})
	if err != nil {
		t.Fatalf("NewVideoTrack: %v", err)
	}

	valid := VideoPublishConfig{
		Encodings: []VideoPublishEncoding{{
			Track:                 videoTrack,
			Width:                 1280,
			Height:                720,
			Bitrate:               700_000,
			ScaleResolutionDownBy: 1,
			Active:                true,
		}},
	}

	if _, err := PublishVideo(nil, valid); err != ErrNilPeerConnection {
		t.Fatalf("PublishVideo(nil) error = %v, want %v", err, ErrNilPeerConnection)
	}

	for _, tc := range []struct {
		name string
		cfg  VideoPublishConfig
	}{
		{name: "empty", cfg: VideoPublishConfig{}},
		{name: "nil track", cfg: VideoPublishConfig{Encodings: []VideoPublishEncoding{{Width: 1280, Height: 720, Bitrate: 700_000, ScaleResolutionDownBy: 1, Active: true}}}},
		{name: "odd width", cfg: VideoPublishConfig{Encodings: []VideoPublishEncoding{{Track: videoTrack, Width: 1279, Height: 720, Bitrate: 700_000, ScaleResolutionDownBy: 1, Active: true}}}},
		{name: "bad scale", cfg: VideoPublishConfig{Encodings: []VideoPublishEncoding{{Track: videoTrack, Width: 1280, Height: 720, Bitrate: 700_000, ScaleResolutionDownBy: 0.5, Active: true}}}},
		{name: "mismatched track identity", cfg: VideoPublishConfig{Encodings: []VideoPublishEncoding{
			{Track: videoTrack, Width: 1280, Height: 720, Bitrate: 700_000, ScaleResolutionDownBy: 1, Active: true},
			{
				Track: func() *track.VideoTrack {
					other, err := track.NewVideoTrack(track.VideoTrackConfig{
						ID:       "other-track",
						StreamID: "stream-video",
						Codec:    codec.VP8,
						Width:    640,
						Height:   360,
						Bitrate:  240_000,
						FPS:      30,
						MTU:      1200,
					})
					if err != nil {
						t.Fatalf("NewVideoTrack(other): %v", err)
					}
					return other
				}(),
				Width:                 640,
				Height:                360,
				Bitrate:               240_000,
				ScaleResolutionDownBy: 2,
				Active:                true,
			},
		}}},
	} {
		if _, err := PublishVideo(&webrtc.PeerConnection{}, tc.cfg); err != ErrInvalidConfig {
			t.Fatalf("PublishVideo(%s) error = %v, want %v", tc.name, err, ErrInvalidConfig)
		}
	}
}

func TestPublishVideoCopiesRequiredHeaderExtensions(t *testing.T) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	videoTrack, err := track.NewVideoTrack(track.VideoTrackConfig{
		ID:       "video-track",
		StreamID: "stream-video",
		Codec:    codec.VP8,
		Width:    1280,
		Height:   720,
		Bitrate:  700_000,
		FPS:      30,
		MTU:      1200,
	})
	if err != nil {
		t.Fatalf("NewVideoTrack: %v", err)
	}

	required := []string{midRTPHeaderExtensionURI}
	published, err := PublishVideo(pc, VideoPublishConfig{
		Encodings: []VideoPublishEncoding{{
			Track:                 videoTrack,
			Width:                 1280,
			Height:                720,
			Bitrate:               700_000,
			ScaleResolutionDownBy: 1,
			Active:                true,
		}},
		RequiredHeaderExtensions: required,
	})
	if err != nil {
		t.Fatalf("PublishVideo: %v", err)
	}
	defer published.Close()

	required[0] = "mutated"
	video := published.(*publishedVideo)
	if got := video.cfg.RequiredHeaderExtensions[0]; got != midRTPHeaderExtensionURI {
		t.Fatalf("RequiredHeaderExtensions[0] = %q, want %q", got, midRTPHeaderExtensionURI)
	}
}

func TestAllocScaledFrameRequiresExplicitLayout(t *testing.T) {
	track, err := track.NewVideoTrack(track.VideoTrackConfig{
		ID:       "video-track",
		StreamID: "stream-video",
		Codec:    codec.VP8,
		Width:    1280,
		Height:   720,
		Bitrate:  700_000,
		FPS:      30,
		MTU:      1200,
	})
	if err != nil {
		t.Fatalf("NewVideoTrack: %v", err)
	}

	if frame := allocScaledFrame(VideoPublishEncoding{
		Track:                 track,
		Width:                 320,
		Height:                180,
		Bitrate:               120_000,
		ScaleResolutionDownBy: 4,
	}); frame == nil || frame.Width != 320 || frame.Height != 180 {
		t.Fatalf("allocScaledFrame() = %+v, want 320x180 frame", frame)
	}

	if frame := allocScaledFrame(VideoPublishEncoding{
		Track:                 track,
		Width:                 1280,
		Height:                720,
		Bitrate:               700_000,
		ScaleResolutionDownBy: 1,
	}); frame != nil {
		t.Fatalf("allocScaledFrame(scale=1) = %+v, want nil", frame)
	}
}
