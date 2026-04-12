package pioncodec

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pion/webrtc/v4"

	libcodec "github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/decoder"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
)

// ErrInvalidConfig reports invalid codec-factory configuration or missing
// required codec parameters.
var ErrInvalidConfig = errors.New("invalid codec factory config")

// VideoFactoryConfig configures libgowebrtc-backed Pion video factories.
type VideoFactoryConfig struct {
	Width       int                 // Width is the expected input frame width in pixels.
	Height      int                 // Height is the expected input frame height in pixels.
	Bitrate     uint32              // Bitrate overrides the encoder target bitrate in bps.
	FPS         float64             // FPS overrides the encoder target frame rate.
	KeyInterval int                 // KeyInterval overrides the keyframe cadence in frames.
	PreferHW    *bool               // PreferHW overrides whether hardware encoding should be preferred.
	SVC         *libcodec.SVCConfig // SVC configures temporal/spatial layering when supported.
}

// AudioFactoryConfig configures libgowebrtc-backed Pion audio factories.
type AudioFactoryConfig struct {
	SampleRate int    // SampleRate is the decoded PCM sample rate in Hz.
	Channels   int    // Channels is the number of PCM channels to encode.
	Bitrate    uint32 // Bitrate overrides the encoder target bitrate in bps.
}

// NewVideoEncoder creates a libgowebrtc encoder from Pion codec parameters.
func NewVideoEncoder(params webrtc.RTPCodecParameters, cfg VideoFactoryConfig) (encoder.VideoEncoder, error) {
	params = normalizeCodecParameters(params)
	codecType, ok := libcodec.ParseMimeType(params.MimeType)
	if !ok || !codecType.IsVideo() {
		return nil, encoder.ErrUnsupportedCodec
	}

	switch codecType {
	case libcodec.H264:
		h264Cfg := libcodec.H264Config{
			Width:       cfg.Width,
			Height:      cfg.Height,
			Bitrate:     cfg.Bitrate,
			FPS:         cfg.FPS,
			KeyInterval: cfg.KeyInterval,
			RateControl: libcodec.RateControlVBR,
			Profile:     libcodec.H264ProfileConstrainedBase,
			LowDelay:    true,
			PreferHW:    false,
		}
		if profile, ok := libcodec.H264ProfileFromFMTP(params.SDPFmtpLine); ok {
			h264Cfg.Profile = profile
		}
		if mode := libcodec.H264PacketizationModeFromFMTP(params.SDPFmtpLine); mode == 0 {
			h264Cfg.ZeroLatency = false
		}
		if cfg.PreferHW != nil {
			h264Cfg.PreferHW = *cfg.PreferHW
		}
		return encoder.NewH264Encoder(h264Cfg)
	case libcodec.VP8:
		vp8Cfg := libcodec.VP8Config{
			Width:          cfg.Width,
			Height:         cfg.Height,
			Bitrate:        cfg.Bitrate,
			FPS:            cfg.FPS,
			KeyInterval:    cfg.KeyInterval,
			RateControl:    libcodec.RateControlVBR,
			Deadline:       2,
			LowDelay:       true,
			ErrorResilient: true,
			PreferHW:       false,
		}
		if cfg.PreferHW != nil {
			vp8Cfg.PreferHW = *cfg.PreferHW
		}
		return encoder.NewVP8Encoder(vp8Cfg)
	case libcodec.VP9:
		vp9Cfg := libcodec.VP9Config{
			Width:       cfg.Width,
			Height:      cfg.Height,
			Bitrate:     cfg.Bitrate,
			FPS:         cfg.FPS,
			KeyInterval: cfg.KeyInterval,
			RateControl: libcodec.RateControlVBR,
			Profile:     libcodec.VP9ProfileIDFromFMTP(params.SDPFmtpLine),
			Speed:       6,
			LowDelay:    true,
			PreferHW:    false,
			SVC:         cfg.SVC,
		}
		if cfg.PreferHW != nil {
			vp9Cfg.PreferHW = *cfg.PreferHW
		}
		return encoder.NewVP9Encoder(vp9Cfg)
	case libcodec.AV1:
		av1Cfg := libcodec.AV1Config{
			Width:       cfg.Width,
			Height:      cfg.Height,
			Bitrate:     cfg.Bitrate,
			FPS:         cfg.FPS,
			KeyInterval: cfg.KeyInterval,
			RateControl: libcodec.RateControlVBR,
			Profile:     libcodec.AV1ProfileMain,
			Speed:       8,
			LowDelay:    true,
			PreferHW:    false,
			SVC:         cfg.SVC,
		}
		if cfg.PreferHW != nil {
			av1Cfg.PreferHW = *cfg.PreferHW
		}
		return encoder.NewAV1Encoder(av1Cfg)
	default:
		return nil, encoder.ErrUnsupportedCodec
	}
}

// NewAudioEncoder creates a libgowebrtc audio encoder from Pion codec parameters.
func NewAudioEncoder(params webrtc.RTPCodecParameters, cfg AudioFactoryConfig) (encoder.AudioEncoder, error) {
	params = normalizeCodecParameters(params)
	codecType, ok := libcodec.ParseMimeType(params.MimeType)
	if !ok || !codecType.IsAudio() {
		return nil, encoder.ErrUnsupportedCodec
	}
	if codecType != libcodec.Opus {
		return nil, encoder.ErrUnsupportedCodec
	}
	if cfg.SampleRate <= 0 || cfg.Channels <= 0 || cfg.Bitrate == 0 {
		return nil, ErrInvalidConfig
	}
	opusCfg := libcodec.OpusConfig{
		SampleRate: cfg.SampleRate,
		Channels:   cfg.Channels,
		Bitrate:    cfg.Bitrate,
	}
	return encoder.NewOpusEncoder(opusCfg)
}

// NewVideoDecoder creates a libgowebrtc video decoder from Pion codec parameters.
func NewVideoDecoder(params webrtc.RTPCodecParameters) (decoder.VideoDecoder, error) {
	params = normalizeCodecParameters(params)
	codecType, ok := libcodec.ParseMimeType(params.MimeType)
	if !ok || !codecType.IsVideo() {
		return nil, decoder.ErrUnsupportedCodec
	}
	return decoder.NewVideoDecoder(codecType)
}

// NewAudioDecoder creates a libgowebrtc audio decoder from Pion codec parameters.
func NewAudioDecoder(params webrtc.RTPCodecParameters) (decoder.AudioDecoder, error) {
	params = normalizeCodecParameters(params)
	codecType, ok := libcodec.ParseMimeType(params.MimeType)
	if !ok || !codecType.IsAudio() {
		return nil, decoder.ErrUnsupportedCodec
	}
	if codecType != libcodec.Opus {
		return nil, decoder.ErrUnsupportedCodec
	}
	if params.ClockRate == 0 || params.Channels == 0 {
		return nil, ErrInvalidConfig
	}
	return decoder.NewAudioDecoder(codecType, int(params.ClockRate), int(params.Channels))
}

func codecCacheKey(params webrtc.RTPCodecParameters) (string, error) {
	params = normalizeCodecParameters(params)
	codecType, ok := libcodec.ParseMimeType(params.MimeType)
	if !ok {
		return "", fmt.Errorf("unsupported codec %q", params.MimeType)
	}

	parts := []string{
		strings.ToLower(params.MimeType),
		fmt.Sprintf("%d", params.ClockRate),
		fmt.Sprintf("%d", params.Channels),
		params.SDPFmtpLine,
		fmt.Sprintf("%d", codecType),
	}
	return strings.Join(parts, "|"), nil
}
