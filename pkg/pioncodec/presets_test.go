package pioncodec

import (
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestBrowserPresetSupportedOrder(t *testing.T) {
	chromeEncode := BrowserPreset(BrowserChrome, DirectionEncode, PresetModeSupported)
	video := chromeEncode.VideoCodecs()
	audio := chromeEncode.AudioCodecs()

	if len(audio) != 1 || audio[0].MimeType != webrtc.MimeTypeOpus {
		t.Fatalf("audio codecs = %+v, want only opus", audio)
	}
	if len(video) != 4 {
		t.Fatalf("video codecs len = %d, want 4", len(video))
	}

	want := []string{webrtc.MimeTypeVP8, webrtc.MimeTypeH264, webrtc.MimeTypeVP9, webrtc.MimeTypeAV1}
	for i, mime := range want {
		if video[i].MimeType != mime {
			t.Fatalf("video[%d].MimeType = %q, want %q", i, video[i].MimeType, mime)
		}
	}
}

func TestBrowserPresetNegotiationKeepsVariants(t *testing.T) {
	safari := BrowserPreset(BrowserSafari, DirectionEncode, PresetModeNegotiation)
	video := safari.VideoCodecs()
	if len(video) < 4 {
		t.Fatalf("video codecs len = %d, want at least 4 negotiation entries", len(video))
	}
	if video[0].MimeType != webrtc.MimeTypeH264 {
		t.Fatalf("video[0].MimeType = %q, want %q", video[0].MimeType, webrtc.MimeTypeH264)
	}
	if video[1].MimeType != webrtc.MimeTypeRTX {
		t.Fatalf("video[1].MimeType = %q, want RTX paired with first H264 entry", video[1].MimeType)
	}

	supported := safari.SupportedOnly().VideoCodecs()
	if len(supported) != 2 {
		t.Fatalf("supported video codecs len = %d, want 2", len(supported))
	}
	if supported[0].MimeType != webrtc.MimeTypeH264 || supported[1].MimeType != webrtc.MimeTypeVP8 {
		t.Fatalf("supported video codecs = %+v, want H264 then VP8", supported)
	}
}

func TestCodecSetSelectUsesCanonicalFMTPMatching(t *testing.T) {
	set := BrowserPreset(BrowserChrome, DirectionEncode, PresetModeSupported)
	offered := []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   90000,
				SDPFmtpLine: "profile-level-id=42E01F;packetization-mode=1;level-asymmetry-allowed=1",
			},
			PayloadType: 120,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
			},
			PayloadType: 96,
		},
	}

	selected, ok := set.Select(offered)
	if !ok {
		t.Fatal("Select() ok = false, want true")
	}
	if selected.MimeType != webrtc.MimeTypeVP8 {
		t.Fatalf("Select().MimeType = %q, want %q (Chrome encode prefers VP8 first)", selected.MimeType, webrtc.MimeTypeVP8)
	}
}

func TestCodecSetSelectRejectsUnsupportedH264Variant(t *testing.T) {
	set := BrowserPreset(BrowserSafari, DirectionEncode, PresetModeSupported)
	offered := []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   90000,
				SDPFmtpLine: "profile-level-id=64001f;packetization-mode=1;level-asymmetry-allowed=1",
			},
			PayloadType: 112,
		},
	}

	if _, ok := set.Select(offered); ok {
		t.Fatal("Select() ok = true, want false for non-preferred H264 profile in supported mode")
	}
}
