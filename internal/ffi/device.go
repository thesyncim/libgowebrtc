// Package ffi provides purego-based FFI bindings to the libwebrtc shim library.
// This file contains device capture FFI bindings.
package ffi

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// DeviceKind represents the type of media device.
type DeviceKind int

const (
	// DeviceKindVideoInput represents a camera/video input device.
	DeviceKindVideoInput DeviceKind = 0
	// DeviceKindAudioInput represents a microphone/audio input device.
	DeviceKindAudioInput DeviceKind = 1
	// DeviceKindAudioOutput represents a speaker/audio output device.
	DeviceKindAudioOutput DeviceKind = 2
)

// String returns a string representation of the device kind.
func (k DeviceKind) String() string {
	switch k {
	case DeviceKindVideoInput:
		return "videoinput"
	case DeviceKindAudioInput:
		return "audioinput"
	case DeviceKindAudioOutput:
		return "audiooutput"
	default:
		return "unknown"
	}
}

// DeviceInfo represents information about a media device.
type DeviceInfo struct {
	DeviceID string
	Label    string
	Kind     DeviceKind
}

// shimDeviceInfo matches ShimDeviceInfo in shim.h
type shimDeviceInfo struct {
	deviceID [256]byte
	label    [256]byte
	kind     int32
}

// CapturedVideoFrame represents a video frame captured from a device.
// The frame data is in I420 (YUV420P) format.
type CapturedVideoFrame struct {
	YPlane      []byte
	UPlane      []byte
	VPlane      []byte
	Width       int
	Height      int
	YStride     int
	UStride     int
	VStride     int
	TimestampUs int64
}

// CapturedAudioFrame represents an audio frame captured from a device.
// Samples are S16LE interleaved.
type CapturedAudioFrame struct {
	Samples     []int16
	NumChannels int
	SampleRate  int
	TimestampUs int64
}

// VideoCaptureCallback is called for each captured video frame.
type VideoCaptureCallback func(frame *CapturedVideoFrame)

// AudioCaptureCallback is called for each captured audio frame.
type AudioCaptureCallback func(frame *CapturedAudioFrame)

// VideoCapture wraps a native video capture device.
type VideoCapture struct {
	ptr        uintptr
	callback   VideoCaptureCallback
	callbackFn uintptr // purego callback pointer
	mu         sync.Mutex
	running    bool
}

// AudioCapture wraps a native audio capture device.
type AudioCapture struct {
	ptr        uintptr
	callback   AudioCaptureCallback
	callbackFn uintptr
	mu         sync.Mutex
	running    bool
}

// ScreenInfo represents information about a screen or window for capture.
type ScreenInfo struct {
	ID       int64
	Title    string
	IsWindow bool
}

// shimScreenInfo matches ShimScreenInfo in shim.h
type shimScreenInfo struct {
	id       int64
	title    [256]byte
	isWindow int32
}

// ScreenCapture wraps a native screen/window capture.
type ScreenCapture struct {
	ptr        uintptr
	callback   VideoCaptureCallback
	callbackFn uintptr
	mu         sync.Mutex
	running    bool
}

// Global callback registry for mapping C callbacks to Go callbacks.
// This is needed because purego callbacks can't capture closure state directly.
var (
	videoCaptureRegistry  = make(map[uintptr]*VideoCapture)
	audioCaptureRegistry  = make(map[uintptr]*AudioCapture)
	screenCaptureRegistry = make(map[uintptr]*ScreenCapture)
	captureRegistryMu     sync.RWMutex
)

// Device capture FFI function pointers
var (
	shimEnumerateDevices func(devices uintptr, maxDevices int, outCount uintptr) int

	shimVideoCaptureCreate  func(deviceID uintptr, width, height, fps int) uintptr
	shimVideoCaptureStart   func(capturePtr uintptr, callback uintptr, ctx uintptr) int
	shimVideoCaptureStop    func(capturePtr uintptr)
	shimVideoCaptureDestroy func(capturePtr uintptr)

	shimAudioCaptureCreate  func(deviceID uintptr, sampleRate, channels int) uintptr
	shimAudioCaptureStart   func(capturePtr uintptr, callback uintptr, ctx uintptr) int
	shimAudioCaptureStop    func(capturePtr uintptr)
	shimAudioCaptureDestroy func(capturePtr uintptr)

	shimEnumerateScreens     func(screens uintptr, maxScreens int, outCount uintptr) int
	shimScreenCaptureCreate  func(id int64, isWindow int, fps int) uintptr
	shimScreenCaptureStart   func(capturePtr uintptr, callback uintptr, ctx uintptr) int
	shimScreenCaptureStop    func(capturePtr uintptr)
	shimScreenCaptureDestroy func(capturePtr uintptr)
)

// ErrCaptureNotStarted is returned when trying to stop a capture that wasn't started.
var ErrCaptureNotStarted = errors.New("capture not started")

// ErrCaptureAlreadyStarted is returned when trying to start a capture that's already running.
var ErrCaptureAlreadyStarted = errors.New("capture already started")

// EnumerateDevices returns a list of available media devices.
func EnumerateDevices() ([]DeviceInfo, error) {
	if !libLoaded {
		return nil, ErrLibraryNotLoaded
	}

	const maxDevices = 64
	devices := make([]shimDeviceInfo, maxDevices)
	var count int32

	result := shimEnumerateDevices(
		uintptr(unsafe.Pointer(&devices[0])),
		maxDevices,
		uintptr(unsafe.Pointer(&count)),
	)

	if err := ShimError(result); err != nil {
		return nil, err
	}

	out := make([]DeviceInfo, count)
	for i := int32(0); i < count; i++ {
		out[i] = DeviceInfo{
			DeviceID: CStringToGo(devices[i].deviceID[:]),
			Label:    CStringToGo(devices[i].label[:]),
			Kind:     DeviceKind(devices[i].kind),
		}
	}

	return out, nil
}

// NewVideoCapture creates a new video capture device.
// deviceID can be empty to use the default device.
func NewVideoCapture(deviceID string, width, height, fps int) (*VideoCapture, error) {
	if !libLoaded {
		return nil, ErrLibraryNotLoaded
	}

	var deviceIDPtr uintptr
	var deviceIDBytes []byte
	if deviceID != "" {
		deviceIDBytes = append([]byte(deviceID), 0)
		deviceIDPtr = uintptr(unsafe.Pointer(&deviceIDBytes[0]))
	}

	ptr := shimVideoCaptureCreate(deviceIDPtr, width, height, fps)
	// Keep deviceIDBytes alive until after the FFI call completes
	runtime.KeepAlive(deviceIDBytes)

	if ptr == 0 {
		return nil, errors.New("failed to create video capture")
	}

	return &VideoCapture{ptr: ptr}, nil
}

// videoCaptureCallbackBridge is the C-callable callback that dispatches to Go.
func videoCaptureCallbackBridge(
	ctx uintptr,
	yPlane, uPlane, vPlane uintptr,
	width, height int,
	yStride, uStride, vStride int,
	timestampUs int64,
) {
	captureRegistryMu.RLock()
	capture, ok := videoCaptureRegistry[ctx]
	captureRegistryMu.RUnlock()

	if !ok || capture.callback == nil {
		return
	}

	// Bounds validation to prevent integer overflow and invalid memory access
	if width <= 0 || height <= 0 || width > 16384 || height > 16384 {
		return
	}
	if yStride <= 0 || uStride <= 0 || vStride <= 0 {
		return
	}
	if yStride > 16384 || uStride > 16384 || vStride > 16384 {
		return
	}
	if yPlane == 0 || uPlane == 0 || vPlane == 0 {
		return
	}

	// Calculate plane sizes
	ySize := yStride * height
	uvHeight := (height + 1) / 2
	uSize := uStride * uvHeight
	vSize := vStride * uvHeight

	// Additional sanity check for total size
	const maxFrameSize = 64 * 1024 * 1024 // 64MB max
	if ySize > maxFrameSize || uSize > maxFrameSize || vSize > maxFrameSize {
		return
	}

	// Copy data from C memory to Go-managed memory for safety.
	// This ensures the callback can safely store/use the data after returning.
	yData := make([]byte, ySize)
	uData := make([]byte, uSize)
	vData := make([]byte, vSize)
	copy(yData, unsafe.Slice((*byte)(unsafe.Pointer(yPlane)), ySize))
	copy(uData, unsafe.Slice((*byte)(unsafe.Pointer(uPlane)), uSize))
	copy(vData, unsafe.Slice((*byte)(unsafe.Pointer(vPlane)), vSize))

	frame := &CapturedVideoFrame{
		YPlane:      yData,
		UPlane:      uData,
		VPlane:      vData,
		Width:       width,
		Height:      height,
		YStride:     yStride,
		UStride:     uStride,
		VStride:     vStride,
		TimestampUs: timestampUs,
	}

	safeCallback(func() {
		capture.callback(frame)
	})
}

// Start begins video capture with the given callback.
func (c *VideoCapture) Start(callback VideoCaptureCallback) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ptr == 0 {
		return errors.New("video capture not initialized")
	}
	if c.running {
		return ErrCaptureAlreadyStarted
	}

	c.callback = callback

	// Register in global registry so callback bridge can find us
	captureRegistryMu.Lock()
	videoCaptureRegistry[c.ptr] = c
	captureRegistryMu.Unlock()

	// Create purego callback
	c.callbackFn = purego.NewCallback(videoCaptureCallbackBridge)

	result := shimVideoCaptureStart(c.ptr, c.callbackFn, c.ptr)

	if err := ShimError(result); err != nil {
		captureRegistryMu.Lock()
		delete(videoCaptureRegistry, c.ptr)
		captureRegistryMu.Unlock()
		c.callback = nil
		c.callbackFn = 0
		return err
	}

	c.running = true
	return nil
}

// Stop stops video capture.
func (c *VideoCapture) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ptr == 0 || !c.running {
		return
	}

	shimVideoCaptureStop(c.ptr)

	captureRegistryMu.Lock()
	delete(videoCaptureRegistry, c.ptr)
	captureRegistryMu.Unlock()

	c.callback = nil
	c.callbackFn = 0
	c.running = false
}

// Close releases the video capture device.
func (c *VideoCapture) Close() {
	c.Stop()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ptr != 0 {
		shimVideoCaptureDestroy(c.ptr)
		c.ptr = 0
	}
}

// IsRunning returns true if capture is active.
func (c *VideoCapture) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// NewAudioCapture creates a new audio capture device.
// deviceID can be empty to use the default device.
func NewAudioCapture(deviceID string, sampleRate, channels int) (*AudioCapture, error) {
	if !libLoaded {
		return nil, ErrLibraryNotLoaded
	}

	var deviceIDPtr uintptr
	var deviceIDBytes []byte
	if deviceID != "" {
		deviceIDBytes = append([]byte(deviceID), 0)
		deviceIDPtr = uintptr(unsafe.Pointer(&deviceIDBytes[0]))
	}

	ptr := shimAudioCaptureCreate(deviceIDPtr, sampleRate, channels)
	// Keep deviceIDBytes alive until after the FFI call completes
	runtime.KeepAlive(deviceIDBytes)

	if ptr == 0 {
		return nil, errors.New("failed to create audio capture")
	}

	return &AudioCapture{ptr: ptr}, nil
}

// audioCaptureCallbackBridge is the C-callable callback that dispatches to Go.
func audioCaptureCallbackBridge(
	ctx uintptr,
	samples uintptr,
	numSamples int,
	numChannels int,
	sampleRate int,
	timestampUs int64,
) {
	captureRegistryMu.RLock()
	capture, ok := audioCaptureRegistry[ctx]
	captureRegistryMu.RUnlock()

	if !ok || capture.callback == nil {
		return
	}

	// Bounds validation to prevent integer overflow and invalid memory access
	if numSamples <= 0 || numSamples > 48000 || numChannels <= 0 || numChannels > 8 {
		return
	}
	if samples == 0 {
		return
	}

	// Copy data from C memory to Go-managed memory for safety.
	// This ensures the callback can safely store/use the data after returning.
	sampleCount := numSamples * numChannels
	samplesData := make([]int16, sampleCount)
	copy(samplesData, unsafe.Slice((*int16)(unsafe.Pointer(samples)), sampleCount))

	frame := &CapturedAudioFrame{
		Samples:     samplesData,
		NumChannels: numChannels,
		SampleRate:  sampleRate,
		TimestampUs: timestampUs,
	}

	safeCallback(func() {
		capture.callback(frame)
	})
}

// Start begins audio capture with the given callback.
func (c *AudioCapture) Start(callback AudioCaptureCallback) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ptr == 0 {
		return errors.New("audio capture not initialized")
	}
	if c.running {
		return ErrCaptureAlreadyStarted
	}

	c.callback = callback

	// Register in global registry
	captureRegistryMu.Lock()
	audioCaptureRegistry[c.ptr] = c
	captureRegistryMu.Unlock()

	// Create purego callback
	c.callbackFn = purego.NewCallback(audioCaptureCallbackBridge)

	result := shimAudioCaptureStart(c.ptr, c.callbackFn, c.ptr)

	if err := ShimError(result); err != nil {
		captureRegistryMu.Lock()
		delete(audioCaptureRegistry, c.ptr)
		captureRegistryMu.Unlock()
		c.callback = nil
		c.callbackFn = 0
		return err
	}

	c.running = true
	return nil
}

// Stop stops audio capture.
func (c *AudioCapture) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ptr == 0 || !c.running {
		return
	}

	shimAudioCaptureStop(c.ptr)

	captureRegistryMu.Lock()
	delete(audioCaptureRegistry, c.ptr)
	captureRegistryMu.Unlock()

	c.callback = nil
	c.callbackFn = 0
	c.running = false
}

// Close releases the audio capture device.
func (c *AudioCapture) Close() {
	c.Stop()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ptr != 0 {
		shimAudioCaptureDestroy(c.ptr)
		c.ptr = 0
	}
}

// IsRunning returns true if capture is active.
func (c *AudioCapture) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// EnumerateScreens returns a list of available screens and windows for capture.
func EnumerateScreens() ([]ScreenInfo, error) {
	if !libLoaded {
		return nil, ErrLibraryNotLoaded
	}

	const maxScreens = 64
	screens := make([]shimScreenInfo, maxScreens)
	var count int32

	result := shimEnumerateScreens(
		uintptr(unsafe.Pointer(&screens[0])),
		maxScreens,
		uintptr(unsafe.Pointer(&count)),
	)

	if err := ShimError(result); err != nil {
		return nil, err
	}

	out := make([]ScreenInfo, count)
	for i := int32(0); i < count; i++ {
		out[i] = ScreenInfo{
			ID:       screens[i].id,
			Title:    CStringToGo(screens[i].title[:]),
			IsWindow: screens[i].isWindow != 0,
		}
	}

	return out, nil
}

// NewScreenCapture creates a new screen or window capture.
func NewScreenCapture(id int64, isWindow bool, fps int) (*ScreenCapture, error) {
	if !libLoaded {
		return nil, ErrLibraryNotLoaded
	}

	isWindowInt := 0
	if isWindow {
		isWindowInt = 1
	}

	ptr := shimScreenCaptureCreate(id, isWindowInt, fps)
	if ptr == 0 {
		return nil, errors.New("failed to create screen capture")
	}

	return &ScreenCapture{ptr: ptr}, nil
}

// screenCaptureCallbackBridge is the C-callable callback that dispatches to Go.
// Uses the same signature as videoCaptureCallbackBridge since screen capture produces video frames.
func screenCaptureCallbackBridge(
	ctx uintptr,
	yPlane, uPlane, vPlane uintptr,
	width, height int,
	yStride, uStride, vStride int,
	timestampUs int64,
) {
	captureRegistryMu.RLock()
	capture, ok := screenCaptureRegistry[ctx]
	captureRegistryMu.RUnlock()

	if !ok || capture.callback == nil {
		return
	}

	// Bounds validation to prevent integer overflow and invalid memory access
	if width <= 0 || height <= 0 || width > 16384 || height > 16384 {
		return
	}
	if yStride <= 0 || uStride <= 0 || vStride <= 0 {
		return
	}
	if yStride > 16384 || uStride > 16384 || vStride > 16384 {
		return
	}
	if yPlane == 0 || uPlane == 0 || vPlane == 0 {
		return
	}

	// Calculate plane sizes
	ySize := yStride * height
	uvHeight := (height + 1) / 2
	uSize := uStride * uvHeight
	vSize := vStride * uvHeight

	// Additional sanity check for total size
	const maxFrameSize = 64 * 1024 * 1024 // 64MB max
	if ySize > maxFrameSize || uSize > maxFrameSize || vSize > maxFrameSize {
		return
	}

	// Copy data from C memory to Go-managed memory for safety.
	// This ensures the callback can safely store/use the data after returning.
	yData := make([]byte, ySize)
	uData := make([]byte, uSize)
	vData := make([]byte, vSize)
	copy(yData, unsafe.Slice((*byte)(unsafe.Pointer(yPlane)), ySize))
	copy(uData, unsafe.Slice((*byte)(unsafe.Pointer(uPlane)), uSize))
	copy(vData, unsafe.Slice((*byte)(unsafe.Pointer(vPlane)), vSize))

	frame := &CapturedVideoFrame{
		YPlane:      yData,
		UPlane:      uData,
		VPlane:      vData,
		Width:       width,
		Height:      height,
		YStride:     yStride,
		UStride:     uStride,
		VStride:     vStride,
		TimestampUs: timestampUs,
	}

	safeCallback(func() {
		capture.callback(frame)
	})
}

// Start begins screen capture with the given callback.
func (c *ScreenCapture) Start(callback VideoCaptureCallback) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ptr == 0 {
		return errors.New("screen capture not initialized")
	}
	if c.running {
		return ErrCaptureAlreadyStarted
	}

	c.callback = callback

	// Register in global registry
	captureRegistryMu.Lock()
	screenCaptureRegistry[c.ptr] = c
	captureRegistryMu.Unlock()

	// Create purego callback
	c.callbackFn = purego.NewCallback(screenCaptureCallbackBridge)

	result := shimScreenCaptureStart(c.ptr, c.callbackFn, c.ptr)

	if err := ShimError(result); err != nil {
		captureRegistryMu.Lock()
		delete(screenCaptureRegistry, c.ptr)
		captureRegistryMu.Unlock()
		c.callback = nil
		c.callbackFn = 0
		return err
	}

	c.running = true
	return nil
}

// Stop stops screen capture.
func (c *ScreenCapture) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ptr == 0 || !c.running {
		return
	}

	shimScreenCaptureStop(c.ptr)

	captureRegistryMu.Lock()
	delete(screenCaptureRegistry, c.ptr)
	captureRegistryMu.Unlock()

	c.callback = nil
	c.callbackFn = 0
	c.running = false
}

// Close releases the screen capture.
func (c *ScreenCapture) Close() {
	c.Stop()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ptr != 0 {
		shimScreenCaptureDestroy(c.ptr)
		c.ptr = 0
	}
}

// IsRunning returns true if capture is active.
func (c *ScreenCapture) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// CStringToGo converts a null-terminated C string to a Go string.
func CStringToGo(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
