// Package pioncodec provides Pion-native codec presets, matching, and
// factory helpers for libgowebrtc-backed encode/decode pipelines.
package pioncodec

import (
	"strings"

	"github.com/pion/webrtc/v4"

	libcodec "github.com/thesyncim/libgowebrtc/pkg/codec"
)

// Browser identifies a browser-shaped codec preset.
type Browser string

const (
	BrowserChrome  Browser = "chrome"
	BrowserFirefox Browser = "firefox"
	BrowserSafari  Browser = "safari"
)

// Direction identifies whether a preset is intended for encode or decode.
type Direction string

const (
	DirectionEncode Direction = "encode"
	DirectionDecode Direction = "decode"
)

// PresetMode controls whether a preset is factory-backed or negotiation-shaped.
type PresetMode string

const (
	PresetModeSupported   PresetMode = "supported"
	PresetModeNegotiation PresetMode = "negotiation"
)

// CodecEntry describes a single codec entry in a preset.
type CodecEntry struct {
	Kind      webrtc.RTPCodecType
	Codec     webrtc.RTPCodecParameters
	Supported bool
}

// CodecSet is a browser-shaped codec preset or a caller-provided preference list.
type CodecSet struct {
	Browser   Browser
	Direction Direction
	Mode      PresetMode

	entries []CodecEntry
}

// BrowserPreset returns a browser-shaped codec preset for audio and video.
func BrowserPreset(browser Browser, direction Direction, mode PresetMode) CodecSet {
	if mode == "" {
		mode = PresetModeSupported
	}
	return CodecSet{
		Browser:   browser,
		Direction: direction,
		Mode:      mode,
		entries:   presetEntries(browser, direction, mode),
	}
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
		Browser:   s.Browser,
		Direction: s.Direction,
		Mode:      PresetModeSupported,
		entries:   filtered,
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
				selected := normalizeCodecParameters(candidate)
				if selected.PayloadType == 0 {
					selected.PayloadType = preferred.Codec.PayloadType
				}
				return selected, true
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

func presetEntries(browser Browser, direction Direction, mode PresetMode) []CodecEntry {
	entries := make([]CodecEntry, 0, 24)
	entries = append(entries, browserAudioEntries(browser, direction, mode)...)
	entries = append(entries, browserVideoEntries(browser, direction, mode)...)
	return entries
}

func browserAudioEntries(browser Browser, direction Direction, mode PresetMode) []CodecEntry {
	_ = browser
	_ = direction
	audio := canonicalAudioCodec(webrtc.MimeTypeOpus)
	audio.Supported = true
	if mode == PresetModeNegotiation {
		return []CodecEntry{audio}
	}
	return []CodecEntry{audio}
}

func browserVideoEntries(browser Browser, direction Direction, mode PresetMode) []CodecEntry {
	order := orderedVideoFamilies(browser, direction)
	entries := make([]CodecEntry, 0, 24)

	for _, family := range order {
		switch family {
		case webrtc.MimeTypeVP8:
			entries = append(entries, entriesForMode(mode, canonicalVideoCodec(webrtc.MimeTypeVP8))...)
		case webrtc.MimeTypeAV1:
			entries = append(entries, entriesForMode(mode, canonicalVideoCodec(webrtc.MimeTypeAV1))...)
		case webrtc.MimeTypeVP9:
			entries = append(entries, entriesForMode(mode, canonicalVideoCodec(canonicalVP9Profile0Key))...)
		case webrtc.MimeTypeH264:
			if mode == PresetModeNegotiation {
				entries = append(entries, canonicalH264FamilyNegotiationEntries()...)
			} else {
				entries = append(entries, entriesForMode(mode, canonicalVideoCodec(canonicalH264ConstrainedBasePM1Key))...)
			}
		}
	}

	if mode == PresetModeSupported {
		return markSupported(entries)
	}
	return markNegotiation(entries)
}

func orderedVideoFamilies(browser Browser, direction Direction) []string {
	switch browser {
	case BrowserFirefox:
		if direction == DirectionDecode {
			return []string{webrtc.MimeTypeAV1, webrtc.MimeTypeVP9, webrtc.MimeTypeVP8, webrtc.MimeTypeH264}
		}
		return []string{webrtc.MimeTypeVP8, webrtc.MimeTypeH264, webrtc.MimeTypeVP9, webrtc.MimeTypeAV1}
	case BrowserSafari:
		return []string{webrtc.MimeTypeH264, webrtc.MimeTypeVP8}
	case BrowserChrome:
		fallthrough
	default:
		if direction == DirectionDecode {
			return []string{webrtc.MimeTypeAV1, webrtc.MimeTypeVP9, webrtc.MimeTypeH264, webrtc.MimeTypeVP8}
		}
		return []string{webrtc.MimeTypeVP8, webrtc.MimeTypeH264, webrtc.MimeTypeVP9, webrtc.MimeTypeAV1}
	}
}

func canonicalH264FamilyNegotiationEntries() []CodecEntry {
	keys := []string{
		canonicalH264ConstrainedBasePM1Key,
		canonicalH264ConstrainedBasePM0Key,
		canonicalH264BaselinePM1Key,
		canonicalH264BaselinePM0Key,
		canonicalH264MainPM1Key,
		canonicalH264MainPM0Key,
		canonicalH264HighPM1Key,
	}

	entries := make([]CodecEntry, 0, len(keys)*2)
	for _, key := range keys {
		entries = append(entries, canonicalVideoCodec(key)...)
	}
	return entries
}

func markSupported(entries []CodecEntry) []CodecEntry {
	out := make([]CodecEntry, len(entries))
	for i, entry := range entries {
		out[i] = entry
		out[i].Supported = isLocallyPreferredCodec(entry.Codec)
	}
	return out
}

func markNegotiation(entries []CodecEntry) []CodecEntry {
	out := make([]CodecEntry, len(entries))
	for i, entry := range entries {
		out[i] = entry
		out[i].Supported = isLocallyPreferredCodec(entry.Codec)
	}
	return out
}

func entriesForMode(mode PresetMode, entries []CodecEntry) []CodecEntry {
	if mode == PresetModeNegotiation {
		return entries
	}
	filtered := make([]CodecEntry, 0, len(entries))
	for _, entry := range entries {
		if !isMediaCodec(entry.Codec.MimeType) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
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
