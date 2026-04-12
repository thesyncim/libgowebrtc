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

func newTestAudioTrack(t *testing.T) *track.AudioTrack {
	t.Helper()

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
	return audioTrack
}

func newTestVideoTrack(t *testing.T, id, rid string, width, height int, bitrate uint32, autoAdapt bool) *track.VideoTrack {
	t.Helper()

	videoTrack, err := track.NewVideoTrack(track.VideoTrackConfig{
		ID:             id,
		StreamID:       "stream-video",
		RID:            rid,
		Codec:          codec.VP8,
		Width:          width,
		Height:         height,
		Bitrate:        bitrate,
		FPS:            30,
		MTU:            1200,
		AutoBitrate:    autoAdapt,
		AutoFramerate:  autoAdapt,
		AutoResolution: autoAdapt,
		AutoKeyframe:   autoAdapt,
	})
	if err != nil {
		t.Fatalf("NewVideoTrack(%s): %v", id, err)
	}
	return videoTrack
}

func explicitAudioPublishConfig(t *testing.T) AudioPublishConfig {
	t.Helper()
	return AudioPublishConfig{Track: newTestAudioTrack(t)}
}

func explicitVideoPublishConfig(t *testing.T) VideoPublishConfig {
	t.Helper()

	videoTrack := newTestVideoTrack(t, "video-track", "", 1280, 720, 700_000, false)
	return VideoPublishConfig{
		Encodings: []VideoPublishEncoding{{
			Track:                 videoTrack,
			Width:                 1280,
			Height:                720,
			Bitrate:               700_000,
			ScaleResolutionDownBy: 1,
			Active:                true,
		}},
		RequiredHeaderExtensions: []string{midRTPHeaderExtensionURI},
	}
}

func TestPublishAudioLifecycle(t *testing.T) {
	pc := newTestPeerConnection(t)
	cfg := explicitAudioPublishConfig(t)

	published, err := PublishAudio(pc, cfg)
	if err != nil {
		t.Fatalf("PublishAudio: %v", err)
	}

	audio := published.(*publishedAudio)
	if audio.track != cfg.Track {
		t.Fatal("PublishAudio should reuse caller-provided track")
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

func TestPublishedAudioDelegatesWithoutFramePolicy(t *testing.T) {
	audioTrack := newTestAudioTrack(t)

	audio := &publishedAudio{
		cfg:   AudioPublishConfig{Track: audioTrack},
		track: audioTrack,
	}

	mismatched := frame.NewAudioFrameS16(44_100, 1, 480)
	if err := audio.WriteFrame(mismatched); !errors.Is(err, track.ErrNotBound) {
		t.Fatalf("WriteFrame(mismatched, unbound) error = %v, want %v", err, track.ErrNotBound)
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

func TestPublishVideoLifecycle(t *testing.T) {
	pc := newTestPeerConnection(t)

	published, err := PublishVideo(pc, explicitVideoPublishConfig(t))
	if err != nil {
		t.Fatalf("PublishVideo: %v", err)
	}

	video := published.(*publishedVideo)
	if video.Sender() == nil {
		t.Fatal("Sender() = nil, want sender")
	}

	encodings := video.Encodings()
	if len(encodings) != 1 {
		t.Fatalf("len(Encodings()) = %d, want 1", len(encodings))
	}
	if encodings[0].Track == nil {
		t.Fatal("encoding.Track = nil, want track")
	}
	if encodings[0].Width != 1280 || encodings[0].Height != 720 || encodings[0].Bitrate != 700_000 {
		t.Fatalf("encoding = %+v, want 1280x720@700000", encodings[0])
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

func TestPublishVideoLeavesAdaptationPolicyToCaller(t *testing.T) {
	pc := newTestPeerConnection(t)

	t.Run("NoAutoAdaptation", func(t *testing.T) {
		published, err := PublishVideo(pc, explicitVideoPublishConfig(t))
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
		defer video.encodings[0].videoTrack.SetBWESource(nil)

		time.Sleep(250 * time.Millisecond)
		if got := atomic.LoadInt32(&calls); got != 0 {
			t.Fatalf("BWE source calls = %d, want 0 when caller did not enable adaptation", got)
		}
	})

	t.Run("CallerOptIn", func(t *testing.T) {
		published, err := PublishVideo(pc, VideoPublishConfig{
			Encodings: []VideoPublishEncoding{{
				Track:                 newTestVideoTrack(t, "video-auto", "", 1280, 720, 700_000, true),
				Width:                 1280,
				Height:                720,
				Bitrate:               700_000,
				ScaleResolutionDownBy: 1,
				Active:                true,
			}},
		})
		if err != nil {
			t.Fatalf("PublishVideo(auto): %v", err)
		}
		video := published.(*publishedVideo)
		defer video.Close()

		var calls int32
		video.encodings[0].videoTrack.SetBWESource(func() *track.BandwidthEstimate {
			atomic.AddInt32(&calls, 1)
			return &track.BandwidthEstimate{TargetBitrateBps: 1_000_000}
		})
		defer video.encodings[0].videoTrack.SetBWESource(nil)

		deadline := time.Now().Add(500 * time.Millisecond)
		for atomic.LoadInt32(&calls) == 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if atomic.LoadInt32(&calls) == 0 {
			t.Fatal("SetBWESource did not start adaptation loop, want caller opt-in to work")
		}
	})
}

func TestPublishVideoSimulcastControls(t *testing.T) {
	pc := newTestPeerConnection(t)

	published, err := PublishVideo(pc, VideoPublishConfig{
		Encodings: []VideoPublishEncoding{
			{
				Track:                 newTestVideoTrack(t, "video-track", "q", 320, 180, 120_000, false),
				Width:                 320,
				Height:                180,
				Bitrate:               120_000,
				ScaleResolutionDownBy: 4,
				Active:                true,
			},
			{
				Track:                 newTestVideoTrack(t, "video-track", "h", 640, 360, 240_000, false),
				Width:                 640,
				Height:                360,
				Bitrate:               240_000,
				ScaleResolutionDownBy: 2,
				Active:                true,
			},
			{
				Track:                 newTestVideoTrack(t, "video-track", "f", 1280, 720, 480_000, false),
				Width:                 1280,
				Height:                720,
				Bitrate:               480_000,
				ScaleResolutionDownBy: 1,
				Active:                true,
			},
		},
		RequiredHeaderExtensions: []string{
			midRTPHeaderExtensionURI,
			rtpStreamIDHeaderExtensionURI,
			repairedStreamIDHeaderExtensionURI,
		},
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
	firstTrack := newTestVideoTrack(t, "video-1", "f", 640, 360, 300_000, false)
	secondTrack := newTestVideoTrack(t, "video-2", "h", 320, 180, 120_000, false)

	published := &publishedVideo{
		cfg: VideoPublishConfig{
			RequiredHeaderExtensions: []string{midRTPHeaderExtensionURI},
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

	if err := closePublishedTracks(published.encodings); err != nil {
		t.Fatalf("closePublishedTracks: %v", err)
	}
	if err := firstTrack.WriteFrame(nil, false); err != track.ErrTrackClosed {
		t.Fatalf("firstTrack.WriteFrame after close error = %v, want %v", err, track.ErrTrackClosed)
	}
	if err := secondTrack.WriteFrame(nil, false); err != track.ErrTrackClosed {
		t.Fatalf("secondTrack.WriteFrame after close error = %v, want %v", err, track.ErrTrackClosed)
	}
}

func TestPublishedVideoRejectsUnsupportedScaledInput(t *testing.T) {
	videoTrack := newTestVideoTrack(t, "video-invalid-scaled", "", 640, 360, 300_000, false)
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
