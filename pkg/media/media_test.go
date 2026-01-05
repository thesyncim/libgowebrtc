package media

import (
	"errors"
	"testing"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

func TestMediaErrors(t *testing.T) {
	errs := []error{
		ErrInvalidConstraints,
		ErrTrackNotFound,
		ErrStreamClosed,
	}

	for _, err := range errs {
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

// --- Device Enumeration Tests ---

func TestMediaDeviceKindString(t *testing.T) {
	tests := []struct {
		kind MediaDeviceKind
		want string
	}{
		{MediaDeviceKindVideoInput, "videoinput"},
		{MediaDeviceKindAudioInput, "audioinput"},
		{MediaDeviceKindAudioOutput, "audiooutput"},
	}

	for _, tt := range tests {
		if string(tt.kind) != tt.want {
			t.Errorf("MediaDeviceKind = %q, want %q", tt.kind, tt.want)
		}
	}
}

func TestMediaDeviceInfo(t *testing.T) {
	info := MediaDeviceInfo{
		DeviceID: "device-123",
		Kind:     MediaDeviceKindVideoInput,
		Label:    "FaceTime HD Camera",
		GroupID:  "group-1",
	}

	if info.DeviceID != "device-123" {
		t.Errorf("DeviceID = %q, want %q", info.DeviceID, "device-123")
	}
	if info.Kind != MediaDeviceKindVideoInput {
		t.Errorf("Kind = %v, want %v", info.Kind, MediaDeviceKindVideoInput)
	}
	if info.Label != "FaceTime HD Camera" {
		t.Errorf("Label = %q, want %q", info.Label, "FaceTime HD Camera")
	}
}

func TestEnumerateDevicesWithoutLibrary(t *testing.T) {
	// Without the shim library loaded, should return empty list (browser-like behavior)
	devices, err := EnumerateDevices()
	if err != nil {
		t.Errorf("EnumerateDevices() error = %v, want nil", err)
	}
	// Empty list is expected when library not loaded
	if devices == nil {
		t.Error("EnumerateDevices() should return empty slice, not nil")
	}
}

func TestEnumerateScreensWithoutLibrary(t *testing.T) {
	// Without the shim library loaded, should return empty list
	screens, err := EnumerateScreens()
	if err != nil {
		t.Errorf("EnumerateScreens() error = %v, want nil", err)
	}
	if screens == nil {
		t.Error("EnumerateScreens() should return empty slice, not nil")
	}
}

func TestScreenInfo(t *testing.T) {
	screen := ScreenInfo{
		ID:       12345,
		Title:    "Display 1",
		IsWindow: false,
	}

	if screen.ID != 12345 {
		t.Errorf("ID = %d, want 12345", screen.ID)
	}
	if screen.Title != "Display 1" {
		t.Errorf("Title = %q, want %q", screen.Title, "Display 1")
	}
	if screen.IsWindow {
		t.Error("IsWindow should be false for screen")
	}

	window := ScreenInfo{
		ID:       67890,
		Title:    "My App",
		IsWindow: true,
	}

	if !window.IsWindow {
		t.Error("IsWindow should be true for window")
	}
}

func TestDisplayConstraints(t *testing.T) {
	c := DisplayConstraints{
		Video: &DisplayVideoConstraints{
			ScreenID:  0,
			FrameRate: 30,
			Width:     1920,
			Height:    1080,
			Codec:     codec.VP9,
			Bitrate:   3_000_000,
			SVC:       codec.SVCPresetScreenShare(),
		},
		Audio: nil, // Screen share typically no audio
	}

	if c.Video == nil {
		t.Fatal("Video constraints should not be nil")
	}
	if c.Video.FrameRate != 30 {
		t.Errorf("FrameRate = %v, want 30", c.Video.FrameRate)
	}
	if c.Video.Codec != codec.VP9 {
		t.Errorf("Codec = %v, want VP9", c.Video.Codec)
	}
	if c.Audio != nil {
		t.Error("Audio should be nil for screen share")
	}
}

func TestDisplayVideoConstraintsWindow(t *testing.T) {
	c := DisplayVideoConstraints{
		WindowID:  12345,
		FrameRate: 60,
	}

	if c.WindowID != 12345 {
		t.Errorf("WindowID = %d, want 12345", c.WindowID)
	}
	if c.ScreenID != 0 {
		t.Errorf("ScreenID = %d, want 0", c.ScreenID)
	}
	if c.FrameRate != 60 {
		t.Errorf("FrameRate = %v, want 60", c.FrameRate)
	}
}

func TestGetDisplayMediaWithDisplayConstraints(t *testing.T) {
	stream, err := GetDisplayMedia(DisplayConstraints{
		Video: &DisplayVideoConstraints{
			ScreenID:  0,
			Width:     1920,
			Height:    1080,
			FrameRate: 30,
		},
	})

	if err != nil {
		t.Fatalf("GetDisplayMedia() error = %v", err)
	}
	if stream == nil {
		t.Fatal("GetDisplayMedia() stream is nil")
	}

	videoTracks := stream.GetVideoTracks()
	if len(videoTracks) != 1 {
		t.Errorf("GetVideoTracks() len = %d, want 1", len(videoTracks))
	}

	if len(videoTracks) > 0 {
		track := videoTracks[0]
		if track.Kind() != "video" {
			t.Errorf("Kind() = %q, want %q", track.Kind(), "video")
		}
		if track.Label() != "screen-capture" {
			t.Errorf("Label() = %q, want %q", track.Label(), "screen-capture")
		}
	}
}

func TestGetDisplayMediaWithWindowID(t *testing.T) {
	stream, err := GetDisplayMedia(DisplayConstraints{
		Video: &DisplayVideoConstraints{
			WindowID: 12345,
		},
	})

	if err != nil {
		t.Fatalf("GetDisplayMedia() error = %v", err)
	}

	videoTracks := stream.GetVideoTracks()
	if len(videoTracks) > 0 {
		track := videoTracks[0]
		if track.Label() != "window-capture" {
			t.Errorf("Label() = %q, want %q", track.Label(), "window-capture")
		}
	}
}

func TestGetDisplayMediaLegacyConstraints(t *testing.T) {
	// Test that legacy Constraints still work
	stream, err := GetDisplayMedia(Constraints{
		Video: &VideoConstraints{
			Width:  1920,
			Height: 1080,
		},
	})

	if err != nil {
		t.Fatalf("GetDisplayMedia() error = %v", err)
	}
	if stream == nil {
		t.Fatal("GetDisplayMedia() stream is nil")
	}
}

func TestGetDisplayMediaInvalidConstraints(t *testing.T) {
	_, err := GetDisplayMedia("invalid")

	if !errors.Is(err, ErrInvalidConstraints) {
		t.Errorf("GetDisplayMedia() error = %v, want %v", err, ErrInvalidConstraints)
	}
}
