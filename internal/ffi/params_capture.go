package ffi

// shimEnumerateDevicesParams matches ShimEnumerateDevicesParams in shim.h.
type shimEnumerateDevicesParams struct {
	Devices    uintptr
	MaxDevices int32
	OutCount   int32
	ErrorOut   uintptr
}

// shimVideoCaptureCreateParams matches ShimVideoCaptureCreateParams in shim.h.
type shimVideoCaptureCreateParams struct {
	DeviceID uintptr
	Width    int32
	Height   int32
	FPS      int32
	ErrorOut uintptr
}

// shimVideoCaptureStartParams matches ShimVideoCaptureStartParams in shim.h.
type shimVideoCaptureStartParams struct {
	Cap      uintptr
	Callback uintptr
	Ctx      uintptr
	ErrorOut uintptr
}

// shimAudioCaptureCreateParams matches ShimAudioCaptureCreateParams in shim.h.
type shimAudioCaptureCreateParams struct {
	DeviceID   uintptr
	SampleRate int32
	Channels   int32
	ErrorOut   uintptr
}

// shimAudioCaptureStartParams matches ShimAudioCaptureStartParams in shim.h.
type shimAudioCaptureStartParams struct {
	Cap      uintptr
	Callback uintptr
	Ctx      uintptr
	ErrorOut uintptr
}

// shimEnumerateScreensParams matches ShimEnumerateScreensParams in shim.h.
type shimEnumerateScreensParams struct {
	Screens    uintptr
	MaxScreens int32
	OutCount   int32
	ErrorOut   uintptr
}

// shimScreenCaptureCreateParams matches ShimScreenCaptureCreateParams in shim.h.
type shimScreenCaptureCreateParams struct {
	ScreenOrWindowID int64
	IsWindow         int32
	FPS              int32
	ErrorOut         uintptr
}

// shimScreenCaptureStartParams matches ShimScreenCaptureStartParams in shim.h.
type shimScreenCaptureStartParams struct {
	Cap      uintptr
	Callback uintptr
	Ctx      uintptr
	ErrorOut uintptr
}
