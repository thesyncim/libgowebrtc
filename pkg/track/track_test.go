package track

import (
	"testing"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

func TestTrackErrors(t *testing.T) {
	errors := []error{
		ErrTrackClosed,
		ErrNotBound,
		ErrAlreadyBound,
		ErrEncodeFailed,
		ErrInvalidConfig,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("Error should not be nil")
		}
		if err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	}
}

func TestVideoTrackConfig(t *testing.T) {
	cfg := VideoTrackConfig{
		ID:       "video-0",
		StreamID: "stream-0",
		Codec:    codec.H264,
		Width:    1920,
		Height:   1080,
		Bitrate:  4_000_000,
		FPS:      30,
		MTU:      1200,
	}

	if cfg.ID != "video-0" {
		t.Errorf("ID = %v, want video-0", cfg.ID)
	}
	if cfg.Codec != codec.H264 {
		t.Errorf("Codec = %v, want H264", cfg.Codec)
	}
	if cfg.Width != 1920 {
		t.Errorf("Width = %v, want 1920", cfg.Width)
	}
}

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

func TestAudioTrackConfig(t *testing.T) {
	cfg := AudioTrackConfig{
		ID:         "audio-0",
		StreamID:   "stream-0",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
		MTU:        1200,
	}

	if cfg.ID != "audio-0" {
		t.Errorf("ID = %v, want audio-0", cfg.ID)
	}
	if cfg.SampleRate != 48000 {
		t.Errorf("SampleRate = %v, want 48000", cfg.SampleRate)
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

// Interface compliance tests
func TestVideoTrackImplementsTrackLocal(t *testing.T) {
	var _ webrtc.TrackLocal = (*VideoTrack)(nil)
}

func TestAudioTrackImplementsTrackLocal(t *testing.T) {
	var _ webrtc.TrackLocal = (*AudioTrack)(nil)
}

func BenchmarkNewVideoTrack(b *testing.B) {
	cfg := VideoTrackConfig{
		ID:       "video-0",
		StreamID: "stream-0",
		Codec:    codec.H264,
		Width:    1920,
		Height:   1080,
		Bitrate:  4_000_000,
		FPS:      30,
		MTU:      1200,
	}

	for i := 0; i < b.N; i++ {
		track, _ := NewVideoTrack(cfg)
		track.Close()
	}
}

func BenchmarkNewAudioTrack(b *testing.B) {
	cfg := AudioTrackConfig{
		ID:         "audio-0",
		StreamID:   "stream-0",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
		MTU:        1200,
	}

	for i := 0; i < b.N; i++ {
		track, _ := NewAudioTrack(cfg)
		track.Close()
	}
}
