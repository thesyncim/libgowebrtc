package ffi

import (
	"sync"
	"testing"
	"time"
)

func TestCStringToGo(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{
			name:  "simple string",
			input: []byte{'h', 'e', 'l', 'l', 'o', 0, 0, 0},
			want:  "hello",
		},
		{
			name:  "empty string",
			input: []byte{0, 0, 0},
			want:  "",
		},
		{
			name:  "full buffer without null",
			input: []byte{'a', 'b', 'c'},
			want:  "abc",
		},
		{
			name:  "string with garbage after null",
			input: []byte{'t', 'e', 's', 't', 0, 'x', 'y', 'z'},
			want:  "test",
		},
		{
			name:  "unicode characters",
			input: []byte{0xC3, 0xA9, 0, 0}, // é in UTF-8
			want:  "é",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CStringToGo(tt.input)
			if got != tt.want {
				t.Errorf("CStringToGo(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestVideoCaptureLifecycle(t *testing.T) {
	// Test the lifecycle without library (error paths)
	if IsLoaded() {
		t.Skip("Library is loaded, skipping no-library test")
	}

	// NewVideoCapture should fail without library
	vc, err := NewVideoCapture("device-id", 1280, 720, 30)
	if err != ErrLibraryNotLoaded {
		t.Errorf("NewVideoCapture() error = %v, want %v", err, ErrLibraryNotLoaded)
	}
	if vc != nil {
		t.Errorf("NewVideoCapture() = %v, want nil", vc)
	}
}

func TestAudioCaptureLifecycle(t *testing.T) {
	if IsLoaded() {
		t.Skip("Library is loaded, skipping no-library test")
	}

	ac, err := NewAudioCapture("device-id", 48000, 2)
	if err != ErrLibraryNotLoaded {
		t.Errorf("NewAudioCapture() error = %v, want %v", err, ErrLibraryNotLoaded)
	}
	if ac != nil {
		t.Errorf("NewAudioCapture() = %v, want nil", ac)
	}
}

func TestScreenCaptureLifecycle(t *testing.T) {
	if IsLoaded() {
		t.Skip("Library is loaded, skipping no-library test")
	}

	sc, err := NewScreenCapture(0, false, 30)
	if err != ErrLibraryNotLoaded {
		t.Errorf("NewScreenCapture() error = %v, want %v", err, ErrLibraryNotLoaded)
	}
	if sc != nil {
		t.Errorf("NewScreenCapture() = %v, want nil", sc)
	}
}

func TestVideoCaptureCloseIdempotent(t *testing.T) {
	// Test that Close can be called multiple times without panic
	vc := &VideoCapture{ptr: 0}
	vc.Close()
	vc.Close() // Should not panic
	vc.Close() // Should not panic
}

func TestAudioCaptureCloseIdempotent(t *testing.T) {
	ac := &AudioCapture{ptr: 0}
	ac.Close()
	ac.Close()
	ac.Close()
}

func TestScreenCaptureCloseIdempotent(t *testing.T) {
	sc := &ScreenCapture{ptr: 0}
	sc.Close()
	sc.Close()
	sc.Close()
}

func TestVideoCaptureIsRunning(t *testing.T) {
	vc := &VideoCapture{ptr: 0, running: false}
	if vc.IsRunning() {
		t.Error("IsRunning() should return false for stopped capture")
	}

	vc.running = true
	if !vc.IsRunning() {
		t.Error("IsRunning() should return true for running capture")
	}
}

func TestAudioCaptureIsRunning(t *testing.T) {
	ac := &AudioCapture{ptr: 0, running: false}
	if ac.IsRunning() {
		t.Error("IsRunning() should return false for stopped capture")
	}

	ac.running = true
	if !ac.IsRunning() {
		t.Error("IsRunning() should return true for running capture")
	}
}

func TestScreenCaptureIsRunning(t *testing.T) {
	sc := &ScreenCapture{ptr: 0, running: false}
	if sc.IsRunning() {
		t.Error("IsRunning() should return false for stopped capture")
	}

	sc.running = true
	if !sc.IsRunning() {
		t.Error("IsRunning() should return true for running capture")
	}
}

func TestCaptureRegistryConcurrency(t *testing.T) {
	// Test that registry operations are thread-safe
	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ptr := uintptr(id + 1000)
			vc := &VideoCapture{ptr: ptr}

			// Simulate registration
			captureRegistryMu.Lock()
			videoCaptureRegistry[ptr] = vc
			captureRegistryMu.Unlock()

			// Small delay to increase contention
			time.Sleep(time.Microsecond)

			// Read from registry
			captureRegistryMu.RLock()
			_ = videoCaptureRegistry[ptr]
			captureRegistryMu.RUnlock()

			// Simulate unregistration
			captureRegistryMu.Lock()
			delete(videoCaptureRegistry, ptr)
			captureRegistryMu.Unlock()
		}(i)
	}

	wg.Wait()

	// Registry should be empty after all goroutines complete
	captureRegistryMu.RLock()
	count := len(videoCaptureRegistry)
	captureRegistryMu.RUnlock()

	if count != 0 {
		t.Errorf("Registry should be empty, has %d entries", count)
	}
}

func TestEnumerateDevicesWithoutLibrary(t *testing.T) {
	if IsLoaded() {
		t.Skip("Library is loaded, skipping no-library test")
	}

	devices, err := EnumerateDevices()
	if err != ErrLibraryNotLoaded {
		t.Errorf("EnumerateDevices() error = %v, want %v", err, ErrLibraryNotLoaded)
	}
	if devices != nil {
		t.Errorf("EnumerateDevices() = %v, want nil", devices)
	}
}

func TestEnumerateScreensWithoutLibrary(t *testing.T) {
	if IsLoaded() {
		t.Skip("Library is loaded, skipping no-library test")
	}

	screens, err := EnumerateScreens()
	if err != ErrLibraryNotLoaded {
		t.Errorf("EnumerateScreens() error = %v, want %v", err, ErrLibraryNotLoaded)
	}
	if screens != nil {
		t.Errorf("EnumerateScreens() = %v, want nil", screens)
	}
}

func TestShimStructSizes(t *testing.T) {
	// Ensure Go struct field sizes match expected C struct layout
	var deviceInfo shimDeviceInfo
	if len(deviceInfo.deviceID) != 256 {
		t.Errorf("shimDeviceInfo.deviceID size = %d, want 256", len(deviceInfo.deviceID))
	}
	if len(deviceInfo.label) != 256 {
		t.Errorf("shimDeviceInfo.label size = %d, want 256", len(deviceInfo.label))
	}

	var screenInfo shimScreenInfo
	if len(screenInfo.title) != 256 {
		t.Errorf("shimScreenInfo.title size = %d, want 256", len(screenInfo.title))
	}
}

func BenchmarkCStringToGo(b *testing.B) {
	input := [256]byte{}
	copy(input[:], "Test Device Name with Some Extra Characters")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CStringToGo(input[:])
	}
}

func BenchmarkRegistryLookup(b *testing.B) {
	// Pre-populate registry
	ptr := uintptr(999999)
	vc := &VideoCapture{ptr: ptr}
	captureRegistryMu.Lock()
	videoCaptureRegistry[ptr] = vc
	captureRegistryMu.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		captureRegistryMu.RLock()
		_ = videoCaptureRegistry[ptr]
		captureRegistryMu.RUnlock()
	}

	// Cleanup
	captureRegistryMu.Lock()
	delete(videoCaptureRegistry, ptr)
	captureRegistryMu.Unlock()
}
