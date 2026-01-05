package ffi

import (
	"unsafe"
)

// CreateVideoEncoder creates a video encoder for the specified codec.
func CreateVideoEncoder(codec CodecType, config *VideoEncoderConfig) uintptr {
	if !libLoaded {
		return 0
	}
	return shimVideoEncoderCreate(int(codec), config.Ptr())
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
	if !libLoaded {
		return 0, false, ErrLibraryNotLoaded
	}

	var outSize int
	var outIsKeyframe int32

	forceKF := 0
	if forceKeyframe {
		forceKF = 1
	}

	// Pass dst buffer pointer - shim writes directly into it
	result := shimVideoEncoderEncode(
		encoder,
		ByteSlicePtr(yPlane),
		ByteSlicePtr(uPlane),
		ByteSlicePtr(vPlane),
		yStride, uStride, vStride,
		timestamp,
		forceKF,
		ByteSlicePtr(dst), // dst buffer for shim to write into
		IntPtr(&outSize),
		BoolPtr(&outIsKeyframe),
	)

	if err := ShimError(result); err != nil {
		return 0, false, err
	}

	return outSize, outIsKeyframe != 0, nil
}

// VideoEncoderSetBitrate updates the encoder bitrate.
func VideoEncoderSetBitrate(encoder uintptr, bitrate uint32) error {
	if !libLoaded {
		return ErrLibraryNotLoaded
	}
	result := shimVideoEncoderSetBitrate(encoder, bitrate)
	return ShimError(result)
}

// VideoEncoderSetFramerate updates the encoder framerate.
func VideoEncoderSetFramerate(encoder uintptr, framerate float32) error {
	if !libLoaded {
		return ErrLibraryNotLoaded
	}
	result := shimVideoEncoderSetFramerate(encoder, framerate)
	return ShimError(result)
}

// VideoEncoderRequestKeyframe requests the encoder to produce a keyframe.
func VideoEncoderRequestKeyframe(encoder uintptr) error {
	if !libLoaded {
		return ErrLibraryNotLoaded
	}
	result := shimVideoEncoderRequestKeyframe(encoder)
	return ShimError(result)
}

// VideoEncoderDestroy destroys a video encoder.
func VideoEncoderDestroy(encoder uintptr) {
	if !libLoaded {
		return
	}
	shimVideoEncoderDestroy(encoder)
}

// CreateAudioEncoder creates an audio encoder.
func CreateAudioEncoder(config *AudioEncoderConfig) uintptr {
	if !libLoaded {
		return 0
	}
	return shimAudioEncoderCreate(config.Ptr())
}

// AudioEncoderEncodeInto encodes audio samples into a pre-allocated buffer.
// Returns the number of bytes written.
func AudioEncoderEncodeInto(encoder uintptr, samples []byte, numSamples int, dst []byte) (int, error) {
	if !libLoaded {
		return 0, ErrLibraryNotLoaded
	}

	var outSize int

	result := shimAudioEncoderEncode(
		encoder,
		ByteSlicePtr(samples),
		numSamples,
		ByteSlicePtr(dst),
		IntPtr(&outSize),
	)

	if err := ShimError(result); err != nil {
		return 0, err
	}

	return outSize, nil
}

// AudioEncoderSetBitrate updates the encoder bitrate.
func AudioEncoderSetBitrate(encoder uintptr, bitrate uint32) error {
	if !libLoaded {
		return ErrLibraryNotLoaded
	}
	result := shimAudioEncoderSetBitrate(encoder, bitrate)
	return ShimError(result)
}

// AudioEncoderDestroy destroys an audio encoder.
func AudioEncoderDestroy(encoder uintptr) {
	if !libLoaded {
		return
	}
	shimAudioEncoderDestroy(encoder)
}

// Helper to get raw pointer from byte slice for FFI output parameters
func rawPtr(p *uintptr) uintptr {
	return uintptr(unsafe.Pointer(p))
}
