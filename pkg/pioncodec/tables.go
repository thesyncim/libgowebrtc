package pioncodec

import (
	"strings"

	"github.com/pion/webrtc/v4"

	libcodec "github.com/thesyncim/libgowebrtc/pkg/codec"
)

const (
	canonicalVP9Profile0Key            = "video/vp9:profile0"
	canonicalVP8Key                    = "video/vp8"
	canonicalAV1Key                    = "video/av1"
	canonicalOpusKey                   = "audio/opus"
	canonicalH264BaselinePM1Key        = "video/h264:42001f:1"
	canonicalH264BaselinePM0Key        = "video/h264:42001f:0"
	canonicalH264ConstrainedBasePM1Key = "video/h264:42e01f:1"
	canonicalH264ConstrainedBasePM0Key = "video/h264:42e01f:0"
	canonicalH264MainPM1Key            = "video/h264:4d001f:1"
	canonicalH264MainPM0Key            = "video/h264:4d001f:0"
	canonicalH264HighPM1Key            = "video/h264:64001f:1"
)

func canonicalAudioCodec(mime string) CodecEntry {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case strings.ToLower(webrtc.MimeTypeOpus):
		return CodecEntry{
			Kind: webrtc.RTPCodecTypeAudio,
			Codec: webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:    webrtc.MimeTypeOpus,
					ClockRate:   48000,
					Channels:    2,
					SDPFmtpLine: "minptime=10;useinbandfec=1",
				},
				PayloadType: 111,
			},
		}
	default:
		return CodecEntry{}
	}
}

func canonicalVideoCodec(key string) []CodecEntry {
	key = strings.ToLower(strings.TrimSpace(key))
	videoRTCPFeedback := []webrtc.RTCPFeedback{
		{Type: "goog-remb"},
		{Type: "ccm", Parameter: "fir"},
		{Type: "nack"},
		{Type: "nack", Parameter: "pli"},
	}

	switch key {
	case canonicalVP8Key, strings.ToLower(webrtc.MimeTypeVP8):
		return []CodecEntry{
			{
				Kind: webrtc.RTPCodecTypeVideo,
				Codec: webrtc.RTPCodecParameters{
					RTPCodecCapability: webrtc.RTPCodecCapability{
						MimeType:     webrtc.MimeTypeVP8,
						ClockRate:    90000,
						RTCPFeedback: videoRTCPFeedback,
					},
					PayloadType: 96,
				},
			},
			newRTXEntry(97, 96),
		}
	case canonicalAV1Key, strings.ToLower(webrtc.MimeTypeAV1):
		return []CodecEntry{
			{
				Kind: webrtc.RTPCodecTypeVideo,
				Codec: webrtc.RTPCodecParameters{
					RTPCodecCapability: webrtc.RTPCodecCapability{
						MimeType:     webrtc.MimeTypeAV1,
						ClockRate:    90000,
						RTCPFeedback: videoRTCPFeedback,
					},
					PayloadType: 45,
				},
			},
			newRTXEntry(46, 45),
		}
	case canonicalVP9Profile0Key:
		return []CodecEntry{
			{
				Kind: webrtc.RTPCodecTypeVideo,
				Codec: webrtc.RTPCodecParameters{
					RTPCodecCapability: webrtc.RTPCodecCapability{
						MimeType:     webrtc.MimeTypeVP9,
						ClockRate:    90000,
						SDPFmtpLine:  libcodec.CanonicalVP9FMTP(libcodec.VP9Profile0),
						RTCPFeedback: videoRTCPFeedback,
					},
					PayloadType: 98,
				},
			},
			newRTXEntry(99, 98),
		}
	case canonicalH264BaselinePM1Key:
		return h264Entries(libcodec.H264ProfileBaseline, 1, 102, 103, videoRTCPFeedback)
	case canonicalH264BaselinePM0Key:
		return h264Entries(libcodec.H264ProfileBaseline, 0, 104, 105, videoRTCPFeedback)
	case canonicalH264ConstrainedBasePM1Key:
		return h264Entries(libcodec.H264ProfileConstrainedBase, 1, 106, 107, videoRTCPFeedback)
	case canonicalH264ConstrainedBasePM0Key:
		return h264Entries(libcodec.H264ProfileConstrainedBase, 0, 108, 109, videoRTCPFeedback)
	case canonicalH264MainPM1Key:
		return h264Entries(libcodec.H264ProfileMain, 1, 127, 125, videoRTCPFeedback)
	case canonicalH264MainPM0Key:
		return h264Entries(libcodec.H264ProfileMain, 0, 39, 40, videoRTCPFeedback)
	case canonicalH264HighPM1Key:
		return h264Entries(libcodec.H264ProfileHigh, 1, 112, 113, videoRTCPFeedback)
	default:
		return nil
	}
}

func h264Entries(
	profile libcodec.H264Profile,
	packetizationMode int,
	payloadType webrtc.PayloadType,
	rtxPayloadType webrtc.PayloadType,
	rtcpFeedback []webrtc.RTCPFeedback,
) []CodecEntry {
	return []CodecEntry{
		{
			Kind: webrtc.RTPCodecTypeVideo,
			Codec: webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:     webrtc.MimeTypeH264,
					ClockRate:    90000,
					SDPFmtpLine:  libcodec.CanonicalH264FMTP(profile, packetizationMode),
					RTCPFeedback: rtcpFeedback,
				},
				PayloadType: payloadType,
			},
		},
		newRTXEntry(rtxPayloadType, payloadType),
	}
}

func newRTXEntry(payloadType, apt webrtc.PayloadType) CodecEntry {
	return CodecEntry{
		Kind: webrtc.RTPCodecTypeVideo,
		Codec: webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeRTX,
				ClockRate:   90000,
				SDPFmtpLine: "apt=" + itoa(int(apt)),
			},
			PayloadType: payloadType,
		},
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	negative := v < 0
	if negative {
		v = -v
	}
	buf := make([]byte, 0, 6)
	for v > 0 {
		buf = append(buf, byte('0'+(v%10)))
		v /= 10
	}
	if negative {
		buf = append(buf, '-')
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
