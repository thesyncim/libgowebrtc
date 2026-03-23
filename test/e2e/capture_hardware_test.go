//go:build hardware

package e2e

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
)

const (
	captureDuration      = 250 * time.Millisecond
	shortCaptureDuration = 200 * time.Millisecond
)

// TestEnumerateDevices tests device enumeration with the real shim library.
func TestEnumerateDevices(t *testing.T) {
	devices, err := ffi.EnumerateDevices()
	if err != nil {
		t.Fatalf("EnumerateDevices failed: %v", err)
	}

	t.Logf("Found %d devices:", len(devices))
	for i, d := range devices {
		t.Logf("  [%d] %s: %s (ID: %s)", i, d.Kind, d.Label, d.DeviceID)
	}

	if len(devices) == 0 {
		t.Log("No devices found (expected in headless CI environment)")
	}
}

// TestEnumerateScreens tests screen enumeration with the real shim library.
func TestEnumerateScreens(t *testing.T) {
	screens, err := ffi.EnumerateScreens()
	if err != nil {
		t.Fatalf("EnumerateScreens failed: %v", err)
	}

	t.Logf("Found %d screens/windows:", len(screens))
	for i, s := range screens {
		windowType := "screen"
		if s.IsWindow {
			windowType = "window"
		}
		t.Logf("  [%d] %s: %s (ID: %d)", i, windowType, s.Title, s.ID)
	}

	if len(screens) == 0 {
		t.Log("No screens found (expected in headless CI environment)")
	}
}

// TestScreenCapture tests actual screen capture functionality.
func TestScreenCapture(t *testing.T) {
	screens, err := ffi.EnumerateScreens()
	if err != nil {
		t.Fatalf("EnumerateScreens failed: %v", err)
	}

	if len(screens) == 0 {
		t.Skip("no screens available for capture")
	}

	screen := screens[0]
	t.Logf("Capturing from screen: %s (ID: %d)", screen.Title, screen.ID)

	capture, err := ffi.NewScreenCapture(screen.ID, screen.IsWindow, 10)
	if err != nil {
		t.Fatalf("NewScreenCapture failed: %v", err)
	}
	defer capture.Close()

	var frameCount atomic.Int32
	var lastWidth, lastHeight int

	err = capture.Start(func(frame *ffi.CapturedVideoFrame) {
		frameCount.Add(1)
		lastWidth = int(frame.Width)
		lastHeight = int(frame.Height)
	})
	if err != nil {
		t.Skipf("Start failed (likely missing screen capture permission): %v", err)
	}

	time.Sleep(captureDuration)
	capture.Stop()

	count := frameCount.Load()
	t.Logf("Captured %d frames, last frame: %dx%d", count, lastWidth, lastHeight)

	if count == 0 {
		t.Skip("No frames captured (likely missing screen capture permission)")
	} else if count < 2 {
		t.Logf("Warning: fewer frames than expected (got %d, expected ~2)", count)
	}
}

// TestVideoCaptureWithDevice tests video capture from a camera device.
func TestVideoCaptureWithDevice(t *testing.T) {
	devices, err := ffi.EnumerateDevices()
	if err != nil {
		t.Fatalf("EnumerateDevices failed: %v", err)
	}

	var videoDevice *ffi.DeviceInfo
	for i := range devices {
		if devices[i].Kind == ffi.DeviceKindVideoInput {
			videoDevice = &devices[i]
			break
		}
	}

	if videoDevice == nil {
		t.Skip("no video input device available")
	}

	t.Logf("Using video device: %s (ID: %s)", videoDevice.Label, videoDevice.DeviceID)

	capture, err := ffi.NewVideoCapture(videoDevice.DeviceID, 640, 480, 15)
	if err != nil {
		t.Fatalf("NewVideoCapture failed: %v", err)
	}
	defer capture.Close()

	var frameCount atomic.Int32
	var lastWidth, lastHeight int

	err = capture.Start(func(frame *ffi.CapturedVideoFrame) {
		frameCount.Add(1)
		lastWidth = int(frame.Width)
		lastHeight = int(frame.Height)
	})
	if err != nil {
		t.Skipf("Start failed (likely missing camera permission): %v", err)
	}

	time.Sleep(captureDuration)
	capture.Stop()

	count := frameCount.Load()
	t.Logf("Captured %d frames from camera, last frame: %dx%d", count, lastWidth, lastHeight)

	if count == 0 {
		t.Skip("No frames captured from camera (likely missing permission)")
	}
}

// TestAudioCaptureWithDevice tests audio capture from a microphone device.
func TestAudioCaptureWithDevice(t *testing.T) {
	devices, err := ffi.EnumerateDevices()
	if err != nil {
		t.Fatalf("EnumerateDevices failed: %v", err)
	}

	var audioDevice *ffi.DeviceInfo
	for i := range devices {
		if devices[i].Kind == ffi.DeviceKindAudioInput {
			audioDevice = &devices[i]
			break
		}
	}

	if audioDevice == nil {
		t.Skip("no audio input device available")
	}

	t.Logf("Using audio device: %s (ID: %s)", audioDevice.Label, audioDevice.DeviceID)

	capture, err := ffi.NewAudioCapture(audioDevice.DeviceID, 48000, 2)
	if err != nil {
		t.Fatalf("NewAudioCapture failed: %v", err)
	}
	defer capture.Close()

	var frameCount atomic.Int32
	var totalSamples int

	err = capture.Start(func(frame *ffi.CapturedAudioFrame) {
		frameCount.Add(1)
		totalSamples += len(frame.Samples)
	})
	if err != nil {
		t.Skipf("Start failed (likely missing microphone permission): %v", err)
	}

	time.Sleep(shortCaptureDuration)
	capture.Stop()

	count := frameCount.Load()
	t.Logf("Captured %d audio frames, total samples: %d", count, totalSamples)

	if count == 0 {
		t.Skip("No audio frames captured (likely missing permission)")
	}
}

// TestDefaultVideoCapture tests video capture with the default device.
func TestDefaultVideoCapture(t *testing.T) {
	capture, err := ffi.NewVideoCapture("", 320, 240, 10)
	if err != nil {
		t.Logf("NewVideoCapture with default device failed (expected if no camera): %v", err)
		return
	}
	defer capture.Close()

	var frameCount atomic.Int32

	err = capture.Start(func(frame *ffi.CapturedVideoFrame) {
		frameCount.Add(1)
	})
	if err != nil {
		t.Logf("Start failed (expected if no camera permission): %v", err)
		return
	}

	time.Sleep(shortCaptureDuration)
	capture.Stop()

	t.Logf("Captured %d frames from default camera", frameCount.Load())
}
