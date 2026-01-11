package ffi

import (
	"runtime"
	"unsafe"
)

// CreateVideoEncoder creates a video encoder for the specified codec.
func CreateVideoEncoder(codec CodecType, config *VideoEncoderConfig) (uintptr, error) {
	if !libLoaded.Load() {
		return 0, ErrLibraryNotLoaded
	}
	if config == nil {
		return 0, ErrInvalidParam
	}
	if codec == CodecH264 {
		preferHW := runtime.GOOS == "darwin"
		preferHW = config.PreferHW != 0
		if err := ensureOpenH264(shouldRequireOpenH264(preferHW)); err != nil {
			return 0, err
		}
	}
	var errBuf ShimErrorBuffer
	params := shimVideoEncoderCreateParams{
		Codec:    int32(codec),
		Config:   config.Ptr(),
		ErrorOut: errBuf.Ptr(),
	}
	encoder := shimVideoEncoderCreate(uintptr(unsafe.Pointer(&params)))
	runtime.KeepAlive(config)
	runtime.KeepAlive(&params)
	runtime.KeepAlive(&errBuf)
	if encoder == 0 {
		msg := errBuf.String()
		if msg != "" {
			return 0, &ShimErrorWithMessage{Code: ShimErrInitFailed, Message: msg}
		}
		return 0, ErrInitFailed
	}
	return encoder, nil
}

// VideoEncoderEncodeInto encodes a video frame into a pre-allocated buffer.
// Returns the number of bytes written, isKeyframe flag, and error.
// This is the allocation-free version - data is written directly to dst.
func VideoEncoderEncodeInto(
	encoder uintptr,
	yPlane, uPlane, vPlane []byte,
	yStride, uStride, vStride int,
	timestamp uint32,
	forceKeyframe bool,
	dst []byte,
) (n int, isKeyframe bool, err error) {
	if !libLoaded.Load() {
		return 0, false, ErrLibraryNotLoaded
	}

	var forceKF int32
	if forceKeyframe {
		forceKF = 1
	}

	var errBuf ShimErrorBuffer
	params := shimVideoEncoderEncodeParams{
		YPlane:        ByteSlicePtr(yPlane),
		UPlane:        ByteSlicePtr(uPlane),
		VPlane:        ByteSlicePtr(vPlane),
		YStride:       int32(yStride),
		UStride:       int32(uStride),
		VStride:       int32(vStride),
		Timestamp:     timestamp,
		ForceKeyframe: forceKF,
		DstBuffer:     ByteSlicePtr(dst),
		DstBufferSize: int32(len(dst)),
		ErrorOut:      errBuf.Ptr(),
	}

	result := shimVideoEncoderEncode(encoder, uintptr(unsafe.Pointer(&params)))

	err = errBuf.ToError(result)
	runtime.KeepAlive(&params)
	runtime.KeepAlive(&errBuf)
	runtime.KeepAlive(yPlane)
	runtime.KeepAlive(uPlane)
	runtime.KeepAlive(vPlane)
	runtime.KeepAlive(dst)
	if err != nil {
		return 0, false, err
	}

	return int(params.OutSize), params.OutIsKeyframe != 0, nil
}

// VideoEncoderSetBitrate updates the encoder bitrate.
func VideoEncoderSetBitrate(encoder uintptr, bitrate uint32) error {
	if !libLoaded.Load() {
		return ErrLibraryNotLoaded
	}
	var errBuf ShimErrorBuffer
	params := shimVideoEncoderSetBitrateParams{
		Encoder:    encoder,
		BitrateBps: bitrate,
		ErrorOut:   errBuf.Ptr(),
	}
	result := shimVideoEncoderSetBitrate(uintptr(unsafe.Pointer(&params)))
	runtime.KeepAlive(&params)
	runtime.KeepAlive(&errBuf)
	return errBuf.ToError(result)
}

// VideoEncoderSetFramerate updates the encoder framerate.
func VideoEncoderSetFramerate(encoder uintptr, framerate float32) error {
	if !libLoaded.Load() {
		return ErrLibraryNotLoaded
	}
	var errBuf ShimErrorBuffer
	params := shimVideoEncoderSetFramerateParams{
		Encoder:   encoder,
		Framerate: framerate,
		ErrorOut:  errBuf.Ptr(),
	}
	result := shimVideoEncoderSetFramerate(uintptr(unsafe.Pointer(&params)))
	runtime.KeepAlive(&params)
	runtime.KeepAlive(&errBuf)
	return errBuf.ToError(result)
}

// VideoEncoderRequestKeyframe requests the encoder to produce a keyframe.
func VideoEncoderRequestKeyframe(encoder uintptr) error {
	if !libLoaded.Load() {
		return ErrLibraryNotLoaded
	}
	result := shimVideoEncoderRequestKeyframe(encoder)
	return ShimError(result)
}

// VideoEncoderDestroy destroys a video encoder.
func VideoEncoderDestroy(encoder uintptr) {
	if !libLoaded.Load() {
		return
	}
	shimVideoEncoderDestroy(encoder)
}

// CreateAudioEncoder creates an audio encoder.
func CreateAudioEncoder(config *AudioEncoderConfig) (uintptr, error) {
	if !libLoaded.Load() {
		return 0, ErrLibraryNotLoaded
	}
	if config == nil {
		return 0, ErrInvalidParam
	}
	var errBuf ShimErrorBuffer
	params := shimAudioEncoderCreateParams{
		Config:   config.Ptr(),
		ErrorOut: errBuf.Ptr(),
	}
	encoder := shimAudioEncoderCreate(uintptr(unsafe.Pointer(&params)))
	runtime.KeepAlive(config)
	runtime.KeepAlive(&params)
	runtime.KeepAlive(&errBuf)
	if encoder == 0 {
		msg := errBuf.String()
		if msg != "" {
			return 0, &ShimErrorWithMessage{Code: ShimErrInitFailed, Message: msg}
		}
		return 0, ErrInitFailed
	}
	return encoder, nil
}

// AudioEncoderEncodeInto encodes audio samples into a pre-allocated buffer.
// Returns the number of bytes written.
func AudioEncoderEncodeInto(encoder uintptr, samples []byte, numSamples int, dst []byte) (int, error) {
	if !libLoaded.Load() {
		return 0, ErrLibraryNotLoaded
	}

	params := shimAudioEncoderEncodeParams{
		Samples:    ByteSlicePtr(samples),
		NumSamples: int32(numSamples),
		DstBuffer:  ByteSlicePtr(dst),
	}

	result := shimAudioEncoderEncode(encoder, uintptr(unsafe.Pointer(&params)))

	err := ShimError(result)
	runtime.KeepAlive(&params)
	runtime.KeepAlive(samples)
	runtime.KeepAlive(dst)
	if err != nil {
		return 0, err
	}

	return int(params.OutSize), nil
}

// AudioEncoderSetBitrate updates the encoder bitrate.
func AudioEncoderSetBitrate(encoder uintptr, bitrate uint32) error {
	if !libLoaded.Load() {
		return ErrLibraryNotLoaded
	}
	var errBuf ShimErrorBuffer
	params := shimAudioEncoderSetBitrateParams{
		Encoder:    encoder,
		BitrateBps: bitrate,
		ErrorOut:   errBuf.Ptr(),
	}
	result := shimAudioEncoderSetBitrate(uintptr(unsafe.Pointer(&params)))
	runtime.KeepAlive(&params)
	runtime.KeepAlive(&errBuf)
	return errBuf.ToError(result)
}

// AudioEncoderDestroy destroys an audio encoder.
func AudioEncoderDestroy(encoder uintptr) {
	if !libLoaded.Load() {
		return
	}
	shimAudioEncoderDestroy(encoder)
}
