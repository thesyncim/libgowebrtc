package pioncodec

import (
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestVideoAndAudioCodecParametersPreserveExplicitOrder(t *testing.T) {
	codecs := []webrtc.RTPCodecParameters{
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
	}

	video := VideoCodecParameters(codecs)
	audio := AudioCodecParameters(codecs)

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

func TestCloneCodecParametersReturnsIndependentCopy(t *testing.T) {
	codecs := []webrtc.RTPCodecParameters{
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
	}

	cloned := CloneCodecParameters(codecs)
	if len(cloned) != len(codecs) {
		t.Fatalf("len(cloned) = %d, want %d", len(cloned), len(codecs))
	}

	codecs[0].MimeType = webrtc.MimeTypeH264
	if cloned[0].MimeType != webrtc.MimeTypeVP8 {
		t.Fatalf("cloned[0].MimeType = %q, want %q", cloned[0].MimeType, webrtc.MimeTypeVP8)
	}
}

func TestSelectCodecUsesPreferredOrderAndCanonicalFMTPMatching(t *testing.T) {
	preferred := []webrtc.RTPCodecParameters{
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
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
			},
			PayloadType: 96,
		},
	}
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

	selected, ok := SelectCodec(preferred, offered)
	if !ok {
		t.Fatal("SelectCodec() ok = false, want true")
	}
	if selected.MimeType != webrtc.MimeTypeH264 {
		t.Fatalf("SelectCodec().MimeType = %q, want %q", selected.MimeType, webrtc.MimeTypeH264)
	}
}

func TestSelectCodecRejectsMismatchedH264Variant(t *testing.T) {
	preferred := []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   90000,
				SDPFmtpLine: "profile-level-id=42e01f;packetization-mode=1;level-asymmetry-allowed=1",
			},
			PayloadType: 120,
		},
	}
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

	if _, ok := SelectCodec(preferred, offered); ok {
		t.Fatal("SelectCodec() ok = true, want false for mismatched H264 profile")
	}
}

func TestSelectCodecKeepsOfferedPayloadTypeExact(t *testing.T) {
	preferred := []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
			},
			PayloadType: 96,
		},
	}
	offered := []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
			},
		},
	}

	selected, ok := SelectCodec(preferred, offered)
	if !ok {
		t.Fatal("SelectCodec() ok = false, want true")
	}
	if selected.PayloadType != 0 {
		t.Fatalf("SelectCodec().PayloadType = %d, want 0 from offered codec without backfill", selected.PayloadType)
	}
}
