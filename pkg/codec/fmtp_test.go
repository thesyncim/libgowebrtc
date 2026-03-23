package codec

import "testing"

func TestParseFMTP(t *testing.T) {
	got := ParseFMTP(" profile-level-id=42E01F ; packetization-mode = 1 ; level-asymmetry-allowed=1 ")
	if got["profile-level-id"] != "42E01F" {
		t.Fatalf("profile-level-id = %q, want %q", got["profile-level-id"], "42E01F")
	}
	if got["packetization-mode"] != "1" {
		t.Fatalf("packetization-mode = %q, want %q", got["packetization-mode"], "1")
	}
	if got["level-asymmetry-allowed"] != "1" {
		t.Fatalf("level-asymmetry-allowed = %q, want %q", got["level-asymmetry-allowed"], "1")
	}
}

func TestCanonicalizeFMTP(t *testing.T) {
	got := CanonicalizeFMTP("packetization-mode=1; profile-level-id=42e01f; level-asymmetry-allowed=1")
	want := "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"
	if got != want {
		t.Fatalf("CanonicalizeFMTP() = %q, want %q", got, want)
	}
}

func TestH264FMTPHelpers(t *testing.T) {
	line := "packetization-mode=1;profile-level-id=42E01F"

	parsed := ParseH264FMTP(line)
	if parsed.PacketizationMode != 1 {
		t.Fatalf("PacketizationMode = %d, want 1", parsed.PacketizationMode)
	}
	if parsed.ProfileLevelID != "42e01f" {
		t.Fatalf("ProfileLevelID = %q, want %q", parsed.ProfileLevelID, "42e01f")
	}
	if parsed.LevelAsymmetryAllowed != "1" {
		t.Fatalf("LevelAsymmetryAllowed = %q, want %q", parsed.LevelAsymmetryAllowed, "1")
	}

	if got := CanonicalizeH264FMTP(line); got != "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f" {
		t.Fatalf("CanonicalizeH264FMTP() = %q", got)
	}

	profile, ok := H264ProfileFromFMTP(line)
	if !ok {
		t.Fatal("H264ProfileFromFMTP() ok = false, want true")
	}
	if profile != H264ProfileConstrainedBase {
		t.Fatalf("H264ProfileFromFMTP() = %q, want %q", profile, H264ProfileConstrainedBase)
	}

	if !H264FMTPMatches(
		"profile-level-id=42e01f;packetization-mode=1",
		"packetization-mode=1;profile-level-id=42E01F;level-asymmetry-allowed=1",
	) {
		t.Fatal("H264FMTPMatches() = false, want true")
	}
	if H264FMTPMatches(
		"profile-level-id=42e01f;packetization-mode=1",
		"profile-level-id=42e01f;packetization-mode=0",
	) {
		t.Fatal("H264FMTPMatches() = true, want false")
	}
}

func TestVP9FMTPHelpers(t *testing.T) {
	if got := VP9ProfileIDFromFMTP(""); got != VP9Profile0 {
		t.Fatalf("VP9ProfileIDFromFMTP(\"\") = %d, want %d", got, VP9Profile0)
	}
	if got := VP9ProfileIDFromFMTP("profile-id=2"); got != VP9Profile2 {
		t.Fatalf("VP9ProfileIDFromFMTP(profile-id=2) = %d, want %d", got, VP9Profile2)
	}
	if got := CanonicalizeVP9FMTP("profile-id=0"); got != "profile-id=0" {
		t.Fatalf("CanonicalizeVP9FMTP() = %q, want %q", got, "profile-id=0")
	}
	if !VP9FMTPMatches("", "profile-id=0") {
		t.Fatal("VP9FMTPMatches() = false, want true for default profile 0")
	}
	if VP9FMTPMatches("profile-id=0", "profile-id=2") {
		t.Fatal("VP9FMTPMatches() = true, want false")
	}
}
