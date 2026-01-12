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
	params := shimVideoDecoderCreateParams{
		Codec:    int32(codec),
		ErrorOut: errBuf.Ptr(),
	}
	decoder := shimVideoDecoderCreate(uintptr(unsafe.Pointer(&params)))
	runtime.KeepAlive(&params)
	runtime.KeepAlive(&errBuf)
	if decoder == 0 {
		msg := errBuf.String()
		if msg != "" {
			return 0, &ShimErrorWithMessage{Code: ShimErrInitFailed, Message: msg}
		}
		return 0, ErrInitFailed
	}
	return decoder, nil
}

// videoDecoderDecodeState holds heap-allocated buffers for FFI calls.
// This prevents stack corruption from affecting Go's runtime if the shim
// writes past buffer boundaries.
type videoDecoderDecodeState struct {
	params shimVideoDecoderDecodeParams
	errBuf ShimErrorBuffer
	// padding provides extra space to absorb any buffer overflows from the shim
	padding [64]byte
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

	// Heap-allocate the FFI state to isolate any buffer overflows from the Go stack.
	// This prevents shim bugs from corrupting Go's runtime data structures.
	state := new(videoDecoderDecodeState)
	state.params = shimVideoDecoderDecodeParams{
		Data:       ByteSlicePtr(src),
		Size:       int32(len(src)),
		Timestamp:  timestamp,
		IsKeyframe: keyframe,
		YDst:       ByteSlicePtr(yDst),
		UDst:       ByteSlicePtr(uDst),
		VDst:       ByteSlicePtr(vDst),
		ErrorOut:   state.errBuf.Ptr(),
	}

	result := shimVideoDecoderDecode(decoder, uintptr(unsafe.Pointer(&state.params)))

	err = state.errBuf.ToError(result)
	runtime.KeepAlive(state)
	runtime.KeepAlive(src)
	runtime.KeepAlive(yDst)
	runtime.KeepAlive(uDst)
	runtime.KeepAlive(vDst)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}

	return int(state.params.OutWidth), int(state.params.OutHeight), int(state.params.OutYStride), int(state.params.OutUStride), int(state.params.OutVStride), nil
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
	params := shimAudioDecoderCreateParams{
		SampleRate: int32(sampleRate),
		Channels:   int32(channels),
		ErrorOut:   errBuf.Ptr(),
	}
	decoder := shimAudioDecoderCreate(uintptr(unsafe.Pointer(&params)))
	runtime.KeepAlive(&params)
	runtime.KeepAlive(&errBuf)
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
