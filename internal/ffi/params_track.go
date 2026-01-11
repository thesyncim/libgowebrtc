package ffi

// shimVideoTrackSourceCreateParams matches ShimVideoTrackSourceCreateParams in shim.h.
type shimVideoTrackSourceCreateParams struct {
	PC     uintptr
	Width  int32
	Height int32
}

// shimVideoTrackSourcePushFrameParams matches ShimVideoTrackSourcePushFrameParams in shim.h.
type shimVideoTrackSourcePushFrameParams struct {
	Source      uintptr
	YPlane      uintptr
	UPlane      uintptr
	VPlane      uintptr
	YStride     int32
	UStride     int32
	VStride     int32
	TimestampUs int64
}

// shimPeerConnectionAddVideoTrackFromSourceParams matches ShimPeerConnectionAddVideoTrackFromSourceParams in shim.h.
type shimPeerConnectionAddVideoTrackFromSourceParams struct {
	PC       uintptr
	Source   uintptr
	TrackID  uintptr
	StreamID uintptr
	ErrorOut uintptr
}

// shimAudioTrackSourceCreateParams matches ShimAudioTrackSourceCreateParams in shim.h.
type shimAudioTrackSourceCreateParams struct {
	PC         uintptr
	SampleRate int32
	Channels   int32
}

// shimAudioTrackSourcePushFrameParams matches ShimAudioTrackSourcePushFrameParams in shim.h.
type shimAudioTrackSourcePushFrameParams struct {
	Source      uintptr
	Samples     uintptr
	NumSamples  int32
	TimestampUs int64
}

// shimPeerConnectionAddAudioTrackFromSourceParams matches ShimPeerConnectionAddAudioTrackFromSourceParams in shim.h.
type shimPeerConnectionAddAudioTrackFromSourceParams struct {
	PC       uintptr
	Source   uintptr
	TrackID  uintptr
	StreamID uintptr
	ErrorOut uintptr
}

// shimTrackSetVideoSinkParams matches ShimTrackSetVideoSinkParams in shim.h.
type shimTrackSetVideoSinkParams struct {
	Track    uintptr
	Callback uintptr
	Ctx      uintptr
}

// shimTrackSetAudioSinkParams matches ShimTrackSetAudioSinkParams in shim.h.
type shimTrackSetAudioSinkParams struct {
	Track    uintptr
	Callback uintptr
	Ctx      uintptr
}
