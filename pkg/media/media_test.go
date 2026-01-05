package media

import (
	"testing"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

func TestMediaErrors(t *testing.T) {
	errors := []error{
		ErrInvalidConstraints,
		ErrTrackNotFound,
		ErrStreamClosed,
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

func TestVideoConstraints(t *testing.T) {
	constraints := VideoConstraints{
		Width:      1920,
		Height:     1080,
		FrameRate:  30,
		FacingMode: "user",
		DeviceID:   "camera-0",
		Codec:      codec.H264,
		Bitrate:    4_000_000,
		SVC:        codec.SVCPresetChrome(),
	}

	if constraints.Width != 1920 {
		t.Errorf("Width = %v, want 1920", constraints.Width)
	}
	if constraints.Height != 1080 {
		t.Errorf("Height = %v, want 1080", constraints.Height)
	}
	if constraints.FrameRate != 30 {
		t.Errorf("FrameRate = %v, want 30", constraints.FrameRate)
	}
	if constraints.Codec != codec.H264 {
		t.Errorf("Codec = %v, want H264", constraints.Codec)
	}
	if constraints.SVC == nil {
		t.Error("SVC should not be nil")
	}
}

func TestAudioConstraints(t *testing.T) {
	constraints := AudioConstraints{
		SampleRate:       48000,
		ChannelCount:     2,
		EchoCancellation: true,
		NoiseSuppression: true,
		AutoGainControl:  true,
		DeviceID:         "mic-0",
		Bitrate:          64000,
	}

	if constraints.SampleRate != 48000 {
		t.Errorf("SampleRate = %v, want 48000", constraints.SampleRate)
	}
	if constraints.ChannelCount != 2 {
		t.Errorf("ChannelCount = %v, want 2", constraints.ChannelCount)
	}
	if !constraints.EchoCancellation {
		t.Error("EchoCancellation should be true")
	}
	if !constraints.NoiseSuppression {
		t.Error("NoiseSuppression should be true")
	}
	if !constraints.AutoGainControl {
		t.Error("AutoGainControl should be true")
	}
}

func TestConstraints(t *testing.T) {
	tests := []struct {
		name     string
		video    *VideoConstraints
		audio    *AudioConstraints
		hasVideo bool
		hasAudio bool
	}{
		{
			name:     "video only",
			video:    &VideoConstraints{Width: 1920, Height: 1080},
			audio:    nil,
			hasVideo: true,
			hasAudio: false,
		},
		{
			name:     "audio only",
			video:    nil,
			audio:    &AudioConstraints{SampleRate: 48000},
			hasVideo: false,
			hasAudio: true,
		},
		{
			name:     "video and audio",
			video:    &VideoConstraints{Width: 1280, Height: 720},
			audio:    &AudioConstraints{ChannelCount: 2},
			hasVideo: true,
			hasAudio: true,
		},
		{
			name:     "neither",
			video:    nil,
			audio:    nil,
			hasVideo: false,
			hasAudio: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Constraints{
				Video: tt.video,
				Audio: tt.audio,
			}

			if (c.Video != nil) != tt.hasVideo {
				t.Errorf("hasVideo = %v, want %v", c.Video != nil, tt.hasVideo)
			}
			if (c.Audio != nil) != tt.hasAudio {
				t.Errorf("hasAudio = %v, want %v", c.Audio != nil, tt.hasAudio)
			}
		})
	}
}

func TestMediaStreamTrackInterface(t *testing.T) {
	// Compile-time interface check
	var _ MediaStreamTrack = (MediaStreamTrack)(nil)
}

func TestVideoConstraintsCodecs(t *testing.T) {
	codecs := []codec.Type{codec.H264, codec.VP8, codec.VP9, codec.AV1}

	for _, c := range codecs {
		constraints := VideoConstraints{
			Width:  1920,
			Height: 1080,
			Codec:  c,
		}

		if constraints.Codec != c {
			t.Errorf("Codec = %v, want %v", constraints.Codec, c)
		}
	}
}

func TestVideoConstraintsSVCPresets(t *testing.T) {
	presets := []struct {
		name   string
		preset func() *codec.SVCConfig
	}{
		{"None", codec.SVCPresetNone},
		{"ScreenShare", codec.SVCPresetScreenShare},
		{"LowLatency", codec.SVCPresetLowLatency},
		{"SFU", codec.SVCPresetSFU},
		{"SFULite", codec.SVCPresetSFULite},
		{"Simulcast", codec.SVCPresetSimulcast},
		{"SimulcastLite", codec.SVCPresetSimulcastLite},
		{"Chrome", codec.SVCPresetChrome},
		{"Firefox", codec.SVCPresetFirefox},
	}

	for _, p := range presets {
		t.Run(p.name, func(t *testing.T) {
			constraints := VideoConstraints{
				Width:  1920,
				Height: 1080,
				Codec:  codec.VP9,
				SVC:    p.preset(),
			}

			// Just verify it can be set
			_ = constraints
		})
	}
}

func TestAudioConstraintsDefaults(t *testing.T) {
	// Empty constraints should work
	constraints := AudioConstraints{}

	// Verify zero values
	if constraints.SampleRate != 0 {
		t.Errorf("SampleRate default = %v, want 0", constraints.SampleRate)
	}
	if constraints.ChannelCount != 0 {
		t.Errorf("ChannelCount default = %v, want 0", constraints.ChannelCount)
	}
	if constraints.EchoCancellation {
		t.Error("EchoCancellation should default to false")
	}
}

func TestVideoConstraintsFacingModes(t *testing.T) {
	modes := []string{"user", "environment", "left", "right"}

	for _, mode := range modes {
		constraints := VideoConstraints{
			FacingMode: mode,
		}

		if constraints.FacingMode != mode {
			t.Errorf("FacingMode = %v, want %v", constraints.FacingMode, mode)
		}
	}
}

func BenchmarkConstraintsAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := Constraints{
			Video: &VideoConstraints{
				Width:     1920,
				Height:    1080,
				FrameRate: 30,
				Codec:     codec.H264,
				Bitrate:   4_000_000,
			},
			Audio: &AudioConstraints{
				SampleRate:       48000,
				ChannelCount:     2,
				EchoCancellation: true,
				NoiseSuppression: true,
				AutoGainControl:  true,
				Bitrate:          64000,
			},
		}
		_ = c
	}
}

func BenchmarkVideoConstraintsWithSVC(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := VideoConstraints{
			Width:     1920,
			Height:    1080,
			FrameRate: 30,
			Codec:     codec.VP9,
			Bitrate:   4_000_000,
			SVC:       codec.SVCPresetSFU(),
		}
		_ = c
	}
}
