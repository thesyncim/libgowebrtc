// Package pioncodec provides Pion-native codec sets, matching, and factory
// helpers for libgowebrtc-backed encode/decode pipelines.
package pioncodec

import (
	"strings"

	"github.com/pion/webrtc/v4"

	libcodec "github.com/thesyncim/libgowebrtc/pkg/codec"
)

// Browser identifies a browser-shaped compatibility label.
type Browser string

// Browser values identify the browser compatibility profile label.
const (
	BrowserChrome  Browser = "chrome"
	BrowserFirefox Browser = "firefox"
	BrowserSafari  Browser = "safari"
)

// CodecEntry describes a single codec entry in a preset.
type CodecEntry struct {
	Kind      webrtc.RTPCodecType       // Kind identifies whether Codec is audio or video.
	Codec     webrtc.RTPCodecParameters // Codec is the canonical RTP codec description for the entry.
	Supported bool                      // Supported reports whether libgowebrtc can instantiate this codec locally.
}

// CodecSet is a caller-provided codec preference list.
type CodecSet struct {
	entries []CodecEntry
}

// CodecSetFromParameters builds a codec set from an explicit preference list.
func CodecSetFromParameters(codecs []webrtc.RTPCodecParameters) CodecSet {
	return codecSetFromParameters(codecs)
}

// Entries returns a copy of the preset entries.
func (s CodecSet) Entries() []CodecEntry {
	out := make([]CodecEntry, len(s.entries))
	copy(out, s.entries)
	return out
}

// SupportedOnly returns only entries intended for local libgowebrtc-backed use.
func (s CodecSet) SupportedOnly() CodecSet {
	filtered := make([]CodecEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		if !entry.Supported || !isMediaCodec(entry.Codec.MimeType) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return CodecSet{
		entries: filtered,
	}
}

// VideoCodecs returns the video codecs in preference order.
func (s CodecSet) VideoCodecs() []webrtc.RTPCodecParameters {
	return codecsByKind(s.entries, webrtc.RTPCodecTypeVideo)
}

// AudioCodecs returns the audio codecs in preference order.
func (s CodecSet) AudioCodecs() []webrtc.RTPCodecParameters {
	return codecsByKind(s.entries, webrtc.RTPCodecTypeAudio)
}

// Select chooses the first locally supported media codec that matches the offer.
func (s CodecSet) Select(offered []webrtc.RTPCodecParameters) (webrtc.RTPCodecParameters, bool) {
	for _, preferred := range s.SupportedOnly().entries {
		for _, candidate := range offered {
			if codecParametersMatch(preferred.Codec, candidate) {
				return normalizeCodecParameters(candidate), true
			}
		}
	}
	return webrtc.RTPCodecParameters{}, false
}

// Select chooses the first locally supported codec from a preference list.
func Select(preferred, offered []webrtc.RTPCodecParameters) (webrtc.RTPCodecParameters, bool) {
	return codecSetFromParameters(preferred).Select(offered)
}

func codecSetFromParameters(codecs []webrtc.RTPCodecParameters) CodecSet {
	entries := make([]CodecEntry, 0, len(codecs))
	for _, codec := range codecs {
		entries = append(entries, CodecEntry{
			Kind:      kindFromMime(codec.MimeType),
			Codec:     normalizeCodecParameters(codec),
			Supported: isLocallyPreferredCodec(codec),
		})
	}
	return CodecSet{entries: entries}
}

func codecsByKind(entries []CodecEntry, kind webrtc.RTPCodecType) []webrtc.RTPCodecParameters {
	out := make([]webrtc.RTPCodecParameters, 0, len(entries))
	for _, entry := range entries {
		if entry.Kind != kind {
			continue
		}
		out = append(out, entry.Codec)
	}
	return out
}

func codecParametersMatch(preferred, offered webrtc.RTPCodecParameters) bool {
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

func isMediaCodec(mime string) bool {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case strings.ToLower(webrtc.MimeTypeVP8),
		strings.ToLower(webrtc.MimeTypeVP9),
		strings.ToLower(webrtc.MimeTypeAV1),
		strings.ToLower(webrtc.MimeTypeH264),
		strings.ToLower(webrtc.MimeTypeOpus):
		return true
	default:
		return false
	}
}

func isLocallyPreferredCodec(codec webrtc.RTPCodecParameters) bool {
	switch strings.ToLower(strings.TrimSpace(codec.MimeType)) {
	case strings.ToLower(webrtc.MimeTypeVP8),
		strings.ToLower(webrtc.MimeTypeAV1),
		strings.ToLower(webrtc.MimeTypeOpus):
		return true
	case strings.ToLower(webrtc.MimeTypeVP9):
		return libcodec.VP9ProfileIDFromFMTP(codec.SDPFmtpLine) == libcodec.VP9Profile0
	case strings.ToLower(webrtc.MimeTypeH264):
		profile, ok := libcodec.H264ProfileFromFMTP(codec.SDPFmtpLine)
		return ok &&
			profile == libcodec.H264ProfileConstrainedBase &&
			libcodec.H264PacketizationModeFromFMTP(codec.SDPFmtpLine) == 1
	default:
		return false
	}
}
