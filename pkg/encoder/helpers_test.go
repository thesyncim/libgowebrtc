package encoder

import (
	"errors"
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/decoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

const (
	encodeRetryStep     = 3000
	encodeRetryAttempts = 5
	decodeRetryAttempts = 5
	audioRetryAttempts  = 5
)

func encodeUntilOutput(t testing.TB, enc VideoEncoder, src *frame.VideoFrame, dst []byte, forceKeyframe bool) (EncodeResult, error) {
	t.Helper()

	if src == nil {
		return EncodeResult{}, ErrInvalidFrame
	}

	basePTS := src.PTS
	defer func() { src.PTS = basePTS }()

	for i := 0; i < encodeRetryAttempts; i++ {
		src.PTS = basePTS + uint32(i*encodeRetryStep)
		result, err := enc.EncodeInto(src, dst, forceKeyframe)
		if err == nil && result.N > 0 {
			return result, nil
		}
		if err != nil && !errors.Is(err, ffi.ErrNeedMoreData) {
			return EncodeResult{}, err
		}
	}

	return EncodeResult{}, ffi.ErrNeedMoreData
}

func decodeUntilOutput(t testing.TB, dec decoder.VideoDecoder, encoded []byte, dst *frame.VideoFrame, timestamp uint32, isKeyframe bool) error {
	t.Helper()

	for i := 0; i < decodeRetryAttempts; i++ {
		err := dec.DecodeInto(encoded, dst, timestamp, isKeyframe)
		if err == nil {
			// Verify actual output was produced (not just no error)
			if dst.Width > 0 && dst.Height > 0 {
				return nil
			}
			// Decoder returned success but no output - treat as need more data
			continue
		}
		if errors.Is(err, decoder.ErrNeedMoreData) {
			continue
		}
		return err
	}

	return decoder.ErrNeedMoreData
}

func encodeAudioUntilOutput(t testing.TB, enc AudioEncoder, src *frame.AudioFrame, dst []byte) (int, error) {
	t.Helper()

	if src == nil {
		return 0, ErrInvalidFrame
	}

	for i := 0; i < audioRetryAttempts; i++ {
		n, err := enc.EncodeInto(src, dst)
		if err != nil {
			return 0, err
		}
		if n > 0 {
			return n, nil
		}
	}

	return 0, ffi.ErrNeedMoreData
}

func decodeAudioUntilOutput(t testing.TB, dec decoder.AudioDecoder, encoded []byte, dst *frame.AudioFrame) (int, error) {
	t.Helper()

	for i := 0; i < audioRetryAttempts; i++ {
		numSamples, err := dec.DecodeInto(encoded, dst)
		if err == nil {
			if numSamples > 0 {
				return numSamples, nil
			}
			continue
		}
		if errors.Is(err, decoder.ErrNeedMoreData) {
			continue
		}
		return 0, err
	}

	return 0, decoder.ErrNeedMoreData
}
