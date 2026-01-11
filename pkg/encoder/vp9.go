package encoder

import (
	"sync"
	"sync/atomic"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

type vp9Encoder struct {
	handle        uintptr
	config        codec.VP9Config
	closed        atomic.Bool
	forceKeyframe atomic.Bool
	mu            sync.Mutex
}

func NewVP9Encoder(cfg codec.VP9Config) (VideoEncoder, error) {
	if err := validateVP9Config(cfg); err != nil {
		return nil, err
	}

	if err := ffi.LoadLibrary(); err != nil {
		return nil, err
	}

	enc := &vp9Encoder{config: cfg}
	if err := enc.init(); err != nil {
		return nil, err
	}

	return enc, nil
}

func validateVP9Config(cfg codec.VP9Config) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return ErrInvalidConfig
	}
	if cfg.Bitrate == 0 || cfg.FPS <= 0 {
		return ErrInvalidConfig
	}
	return nil
}

func (e *vp9Encoder) init() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	ffiConfig := &ffi.VideoEncoderConfig{
		Width:            int32(e.config.Width),
		Height:           int32(e.config.Height),
		BitrateBps:       e.config.Bitrate,
		Framerate:        float32(e.config.FPS),
		KeyframeInterval: int32(e.config.KeyInterval),
		VP9Profile:       int32(e.config.Profile),
		PreferHW:         boolToInt32(e.config.PreferHW),
	}

	handle, err := ffi.CreateVideoEncoder(ffi.CodecVP9, ffiConfig)
	if err != nil {
		return err
	}

	e.handle = handle
	return nil
}

func (e *vp9Encoder) EncodeInto(src *frame.VideoFrame, dst []byte, forceKeyframe bool) (EncodeResult, error) {
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

func (e *vp9Encoder) MaxEncodedSize() int {
	return e.config.Width * e.config.Height * 3 / 2
}

func (e *vp9Encoder) SetBitrate(bps uint32) error {
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

func (e *vp9Encoder) SetFramerate(fps float64) error {
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

func (e *vp9Encoder) RequestKeyFrame() {
	e.forceKeyframe.Store(true)
}

func (e *vp9Encoder) Codec() codec.Type {
	return codec.VP9
}

func (e *vp9Encoder) Close() error {
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
