// Package encoder provides video and audio encoder interfaces using libwebrtc.
package encoder

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
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

// videoEncoder is the generic video encoder implementation.
type videoEncoder struct {
	handle        uintptr
	codecType     codec.Type
	width, height int
	closed        atomic.Bool
	forceKeyframe atomic.Bool
	mu            sync.Mutex
}

// NewVideoEncoder creates a video encoder for any supported codec.
func NewVideoEncoder(cfg codec.VideoEncoderConfig) (VideoEncoder, error) {
	if err := validateVideoEncoderConfig(cfg); err != nil {
		return nil, err
	}

	if err := ffi.LoadLibrary(); err != nil {
		return nil, err
	}

	enc := &videoEncoder{
		codecType: cfg.Codec,
		width:     cfg.Width,
		height:    cfg.Height,
	}

	if err := enc.init(cfg); err != nil {
		return nil, err
	}

	return enc, nil
}

func validateVideoEncoderConfig(cfg codec.VideoEncoderConfig) error {
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

func (e *videoEncoder) init(cfg codec.VideoEncoderConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	ffiConfig := &ffi.VideoEncoderConfig{
		Width:            int32(cfg.Width),
		Height:           int32(cfg.Height),
		BitrateBps:       cfg.Bitrate,
		Framerate:        float32(cfg.FPS),
		KeyframeInterval: int32(cfg.KeyInterval),
		PreferHW:         boolToInt32(cfg.PreferHW),
	}

	// Codec-specific configuration
	switch cfg.Codec {
	case codec.H264:
		profile := string(cfg.Profile)
		if profile == "" {
			profile = string(codec.H264ProfileConstrainedBase)
		}
		profileBytes := ffi.CString(profile)
		ffiConfig.H264Profile = &profileBytes[0]
	case codec.VP9:
		ffiConfig.VP9Profile = int32(cfg.VP9Profile)
	}

	ffiCodec := codecTypeToFFI(cfg.Codec)
	handle, err := ffi.CreateVideoEncoder(ffiCodec, ffiConfig)
	if err != nil {
		return fmt.Errorf("create %s encoder: %w", cfg.Codec, err)
	}

	e.handle = handle
	return nil
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

func (e *videoEncoder) EncodeInto(src *frame.VideoFrame, dst []byte, forceKeyframe bool) (EncodeResult, error) {
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

func (e *videoEncoder) MaxEncodedSize() int {
	return e.width * e.height * 3 / 2
}

func (e *videoEncoder) SetBitrate(bps uint32) error {
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

func (e *videoEncoder) SetFramerate(fps float64) error {
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

func (e *videoEncoder) RequestKeyFrame() {
	e.forceKeyframe.Store(true)
}

func (e *videoEncoder) Codec() codec.Type {
	return e.codecType
}

func (e *videoEncoder) Close() error {
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

// boolToInt32 converts bool to int32 for FFI.
func boolToInt32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// NewH264Encoder creates a new H.264 encoder.
// Deprecated: Use NewVideoEncoder with codec.DefaultVideoEncoderConfig(codec.H264, w, h).
func NewH264Encoder(cfg codec.VideoEncoderConfig) (VideoEncoder, error) {
	cfg.Codec = codec.H264
	return NewVideoEncoder(cfg)
}

// NewVP8Encoder creates a new VP8 encoder.
// Deprecated: Use NewVideoEncoder with codec.DefaultVideoEncoderConfig(codec.VP8, w, h).
func NewVP8Encoder(cfg codec.VideoEncoderConfig) (VideoEncoder, error) {
	cfg.Codec = codec.VP8
	return NewVideoEncoder(cfg)
}

// NewVP9Encoder creates a new VP9 encoder.
// Deprecated: Use NewVideoEncoder with codec.DefaultVideoEncoderConfig(codec.VP9, w, h).
func NewVP9Encoder(cfg codec.VideoEncoderConfig) (VideoEncoder, error) {
	cfg.Codec = codec.VP9
	return NewVideoEncoder(cfg)
}

// NewAV1Encoder creates a new AV1 encoder.
// Deprecated: Use NewVideoEncoder with codec.DefaultVideoEncoderConfig(codec.AV1, w, h).
func NewAV1Encoder(cfg codec.VideoEncoderConfig) (VideoEncoder, error) {
	cfg.Codec = codec.AV1
	return NewVideoEncoder(cfg)
}
