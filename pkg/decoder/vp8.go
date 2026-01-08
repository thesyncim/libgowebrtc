package decoder

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

type vp8Decoder struct {
	handle uintptr
	closed atomic.Bool
	mu     sync.Mutex
}

func NewVP8Decoder() (VideoDecoder, error) {
	if err := ffi.LoadLibrary(); err != nil {
		return nil, err
	}

	dec := &vp8Decoder{}
	if err := dec.init(); err != nil {
		return nil, err
	}

	return dec, nil
}

func (d *vp8Decoder) init() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	handle, err := ffi.CreateVideoDecoder(ffi.CodecVP8)
	if err != nil {
		return err
	}

	d.handle = handle
	return nil
}

func (d *vp8Decoder) DecodeInto(src []byte, dst *frame.VideoFrame, timestamp uint32, isKeyframe bool) error {
	if d.closed.Load() {
		return ErrDecoderClosed
	}
	if len(src) == 0 {
		return ErrInvalidData
	}
	if dst == nil || len(dst.Data) < 3 || len(dst.Stride) < 3 {
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
		if errors.Is(err, ffi.ErrNeedMoreData) {
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

func (d *vp8Decoder) Codec() codec.Type {
	return codec.VP8
}

func (d *vp8Decoder) Close() error {
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
