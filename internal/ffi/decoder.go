package ffi

// CreateVideoDecoder creates a video decoder for the specified codec.
func CreateVideoDecoder(codec CodecType) uintptr {
	if !libLoaded.Load() {
		return 0
	}
	return shimVideoDecoderCreate(int32(codec))
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

	var outW, outH, outYStride, outUStride, outVStride int32

	var keyframe int32 = 0
	if isKeyframe {
		keyframe = 1
	}

	// Pass destination buffers - shim writes directly into them
	result := shimVideoDecoderDecode(
		decoder,
		ByteSlicePtr(src),
		int32(len(src)),
		timestamp,
		keyframe,
		ByteSlicePtr(yDst),
		ByteSlicePtr(uDst),
		ByteSlicePtr(vDst),
		Int32Ptr(&outW),
		Int32Ptr(&outH),
		Int32Ptr(&outYStride),
		Int32Ptr(&outUStride),
		Int32Ptr(&outVStride),
	)

	if err := ShimError(result); err != nil {
		return 0, 0, 0, 0, 0, err
	}

	return int(outW), int(outH), int(outYStride), int(outUStride), int(outVStride), nil
}

// VideoDecoderDestroy destroys a video decoder.
func VideoDecoderDestroy(decoder uintptr) {
	if !libLoaded.Load() {
		return
	}
	shimVideoDecoderDestroy(decoder)
}

// CreateAudioDecoder creates an audio decoder.
func CreateAudioDecoder(sampleRate, channels int) uintptr {
	if !libLoaded.Load() {
		return 0
	}
	return shimAudioDecoderCreate(int32(sampleRate), int32(channels))
}

// AudioDecoderDecodeInto decodes encoded audio into a pre-allocated buffer.
// samplesDst must be pre-allocated (as bytes, will hold int16 samples).
// Returns the number of samples per channel decoded.
func AudioDecoderDecodeInto(decoder uintptr, src []byte, samplesDst []byte) (numSamples int, err error) {
	if !libLoaded.Load() {
		return 0, ErrLibraryNotLoaded
	}

	var outNumSamples int32

	result := shimAudioDecoderDecode(
		decoder,
		ByteSlicePtr(src),
		int32(len(src)),
		ByteSlicePtr(samplesDst),
		Int32Ptr(&outNumSamples),
	)

	if err := ShimError(result); err != nil {
		return 0, err
	}

	return int(outNumSamples), nil
}

// AudioDecoderDestroy destroys an audio decoder.
func AudioDecoderDestroy(decoder uintptr) {
	if !libLoaded.Load() {
		return
	}
	shimAudioDecoderDestroy(decoder)
}
