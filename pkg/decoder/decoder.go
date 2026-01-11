// Package decoder provides video and audio decoder interfaces using libwebrtc.
package decoder

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

// Common errors
var (
	ErrDecoderClosed    = errors.New("decoder is closed")
	ErrInvalidData      = errors.New("invalid encoded data")
	ErrDecodeFailed     = errors.New("decode failed")
	ErrUnsupportedCodec = errors.New("unsupported codec")
	ErrNeedMoreData     = errors.New("need more data to decode")
	ErrBufferTooSmall   = errors.New("destination buffer too small")
)

// VideoDecoder decodes compressed video bitstream to raw frames.
// All operations are allocation-free - caller provides buffers.
type VideoDecoder interface {
	// DecodeInto decodes encoded video data into the destination frame.
	// The dst frame must have pre-allocated Data buffers of sufficient size.
	// Use frame.NewI420Frame(width, height) to create a properly sized frame.
	// Returns ErrNeedMoreData if more data is required (e.g., B-frames).
	DecodeInto(src []byte, dst *frame.VideoFrame, timestamp uint32, isKeyframe bool) error

	// Codec returns the codec type of this decoder.
	Codec() codec.Type

	// Close releases all decoder resources.
	Close() error
}

// AudioDecoder decodes compressed audio bitstream to raw samples.
// All operations are allocation-free - caller provides buffers.
type AudioDecoder interface {
	// DecodeInto decodes encoded audio data into the destination frame.
	// The dst frame must have pre-allocated Samples buffer of sufficient size.
	// Returns the number of samples decoded per channel.
	DecodeInto(src []byte, dst *frame.AudioFrame) (numSamples int, err error)

	// MaxSamplesPerFrame returns the maximum samples per channel that can
	// be decoded from a single encoded frame. Use this to size buffers.
	MaxSamplesPerFrame() int

	// Codec returns the codec type of this decoder.
	Codec() codec.Type

	// Close releases all decoder resources.
	Close() error
}

// NewVideoDecoder creates a video decoder for the specified codec.
func NewVideoDecoder(codecType codec.Type) (VideoDecoder, error) {
	// H264 has special handling for AVCC/AnnexB conversion
	if codecType == codec.H264 {
		return NewH264Decoder()
	}
	return newVideoDecoder(codecType)
}

// NewAudioDecoder creates an audio decoder for the specified codec.
func NewAudioDecoder(codecType codec.Type, sampleRate, channels int) (AudioDecoder, error) {
	switch codecType {
	case codec.Opus:
		return NewOpusDecoder(sampleRate, channels)
	default:
		return nil, ErrUnsupportedCodec
	}
}

// videoDecoder is the generic video decoder implementation.
type videoDecoder struct {
	handle    uintptr
	codecType codec.Type
	closed    atomic.Bool
	mu        sync.Mutex
}

// newVideoDecoder creates a generic video decoder.
func newVideoDecoder(codecType codec.Type) (*videoDecoder, error) {
	// Validate codec type
	switch codecType {
	case codec.VP8, codec.VP9, codec.AV1:
		// Valid video codecs
	default:
		return nil, ErrUnsupportedCodec
	}

	if err := ffi.LoadLibrary(); err != nil {
		return nil, err
	}

	dec := &videoDecoder{codecType: codecType}
	if err := dec.init(); err != nil {
		return nil, err
	}

	return dec, nil
}

func codecTypeToFFI(t codec.Type) ffi.CodecType {
	switch t {
	case codec.H264:
		return ffi.CodecH264
	case codec.VP8:
		return ffi.CodecVP8
	case codec.VP9:
		return ffi.CodecVP9
	case codec.AV1:
		return ffi.CodecAV1
	default:
		return ffi.CodecH264
	}
}

func (d *videoDecoder) init() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	handle, err := ffi.CreateVideoDecoder(codecTypeToFFI(d.codecType))
	if err != nil {
		return err
	}

	d.handle = handle
	return nil
}

func (d *videoDecoder) DecodeInto(src []byte, dst *frame.VideoFrame, timestamp uint32, isKeyframe bool) error {
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

func (d *videoDecoder) Codec() codec.Type {
	return d.codecType
}

func (d *videoDecoder) Close() error {
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

// NewVP8Decoder creates a new VP8 decoder.
// Deprecated: Use NewVideoDecoder(codec.VP8) instead.
func NewVP8Decoder() (VideoDecoder, error) {
	return newVideoDecoder(codec.VP8)
}

// NewVP9Decoder creates a new VP9 decoder.
// Deprecated: Use NewVideoDecoder(codec.VP9) instead.
func NewVP9Decoder() (VideoDecoder, error) {
	return newVideoDecoder(codec.VP9)
}

// NewAV1Decoder creates a new AV1 decoder.
// Deprecated: Use NewVideoDecoder(codec.AV1) instead.
func NewAV1Decoder() (VideoDecoder, error) {
	return newVideoDecoder(codec.AV1)
}
