package track

import (
	"testing"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

func TestNewVideoTrack(t *testing.T) {
	tests := []struct {
		name      string
		cfg       VideoTrackConfig
		expectErr bool
	}{
		{
			name: "valid H264",
			cfg: VideoTrackConfig{
				ID:       "video-0",
				StreamID: "stream-0",
				Codec:    codec.H264,
				Width:    1920,
				Height:   1080,
				Bitrate:  4_000_000,
			},
			expectErr: false,
		},
		{
			name: "valid VP8",
			cfg: VideoTrackConfig{
				ID:      "video-1",
				Codec:   codec.VP8,
				Width:   1280,
				Height:  720,
				Bitrate: 2_000_000,
			},
			expectErr: false,
		},
		{
			name: "valid VP9",
			cfg: VideoTrackConfig{
				ID:      "video-2",
				Codec:   codec.VP9,
				Width:   1920,
				Height:  1080,
				Bitrate: 3_000_000,
			},
			expectErr: false,
		},
		{
			name: "valid AV1",
			cfg: VideoTrackConfig{
				ID:      "video-3",
				Codec:   codec.AV1,
				Width:   1920,
				Height:  1080,
				Bitrate: 2_500_000,
			},
			expectErr: false,
		},
		{
			name: "empty ID",
			cfg: VideoTrackConfig{
				Codec:   codec.H264,
				Width:   1920,
				Height:  1080,
				Bitrate: 4_000_000,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			track, err := NewVideoTrack(tt.cfg)
			if tt.expectErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if track == nil {
				t.Error("Track should not be nil")
				return
			}
			if track.ID() != tt.cfg.ID {
				t.Errorf("ID = %v, want %v", track.ID(), tt.cfg.ID)
			}
		})
	}
}

func TestVideoTrackDefaults(t *testing.T) {
	track, err := NewVideoTrack(VideoTrackConfig{
		ID:     "video-0",
		Codec:  codec.H264,
		Width:  1920,
		Height: 1080,
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check StreamID defaults to ID
	if track.StreamID() != "video-0" {
		t.Errorf("StreamID should default to ID, got %v", track.StreamID())
	}
}

func TestVideoTrackProperties(t *testing.T) {
	track, err := NewVideoTrack(VideoTrackConfig{
		ID:       "video-test",
		StreamID: "stream-test",
		Codec:    codec.VP9,
		Width:    1920,
		Height:   1080,
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if track.ID() != "video-test" {
		t.Errorf("ID = %v, want video-test", track.ID())
	}
	if track.StreamID() != "stream-test" {
		t.Errorf("StreamID = %v, want stream-test", track.StreamID())
	}
	if track.Kind() != webrtc.RTPCodecTypeVideo {
		t.Errorf("Kind = %v, want RTPCodecTypeVideo", track.Kind())
	}
	if track.RID() != "" {
		t.Errorf("RID should be empty, got %v", track.RID())
	}
}

func TestVideoTrackNotBound(t *testing.T) {
	track, _ := NewVideoTrack(VideoTrackConfig{
		ID:     "video-0",
		Codec:  codec.H264,
		Width:  1920,
		Height: 1080,
	})

	// WriteFrame should fail when not bound
	err := track.WriteFrame(nil, false)
	if err != ErrNotBound {
		t.Errorf("Expected ErrNotBound, got %v", err)
	}

	// WriteEncodedData should fail when not bound
	err = track.WriteEncodedData([]byte{1, 2, 3}, 0, false)
	if err != ErrNotBound {
		t.Errorf("Expected ErrNotBound, got %v", err)
	}
}

func TestVideoTrackClose(t *testing.T) {
	track, _ := NewVideoTrack(VideoTrackConfig{
		ID:     "video-0",
		Codec:  codec.H264,
		Width:  1920,
		Height: 1080,
	})

	// Close should succeed
	err := track.Close()
	if err != nil {
		t.Errorf("Unexpected error on close: %v", err)
	}

	// Double close should be safe
	err = track.Close()
	if err != nil {
		t.Errorf("Double close should be safe: %v", err)
	}

	// Operations after close should fail
	err = track.WriteFrame(nil, false)
	if err != ErrTrackClosed {
		t.Errorf("Expected ErrTrackClosed, got %v", err)
	}
}

func TestVideoTrackSetBitrateUnbound(t *testing.T) {
	track, _ := NewVideoTrack(VideoTrackConfig{
		ID:      "video-0",
		Codec:   codec.H264,
		Width:   1920,
		Height:  1080,
		Bitrate: 2_000_000,
	})

	// SetBitrate should succeed even when unbound (stores for later)
	err := track.SetBitrate(4_000_000)
	if err != nil {
		t.Errorf("SetBitrate should succeed when unbound: %v", err)
	}
}

func TestVideoTrackSetFramerateUnbound(t *testing.T) {
	track, _ := NewVideoTrack(VideoTrackConfig{
		ID:     "video-0",
		Codec:  codec.H264,
		Width:  1920,
		Height: 1080,
		FPS:    30,
	})

	err := track.SetFramerate(60)
	if err != nil {
		t.Errorf("SetFramerate should succeed when unbound: %v", err)
	}
}

func TestNewAudioTrack(t *testing.T) {
	tests := []struct {
		name      string
		cfg       AudioTrackConfig
		expectErr bool
	}{
		{
			name: "valid config",
			cfg: AudioTrackConfig{
				ID:         "audio-0",
				StreamID:   "stream-0",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    64000,
			},
			expectErr: false,
		},
		{
			name: "minimal config",
			cfg: AudioTrackConfig{
				ID: "audio-1",
			},
			expectErr: false,
		},
		{
			name: "empty ID",
			cfg: AudioTrackConfig{
				SampleRate: 48000,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			track, err := NewAudioTrack(tt.cfg)
			if tt.expectErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if track == nil {
				t.Error("Track should not be nil")
			}
		})
	}
}

func TestAudioTrackDefaults(t *testing.T) {
	track, err := NewAudioTrack(AudioTrackConfig{
		ID: "audio-0",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check defaults
	if track.StreamID() != "audio-0" {
		t.Errorf("StreamID should default to ID")
	}
	if track.config.SampleRate != 48000 {
		t.Errorf("SampleRate should default to 48000, got %v", track.config.SampleRate)
	}
	if track.config.Channels != 2 {
		t.Errorf("Channels should default to 2, got %v", track.config.Channels)
	}
	if track.config.Bitrate != 64000 {
		t.Errorf("Bitrate should default to 64000, got %v", track.config.Bitrate)
	}
	if track.config.MTU != 1200 {
		t.Errorf("MTU should default to 1200, got %v", track.config.MTU)
	}
}

func TestAudioTrackProperties(t *testing.T) {
	track, err := NewAudioTrack(AudioTrackConfig{
		ID:       "audio-test",
		StreamID: "stream-test",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if track.ID() != "audio-test" {
		t.Errorf("ID = %v, want audio-test", track.ID())
	}
	if track.StreamID() != "stream-test" {
		t.Errorf("StreamID = %v, want stream-test", track.StreamID())
	}
	if track.Kind() != webrtc.RTPCodecTypeAudio {
		t.Errorf("Kind = %v, want RTPCodecTypeAudio", track.Kind())
	}
	if track.RID() != "" {
		t.Errorf("RID should be empty, got %v", track.RID())
	}
}

func TestAudioTrackNotBound(t *testing.T) {
	track, _ := NewAudioTrack(AudioTrackConfig{
		ID: "audio-0",
	})

	err := track.WriteFrame(nil)
	if err != ErrNotBound {
		t.Errorf("Expected ErrNotBound, got %v", err)
	}

	err = track.WriteEncodedData([]byte{1, 2, 3}, 0)
	if err != ErrNotBound {
		t.Errorf("Expected ErrNotBound, got %v", err)
	}
}

func TestAudioTrackClose(t *testing.T) {
	track, _ := NewAudioTrack(AudioTrackConfig{
		ID: "audio-0",
	})

	err := track.Close()
	if err != nil {
		t.Errorf("Unexpected error on close: %v", err)
	}

	// Double close should be safe
	err = track.Close()
	if err != nil {
		t.Errorf("Double close should be safe: %v", err)
	}

	// Operations after close should fail
	err = track.WriteFrame(nil)
	if err != ErrTrackClosed {
		t.Errorf("Expected ErrTrackClosed, got %v", err)
	}
}

func TestAudioTrackSetBitrateUnbound(t *testing.T) {
	track, _ := NewAudioTrack(AudioTrackConfig{
		ID:      "audio-0",
		Bitrate: 64000,
	})

	err := track.SetBitrate(128000)
	if err != nil {
		t.Errorf("SetBitrate should succeed when unbound: %v", err)
	}
}

func TestVideoTrackSetParameters(t *testing.T) {
	track, err := NewVideoTrack(VideoTrackConfig{
		ID:      "video-0",
		Codec:   codec.H264,
		Width:   1920,
		Height:  1080,
		Bitrate: 2_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("Failed to create track: %v", err)
	}
	defer track.Close()

	// Test SetParameters
	err = track.SetParameters(TrackParameters{
		Active:                true,
		MaxBitrate:            1_000_000,
		MaxFramerate:          15,
		ScaleResolutionDownBy: 2.0,
	})
	if err != nil {
		t.Errorf("SetParameters failed: %v", err)
	}

	// Verify values were applied
	if track.adaptation.currentBitrate != 1_000_000 {
		t.Errorf("Expected bitrate 1000000, got %d", track.adaptation.currentBitrate)
	}
	if track.adaptation.currentFramerate != 15 {
		t.Errorf("Expected framerate 15, got %f", track.adaptation.currentFramerate)
	}
	if track.scaleFactor != 2.0 {
		t.Errorf("Expected scale factor 2.0, got %f", track.scaleFactor)
	}

	// Test pausing track
	err = track.SetParameters(TrackParameters{
		Active: false,
	})
	if err != nil {
		t.Errorf("SetParameters (pause) failed: %v", err)
	}
	if !track.paused.Load() {
		t.Error("Track should be paused")
	}

	// Test resuming track
	err = track.SetParameters(TrackParameters{
		Active: true,
	})
	if err != nil {
		t.Errorf("SetParameters (resume) failed: %v", err)
	}
	if track.paused.Load() {
		t.Error("Track should not be paused")
	}
}

func TestVideoTrackAutoAdaptationDefaults(t *testing.T) {
	// Test that auto adaptation defaults to true
	track, err := NewVideoTrack(VideoTrackConfig{
		ID:      "video-0",
		Codec:   codec.H264,
		Width:   1280,
		Height:  720,
		Bitrate: 2_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("Failed to create track: %v", err)
	}
	defer track.Close()

	// All auto features should default to true
	if !track.config.AutoKeyframe {
		t.Error("AutoKeyframe should default to true")
	}
	if !track.config.AutoBitrate {
		t.Error("AutoBitrate should default to true")
	}
	if !track.config.AutoFramerate {
		t.Error("AutoFramerate should default to true")
	}
	if !track.config.AutoResolution {
		t.Error("AutoResolution should default to true")
	}
}

func TestVideoTrackHandleRTCPFeedback(t *testing.T) {
	track, err := NewVideoTrack(VideoTrackConfig{
		ID:           "video-0",
		Codec:        codec.H264,
		Width:        1280,
		Height:       720,
		AutoKeyframe: true,
	})
	if err != nil {
		t.Fatalf("Failed to create track: %v", err)
	}
	defer track.Close()

	// PLI should set pending keyframe
	track.HandleRTCPFeedback(0, 12345) // PLI
	if !track.keyframePend.Load() {
		t.Error("PLI should set pending keyframe")
	}

	// Reset
	track.keyframePend.Store(false)

	// FIR should also set pending keyframe
	track.HandleRTCPFeedback(1, 12345) // FIR
	if !track.keyframePend.Load() {
		t.Error("FIR should set pending keyframe")
	}
}

func TestVideoTrackSetBWESource(t *testing.T) {
	track, err := NewVideoTrack(VideoTrackConfig{
		ID:          "video-0",
		Codec:       codec.H264,
		Width:       1280,
		Height:      720,
		AutoBitrate: true,
	})
	if err != nil {
		t.Fatalf("Failed to create track: %v", err)
	}

	// Setting BWE source should start adaptation loop
	bwe := &BandwidthEstimate{
		TargetBitrateBps: 1_000_000,
	}
	track.SetBWESource(func() *BandwidthEstimate {
		return bwe
	})

	// Close should stop the adapt loop cleanly
	err = track.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestScaleI420Frame(t *testing.T) {
	// Test scaling a 640x480 frame down to 320x240 (scale factor 2.0)
	srcW, srcH := 640, 480
	dstW, dstH := 320, 240

	// Create source frame with test pattern
	src := &frame.VideoFrame{
		Width:  srcW,
		Height: srcH,
		Format: frame.PixelFormatI420,
		Data: [][]byte{
			make([]byte, srcW*srcH),         // Y
			make([]byte, (srcW/2)*(srcH/2)), // U
			make([]byte, (srcW/2)*(srcH/2)), // V
		},
		Stride: []int{srcW, srcW / 2, srcW / 2},
	}

	// Fill source with gradient (different values in different areas)
	for y := 0; y < srcH; y++ {
		for x := 0; x < srcW; x++ {
			src.Data[0][y*srcW+x] = byte((x + y) % 256)
		}
	}
	for y := 0; y < srcH/2; y++ {
		for x := 0; x < srcW/2; x++ {
			src.Data[1][y*(srcW/2)+x] = 128 // U
			src.Data[2][y*(srcW/2)+x] = 128 // V
		}
	}

	// Create destination frame
	dst := &frame.VideoFrame{
		Width:  dstW,
		Height: dstH,
		Format: frame.PixelFormatI420,
		Data: [][]byte{
			make([]byte, dstW*dstH),         // Y
			make([]byte, (dstW/2)*(dstH/2)), // U
			make([]byte, (dstW/2)*(dstH/2)), // V
		},
		Stride: []int{dstW, dstW / 2, dstW / 2},
	}

	// Scale
	ScaleI420Frame(src, dst, 2.0)

	// Verify dimensions match
	if len(dst.Data[0]) != dstW*dstH {
		t.Errorf("Y plane size mismatch: got %d, want %d", len(dst.Data[0]), dstW*dstH)
	}

	// Verify some pixels are non-zero (scaling happened)
	nonZero := 0
	for _, v := range dst.Data[0] {
		if v != 0 {
			nonZero++
		}
	}
	if nonZero == 0 {
		t.Error("All Y pixels are zero after scaling")
	}
}

func TestScaleI420FrameFast(t *testing.T) {
	// Test fast (nearest neighbor) scaling
	srcW, srcH := 640, 480
	dstW, dstH := 320, 240

	src := &frame.VideoFrame{
		Width:  srcW,
		Height: srcH,
		Format: frame.PixelFormatI420,
		Data: [][]byte{
			make([]byte, srcW*srcH),
			make([]byte, (srcW/2)*(srcH/2)),
			make([]byte, (srcW/2)*(srcH/2)),
		},
		Stride: []int{srcW, srcW / 2, srcW / 2},
	}

	// Fill with known pattern
	for i := range src.Data[0] {
		src.Data[0][i] = 200
	}
	for i := range src.Data[1] {
		src.Data[1][i] = 128
		src.Data[2][i] = 128
	}

	dst := &frame.VideoFrame{
		Width:  dstW,
		Height: dstH,
		Format: frame.PixelFormatI420,
		Data: [][]byte{
			make([]byte, dstW*dstH),
			make([]byte, (dstW/2)*(dstH/2)),
			make([]byte, (dstW/2)*(dstH/2)),
		},
		Stride: []int{dstW, dstW / 2, dstW / 2},
	}

	ScaleI420FrameFast(src, dst, 2.0)

	// All Y values should be 200 (nearest neighbor preserves exact values)
	for i, v := range dst.Data[0] {
		if v != 200 {
			t.Errorf("Y[%d] = %d, want 200", i, v)
			break
		}
	}
}
