package encoder

import (
	"sync"
	"sync/atomic"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

// Maximum Opus frame size: 120ms at 48kHz stereo = 5760 samples * 2 channels * 2 bytes
// But encoded Opus is much smaller - max ~4000 bytes for highest quality
const maxOpusEncodedSize = 4000

type opusEncoder struct {
	handle uintptr
	config codec.OpusConfig
	closed atomic.Bool
	mu     sync.Mutex
}

func NewOpusEncoder(cfg codec.OpusConfig) (AudioEncoder, error) {
	if err := validateOpusConfig(cfg); err != nil {
		return nil, err
	}

	if err := ffi.LoadLibrary(); err != nil {
		return nil, err
	}

	enc := &opusEncoder{config: cfg}
	if err := enc.init(); err != nil {
		return nil, err
	}

	return enc, nil
}

func validateOpusConfig(cfg codec.OpusConfig) error {
	validRates := map[int]bool{8000: true, 12000: true, 16000: true, 24000: true, 48000: true}
	if !validRates[cfg.SampleRate] {
		return ErrInvalidConfig
	}
	if cfg.Channels < 1 || cfg.Channels > 2 {
		return ErrInvalidConfig
	}
	if cfg.Bitrate == 0 {
		return ErrInvalidConfig
	}
	return nil
}

func (e *opusEncoder) init() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	ffiConfig := &ffi.AudioEncoderConfig{
		SampleRate: int32(e.config.SampleRate),
		Channels:   int32(e.config.Channels),
		BitrateBps: e.config.Bitrate,
	}

	handle, err := ffi.CreateAudioEncoder(ffiConfig)
	if err != nil {
		return err
	}

	e.handle = handle
	return nil
}

func (e *opusEncoder) EncodeInto(src *frame.AudioFrame, dst []byte) (int, error) {
	if e.closed.Load() {
		return 0, ErrEncoderClosed
	}
	if src == nil {
		return 0, ErrInvalidFrame
	}
	if len(dst) < e.MaxEncodedSize() {
		return 0, ErrBufferTooSmall
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.handle == 0 {
		return 0, ErrEncoderClosed
	}

	// Pass samples per channel, not total samples - shim multiplies by channels
	n, err := ffi.AudioEncoderEncodeInto(e.handle, src.Samples, src.NumSamples, dst)
	if err != nil {
		return 0, err
	}

	return n, nil
}

func (e *opusEncoder) MaxEncodedSize() int {
	return maxOpusEncodedSize
}

func (e *opusEncoder) SetBitrate(bps uint32) error {
	if e.closed.Load() {
		return ErrEncoderClosed
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle == 0 {
		return ErrEncoderClosed
	}
	return ffi.AudioEncoderSetBitrate(e.handle, bps)
}

func (e *opusEncoder) Codec() codec.Type {
	return codec.Opus
}

func (e *opusEncoder) Close() error {
	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle != 0 {
		ffi.AudioEncoderDestroy(e.handle)
		e.handle = 0
	}
	return nil
}
