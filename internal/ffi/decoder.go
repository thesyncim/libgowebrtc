package ffi

import (
	"runtime"
	"unsafe"
)

// CreateVideoDecoder creates a video decoder for the specified codec.
func CreateVideoDecoder(codec CodecType) (uintptr, error) {
	if !libLoaded.Load() {
		return 0, ErrLibraryNotLoaded
	}
	if codec == CodecH264 {
		preferHW := runtime.GOOS == "darwin"
		if err := ensureOpenH264(shouldRequireOpenH264(preferHW)); err != nil {
			return 0, err
		}
	}
	var errBuf ShimErrorBuffer
	decoder := shimVideoDecoderCreate(int32(codec), errBuf.Ptr())
	if decoder == 0 {
		msg := errBuf.String()
		if msg != "" {
			return 0, &ShimErrorWithMessage{Code: ShimErrInitFailed, Message: msg}
		}
		return 0, ErrInitFailed
	}
	return decoder, nil
}

// VideoDecoderDecodeInto decodes encoded video data into pre-allocated buffers.
// yDst, uDst, vDst must be pre-allocated with sufficient space.
// Returns the actual dimensions decoded.
func VideoDecoderDecodeInto(
	decoder uintptr,
	src []byte,
	timestamp uint32,
	isKeyframe bool,
	yDst, uDst, vDst []byte,
) (width, height, yStride, uStride, vStride int, err error) {
	if !libLoaded.Load() {
		return 0, 0, 0, 0, 0, ErrLibraryNotLoaded
	}

	var keyframe int32
	if isKeyframe {
		keyframe = 1
	}

	var errBuf ShimErrorBuffer
	params := shimVideoDecoderDecodeParams{
		Data:       ByteSlicePtr(src),
		Size:       int32(len(src)),
		Timestamp:  timestamp,
		IsKeyframe: keyframe,
		YDst:       ByteSlicePtr(yDst),
		UDst:       ByteSlicePtr(uDst),
		VDst:       ByteSlicePtr(vDst),
		ErrorOut:   errBuf.Ptr(),
	}

	result := shimVideoDecoderDecode(decoder, uintptr(unsafe.Pointer(&params)))

	err = errBuf.ToError(result)
	runtime.KeepAlive(&params)
	runtime.KeepAlive(&errBuf)
	runtime.KeepAlive(src)
	runtime.KeepAlive(yDst)
	runtime.KeepAlive(uDst)
	runtime.KeepAlive(vDst)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}

	return int(params.OutWidth), int(params.OutHeight), int(params.OutYStride), int(params.OutUStride), int(params.OutVStride), nil
}

// VideoDecoderDestroy destroys a video decoder.
func VideoDecoderDestroy(decoder uintptr) {
	if !libLoaded.Load() {
		return
	}
	shimVideoDecoderDestroy(decoder)
}

// CreateAudioDecoder creates an audio decoder.
func CreateAudioDecoder(sampleRate, channels int) (uintptr, error) {
	if !libLoaded.Load() {
		return 0, ErrLibraryNotLoaded
	}
	var errBuf ShimErrorBuffer
	decoder := shimAudioDecoderCreate(int32(sampleRate), int32(channels), errBuf.Ptr())
	if decoder == 0 {
		msg := errBuf.String()
		if msg != "" {
			return 0, &ShimErrorWithMessage{Code: ShimErrInitFailed, Message: msg}
		}
		return 0, ErrInitFailed
	}
	return decoder, nil
}

// AudioDecoderDecodeInto decodes encoded audio into a pre-allocated buffer.
// samplesDst must be pre-allocated (as bytes, will hold int16 samples).
// Returns the number of samples per channel decoded.
func AudioDecoderDecodeInto(decoder uintptr, src []byte, samplesDst []byte) (numSamples int, err error) {
	if !libLoaded.Load() {
		return 0, ErrLibraryNotLoaded
	}

	var errBuf ShimErrorBuffer
	params := shimAudioDecoderDecodeParams{
		Data:       ByteSlicePtr(src),
		Size:       int32(len(src)),
		DstSamples: ByteSlicePtr(samplesDst),
		ErrorOut:   errBuf.Ptr(),
	}

	result := shimAudioDecoderDecode(decoder, uintptr(unsafe.Pointer(&params)))

	err = errBuf.ToError(result)
	runtime.KeepAlive(&params)
	runtime.KeepAlive(&errBuf)
	runtime.KeepAlive(src)
	runtime.KeepAlive(samplesDst)
	if err != nil {
		return 0, err
	}

	return int(params.OutNumSamples), nil
}

// AudioDecoderDestroy destroys an audio decoder.
func AudioDecoderDestroy(decoder uintptr) {
	if !libLoaded.Load() {
		return
	}
	shimAudioDecoderDestroy(decoder)
}
