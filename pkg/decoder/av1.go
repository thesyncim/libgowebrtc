package decoder

import (
	"sync"
	"sync/atomic"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

type av1Decoder struct {
	handle uintptr
	closed atomic.Bool
	mu     sync.Mutex
}

func NewAV1Decoder() (VideoDecoder, error) {
	if err := ffi.LoadLibrary(); err != nil {
		return nil, err
	}

	dec := &av1Decoder{}
	if err := dec.init(); err != nil {
		return nil, err
	}

	return dec, nil
}

func (d *av1Decoder) init() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	handle := ffi.CreateVideoDecoder(ffi.CodecAV1)
	if handle == 0 {
		return ErrDecodeFailed
	}

	d.handle = handle
	return nil
}

func (d *av1Decoder) DecodeInto(src []byte, dst *frame.VideoFrame, timestamp uint32, isKeyframe bool) error {
	if d.closed.Load() {
		return ErrDecoderClosed
	}
	if len(src) == 0 {
		return ErrInvalidData
	}
	if dst == nil || len(dst.Data) < 3 {
		return ErrBufferTooSmall
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.handle == 0 {
		return ErrDecoderClosed
	}

	width, height, yStride, uStride, vStride, err := ffi.VideoDecoderDecodeInto(
		d.handle, src, timestamp, isKeyframe,
		dst.Data[0], dst.Data[1], dst.Data[2],
	)
	if err != nil {
		if err.Error() == "need more data" {
			return ErrNeedMoreData
		}
		return err
	}

	dst.Width = width
	dst.Height = height
	dst.Stride[0] = yStride
	dst.Stride[1] = uStride
	dst.Stride[2] = vStride
	dst.PTS = timestamp
	dst.Format = frame.PixelFormatI420

	return nil
}

func (d *av1Decoder) Codec() codec.Type {
	return codec.AV1
}

func (d *av1Decoder) Close() error {
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle != 0 {
		ffi.VideoDecoderDestroy(d.handle)
		d.handle = 0
	}
	return nil
}
