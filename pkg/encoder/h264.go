package encoder

import (
	"sync"
	"sync/atomic"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

// h264Encoder wraps the libwebrtc H.264 encoder.
type h264Encoder struct {
	handle        uintptr
	config        codec.H264Config
	closed        atomic.Bool
	forceKeyframe atomic.Bool
	mu            sync.Mutex
}

// NewH264Encoder creates a new H.264 encoder.
func NewH264Encoder(cfg codec.H264Config) (VideoEncoder, error) {
	if err := validateH264Config(cfg); err != nil {
		return nil, err
	}

	if err := ffi.LoadLibrary(); err != nil {
		return nil, err
	}

	enc := &h264Encoder{
		config: cfg,
	}

	if err := enc.init(); err != nil {
		return nil, err
	}

	return enc, nil
}

func validateH264Config(cfg codec.H264Config) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return ErrInvalidConfig
	}
	if cfg.Bitrate == 0 {
		return ErrInvalidConfig
	}
	if cfg.FPS <= 0 {
		return ErrInvalidConfig
	}
	return nil
}

func (e *h264Encoder) init() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	profile := string(e.config.Profile)
	if profile == "" {
		profile = string(codec.H264ProfileConstrainedBase)
	}
	profileBytes := ffi.CString(profile)

	ffiConfig := &ffi.VideoEncoderConfig{
		Width:            int32(e.config.Width),
		Height:           int32(e.config.Height),
		BitrateBps:       e.config.Bitrate,
		Framerate:        float32(e.config.FPS),
		KeyframeInterval: int32(e.config.KeyInterval),
		H264Profile:      &profileBytes[0],
		PreferHW:         1,
	}

	handle := ffi.CreateVideoEncoder(ffi.CodecH264, ffiConfig)
	if handle == 0 {
		return ErrEncodeFailed
	}

	e.handle = handle
	return nil
}

// EncodeInto implements VideoEncoder.
func (e *h264Encoder) EncodeInto(src *frame.VideoFrame, dst []byte, forceKeyframe bool) (EncodeResult, error) {
	if e.closed.Load() {
		return EncodeResult{}, ErrEncoderClosed
	}

	if src == nil {
		return EncodeResult{}, ErrInvalidFrame
	}

	maxSize := e.MaxEncodedSize()
	if len(dst) < maxSize {
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
		src.PTS,
		forceKeyframe,
		dst,
	)
	if err != nil {
		return EncodeResult{}, err
	}

	return EncodeResult{N: n, IsKeyframe: isKeyframe}, nil
}

// MaxEncodedSize implements VideoEncoder.
func (e *h264Encoder) MaxEncodedSize() int {
	// Worst case: uncompressed frame size (though encoding should always be smaller)
	// For H.264, a reasonable upper bound is width * height * 1.5 (YUV420)
	// In practice, encoded frames are much smaller
	return e.config.Width * e.config.Height * 3 / 2
}

// SetBitrate implements VideoEncoder.
func (e *h264Encoder) SetBitrate(bps uint32) error {
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

// SetFramerate implements VideoEncoder.
func (e *h264Encoder) SetFramerate(fps float64) error {
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

// RequestKeyFrame implements VideoEncoder.
func (e *h264Encoder) RequestKeyFrame() {
	e.forceKeyframe.Store(true)
}

// Codec implements VideoEncoder.
func (e *h264Encoder) Codec() codec.Type {
	return codec.H264
}

// Close implements VideoEncoder.
func (e *h264Encoder) Close() error {
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
