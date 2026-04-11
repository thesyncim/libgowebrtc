package pioncodec

import (
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestCodecSetFromParametersPreservesExplicitOrder(t *testing.T) {
	set := CodecSetFromParameters([]webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
			},
			PayloadType: 96,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   90000,
				SDPFmtpLine: "packetization-mode=1;profile-level-id=42e01f;level-asymmetry-allowed=1",
			},
			PayloadType: 120,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeVP9,
				ClockRate:   90000,
				SDPFmtpLine: "profile-id=0",
			},
			PayloadType: 98,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeOpus,
				ClockRate: 48000,
				Channels:  2,
			},
			PayloadType: 111,
		},
	})

	video := set.VideoCodecs()
	audio := set.AudioCodecs()

	if len(video) != 3 {
		t.Fatalf("video codecs len = %d, want 3", len(video))
	}
	wantVideo := []string{webrtc.MimeTypeVP8, webrtc.MimeTypeH264, webrtc.MimeTypeVP9}
	for i, mime := range wantVideo {
		if video[i].MimeType != mime {
			t.Fatalf("video[%d].MimeType = %q, want %q", i, video[i].MimeType, mime)
		}
	}
	if len(audio) != 1 || audio[0].MimeType != webrtc.MimeTypeOpus {
		t.Fatalf("audio codecs = %+v, want only opus", audio)
	}
}

func TestCodecSetSupportedOnlyFiltersUnsupportedEntries(t *testing.T) {
	set := CodecSetFromParameters([]webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
			},
			PayloadType: 96,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   90000,
				SDPFmtpLine: "profile-level-id=64001f;packetization-mode=1;level-asymmetry-allowed=1",
			},
			PayloadType: 120,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeVP9,
				ClockRate:   90000,
				SDPFmtpLine: "profile-id=0",
			},
			PayloadType: 98,
		},
	})

	supported := set.SupportedOnly().VideoCodecs()
	if len(supported) != 2 {
		t.Fatalf("supported video codecs len = %d, want 2", len(supported))
	}
	if supported[0].MimeType != webrtc.MimeTypeVP8 || supported[1].MimeType != webrtc.MimeTypeVP9 {
		t.Fatalf("supported video codecs = %+v, want VP8 then VP9", supported)
	}
}

func TestCodecSetSelectUsesCanonicalFMTPMatching(t *testing.T) {
	set := CodecSetFromParameters([]webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeH264,
				ClockRate: 90000,
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
	})
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
		t.Fatalf("Select().MimeType = %q, want %q", selected.MimeType, webrtc.MimeTypeVP8)
	}
}

func TestCodecSetSelectRejectsUnsupportedH264Variant(t *testing.T) {
	set := CodecSetFromParameters([]webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeH264,
				ClockRate: 90000,
			},
			PayloadType: 120,
		},
	})
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
