package pioncodec

import (
	"fmt"
	"strings"

	"github.com/pion/webrtc/v4"

	libcodec "github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/decoder"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
)

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
		h264Cfg := libcodec.DefaultH264Config(cfg.Width, cfg.Height)
		overrideCommonVideoConfig(&h264Cfg.Width, &h264Cfg.Height, &h264Cfg.Bitrate, &h264Cfg.FPS, &h264Cfg.KeyInterval, cfg)
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
		vp8Cfg := libcodec.DefaultVP8Config(cfg.Width, cfg.Height)
		overrideCommonVideoConfig(&vp8Cfg.Width, &vp8Cfg.Height, &vp8Cfg.Bitrate, &vp8Cfg.FPS, &vp8Cfg.KeyInterval, cfg)
		if cfg.PreferHW != nil {
			vp8Cfg.PreferHW = *cfg.PreferHW
		}
		return encoder.NewVP8Encoder(vp8Cfg)
	case libcodec.VP9:
		vp9Cfg := libcodec.DefaultVP9Config(cfg.Width, cfg.Height)
		overrideCommonVideoConfig(&vp9Cfg.Width, &vp9Cfg.Height, &vp9Cfg.Bitrate, &vp9Cfg.FPS, &vp9Cfg.KeyInterval, cfg)
		vp9Cfg.Profile = libcodec.VP9ProfileIDFromFMTP(params.SDPFmtpLine)
		vp9Cfg.SVC = cfg.SVC
		if cfg.PreferHW != nil {
			vp9Cfg.PreferHW = *cfg.PreferHW
		}
		return encoder.NewVP9Encoder(vp9Cfg)
	case libcodec.AV1:
		av1Cfg := libcodec.DefaultAV1Config(cfg.Width, cfg.Height)
		overrideCommonVideoConfig(&av1Cfg.Width, &av1Cfg.Height, &av1Cfg.Bitrate, &av1Cfg.FPS, &av1Cfg.KeyInterval, cfg)
		av1Cfg.SVC = cfg.SVC
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

	opusCfg := libcodec.DefaultOpusConfig()
	if cfg.SampleRate > 0 {
		opusCfg.SampleRate = cfg.SampleRate
	} else if params.ClockRate > 0 {
		opusCfg.SampleRate = int(params.ClockRate)
	}
	if cfg.Channels > 0 {
		opusCfg.Channels = cfg.Channels
	} else if params.Channels > 0 {
		opusCfg.Channels = int(params.Channels)
	}
	if cfg.Bitrate != 0 {
		opusCfg.Bitrate = cfg.Bitrate
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

	sampleRate := int(params.ClockRate)
	if sampleRate == 0 {
		sampleRate = int(codecType.ClockRate())
	}
	channels := int(params.Channels)
	if channels == 0 {
		channels = 2
	}
	return decoder.NewAudioDecoder(codecType, sampleRate, channels)
}

func overrideCommonVideoConfig(
	width *int,
	height *int,
	bitrate *uint32,
	fps *float64,
	keyInterval *int,
	cfg VideoFactoryConfig,
) {
	if cfg.Width > 0 {
		*width = cfg.Width
	}
	if cfg.Height > 0 {
		*height = cfg.Height
	}
	if cfg.Bitrate != 0 {
		*bitrate = cfg.Bitrate
	}
	if cfg.FPS > 0 {
		*fps = cfg.FPS
	}
	if cfg.KeyInterval > 0 {
		*keyInterval = cfg.KeyInterval
	}
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
