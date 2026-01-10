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
			return nil
		}
		if errors.Is(err, decoder.ErrNeedMoreData) {
			continue
		}
		return err
	}

	return decoder.ErrNeedMoreData
}
