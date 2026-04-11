package media

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/track"
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
	if !errors.Is(err, ErrCaptureNotSupported) {
		t.Fatalf("EnumerateDevices() error = %v, want wrapped %v", err, ErrCaptureNotSupported)
	}
	if devices != nil {
		t.Fatal("EnumerateDevices() should return nil when capture is unavailable")
	}
}

func TestEnumerateScreensWithoutLibrary(t *testing.T) {
	_ = ffi.Close()

	screens, err := EnumerateScreens()
	if !errors.Is(err, ErrCaptureNotSupported) {
		t.Fatalf("EnumerateScreens() error = %v, want wrapped %v", err, ErrCaptureNotSupported)
	}
	if screens != nil {
		t.Fatal("EnumerateScreens() should return nil when capture is unavailable")
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
		Video: &VideoConstraints{
			Width:     ExactInt(640),
			Height:    ExactInt(480),
			FrameRate: ExactFloat(30),
			Codec:     codec.VP8,
			Bitrate:   500_000,
		},
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
			Bitrate:    500_000,
		},
		Audio: &AudioConstraints{
			SampleRate:       ExactInt(48_000),
			ChannelCount:     ExactInt(2),
			DeviceID:         ExactString("mic-1"),
			EchoCancellation: IdealBool(true),
			Bitrate:          64_000,
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

	video := videoTracks[0]
	audio := audioTracks[0]

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
	if got := audio.Label(); got != "Studio Mic" {
		t.Fatalf("audio Label() = %q, want %q", got, "Studio Mic")
	}
	if got := audio.GetSettings().DeviceID; got != "mic-1" {
		t.Fatalf("audio DeviceID = %q, want %q", got, "mic-1")
	}
	if !audio.GetSettings().EchoCancellation {
		t.Fatal("audio EchoCancellation = false, want true")
	}
}

func TestGetUserMediaRejectsMissingVideoBitrate(t *testing.T) {
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
			Width:     ExactInt(640),
			Height:    ExactInt(480),
			FrameRate: ExactFloat(30),
			Codec:     codec.VP8,
		},
	})
	if !errors.Is(err, ErrInvalidConstraints) {
		t.Fatalf("GetUserMedia() error = %v, want %v", err, ErrInvalidConstraints)
	}
	if stream != nil {
		t.Fatal("GetUserMedia() stream = non-nil, want nil")
	}
}

func TestGetUserMediaRejectsMissingVideoDimensionsOrFramerate(t *testing.T) {
	installMediaFFIStubs(t, mediaFFIStubs{
		loadLibrary: func() error { return nil },
		enumerateDevice: func() ([]ffi.DeviceInfo, error) {
			return []ffi.DeviceInfo{
				{DeviceID: "cam-1", Label: "Front Camera", Kind: ffi.DeviceKindVideoInput},
			}, nil
		},
	})

	cases := []struct {
		name  string
		video VideoConstraints
	}{
		{
			name: "missing width",
			video: VideoConstraints{
				Height:    ExactInt(480),
				FrameRate: ExactFloat(30),
				Codec:     codec.VP8,
				Bitrate:   500_000,
			},
		},
		{
			name: "missing height",
			video: VideoConstraints{
				Width:     ExactInt(640),
				FrameRate: ExactFloat(30),
				Codec:     codec.VP8,
				Bitrate:   500_000,
			},
		},
		{
			name: "missing frame rate",
			video: VideoConstraints{
				Width:   ExactInt(640),
				Height:  ExactInt(480),
				Codec:   codec.VP8,
				Bitrate: 500_000,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stream, err := GetUserMedia(Constraints{Video: &tc.video})
			if !errors.Is(err, ErrInvalidConstraints) {
				t.Fatalf("GetUserMedia() error = %v, want %v", err, ErrInvalidConstraints)
			}
			if stream != nil {
				t.Fatal("GetUserMedia() stream = non-nil, want nil")
			}
		})
	}
}

func TestGetUserMediaRejectsMissingVideoCodec(t *testing.T) {
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
			Width:     ExactInt(640),
			Height:    ExactInt(480),
			FrameRate: ExactFloat(30),
			Bitrate:   500_000,
		},
	})
	if !errors.Is(err, ErrInvalidConstraints) {
		t.Fatalf("GetUserMedia() error = %v, want %v", err, ErrInvalidConstraints)
	}
	if stream != nil {
		t.Fatal("GetUserMedia() stream = non-nil, want nil")
	}
}

func TestGetUserMediaRejectsMissingAudioBitrate(t *testing.T) {
	installMediaFFIStubs(t, mediaFFIStubs{
		loadLibrary: func() error { return nil },
		enumerateDevice: func() ([]ffi.DeviceInfo, error) {
			return []ffi.DeviceInfo{
				{DeviceID: "mic-1", Label: "Built-in Mic", Kind: ffi.DeviceKindAudioInput},
			}, nil
		},
	})

	stream, err := GetUserMedia(Constraints{
		Audio: &AudioConstraints{
			SampleRate:   ExactInt(48_000),
			ChannelCount: ExactInt(2),
		},
	})
	if !errors.Is(err, ErrInvalidConstraints) {
		t.Fatalf("GetUserMedia() error = %v, want %v", err, ErrInvalidConstraints)
	}
	if stream != nil {
		t.Fatal("GetUserMedia() stream = non-nil, want nil")
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
			DeviceID:  ExactString("cam-404"),
			Width:     ExactInt(640),
			Height:    ExactInt(480),
			FrameRate: ExactFloat(30),
			Codec:     codec.VP8,
			Bitrate:   500_000,
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
			Codec:     codec.VP8,
			Bitrate:   3_000_000,
		},
		Audio: &AudioConstraints{
			DeviceID: ExactString("mic-1"),
			Bitrate:  64_000,
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
	if got := audioTracks[0].GetSettings().DeviceID; got != "mic-1" {
		t.Fatalf("audio DeviceID = %q, want %q", got, "mic-1")
	}
}

func TestGetDisplayMediaRejectsMissingVideoCodec(t *testing.T) {
	installMediaFFIStubs(t, mediaFFIStubs{
		loadLibrary: func() error { return nil },
		enumerateScreen: func() ([]ffi.ScreenInfo, error) {
			return []ffi.ScreenInfo{
				{ID: 1, Title: "Main Display", IsWindow: false},
			}, nil
		},
	})

	stream, err := GetDisplayMedia(DisplayConstraints{
		Video: &DisplayVideoConstraints{
			ScreenID:  1,
			Width:     ExactInt(1920),
			Height:    ExactInt(1080),
			FrameRate: ExactFloat(30),
			Bitrate:   3_000_000,
		},
	})
	if !errors.Is(err, ErrInvalidConstraints) {
		t.Fatalf("GetDisplayMedia() error = %v, want %v", err, ErrInvalidConstraints)
	}
	if stream != nil {
		t.Fatal("GetDisplayMedia() stream = non-nil, want nil")
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

func TestBuildVideoTrackConfigPrefersCodecPreferences(t *testing.T) {
	cfg, resolved := buildVideoTrackConfig(
		VideoConstraints{
			Codec:   codec.H264,
			Bitrate: 500_000,
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
	)

	if cfg.Codec != codec.VP8 {
		t.Fatalf("cfg.Codec = %v, want %v", cfg.Codec, codec.VP8)
	}
	if len(cfg.CodecPreferences) != 1 || cfg.CodecPreferences[0].MimeType != webrtc.MimeTypeVP8 {
		t.Fatalf("cfg.CodecPreferences = %+v, want VP8 preference", cfg.CodecPreferences)
	}
	if cfg.Bitrate != 500_000 {
		t.Fatalf("cfg.Bitrate = %d, want %d", cfg.Bitrate, 500_000)
	}
	if resolved.Codec != codec.VP8 {
		t.Fatalf("resolved.Codec = %v, want %v", resolved.Codec, codec.VP8)
	}
	if resolved.Bitrate != 500_000 {
		t.Fatalf("resolved.Bitrate = %d, want %d", resolved.Bitrate, 500_000)
	}
	if got := resolved.Width; !got.IsExact() || got.Exact == nil || *got.Exact != 640 {
		t.Fatalf("resolved.Width = %+v, want exact 640", got)
	}
	if got := resolved.Height; !got.IsExact() || got.Exact == nil || *got.Exact != 480 {
		t.Fatalf("resolved.Height = %+v, want exact 480", got)
	}
	if got := resolved.FrameRate; !got.IsExact() || got.Exact == nil || *got.Exact != 30 {
		t.Fatalf("resolved.FrameRate = %+v, want exact 30", got)
	}
}

func TestNewVideoStreamTrackOptsInToAutoAdaptation(t *testing.T) {
	constraints := VideoConstraints{
		Codec:   codec.H264,
		Bitrate: 500_000,
	}
	settings := VideoTrackSettings{
		Width:     640,
		Height:    480,
		FrameRate: 30,
	}
	cfg, resolved := buildVideoTrackConfig(constraints, settings)
	video, err := newVideoStreamTrack(
		cfg,
		resolved,
		settings,
		"camera",
	)
	if err != nil {
		t.Fatalf("newVideoStreamTrack: %v", err)
	}
	t.Cleanup(func() {
		video.Stop()
	})

	var calls int32
	video.track.SetBWESource(func() *track.BandwidthEstimate {
		atomic.AddInt32(&calls, 1)
		return &track.BandwidthEstimate{TargetBitrateBps: 1_000_000}
	})
	t.Cleanup(func() {
		video.track.SetBWESource(nil)
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for atomic.LoadInt32(&calls) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&calls) == 0 {
		t.Fatal("SetBWESource did not start adaptation loop, want browser-like helper opt-in")
	}
}

func TestBuildAudioTrackConfigPreservesCodecPreferences(t *testing.T) {
	cfg, resolved := buildAudioTrackConfig(
		AudioConstraints{
			Bitrate: 64_000,
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
	)
	if cfg.Bitrate != 64_000 {
		t.Fatalf("cfg.Bitrate = %d, want %d", cfg.Bitrate, 64_000)
	}
	if len(cfg.CodecPreferences) != 1 || cfg.CodecPreferences[0].MimeType != webrtc.MimeTypeOpus {
		t.Fatalf("cfg.CodecPreferences = %+v, want Opus preference", cfg.CodecPreferences)
	}
	if resolved.Bitrate != 64_000 {
		t.Fatalf("resolved.Bitrate = %d, want %d", resolved.Bitrate, 64_000)
	}
	if got := resolved.SampleRate; !got.IsExact() || got.Exact == nil || *got.Exact != 48_000 {
		t.Fatalf("resolved.SampleRate = %+v, want exact 48000", got)
	}
	if got := resolved.ChannelCount; !got.IsExact() || got.Exact == nil || *got.Exact != 2 {
		t.Fatalf("resolved.ChannelCount = %+v, want exact 2", got)
	}
}
