// Package pioncodec provides exact Pion codec parameter matching and factory
// helpers for libgowebrtc-backed encode/decode pipelines.
package pioncodec

import (
	"strings"

	"github.com/pion/webrtc/v4"

	libcodec "github.com/thesyncim/libgowebrtc/pkg/codec"
)

// CloneCodecParameters returns a copy of the caller-provided codec list.
func CloneCodecParameters(codecs []webrtc.RTPCodecParameters) []webrtc.RTPCodecParameters {
	if len(codecs) == 0 {
		return nil
	}
	out := make([]webrtc.RTPCodecParameters, len(codecs))
	copy(out, codecs)
	return out
}

// VideoCodecParameters returns only the video codecs from an explicit codec list.
func VideoCodecParameters(codecs []webrtc.RTPCodecParameters) []webrtc.RTPCodecParameters {
	return codecParametersByKind(codecs, webrtc.RTPCodecTypeVideo)
}

// AudioCodecParameters returns only the audio codecs from an explicit codec list.
func AudioCodecParameters(codecs []webrtc.RTPCodecParameters) []webrtc.RTPCodecParameters {
	return codecParametersByKind(codecs, webrtc.RTPCodecTypeAudio)
}

// SelectCodec returns the first preferred codec that exactly matches an offered codec.
func SelectCodec(preferred, offered []webrtc.RTPCodecParameters) (webrtc.RTPCodecParameters, bool) {
	for _, want := range preferred {
		for _, got := range offered {
			if CodecParametersMatch(want, got) {
				return normalizeCodecParameters(got), true
			}
		}
	}
	return webrtc.RTPCodecParameters{}, false
}

// CodecParametersMatch reports whether two codec descriptions refer to the same
// negotiable RTP codec after canonicalizing FMTP details.
func CodecParametersMatch(preferred, offered webrtc.RTPCodecParameters) bool {
	if !strings.EqualFold(preferred.MimeType, offered.MimeType) {
		return false
	}
	if preferred.ClockRate != 0 && offered.ClockRate != 0 && preferred.ClockRate != offered.ClockRate {
		return false
	}
	if preferred.Channels != 0 && offered.Channels != 0 && preferred.Channels != offered.Channels {
		return false
	}

	switch strings.ToLower(preferred.MimeType) {
	case strings.ToLower(webrtc.MimeTypeH264):
		return libcodec.H264FMTPMatches(preferred.SDPFmtpLine, offered.SDPFmtpLine)
	case strings.ToLower(webrtc.MimeTypeVP9):
		return libcodec.VP9FMTPMatches(preferred.SDPFmtpLine, offered.SDPFmtpLine)
	default:
		return true
	}
}

func codecParametersByKind(codecs []webrtc.RTPCodecParameters, kind webrtc.RTPCodecType) []webrtc.RTPCodecParameters {
	out := make([]webrtc.RTPCodecParameters, 0, len(codecs))
	for _, codec := range codecs {
		if kindFromMime(codec.MimeType) != kind {
			continue
		}
		out = append(out, codec)
	}
	return out
}

func normalizeCodecParameters(params webrtc.RTPCodecParameters) webrtc.RTPCodecParameters {
	out := params
	switch strings.ToLower(out.MimeType) {
	case strings.ToLower(webrtc.MimeTypeH264):
		out.SDPFmtpLine = libcodec.CanonicalizeH264FMTP(out.SDPFmtpLine)
	case strings.ToLower(webrtc.MimeTypeVP9):
		out.SDPFmtpLine = libcodec.CanonicalizeVP9FMTP(out.SDPFmtpLine)
	default:
		out.SDPFmtpLine = libcodec.CanonicalizeFMTP(out.SDPFmtpLine)
	}
	return out
}

func kindFromMime(mime string) webrtc.RTPCodecType {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mime)), "audio/") {
		return webrtc.RTPCodecTypeAudio
	}
	return webrtc.RTPCodecTypeVideo
}
