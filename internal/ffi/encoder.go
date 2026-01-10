package ffi

import "runtime"

// CreateVideoEncoder creates a video encoder for the specified codec.
func CreateVideoEncoder(codec CodecType, config *VideoEncoderConfig) (uintptr, error) {
	if !libLoaded.Load() {
		return 0, ErrLibraryNotLoaded
	}
	if codec == CodecH264 {
		preferHW := runtime.GOOS == "darwin"
		if config != nil {
			preferHW = config.PreferHW != 0
		}
		if err := ensureOpenH264(shouldRequireOpenH264(preferHW)); err != nil {
			return 0, err
		}
	}
	var errBuf ShimErrorBuffer
	encoder := shimVideoEncoderCreate(int32(codec), config.Ptr(), errBuf.Ptr())
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

	var outSize int32
	var outIsKeyframe int32

	var forceKF int32
	if forceKeyframe {
		forceKF = 1
	}

	// Pass dst buffer pointer and size - shim writes directly into it
	var errBuf ShimErrorBuffer
	result := shimVideoEncoderEncode(
		encoder,
		ByteSlicePtr(yPlane),
		ByteSlicePtr(uPlane),
		ByteSlicePtr(vPlane),
		int32(yStride), int32(uStride), int32(vStride),
		timestamp,
		forceKF,
		ByteSlicePtr(dst), // dst buffer for shim to write into
		int32(len(dst)),   // buffer size for overflow protection
		Int32Ptr(&outSize),
		Int32Ptr(&outIsKeyframe),
		errBuf.Ptr(),
	)

	if err := errBuf.ToError(result); err != nil {
		return 0, false, err
	}

	return int(outSize), outIsKeyframe != 0, nil
}

// VideoEncoderSetBitrate updates the encoder bitrate.
func VideoEncoderSetBitrate(encoder uintptr, bitrate uint32) error {
	if !libLoaded.Load() {
		return ErrLibraryNotLoaded
	}
	result := shimVideoEncoderSetBitrate(encoder, bitrate)
	return ShimError(result)
}

// VideoEncoderSetFramerate updates the encoder framerate.
func VideoEncoderSetFramerate(encoder uintptr, framerate float32) error {
	if !libLoaded.Load() {
		return ErrLibraryNotLoaded
	}
	result := shimVideoEncoderSetFramerate(encoder, framerate)
	return ShimError(result)
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
	var errBuf ShimErrorBuffer
	encoder := shimAudioEncoderCreate(config.Ptr(), errBuf.Ptr())
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

	var outSize int32

	result := shimAudioEncoderEncode(
		encoder,
		ByteSlicePtr(samples),
		int32(numSamples),
		ByteSlicePtr(dst),
		Int32Ptr(&outSize),
	)

	if err := ShimError(result); err != nil {
		return 0, err
	}

	return int(outSize), nil
}

// AudioEncoderSetBitrate updates the encoder bitrate.
func AudioEncoderSetBitrate(encoder uintptr, bitrate uint32) error {
	if !libLoaded.Load() {
		return ErrLibraryNotLoaded
	}
	result := shimAudioEncoderSetBitrate(encoder, bitrate)
	return ShimError(result)
}

// AudioEncoderDestroy destroys an audio encoder.
func AudioEncoderDestroy(encoder uintptr) {
	if !libLoaded.Load() {
		return
	}
	shimAudioEncoderDestroy(encoder)
}
