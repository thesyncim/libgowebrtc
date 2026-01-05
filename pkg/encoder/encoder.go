// Package encoder provides video and audio encoder interfaces using libwebrtc.
package encoder

import (
	"errors"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

// Common errors
var (
	ErrEncoderClosed    = errors.New("encoder is closed")
	ErrInvalidFrame     = errors.New("invalid frame")
	ErrEncodeFailed     = errors.New("encode failed")
	ErrUnsupportedCodec = errors.New("unsupported codec")
	ErrInvalidConfig    = errors.New("invalid encoder configuration")
	ErrBufferTooSmall   = errors.New("destination buffer too small")
)

// EncodeResult contains the result of an encode operation.
type EncodeResult struct {
	// N is the number of bytes written to the destination buffer.
	N int
	// IsKeyframe indicates if the encoded frame is a keyframe.
	IsKeyframe bool
}

// VideoEncoder encodes raw video frames to compressed bitstream.
// All operations are allocation-free - caller provides buffers.
type VideoEncoder interface {
	// EncodeInto encodes a video frame into the destination buffer.
	// Returns the number of bytes written and whether it's a keyframe.
	// Caller must provide a buffer of at least MaxEncodedSize() bytes.
	EncodeInto(src *frame.VideoFrame, dst []byte, forceKeyframe bool) (EncodeResult, error)

	// MaxEncodedSize returns the maximum possible encoded size for the
	// configured resolution. Use this to allocate destination buffers.
	MaxEncodedSize() int

	// --- Runtime Controls ---

	// SetBitrate updates the target bitrate in bits per second.
	SetBitrate(bps uint32) error

	// SetFramerate updates the target framerate.
	SetFramerate(fps float64) error

	// RequestKeyFrame requests the next frame to be a keyframe.
	RequestKeyFrame()

	// Codec returns the codec type of this encoder.
	Codec() codec.Type

	// Close releases all encoder resources.
	Close() error
}

// VideoEncoderAdvanced extends VideoEncoder with advanced runtime controls.
// Use type assertion to check if an encoder supports these features.
type VideoEncoderAdvanced interface {
	VideoEncoder

	// SetQuality sets the quality level for CQ mode (0-63, lower = better).
	SetQuality(q int) error

	// SetKeyInterval sets the keyframe interval in frames.
	SetKeyInterval(frames int) error

	// SetRateControl changes the rate control mode at runtime.
	SetRateControl(mode codec.RateControlMode) error

	// Stats returns current encoder statistics.
	Stats() EncoderStats
}

// VideoEncoderSVC extends VideoEncoder with SVC/simulcast controls.
// Use type assertion to check if an encoder supports these features.
type VideoEncoderSVC interface {
	VideoEncoder

	// SetSVCMode changes the SVC mode at runtime.
	SetSVCMode(mode codec.SVCMode) error

	// SetLayerBitrate sets bitrate for a specific spatial layer.
	SetLayerBitrate(spatialLayer int, bitrate uint32) error

	// SetLayerActive enables or disables a specific layer.
	SetLayerActive(spatialLayer int, active bool) error

	// SetTemporalLayerBitrate sets bitrate allocation for temporal layer.
	SetTemporalLayerBitrate(temporalLayer int, bitrate uint32) error

	// GetActiveLayerCount returns number of currently active layers.
	GetActiveLayerCount() (spatial, temporal int)

	// RequestLayerKeyFrame requests keyframe for specific spatial layer.
	RequestLayerKeyFrame(spatialLayer int)
}

// LayerInfo describes an SVC/simulcast layer.
type LayerInfo struct {
	SpatialID  int    // Spatial layer ID (0 = lowest resolution)
	TemporalID int    // Temporal layer ID (0 = lowest framerate)
	Width      int    // Layer resolution width
	Height     int    // Layer resolution height
	Bitrate    uint32 // Current bitrate
	FPS        float64
	Active     bool
	IsKeyframe bool // For encoded result
}

// EncoderStats contains encoder runtime statistics.
type EncoderStats struct {
	FramesEncoded   uint64 // Total frames encoded
	BytesEncoded    uint64 // Total bytes produced
	KeyframesForced uint32 // Keyframes due to RequestKeyFrame
	AvgBitrate      uint32 // Average bitrate in bps
	AvgFrameSize    uint32 // Average encoded frame size
	AvgEncodeTimeUs uint32 // Average encode time in microseconds
}

// AudioEncoder encodes raw audio samples to compressed bitstream.
// All operations are allocation-free - caller provides buffers.
type AudioEncoder interface {
	// EncodeInto encodes audio samples into the destination buffer.
	// Returns the number of bytes written.
	// Caller must provide a buffer of at least MaxEncodedSize() bytes.
	EncodeInto(src *frame.AudioFrame, dst []byte) (int, error)

	// MaxEncodedSize returns the maximum possible encoded size for a single
	// audio frame. Use this to allocate destination buffers.
	MaxEncodedSize() int

	// --- Runtime Controls ---

	// SetBitrate updates the target bitrate in bits per second.
	SetBitrate(bps uint32) error

	// Codec returns the codec type of this encoder.
	Codec() codec.Type

	// Close releases all encoder resources.
	Close() error
}

// AudioEncoderAdvanced extends AudioEncoder with advanced runtime controls.
// Use type assertion to check if an encoder supports these features.
type AudioEncoderAdvanced interface {
	AudioEncoder

	// SetComplexity sets encoding complexity (0-10, higher = better quality).
	SetComplexity(c int) error

	// SetFEC enables or disables forward error correction.
	SetFEC(enabled bool) error

	// SetDTX enables or disables discontinuous transmission.
	SetDTX(enabled bool) error

	// SetPacketLossPercentage hints expected packet loss for FEC tuning.
	SetPacketLossPercentage(percent int) error

	// SetBandwidth sets the audio bandwidth constraint.
	SetBandwidth(bw codec.OpusBandwidth) error
}

// NewVideoEncoder creates a video encoder for the specified codec.
func NewVideoEncoder(codecType codec.Type, config interface{}) (VideoEncoder, error) {
	switch codecType {
	case codec.H264:
		cfg, ok := config.(codec.H264Config)
		if !ok {
			return nil, ErrInvalidConfig
		}
		return NewH264Encoder(cfg)
	case codec.VP8:
		cfg, ok := config.(codec.VP8Config)
		if !ok {
			return nil, ErrInvalidConfig
		}
		return NewVP8Encoder(cfg)
	case codec.VP9:
		cfg, ok := config.(codec.VP9Config)
		if !ok {
			return nil, ErrInvalidConfig
		}
		return NewVP9Encoder(cfg)
	case codec.AV1:
		cfg, ok := config.(codec.AV1Config)
		if !ok {
			return nil, ErrInvalidConfig
		}
		return NewAV1Encoder(cfg)
	default:
		return nil, ErrUnsupportedCodec
	}
}

// NewAudioEncoder creates an audio encoder for the specified codec.
func NewAudioEncoder(codecType codec.Type, config interface{}) (AudioEncoder, error) {
	switch codecType {
	case codec.Opus:
		cfg, ok := config.(codec.OpusConfig)
		if !ok {
			return nil, ErrInvalidConfig
		}
		return NewOpusEncoder(cfg)
	default:
		return nil, ErrUnsupportedCodec
	}
}
