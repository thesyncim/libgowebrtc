package media

import (
	"errors"
	"testing"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

type stubVideoCapture struct{}

func (stubVideoCapture) Start(ffi.VideoCaptureCallback) error { return nil }
func (stubVideoCapture) Close()                               {}

type stubAudioCapture struct{}

func (stubAudioCapture) Start(ffi.AudioCaptureCallback) error { return nil }
func (stubAudioCapture) Close()                               {}

type stubScreenCapture struct{}

func (stubScreenCapture) Start(ffi.VideoCaptureCallback) error { return nil }
func (stubScreenCapture) Close()                               {}

type mediaFFIStubs struct {
	loadLibrary     func() error
	enumerateDevice func() ([]ffi.DeviceInfo, error)
	enumerateScreen func() ([]ffi.ScreenInfo, error)
	newVideo        func(deviceID string, width, height, fps int) (videoCaptureHandle, error)
	newAudio        func(deviceID string, sampleRate, channels int) (audioCaptureHandle, error)
	newScreen       func(id int64, isWindow bool, fps int) (screenCaptureHandle, error)
}

func installMediaFFIStubs(t *testing.T, stubs mediaFFIStubs) {
	t.Helper()

	origLoadLibrary := loadCaptureLibrary
	origEnumerateDevices := enumerateDevices
	origEnumerateScreens := enumerateScreens
	origNewVideo := newVideoCapture
	origNewAudio := newAudioCapture
	origNewScreen := newScreenCapture

	if stubs.loadLibrary != nil {
		loadCaptureLibrary = stubs.loadLibrary
	}
	if stubs.enumerateDevice != nil {
		enumerateDevices = stubs.enumerateDevice
	}
	if stubs.enumerateScreen != nil {
		enumerateScreens = stubs.enumerateScreen
	}
	if stubs.newVideo != nil {
		newVideoCapture = stubs.newVideo
	}
	if stubs.newAudio != nil {
		newAudioCapture = stubs.newAudio
	}
	if stubs.newScreen != nil {
		newScreenCapture = stubs.newScreen
	}

	t.Cleanup(func() {
		loadCaptureLibrary = origLoadLibrary
		enumerateDevices = origEnumerateDevices
		enumerateScreens = origEnumerateScreens
		newVideoCapture = origNewVideo
		newAudioCapture = origNewAudio
		newScreenCapture = origNewScreen
	})
}

func TestEnumerateDevicesWithoutLibrary(t *testing.T) {
	_ = ffi.Close()

	devices, err := EnumerateDevices()
	if err != nil {
		t.Fatalf("EnumerateDevices() error = %v, want nil", err)
	}
	if devices == nil {
		t.Fatal("EnumerateDevices() should return an empty slice, not nil")
	}
}

func TestEnumerateScreensWithoutLibrary(t *testing.T) {
	_ = ffi.Close()

	screens, err := EnumerateScreens()
	if err != nil {
		t.Fatalf("EnumerateScreens() error = %v, want nil", err)
	}
	if screens == nil {
		t.Fatal("EnumerateScreens() should return an empty slice, not nil")
	}
}

func TestGetUserMediaRejectsEmptyConstraints(t *testing.T) {
	stream, err := GetUserMedia(Constraints{})
	if !errors.Is(err, ErrInvalidConstraints) {
		t.Fatalf("GetUserMedia() error = %v, want %v", err, ErrInvalidConstraints)
	}
	if stream != nil {
		t.Fatal("GetUserMedia() stream = non-nil, want nil")
	}
}

func TestGetDisplayMediaRejectsMissingVideo(t *testing.T) {
	stream, err := GetDisplayMedia(DisplayConstraints{})
	if !errors.Is(err, ErrInvalidConstraints) {
		t.Fatalf("GetDisplayMedia() error = %v, want %v", err, ErrInvalidConstraints)
	}
	if stream != nil {
		t.Fatal("GetDisplayMedia() stream = non-nil, want nil")
	}
}

func TestGetUserMediaReturnsCaptureNotSupportedWhenLoadFails(t *testing.T) {
	installMediaFFIStubs(t, mediaFFIStubs{
		loadLibrary: func() error { return ffi.ErrLibraryNotLoaded },
	})

	stream, err := GetUserMedia(Constraints{
		Video: &VideoConstraints{Width: ExactInt(640)},
	})
	if !errors.Is(err, ErrCaptureNotSupported) {
		t.Fatalf("GetUserMedia() error = %v, want wrapped %v", err, ErrCaptureNotSupported)
	}
	if stream != nil {
		t.Fatal("GetUserMedia() stream = non-nil, want nil")
	}
}

func TestGetUserMediaRejectsEmptyExactDeviceID(t *testing.T) {
	stream, err := GetUserMedia(Constraints{
		Video: &VideoConstraints{
			DeviceID: ExactString(""),
		},
	})
	if !errors.Is(err, ErrInvalidConstraints) {
		t.Fatalf("GetUserMedia() error = %v, want %v", err, ErrInvalidConstraints)
	}
	if stream != nil {
		t.Fatal("GetUserMedia() stream = non-nil, want nil")
	}
}

func TestGetUserMediaResolvesDevicesAndSettings(t *testing.T) {
	installMediaFFIStubs(t, mediaFFIStubs{
		loadLibrary: func() error { return nil },
		enumerateDevice: func() ([]ffi.DeviceInfo, error) {
			return []ffi.DeviceInfo{
				{DeviceID: "cam-1", Label: "Front Camera", Kind: ffi.DeviceKindVideoInput},
				{DeviceID: "cam-2", Label: "Rear Camera", Kind: ffi.DeviceKindVideoInput},
				{DeviceID: "mic-1", Label: "Studio Mic", Kind: ffi.DeviceKindAudioInput},
			}, nil
		},
		newVideo: func(string, int, int, int) (videoCaptureHandle, error) { return stubVideoCapture{}, nil },
		newAudio: func(string, int, int) (audioCaptureHandle, error) { return stubAudioCapture{}, nil },
	})

	stream, err := GetUserMedia(Constraints{
		Video: &VideoConstraints{
			Width:      IdealInt(640),
			Height:     ExactInt(480),
			FrameRate:  IdealFloat(24),
			DeviceID:   IdealString("cam-2"),
			FacingMode: FacingModeEnvironment,
			Codec:      codec.VP8,
		},
		Audio: &AudioConstraints{
			SampleRate:       ExactInt(48_000),
			ChannelCount:     ExactInt(2),
			DeviceID:         ExactString("mic-1"),
			EchoCancellation: IdealBool(true),
		},
	})
	if err != nil {
		t.Fatalf("GetUserMedia() error = %v", err)
	}

	videoTracks := stream.GetVideoTracks()
	audioTracks := stream.GetAudioTracks()
	if len(videoTracks) != 1 {
		t.Fatalf("GetVideoTracks() len = %d, want 1", len(videoTracks))
	}
	if len(audioTracks) != 1 {
		t.Fatalf("GetAudioTracks() len = %d, want 1", len(audioTracks))
	}

	video := videoTracks[0].(VideoStreamTrack)
	audio := audioTracks[0].(AudioStreamTrack)

	if got := video.Label(); got != "Rear Camera" {
		t.Fatalf("video Label() = %q, want %q", got, "Rear Camera")
	}
	if got := video.GetSettings().DeviceID; got != "cam-2" {
		t.Fatalf("video DeviceID = %q, want %q", got, "cam-2")
	}
	if got := video.GetSettings().FacingMode; got != FacingModeEnvironment {
		t.Fatalf("video FacingMode = %q, want %q", got, FacingModeEnvironment)
	}
	if got := video.GetSettings().FrameRate; got != 24 {
		t.Fatalf("video FrameRate = %.0f, want 24", got)
	}
	if got := video.GetCapabilities().DeviceID; len(got) != 1 || got[0] != "cam-2" {
		t.Fatalf("video DeviceID capability = %v, want [cam-2]", got)
	}
	if got := video.GetCapabilities().FacingMode; len(got) != 1 || got[0] != FacingModeEnvironment {
		t.Fatalf("video FacingMode capability = %v, want [%q]", got, FacingModeEnvironment)
	}

	if got := audio.Label(); got != "Studio Mic" {
		t.Fatalf("audio Label() = %q, want %q", got, "Studio Mic")
	}
	if got := audio.GetSettings().DeviceID; got != "mic-1" {
		t.Fatalf("audio DeviceID = %q, want %q", got, "mic-1")
	}
	if !audio.GetSettings().EchoCancellation {
		t.Fatal("audio EchoCancellation = false, want true")
	}
	if got := audio.GetCapabilities().DeviceID; len(got) != 1 || got[0] != "mic-1" {
		t.Fatalf("audio DeviceID capability = %v, want [mic-1]", got)
	}
}

func TestGetUserMediaMissingExactDeviceReturnsOverconstrained(t *testing.T) {
	installMediaFFIStubs(t, mediaFFIStubs{
		loadLibrary: func() error { return nil },
		enumerateDevice: func() ([]ffi.DeviceInfo, error) {
			return []ffi.DeviceInfo{
				{DeviceID: "cam-1", Label: "Front Camera", Kind: ffi.DeviceKindVideoInput},
			}, nil
		},
	})

	stream, err := GetUserMedia(Constraints{
		Video: &VideoConstraints{
			DeviceID: ExactString("cam-404"),
		},
	})
	if stream != nil {
		t.Fatal("GetUserMedia() stream = non-nil, want nil")
	}

	var overconstrained *OverconstrainedError
	if !errors.As(err, &overconstrained) {
		t.Fatalf("GetUserMedia() error = %v, want OverconstrainedError", err)
	}
	if overconstrained.Constraint != "deviceId" {
		t.Fatalf("Constraint = %q, want %q", overconstrained.Constraint, "deviceId")
	}
}

func TestGetDisplayMediaResolvesRequestedWindowAndOptionalAudio(t *testing.T) {
	installMediaFFIStubs(t, mediaFFIStubs{
		loadLibrary: func() error { return nil },
		enumerateScreen: func() ([]ffi.ScreenInfo, error) {
			return []ffi.ScreenInfo{
				{ID: 1, Title: "Main Display", IsWindow: false},
				{ID: 7, Title: "Slides", IsWindow: true},
			}, nil
		},
		enumerateDevice: func() ([]ffi.DeviceInfo, error) {
			return []ffi.DeviceInfo{
				{DeviceID: "mic-1", Label: "Screen Share Mic", Kind: ffi.DeviceKindAudioInput},
			}, nil
		},
		newScreen: func(int64, bool, int) (screenCaptureHandle, error) { return stubScreenCapture{}, nil },
		newAudio:  func(string, int, int) (audioCaptureHandle, error) { return stubAudioCapture{}, nil },
	})

	stream, err := GetDisplayMedia(DisplayConstraints{
		Video: &DisplayVideoConstraints{
			WindowID:  7,
			Width:     IdealInt(1920),
			Height:    IdealInt(1080),
			FrameRate: IdealFloat(30),
		},
		Audio: &AudioConstraints{
			DeviceID: ExactString("mic-1"),
		},
	})
	if err != nil {
		t.Fatalf("GetDisplayMedia() error = %v", err)
	}

	videoTracks := stream.GetVideoTracks()
	audioTracks := stream.GetAudioTracks()
	if len(videoTracks) != 1 {
		t.Fatalf("GetVideoTracks() len = %d, want 1", len(videoTracks))
	}
	if len(audioTracks) != 1 {
		t.Fatalf("GetAudioTracks() len = %d, want 1", len(audioTracks))
	}

	video := videoTracks[0].(*videoStreamTrack)
	if got := video.Label(); got != "window-capture" {
		t.Fatalf("video Label() = %q, want %q", got, "window-capture")
	}
	if video.displayConstraints == nil || video.displayConstraints.DisplaySurface != DisplaySurfaceWindow {
		t.Fatalf("display surface = %+v, want %q", video.displayConstraints, DisplaySurfaceWindow)
	}
	if got := video.GetCapabilities().DisplaySurface; len(got) != 1 || got[0] != DisplaySurfaceWindow {
		t.Fatalf("display surface capability = %v, want [%q]", got, DisplaySurfaceWindow)
	}
	if got := audioTracks[0].(AudioStreamTrack).GetSettings().DeviceID; got != "mic-1" {
		t.Fatalf("audio DeviceID = %q, want %q", got, "mic-1")
	}
}

func TestGetDisplayMediaRejectsConflictingTargets(t *testing.T) {
	installMediaFFIStubs(t, mediaFFIStubs{
		loadLibrary: func() error {
			t.Fatal("GetDisplayMedia should validate conflicting targets before loading the capture backend")
			return nil
		},
		enumerateScreen: func() ([]ffi.ScreenInfo, error) {
			t.Fatal("GetDisplayMedia should validate conflicting targets before enumerating screens")
			return nil, nil
		},
	})

	stream, err := GetDisplayMedia(DisplayConstraints{
		Video: &DisplayVideoConstraints{
			ScreenID: 1,
			WindowID: 2,
		},
	})
	if !errors.Is(err, ErrInvalidConstraints) {
		t.Fatalf("GetDisplayMedia() error = %v, want %v", err, ErrInvalidConstraints)
	}
	if stream != nil {
		t.Fatal("GetDisplayMedia() stream = non-nil, want nil")
	}
}

func TestNewVideoStreamTrackCodecPreferencesOverrideCodec(t *testing.T) {
	video, err := newVideoStreamTrack(
		VideoConstraints{
			Codec: codec.H264,
			CodecPreferences: []webrtc.RTPCodecParameters{{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypeVP8,
					ClockRate: 90000,
				},
				PayloadType: 96,
			}},
		},
		VideoTrackSettings{
			Width:     640,
			Height:    480,
			FrameRate: 30,
		},
		"camera",
	)
	if err != nil {
		t.Fatalf("newVideoStreamTrack: %v", err)
	}

	constraints := video.GetConstraints()
	if constraints.Codec != codec.VP8 {
		t.Fatalf("constraints.Codec = %v, want %v", constraints.Codec, codec.VP8)
	}
	if len(constraints.CodecPreferences) != 1 || constraints.CodecPreferences[0].MimeType != webrtc.MimeTypeVP8 {
		t.Fatalf("constraints.CodecPreferences = %+v, want VP8 preference", constraints.CodecPreferences)
	}
}

func TestNewAudioStreamTrackPreservesCodecPreferences(t *testing.T) {
	audio, err := newAudioStreamTrack(
		AudioConstraints{
			CodecPreferences: []webrtc.RTPCodecParameters{{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypeOpus,
					ClockRate: 48000,
					Channels:  2,
				},
				PayloadType: 111,
			}},
		},
		AudioTrackSettings{
			SampleRate:   48000,
			ChannelCount: 2,
		},
		"microphone",
	)
	if err != nil {
		t.Fatalf("newAudioStreamTrack: %v", err)
	}

	constraints := audio.GetConstraints()
	if len(constraints.CodecPreferences) != 1 || constraints.CodecPreferences[0].MimeType != webrtc.MimeTypeOpus {
		t.Fatalf("constraints.CodecPreferences = %+v, want Opus preference", constraints.CodecPreferences)
	}
}
