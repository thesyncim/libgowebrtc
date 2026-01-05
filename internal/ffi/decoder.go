package ffi

// CreateVideoDecoder creates a video decoder for the specified codec.
func CreateVideoDecoder(codec CodecType) uintptr {
	if !libLoaded {
		return 0
	}
	return shimVideoDecoderCreate(int(codec))
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
	if !libLoaded {
		return 0, 0, 0, 0, 0, ErrLibraryNotLoaded
	}

	var outW, outH, outYStride, outUStride, outVStride int

	keyframe := 0
	if isKeyframe {
		keyframe = 1
	}

	// Pass destination buffers - shim writes directly into them
	result := shimVideoDecoderDecode(
		decoder,
		ByteSlicePtr(src),
		len(src),
		timestamp,
		keyframe,
		ByteSlicePtr(yDst),
		ByteSlicePtr(uDst),
		ByteSlicePtr(vDst),
		IntPtr(&outW),
		IntPtr(&outH),
		IntPtr(&outYStride),
		IntPtr(&outUStride),
		IntPtr(&outVStride),
	)

	if err := ShimError(result); err != nil {
		return 0, 0, 0, 0, 0, err
	}

	return outW, outH, outYStride, outUStride, outVStride, nil
}

// VideoDecoderDestroy destroys a video decoder.
func VideoDecoderDestroy(decoder uintptr) {
	if !libLoaded {
		return
	}
	shimVideoDecoderDestroy(decoder)
}

// CreateAudioDecoder creates an audio decoder.
func CreateAudioDecoder(sampleRate, channels int) uintptr {
	if !libLoaded {
		return 0
	}
	return shimAudioDecoderCreate(sampleRate, channels)
}

// AudioDecoderDecodeInto decodes encoded audio into a pre-allocated buffer.
// samplesDst must be pre-allocated (as bytes, will hold int16 samples).
// Returns the number of samples per channel decoded.
func AudioDecoderDecodeInto(decoder uintptr, src []byte, samplesDst []byte) (numSamples int, err error) {
	if !libLoaded {
		return 0, ErrLibraryNotLoaded
	}

	var outNumSamples int

	result := shimAudioDecoderDecode(
		decoder,
		ByteSlicePtr(src),
		len(src),
		ByteSlicePtr(samplesDst),
		IntPtr(&outNumSamples),
	)

	if err := ShimError(result); err != nil {
		return 0, err
	}

	return outNumSamples, nil
}

// AudioDecoderDestroy destroys an audio decoder.
func AudioDecoderDestroy(decoder uintptr) {
	if !libLoaded {
		return
	}
	shimAudioDecoderDestroy(decoder)
}
