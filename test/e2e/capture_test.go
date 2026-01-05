package e2e

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
)

// TestEnumerateDevices tests device enumeration with the real shim library.
func TestEnumerateDevices(t *testing.T) {
	if !ffi.IsLoaded() {
		t.Skip("shim library not available")
	}

	devices, err := ffi.EnumerateDevices()
	if err != nil {
		t.Fatalf("EnumerateDevices failed: %v", err)
	}

	t.Logf("Found %d devices:", len(devices))
	for i, d := range devices {
		t.Logf("  [%d] %s: %s (ID: %s)", i, d.Kind, d.Label, d.DeviceID)
	}

	// On most systems, at least one device should exist (even virtual)
	// But don't fail if none - CI environments may not have devices
	if len(devices) == 0 {
		t.Log("No devices found (expected in headless CI environment)")
	}
}

// TestEnumerateScreens tests screen enumeration with the real shim library.
func TestEnumerateScreens(t *testing.T) {
	if !ffi.IsLoaded() {
		t.Skip("shim library not available")
	}

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

	// Screen capture usually finds at least one screen
	if len(screens) == 0 {
		t.Log("No screens found (expected in headless CI environment)")
	}
}

// TestScreenCapture tests actual screen capture functionality.
// This test requires a display to be available.
func TestScreenCapture(t *testing.T) {
	if !ffi.IsLoaded() {
		t.Skip("shim library not available")
	}

	screens, err := ffi.EnumerateScreens()
	if err != nil {
		t.Fatalf("EnumerateScreens failed: %v", err)
	}

	if len(screens) == 0 {
		t.Skip("no screens available for capture")
	}

	// Use first screen
	screen := screens[0]
	t.Logf("Capturing from screen: %s (ID: %d)", screen.Title, screen.ID)

	capture, err := ffi.NewScreenCapture(screen.ID, screen.IsWindow, 10) // Low FPS for test
	if err != nil {
		t.Fatalf("NewScreenCapture failed: %v", err)
	}
	defer capture.Close()

	var frameCount atomic.Int32
	var lastWidth, lastHeight int

	err = capture.Start(func(frame *ffi.CapturedVideoFrame) {
		frameCount.Add(1)
		lastWidth = frame.Width
		lastHeight = frame.Height
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Capture for 1 second
	time.Sleep(1 * time.Second)

	capture.Stop()

	count := frameCount.Load()
	t.Logf("Captured %d frames, last frame: %dx%d", count, lastWidth, lastHeight)

	if count == 0 {
		t.Error("No frames captured")
	} else if count < 5 {
		t.Logf("Warning: fewer frames than expected (got %d, expected ~10)", count)
	}
}

// TestVideoCaptureWithDevice tests video capture from a camera device.
// This test requires a camera to be available.
func TestVideoCaptureWithDevice(t *testing.T) {
	if !ffi.IsLoaded() {
		t.Skip("shim library not available")
	}

	devices, err := ffi.EnumerateDevices()
	if err != nil {
		t.Fatalf("EnumerateDevices failed: %v", err)
	}

	// Find a video input device
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
		lastWidth = frame.Width
		lastHeight = frame.Height
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Capture for 1 second
	time.Sleep(1 * time.Second)

	capture.Stop()

	count := frameCount.Load()
	t.Logf("Captured %d frames from camera, last frame: %dx%d", count, lastWidth, lastHeight)

	if count == 0 {
		t.Error("No frames captured from camera")
	}
}

// TestAudioCaptureWithDevice tests audio capture from a microphone device.
// This test requires a microphone to be available.
func TestAudioCaptureWithDevice(t *testing.T) {
	if !ffi.IsLoaded() {
		t.Skip("shim library not available")
	}

	devices, err := ffi.EnumerateDevices()
	if err != nil {
		t.Fatalf("EnumerateDevices failed: %v", err)
	}

	// Find an audio input device
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
		t.Fatalf("Start failed: %v", err)
	}

	// Capture for 500ms
	time.Sleep(500 * time.Millisecond)

	capture.Stop()

	count := frameCount.Load()
	t.Logf("Captured %d audio frames, total samples: %d", count, totalSamples)

	if count == 0 {
		t.Error("No audio frames captured")
	}
}

// TestDefaultVideoCapture tests video capture with default device (empty deviceID).
func TestDefaultVideoCapture(t *testing.T) {
	if !ffi.IsLoaded() {
		t.Skip("shim library not available")
	}

	// Use empty string for default device
	capture, err := ffi.NewVideoCapture("", 320, 240, 10)
	if err != nil {
		// This is expected to fail if no camera is available
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

	time.Sleep(500 * time.Millisecond)
	capture.Stop()

	count := frameCount.Load()
	t.Logf("Captured %d frames from default camera", count)
}
