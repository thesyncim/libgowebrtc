package ffi

import (
	"errors"
	"testing"
)

const encodeRetryStep = 3000

func encodeUntilOutput(t testing.TB, encoder uintptr, yPlane, uPlane, vPlane []byte, yStride, uStride, vStride int, timestamp uint32, forceKeyframe bool, dst []byte, attempts int) (int, bool, error) {
	t.Helper()

	for i := 0; i < attempts; i++ {
		ts := timestamp + uint32(i*encodeRetryStep)
		n, isKeyframe, err := VideoEncoderEncodeInto(
			encoder,
			yPlane, uPlane, vPlane,
			yStride, uStride, vStride,
			ts,
			forceKeyframe,
			dst,
		)
		if err == nil && n > 0 {
			return n, isKeyframe, nil
		}
		if err != nil && !errors.Is(err, ErrNeedMoreData) {
			return 0, false, err
		}
	}

	return 0, false, ErrNeedMoreData
}
