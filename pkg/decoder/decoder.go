// Package decoder provides video and audio decoder interfaces using libwebrtc.
package decoder

import (
	"errors"
	"fmt"

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
	// Returns ErrNeedMoreData if the decoder accepted input but has not
	// produced PCM output yet.
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

func normalizeVideoDecodeError(err error, isKeyframe bool) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ffi.ErrNeedMoreData):
		return ErrNeedMoreData
	case !isKeyframe && errors.Is(err, ffi.ErrDecodeFailed):
		return ErrNeedMoreData
	case errors.Is(err, ffi.ErrDecodeFailed):
		return fmt.Errorf("%w: %v", ErrDecodeFailed, err)
	default:
		return err
	}
}

func applyVideoDecodeOutput(dst *frame.VideoFrame, width, height, yStride, uStride, vStride int, timestamp uint32) error {
	if width <= 0 || height <= 0 {
		return ErrNeedMoreData
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

func applyAudioDecodeOutput(dst *frame.AudioFrame, sampleRate, channels, numSamples int) (int, error) {
	if numSamples <= 0 {
		return 0, ErrNeedMoreData
	}

	dst.SampleRate = sampleRate
	dst.Channels = channels
	dst.Format = frame.AudioFormatS16
	dst.NumSamples = numSamples

	return numSamples, nil
}
