package media

import (
	"errors"
	"testing"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

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

func TestGetUserMediaWithConstraints(t *testing.T) {
	stream, err := GetUserMedia(Constraints{
		Video: &VideoConstraints{
			Width:     640,
			Height:    480,
			FrameRate: 30,
			Codec:     codec.VP8,
		},
	})

	if err != nil {
		t.Fatalf("GetUserMedia() error = %v", err)
	}
	if stream == nil {
		t.Fatal("GetUserMedia() stream is nil")
	}

	videoTracks := stream.GetVideoTracks()
	if len(videoTracks) != 1 {
		t.Errorf("GetVideoTracks() len = %d, want 1", len(videoTracks))
	}
}

func TestGetUserMediaAudioOnly(t *testing.T) {
	stream, err := GetUserMedia(Constraints{
		Audio: &AudioConstraints{
			SampleRate:   48000,
			ChannelCount: 2,
		},
	})

	if err != nil {
		t.Fatalf("GetUserMedia() error = %v", err)
	}
	if stream == nil {
		t.Fatal("GetUserMedia() stream is nil")
	}

	audioTracks := stream.GetAudioTracks()
	if len(audioTracks) != 1 {
		t.Errorf("GetAudioTracks() len = %d, want 1", len(audioTracks))
	}
}

func TestMediaStreamTracks(t *testing.T) {
	stream, err := GetUserMedia(Constraints{
		Video: &VideoConstraints{Width: 640, Height: 480},
		Audio: &AudioConstraints{SampleRate: 48000},
	})
	if err != nil {
		t.Fatalf("GetUserMedia() error = %v", err)
	}

	// Test GetTracks returns both
	tracks := stream.GetTracks()
	if len(tracks) != 2 {
		t.Errorf("GetTracks() len = %d, want 2", len(tracks))
	}

	// Test ID is set
	if stream.ID() == "" {
		t.Error("Stream ID should not be empty")
	}
}
