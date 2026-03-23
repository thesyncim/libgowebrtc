package codec

import (
	"sort"
	"strconv"
	"strings"
)

// ParseFMTP parses an SDP fmtp line into a canonical key/value map.
// Keys are lower-cased and surrounding whitespace is removed.
func ParseFMTP(line string) map[string]string {
	parsed := make(map[string]string)
	for _, part := range strings.Split(line, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		key, value, ok := strings.Cut(part, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		if !ok {
			parsed[key] = ""
			continue
		}
		parsed[key] = strings.TrimSpace(value)
	}
	return parsed
}

// CanonicalizeFMTP renders an SDP fmtp line in stable key order.
func CanonicalizeFMTP(line string) string {
	return CanonicalizeFMTPMap(ParseFMTP(line))
}

// CanonicalizeFMTPMap renders an SDP fmtp map in stable key order.
func CanonicalizeFMTPMap(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		if strings.TrimSpace(key) == "" {
			continue
		}
		keys = append(keys, strings.ToLower(strings.TrimSpace(key)))
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(values[key])
		if value == "" {
			parts = append(parts, key)
			continue
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, ";")
}

// H264FMTP captures the H264 fmtp parameters that influence compatibility.
type H264FMTP struct {
	LevelAsymmetryAllowed string
	PacketizationMode     int
	ProfileLevelID        string
}

// ParseH264FMTP parses H264-specific fmtp parameters.
func ParseH264FMTP(line string) H264FMTP {
	values := ParseFMTP(line)
	parsed := H264FMTP{
		LevelAsymmetryAllowed: strings.TrimSpace(values["level-asymmetry-allowed"]),
		ProfileLevelID:        strings.ToLower(strings.TrimSpace(values["profile-level-id"])),
	}
	if parsed.LevelAsymmetryAllowed == "" {
		parsed.LevelAsymmetryAllowed = "1"
	}
	if packetization := strings.TrimSpace(values["packetization-mode"]); packetization != "" {
		if mode, err := strconv.Atoi(packetization); err == nil && mode >= 0 {
			parsed.PacketizationMode = mode
		}
	}
	return parsed
}

// CanonicalizeH264FMTP returns the canonical ordering used across the repo.
func CanonicalizeH264FMTP(line string) string {
	parsed := ParseH264FMTP(line)
	values := map[string]string{
		"level-asymmetry-allowed": parsed.LevelAsymmetryAllowed,
		"packetization-mode":      strconv.Itoa(parsed.PacketizationMode),
	}
	if parsed.ProfileLevelID != "" {
		values["profile-level-id"] = parsed.ProfileLevelID
	}
	return CanonicalizeFMTPMap(values)
}

// H264ProfileFromFMTP returns the H264 profile-level-id prefix when present.
func H264ProfileFromFMTP(line string) (H264Profile, bool) {
	parsed := ParseH264FMTP(line)
	if len(parsed.ProfileLevelID) < 6 {
		return "", false
	}
	return H264Profile(parsed.ProfileLevelID[:6]), true
}

// H264PacketizationModeFromFMTP returns the H264 packetization-mode.
func H264PacketizationModeFromFMTP(line string) int {
	return ParseH264FMTP(line).PacketizationMode
}

// H264FMTPMatches compares H264 fmtp lines by profile-level-id and packetization-mode.
func H264FMTPMatches(a, b string) bool {
	left := ParseH264FMTP(a)
	right := ParseH264FMTP(b)
	return left.PacketizationMode == right.PacketizationMode &&
		left.ProfileLevelID == right.ProfileLevelID
}

// CanonicalH264FMTP constructs a canonical H264 fmtp line.
func CanonicalH264FMTP(profile H264Profile, packetizationMode int) string {
	values := map[string]string{
		"level-asymmetry-allowed": "1",
		"packetization-mode":      strconv.Itoa(packetizationMode),
	}
	if profile != "" {
		values["profile-level-id"] = strings.ToLower(string(profile))
	}
	return CanonicalizeFMTPMap(values)
}

// VP9ProfileIDFromFMTP returns the VP9 profile-id, defaulting to profile 0.
func VP9ProfileIDFromFMTP(line string) VP9Profile {
	values := ParseFMTP(line)
	if raw := strings.TrimSpace(values["profile-id"]); raw != "" {
		if id, err := strconv.Atoi(raw); err == nil {
			return VP9Profile(id)
		}
	}
	return VP9Profile0
}

// CanonicalizeVP9FMTP returns the stable VP9 fmtp representation.
func CanonicalizeVP9FMTP(line string) string {
	return CanonicalVP9FMTP(VP9ProfileIDFromFMTP(line))
}

// CanonicalVP9FMTP constructs a canonical VP9 fmtp line.
func CanonicalVP9FMTP(profile VP9Profile) string {
	return CanonicalizeFMTPMap(map[string]string{
		"profile-id": strconv.Itoa(int(profile)),
	})
}

// VP9FMTPMatches compares VP9 fmtp lines by profile-id.
func VP9FMTPMatches(a, b string) bool {
	return VP9ProfileIDFromFMTP(a) == VP9ProfileIDFromFMTP(b)
}
