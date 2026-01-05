// Package decoder provides video and audio decoder interfaces using libwebrtc.
package decoder

import (
	"errors"

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
	switch codecType {
	case codec.H264:
		return NewH264Decoder()
	case codec.VP8:
		return NewVP8Decoder()
	case codec.VP9:
		return NewVP9Decoder()
	case codec.AV1:
		return NewAV1Decoder()
	default:
		return nil, ErrUnsupportedCodec
	}
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
