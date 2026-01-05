package encoder

import (
	"sync"
	"sync/atomic"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

type vp8Encoder struct {
	handle        uintptr
	config        codec.VP8Config
	closed        atomic.Bool
	forceKeyframe atomic.Bool
	mu            sync.Mutex
}

func NewVP8Encoder(cfg codec.VP8Config) (VideoEncoder, error) {
	if err := validateVP8Config(cfg); err != nil {
		return nil, err
	}

	if err := ffi.LoadLibrary(); err != nil {
		return nil, err
	}

	enc := &vp8Encoder{config: cfg}
	if err := enc.init(); err != nil {
		return nil, err
	}

	return enc, nil
}

func validateVP8Config(cfg codec.VP8Config) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return ErrInvalidConfig
	}
	if cfg.Bitrate == 0 || cfg.FPS <= 0 {
		return ErrInvalidConfig
	}
	return nil
}

func (e *vp8Encoder) init() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	ffiConfig := &ffi.VideoEncoderConfig{
		Width:            int32(e.config.Width),
		Height:           int32(e.config.Height),
		BitrateBps:       e.config.Bitrate,
		Framerate:        float32(e.config.FPS),
		KeyframeInterval: int32(e.config.KeyInterval),
		PreferHW:         1,
	}

	handle := ffi.CreateVideoEncoder(ffi.CodecVP8, ffiConfig)
	if handle == 0 {
		return ErrEncodeFailed
	}

	e.handle = handle
	return nil
}

func (e *vp8Encoder) EncodeInto(src *frame.VideoFrame, dst []byte, forceKeyframe bool) (EncodeResult, error) {
	if e.closed.Load() {
		return EncodeResult{}, ErrEncoderClosed
	}
	if src == nil {
		return EncodeResult{}, ErrInvalidFrame
	}
	if len(dst) < e.MaxEncodedSize() {
		return EncodeResult{}, ErrBufferTooSmall
	}

	if e.forceKeyframe.Swap(false) {
		forceKeyframe = true
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.handle == 0 {
		return EncodeResult{}, ErrEncoderClosed
	}

	n, isKeyframe, err := ffi.VideoEncoderEncodeInto(
		e.handle,
		src.Data[0], src.Data[1], src.Data[2],
		src.Stride[0], src.Stride[1], src.Stride[2],
		src.PTS, forceKeyframe, dst,
	)
	if err != nil {
		return EncodeResult{}, err
	}

	return EncodeResult{N: n, IsKeyframe: isKeyframe}, nil
}

func (e *vp8Encoder) MaxEncodedSize() int {
	return e.config.Width * e.config.Height * 3 / 2
}

func (e *vp8Encoder) SetBitrate(bps uint32) error {
	if e.closed.Load() {
		return ErrEncoderClosed
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle == 0 {
		return ErrEncoderClosed
	}
	return ffi.VideoEncoderSetBitrate(e.handle, bps)
}

func (e *vp8Encoder) SetFramerate(fps float64) error {
	if e.closed.Load() {
		return ErrEncoderClosed
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle == 0 {
		return ErrEncoderClosed
	}
	return ffi.VideoEncoderSetFramerate(e.handle, float32(fps))
}

func (e *vp8Encoder) RequestKeyFrame() {
	e.forceKeyframe.Store(true)
}

func (e *vp8Encoder) Codec() codec.Type {
	return codec.VP8
}

func (e *vp8Encoder) Close() error {
	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle != 0 {
		ffi.VideoEncoderDestroy(e.handle)
		e.handle = 0
	}
	return nil
}
