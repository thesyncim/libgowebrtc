package ffi

// shimVideoEncoderCreateParams matches ShimVideoEncoderCreateParams in shim.h.
type shimVideoEncoderCreateParams struct {
	Codec    int32
	Config   uintptr
	ErrorOut uintptr
}

// shimVideoEncoderSetBitrateParams matches ShimVideoEncoderSetBitrateParams in shim.h.
type shimVideoEncoderSetBitrateParams struct {
	Encoder    uintptr
	BitrateBps uint32
	ErrorOut   uintptr
}

// shimVideoEncoderSetFramerateParams matches ShimVideoEncoderSetFramerateParams in shim.h.
type shimVideoEncoderSetFramerateParams struct {
	Encoder   uintptr
	Framerate float32
	ErrorOut  uintptr
}

// shimVideoDecoderCreateParams matches ShimVideoDecoderCreateParams in shim.h.
type shimVideoDecoderCreateParams struct {
	Codec    int32
	ErrorOut uintptr
}

// shimAudioEncoderCreateParams matches ShimAudioEncoderCreateParams in shim.h.
type shimAudioEncoderCreateParams struct {
	Config   uintptr
	ErrorOut uintptr
}

// shimAudioEncoderSetBitrateParams matches ShimAudioEncoderSetBitrateParams in shim.h.
type shimAudioEncoderSetBitrateParams struct {
	Encoder    uintptr
	BitrateBps uint32
	ErrorOut   uintptr
}

// shimAudioDecoderCreateParams matches ShimAudioDecoderCreateParams in shim.h.
type shimAudioDecoderCreateParams struct {
	SampleRate int32
	Channels   int32
	ErrorOut   uintptr
}

// shimVideoEncoderEncodeParams matches ShimVideoEncoderEncodeParams in shim.h.
type shimVideoEncoderEncodeParams struct {
	YPlane        uintptr
	UPlane        uintptr
	VPlane        uintptr
	YStride       int32
	UStride       int32
	VStride       int32
	Timestamp     uint32
	ForceKeyframe int32
	DstBuffer     uintptr
	DstBufferSize int32
	OutSize       int32
	OutIsKeyframe int32
	ErrorOut      uintptr
}

// shimVideoDecoderDecodeParams matches ShimVideoDecoderDecodeParams in shim.h.
type shimVideoDecoderDecodeParams struct {
	Data       uintptr
	Size       int32
	Timestamp  uint32
	IsKeyframe int32
	YDst       uintptr
	UDst       uintptr
	VDst       uintptr
	OutWidth   int32
	OutHeight  int32
	OutYStride int32
	OutUStride int32
	OutVStride int32
	ErrorOut   uintptr
}

// shimAudioEncoderEncodeParams matches ShimAudioEncoderEncodeParams in shim.h.
type shimAudioEncoderEncodeParams struct {
	Samples    uintptr
	NumSamples int32
	DstBuffer  uintptr
	OutSize    int32
}

// shimAudioDecoderDecodeParams matches ShimAudioDecoderDecodeParams in shim.h.
type shimAudioDecoderDecodeParams struct {
	Data          uintptr
	Size          int32
	DstSamples    uintptr
	OutNumSamples int32
	ErrorOut      uintptr
}
