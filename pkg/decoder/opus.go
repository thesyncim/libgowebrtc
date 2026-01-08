package decoder

import (
	"sync"
	"sync/atomic"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

// Opus can decode up to 120ms of audio at 48kHz = 5760 samples per channel
const maxOpusSamplesPerFrame = 5760

type opusDecoder struct {
	handle     uintptr
	sampleRate int
	channels   int
	closed     atomic.Bool
	mu         sync.Mutex
}

func NewOpusDecoder(sampleRate, channels int) (AudioDecoder, error) {
	if err := ffi.LoadLibrary(); err != nil {
		return nil, err
	}

	dec := &opusDecoder{
		sampleRate: sampleRate,
		channels:   channels,
	}

	if err := dec.init(); err != nil {
		return nil, err
	}

	return dec, nil
}

func (d *opusDecoder) init() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	handle, err := ffi.CreateAudioDecoder(d.sampleRate, d.channels)
	if err != nil {
		return err
	}

	d.handle = handle
	return nil
}

func (d *opusDecoder) DecodeInto(src []byte, dst *frame.AudioFrame) (int, error) {
	if d.closed.Load() {
		return 0, ErrDecoderClosed
	}
	if len(src) == 0 {
		return 0, ErrInvalidData
	}
	if dst == nil || len(dst.Samples) < d.MaxSamplesPerFrame()*d.channels*2 {
		return 0, ErrBufferTooSmall
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.handle == 0 {
		return 0, ErrDecoderClosed
	}

	numSamples, err := ffi.AudioDecoderDecodeInto(d.handle, src, dst.Samples)
	if err != nil {
		return 0, err
	}

	dst.SampleRate = d.sampleRate
	dst.Channels = d.channels
	dst.Format = frame.AudioFormatS16
	dst.NumSamples = numSamples / d.channels

	return dst.NumSamples, nil
}

func (d *opusDecoder) MaxSamplesPerFrame() int {
	return maxOpusSamplesPerFrame
}

func (d *opusDecoder) Codec() codec.Type {
	return codec.Opus
}

func (d *opusDecoder) Close() error {
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle != 0 {
		ffi.AudioDecoderDestroy(d.handle)
		d.handle = 0
	}
	return nil
}
