//go:build ffigo_cgo

package ffi

/*
#cgo CFLAGS: -I${SRCDIR}/../../shim
#include "shim.h"
*/
import "C"

import (
	"testing"
	"unsafe"
)

type cShimErrorBuffer C.ShimErrorBuffer
type cShimVideoEncoderConfig C.ShimVideoEncoderConfig
type cShimAudioEncoderConfig C.ShimAudioEncoderConfig
type cShimPacketizerConfig C.ShimPacketizerConfig
type cShimICEServer C.ShimICEServer
type cShimPeerConnectionConfig C.ShimPeerConnectionConfig
type cShimSessionDescription C.ShimSessionDescription
type cShimICECandidate C.ShimICECandidate
type cShimRTPEncodingParameters C.ShimRTPEncodingParameters
type cShimRTPSendParameters C.ShimRTPSendParameters
type cShimRTCStats C.ShimRTCStats
type cShimCodecCapability C.ShimCodecCapability
type cShimBandwidthEstimate C.ShimBandwidthEstimate
type cShimDeviceInfo C.ShimDeviceInfo
type cShimScreenInfo C.ShimScreenInfo

type shimSessionDescription struct {
	Type int32
	SDP  *byte
}

type shimICECandidate struct {
	Candidate     *byte
	SDPMid        *byte
	SDPMLineIndex int32
}

// TestShimStructLayoutCgo compares Go struct layouts against the C shim headers.
func TestShimStructLayoutCgo(t *testing.T) {
	t.Run("ShimErrorBuffer", func(t *testing.T) {
		var goBuf ShimErrorBuffer
		var cBuf cShimErrorBuffer
		checkSizeEqual(t, "ShimErrorBuffer", unsafe.Sizeof(goBuf), unsafe.Sizeof(cBuf))
		checkOffsetEqual(t, "ShimErrorBuffer.Message", unsafe.Offsetof(goBuf.Message), unsafe.Offsetof(cBuf.message))
	})

	t.Run("ShimVideoEncoderConfig", func(t *testing.T) {
		var goCfg VideoEncoderConfig
		var cCfg cShimVideoEncoderConfig
		checkSizeEqual(t, "ShimVideoEncoderConfig", unsafe.Sizeof(goCfg), unsafe.Sizeof(cCfg))
		checkOffsetEqual(t, "ShimVideoEncoderConfig.Width", unsafe.Offsetof(goCfg.Width), unsafe.Offsetof(cCfg.width))
		checkOffsetEqual(t, "ShimVideoEncoderConfig.Height", unsafe.Offsetof(goCfg.Height), unsafe.Offsetof(cCfg.height))
		checkOffsetEqual(t, "ShimVideoEncoderConfig.BitrateBps", unsafe.Offsetof(goCfg.BitrateBps), unsafe.Offsetof(cCfg.bitrate_bps))
		checkOffsetEqual(t, "ShimVideoEncoderConfig.Framerate", unsafe.Offsetof(goCfg.Framerate), unsafe.Offsetof(cCfg.framerate))
		checkOffsetEqual(t, "ShimVideoEncoderConfig.KeyframeInterval", unsafe.Offsetof(goCfg.KeyframeInterval), unsafe.Offsetof(cCfg.keyframe_interval))
		checkOffsetEqual(t, "ShimVideoEncoderConfig.H264Profile", unsafe.Offsetof(goCfg.H264Profile), unsafe.Offsetof(cCfg.h264_profile))
		checkOffsetEqual(t, "ShimVideoEncoderConfig.VP9Profile", unsafe.Offsetof(goCfg.VP9Profile), unsafe.Offsetof(cCfg.vp9_profile))
		checkOffsetEqual(t, "ShimVideoEncoderConfig.PreferHW", unsafe.Offsetof(goCfg.PreferHW), unsafe.Offsetof(cCfg.prefer_hw))
	})

	t.Run("ShimAudioEncoderConfig", func(t *testing.T) {
		var goCfg AudioEncoderConfig
		var cCfg cShimAudioEncoderConfig
		checkSizeEqual(t, "ShimAudioEncoderConfig", unsafe.Sizeof(goCfg), unsafe.Sizeof(cCfg))
		checkOffsetEqual(t, "ShimAudioEncoderConfig.SampleRate", unsafe.Offsetof(goCfg.SampleRate), unsafe.Offsetof(cCfg.sample_rate))
		checkOffsetEqual(t, "ShimAudioEncoderConfig.Channels", unsafe.Offsetof(goCfg.Channels), unsafe.Offsetof(cCfg.channels))
		checkOffsetEqual(t, "ShimAudioEncoderConfig.BitrateBps", unsafe.Offsetof(goCfg.BitrateBps), unsafe.Offsetof(cCfg.bitrate_bps))
	})

	t.Run("ShimPacketizerConfig", func(t *testing.T) {
		var goCfg PacketizerConfig
		var cCfg cShimPacketizerConfig
		checkSizeEqual(t, "ShimPacketizerConfig", unsafe.Sizeof(goCfg), unsafe.Sizeof(cCfg))
		checkOffsetEqual(t, "ShimPacketizerConfig.Codec", unsafe.Offsetof(goCfg.Codec), unsafe.Offsetof(cCfg.codec))
		checkOffsetEqual(t, "ShimPacketizerConfig.SSRC", unsafe.Offsetof(goCfg.SSRC), unsafe.Offsetof(cCfg.ssrc))
		checkOffsetEqual(t, "ShimPacketizerConfig.PayloadType", unsafe.Offsetof(goCfg.PayloadType), unsafe.Offsetof(cCfg.payload_type))
		checkOffsetEqual(t, "ShimPacketizerConfig.MTU", unsafe.Offsetof(goCfg.MTU), unsafe.Offsetof(cCfg.mtu))
		checkOffsetEqual(t, "ShimPacketizerConfig.ClockRate", unsafe.Offsetof(goCfg.ClockRate), unsafe.Offsetof(cCfg.clock_rate))
	})

	t.Run("ShimICEServer", func(t *testing.T) {
		var goCfg ICEServerConfig
		var cCfg cShimICEServer
		checkSizeEqual(t, "ShimICEServer", unsafe.Sizeof(goCfg), unsafe.Sizeof(cCfg))
		checkOffsetEqual(t, "ShimICEServer.URLs", unsafe.Offsetof(goCfg.URLs), unsafe.Offsetof(cCfg.urls))
		checkOffsetEqual(t, "ShimICEServer.URLCount", unsafe.Offsetof(goCfg.URLCount), unsafe.Offsetof(cCfg.url_count))
		checkOffsetEqual(t, "ShimICEServer.Username", unsafe.Offsetof(goCfg.Username), unsafe.Offsetof(cCfg.username))
		checkOffsetEqual(t, "ShimICEServer.Credential", unsafe.Offsetof(goCfg.Credential), unsafe.Offsetof(cCfg.credential))
	})

	t.Run("ShimPeerConnectionConfig", func(t *testing.T) {
		var goCfg PeerConnectionConfig
		var cCfg cShimPeerConnectionConfig
		checkSizeEqual(t, "ShimPeerConnectionConfig", unsafe.Sizeof(goCfg), unsafe.Sizeof(cCfg))
		checkOffsetEqual(t, "ShimPeerConnectionConfig.ICEServers", unsafe.Offsetof(goCfg.ICEServers), unsafe.Offsetof(cCfg.ice_servers))
		checkOffsetEqual(t, "ShimPeerConnectionConfig.ICEServerCount", unsafe.Offsetof(goCfg.ICEServerCount), unsafe.Offsetof(cCfg.ice_server_count))
		checkOffsetEqual(t, "ShimPeerConnectionConfig.ICECandidatePoolSize", unsafe.Offsetof(goCfg.ICECandidatePoolSize), unsafe.Offsetof(cCfg.ice_candidate_pool_size))
		checkOffsetEqual(t, "ShimPeerConnectionConfig.BundlePolicy", unsafe.Offsetof(goCfg.BundlePolicy), unsafe.Offsetof(cCfg.bundle_policy))
		checkOffsetEqual(t, "ShimPeerConnectionConfig.RTCPMuxPolicy", unsafe.Offsetof(goCfg.RTCPMuxPolicy), unsafe.Offsetof(cCfg.rtcp_mux_policy))
		checkOffsetEqual(t, "ShimPeerConnectionConfig.SDPSemantics", unsafe.Offsetof(goCfg.SDPSemantics), unsafe.Offsetof(cCfg.sdp_semantics))
	})

	t.Run("ShimSessionDescription", func(t *testing.T) {
		var goCfg shimSessionDescription
		var cCfg cShimSessionDescription
		checkSizeEqual(t, "ShimSessionDescription", unsafe.Sizeof(goCfg), unsafe.Sizeof(cCfg))
		checkOffsetEqual(t, "ShimSessionDescription.Type", unsafe.Offsetof(goCfg.Type), unsafe.Offsetof(cCfg.type_))
		checkOffsetEqual(t, "ShimSessionDescription.SDP", unsafe.Offsetof(goCfg.SDP), unsafe.Offsetof(cCfg.sdp))
	})

	t.Run("ShimICECandidate", func(t *testing.T) {
		var goCfg shimICECandidate
		var cCfg cShimICECandidate
		checkSizeEqual(t, "ShimICECandidate", unsafe.Sizeof(goCfg), unsafe.Sizeof(cCfg))
		checkOffsetEqual(t, "ShimICECandidate.Candidate", unsafe.Offsetof(goCfg.Candidate), unsafe.Offsetof(cCfg.candidate))
		checkOffsetEqual(t, "ShimICECandidate.SDPMid", unsafe.Offsetof(goCfg.SDPMid), unsafe.Offsetof(cCfg.sdp_mid))
		checkOffsetEqual(t, "ShimICECandidate.SDPMLineIndex", unsafe.Offsetof(goCfg.SDPMLineIndex), unsafe.Offsetof(cCfg.sdp_mline_index))
	})

	t.Run("ShimRTPEncodingParameters", func(t *testing.T) {
		var goCfg RTPEncodingParameters
		var cCfg cShimRTPEncodingParameters
		checkSizeEqual(t, "ShimRTPEncodingParameters", unsafe.Sizeof(goCfg), unsafe.Sizeof(cCfg))
		checkOffsetEqual(t, "ShimRTPEncodingParameters.RID", unsafe.Offsetof(goCfg.RID), unsafe.Offsetof(cCfg.rid))
		checkOffsetEqual(t, "ShimRTPEncodingParameters.MaxBitrateBps", unsafe.Offsetof(goCfg.MaxBitrateBps), unsafe.Offsetof(cCfg.max_bitrate_bps))
		checkOffsetEqual(t, "ShimRTPEncodingParameters.MinBitrateBps", unsafe.Offsetof(goCfg.MinBitrateBps), unsafe.Offsetof(cCfg.min_bitrate_bps))
		checkOffsetEqual(t, "ShimRTPEncodingParameters.MaxFramerate", unsafe.Offsetof(goCfg.MaxFramerate), unsafe.Offsetof(cCfg.max_framerate))
		checkOffsetEqual(t, "ShimRTPEncodingParameters.ScaleResolutionDownBy", unsafe.Offsetof(goCfg.ScaleResolutionDownBy), unsafe.Offsetof(cCfg.scale_resolution_down_by))
		checkOffsetEqual(t, "ShimRTPEncodingParameters.Active", unsafe.Offsetof(goCfg.Active), unsafe.Offsetof(cCfg.active))
		checkOffsetEqual(t, "ShimRTPEncodingParameters.ScalabilityMode", unsafe.Offsetof(goCfg.ScalabilityMode), unsafe.Offsetof(cCfg.scalability_mode))
	})

	t.Run("ShimRTPSendParameters", func(t *testing.T) {
		var goCfg RTPSendParameters
		var cCfg cShimRTPSendParameters
		checkSizeEqual(t, "ShimRTPSendParameters", unsafe.Sizeof(goCfg), unsafe.Sizeof(cCfg))
		checkOffsetEqual(t, "ShimRTPSendParameters.Encodings", unsafe.Offsetof(goCfg.Encodings), unsafe.Offsetof(cCfg.encodings))
		checkOffsetEqual(t, "ShimRTPSendParameters.EncodingCount", unsafe.Offsetof(goCfg.EncodingCount), unsafe.Offsetof(cCfg.encoding_count))
		checkOffsetEqual(t, "ShimRTPSendParameters.TransactionID", unsafe.Offsetof(goCfg.TransactionID), unsafe.Offsetof(cCfg.transaction_id))
	})

	t.Run("ShimRTCStats", func(t *testing.T) {
		var goCfg RTCStats
		var cCfg cShimRTCStats
		checkSizeEqual(t, "ShimRTCStats", unsafe.Sizeof(goCfg), unsafe.Sizeof(cCfg))
		checkOffsetEqual(t, "ShimRTCStats.TimestampUs", unsafe.Offsetof(goCfg.TimestampUs), unsafe.Offsetof(cCfg.timestamp_us))
		checkOffsetEqual(t, "ShimRTCStats.BytesSent", unsafe.Offsetof(goCfg.BytesSent), unsafe.Offsetof(cCfg.bytes_sent))
		checkOffsetEqual(t, "ShimRTCStats.BytesReceived", unsafe.Offsetof(goCfg.BytesReceived), unsafe.Offsetof(cCfg.bytes_received))
		checkOffsetEqual(t, "ShimRTCStats.PacketsSent", unsafe.Offsetof(goCfg.PacketsSent), unsafe.Offsetof(cCfg.packets_sent))
		checkOffsetEqual(t, "ShimRTCStats.PacketsReceived", unsafe.Offsetof(goCfg.PacketsReceived), unsafe.Offsetof(cCfg.packets_received))
		checkOffsetEqual(t, "ShimRTCStats.PacketsLost", unsafe.Offsetof(goCfg.PacketsLost), unsafe.Offsetof(cCfg.packets_lost))
		checkOffsetEqual(t, "ShimRTCStats.RoundTripTimeMs", unsafe.Offsetof(goCfg.RoundTripTimeMs), unsafe.Offsetof(cCfg.round_trip_time_ms))
		checkOffsetEqual(t, "ShimRTCStats.JitterMs", unsafe.Offsetof(goCfg.JitterMs), unsafe.Offsetof(cCfg.jitter_ms))
		checkOffsetEqual(t, "ShimRTCStats.AvailableOutgoingBitrate", unsafe.Offsetof(goCfg.AvailableOutgoingBitrate), unsafe.Offsetof(cCfg.available_outgoing_bitrate))
		checkOffsetEqual(t, "ShimRTCStats.AvailableIncomingBitrate", unsafe.Offsetof(goCfg.AvailableIncomingBitrate), unsafe.Offsetof(cCfg.available_incoming_bitrate))
		checkOffsetEqual(t, "ShimRTCStats.CurrentRTTMs", unsafe.Offsetof(goCfg.CurrentRTTMs), unsafe.Offsetof(cCfg.current_rtt_ms))
		checkOffsetEqual(t, "ShimRTCStats.TotalRTTMs", unsafe.Offsetof(goCfg.TotalRTTMs), unsafe.Offsetof(cCfg.total_rtt_ms))
		checkOffsetEqual(t, "ShimRTCStats.ResponsesReceived", unsafe.Offsetof(goCfg.ResponsesReceived), unsafe.Offsetof(cCfg.responses_received))
		checkOffsetEqual(t, "ShimRTCStats.FramesEncoded", unsafe.Offsetof(goCfg.FramesEncoded), unsafe.Offsetof(cCfg.frames_encoded))
		checkOffsetEqual(t, "ShimRTCStats.FramesDecoded", unsafe.Offsetof(goCfg.FramesDecoded), unsafe.Offsetof(cCfg.frames_decoded))
		checkOffsetEqual(t, "ShimRTCStats.FramesDropped", unsafe.Offsetof(goCfg.FramesDropped), unsafe.Offsetof(cCfg.frames_dropped))
		checkOffsetEqual(t, "ShimRTCStats.KeyFramesEncoded", unsafe.Offsetof(goCfg.KeyFramesEncoded), unsafe.Offsetof(cCfg.key_frames_encoded))
		checkOffsetEqual(t, "ShimRTCStats.KeyFramesDecoded", unsafe.Offsetof(goCfg.KeyFramesDecoded), unsafe.Offsetof(cCfg.key_frames_decoded))
		checkOffsetEqual(t, "ShimRTCStats.NACKCount", unsafe.Offsetof(goCfg.NACKCount), unsafe.Offsetof(cCfg.nack_count))
		checkOffsetEqual(t, "ShimRTCStats.PLICount", unsafe.Offsetof(goCfg.PLICount), unsafe.Offsetof(cCfg.pli_count))
		checkOffsetEqual(t, "ShimRTCStats.FIRCount", unsafe.Offsetof(goCfg.FIRCount), unsafe.Offsetof(cCfg.fir_count))
		checkOffsetEqual(t, "ShimRTCStats.QPSum", unsafe.Offsetof(goCfg.QPSum), unsafe.Offsetof(cCfg.qp_sum))
		checkOffsetEqual(t, "ShimRTCStats.AudioLevel", unsafe.Offsetof(goCfg.AudioLevel), unsafe.Offsetof(cCfg.audio_level))
		checkOffsetEqual(t, "ShimRTCStats.TotalAudioEnergy", unsafe.Offsetof(goCfg.TotalAudioEnergy), unsafe.Offsetof(cCfg.total_audio_energy))
		checkOffsetEqual(t, "ShimRTCStats.ConcealmentEvents", unsafe.Offsetof(goCfg.ConcealmentEvents), unsafe.Offsetof(cCfg.concealment_events))
		checkOffsetEqual(t, "ShimRTCStats.DataChannelsOpened", unsafe.Offsetof(goCfg.DataChannelsOpened), unsafe.Offsetof(cCfg.data_channels_opened))
		checkOffsetEqual(t, "ShimRTCStats.DataChannelsClosed", unsafe.Offsetof(goCfg.DataChannelsClosed), unsafe.Offsetof(cCfg.data_channels_closed))
		checkOffsetEqual(t, "ShimRTCStats.MessagesSent", unsafe.Offsetof(goCfg.MessagesSent), unsafe.Offsetof(cCfg.messages_sent))
		checkOffsetEqual(t, "ShimRTCStats.MessagesReceived", unsafe.Offsetof(goCfg.MessagesReceived), unsafe.Offsetof(cCfg.messages_received))
		checkOffsetEqual(t, "ShimRTCStats.BytesSentDataChannel", unsafe.Offsetof(goCfg.BytesSentDataChannel), unsafe.Offsetof(cCfg.bytes_sent_data_channel))
		checkOffsetEqual(t, "ShimRTCStats.BytesReceivedDataChannel", unsafe.Offsetof(goCfg.BytesReceivedDataChannel), unsafe.Offsetof(cCfg.bytes_received_data_channel))
		checkOffsetEqual(t, "ShimRTCStats.QualityLimitationReason", unsafe.Offsetof(goCfg.QualityLimitationReason), unsafe.Offsetof(cCfg.quality_limitation_reason))
		checkOffsetEqual(t, "ShimRTCStats.QualityLimitationDurationMs", unsafe.Offsetof(goCfg.QualityLimitationDurationMs), unsafe.Offsetof(cCfg.quality_limitation_duration_ms))
		checkOffsetEqual(t, "ShimRTCStats.RemotePacketsLost", unsafe.Offsetof(goCfg.RemotePacketsLost), unsafe.Offsetof(cCfg.remote_packets_lost))
		checkOffsetEqual(t, "ShimRTCStats.RemoteJitterMs", unsafe.Offsetof(goCfg.RemoteJitterMs), unsafe.Offsetof(cCfg.remote_jitter_ms))
		checkOffsetEqual(t, "ShimRTCStats.RemoteRoundTripTimeMs", unsafe.Offsetof(goCfg.RemoteRoundTripTimeMs), unsafe.Offsetof(cCfg.remote_round_trip_time_ms))
		checkOffsetEqual(t, "ShimRTCStats.JitterBufferDelayMs", unsafe.Offsetof(goCfg.JitterBufferDelayMs), unsafe.Offsetof(cCfg.jitter_buffer_delay_ms))
		checkOffsetEqual(t, "ShimRTCStats.JitterBufferTargetDelayMs", unsafe.Offsetof(goCfg.JitterBufferTargetDelayMs), unsafe.Offsetof(cCfg.jitter_buffer_target_delay_ms))
		checkOffsetEqual(t, "ShimRTCStats.JitterBufferMinimumDelayMs", unsafe.Offsetof(goCfg.JitterBufferMinimumDelayMs), unsafe.Offsetof(cCfg.jitter_buffer_minimum_delay_ms))
		checkOffsetEqual(t, "ShimRTCStats.JitterBufferEmittedCount", unsafe.Offsetof(goCfg.JitterBufferEmittedCount), unsafe.Offsetof(cCfg.jitter_buffer_emitted_count))
	})

	t.Run("ShimCodecCapability", func(t *testing.T) {
		var goCfg CodecCapability
		var cCfg cShimCodecCapability
		checkSizeEqual(t, "ShimCodecCapability", unsafe.Sizeof(goCfg), unsafe.Sizeof(cCfg))
		checkOffsetEqual(t, "ShimCodecCapability.MimeType", unsafe.Offsetof(goCfg.MimeType), unsafe.Offsetof(cCfg.mime_type))
		checkOffsetEqual(t, "ShimCodecCapability.ClockRate", unsafe.Offsetof(goCfg.ClockRate), unsafe.Offsetof(cCfg.clock_rate))
		checkOffsetEqual(t, "ShimCodecCapability.Channels", unsafe.Offsetof(goCfg.Channels), unsafe.Offsetof(cCfg.channels))
		checkOffsetEqual(t, "ShimCodecCapability.SDPFmtpLine", unsafe.Offsetof(goCfg.SDPFmtpLine), unsafe.Offsetof(cCfg.sdp_fmtp_line))
		checkOffsetEqual(t, "ShimCodecCapability.PayloadType", unsafe.Offsetof(goCfg.PayloadType), unsafe.Offsetof(cCfg.payload_type))
	})

	t.Run("ShimBandwidthEstimate", func(t *testing.T) {
		var goCfg BandwidthEstimate
		var cCfg cShimBandwidthEstimate
		checkSizeEqual(t, "ShimBandwidthEstimate", unsafe.Sizeof(goCfg), unsafe.Sizeof(cCfg))
		checkOffsetEqual(t, "ShimBandwidthEstimate.TimestampUs", unsafe.Offsetof(goCfg.TimestampUs), unsafe.Offsetof(cCfg.timestamp_us))
		checkOffsetEqual(t, "ShimBandwidthEstimate.TargetBitrateBps", unsafe.Offsetof(goCfg.TargetBitrateBps), unsafe.Offsetof(cCfg.target_bitrate_bps))
		checkOffsetEqual(t, "ShimBandwidthEstimate.AvailableSendBps", unsafe.Offsetof(goCfg.AvailableSendBps), unsafe.Offsetof(cCfg.available_send_bps))
		checkOffsetEqual(t, "ShimBandwidthEstimate.AvailableRecvBps", unsafe.Offsetof(goCfg.AvailableRecvBps), unsafe.Offsetof(cCfg.available_recv_bps))
		checkOffsetEqual(t, "ShimBandwidthEstimate.PacingRateBps", unsafe.Offsetof(goCfg.PacingRateBps), unsafe.Offsetof(cCfg.pacing_rate_bps))
		checkOffsetEqual(t, "ShimBandwidthEstimate.CongestionWindow", unsafe.Offsetof(goCfg.CongestionWindow), unsafe.Offsetof(cCfg.congestion_window))
		checkOffsetEqual(t, "ShimBandwidthEstimate.LossRate", unsafe.Offsetof(goCfg.LossRate), unsafe.Offsetof(cCfg.loss_rate))
	})

	t.Run("ShimDeviceInfo", func(t *testing.T) {
		var goCfg shimDeviceInfo
		var cCfg cShimDeviceInfo
		checkSizeEqual(t, "ShimDeviceInfo", unsafe.Sizeof(goCfg), unsafe.Sizeof(cCfg))
		checkOffsetEqual(t, "ShimDeviceInfo.DeviceID", unsafe.Offsetof(goCfg.deviceID), unsafe.Offsetof(cCfg.device_id))
		checkOffsetEqual(t, "ShimDeviceInfo.Label", unsafe.Offsetof(goCfg.label), unsafe.Offsetof(cCfg.label))
		checkOffsetEqual(t, "ShimDeviceInfo.Kind", unsafe.Offsetof(goCfg.kind), unsafe.Offsetof(cCfg.kind))
	})

	t.Run("ShimScreenInfo", func(t *testing.T) {
		var goCfg shimScreenInfo
		var cCfg cShimScreenInfo
		checkSizeEqual(t, "ShimScreenInfo", unsafe.Sizeof(goCfg), unsafe.Sizeof(cCfg))
		checkOffsetEqual(t, "ShimScreenInfo.ID", unsafe.Offsetof(goCfg.id), unsafe.Offsetof(cCfg.id))
		checkOffsetEqual(t, "ShimScreenInfo.Title", unsafe.Offsetof(goCfg.title), unsafe.Offsetof(cCfg.title))
		checkOffsetEqual(t, "ShimScreenInfo.IsWindow", unsafe.Offsetof(goCfg.isWindow), unsafe.Offsetof(cCfg.is_window))
	})
}

func checkSizeEqual(t *testing.T, name string, goSize, cSize uintptr) {
	t.Helper()
	if goSize != cSize {
		t.Errorf("%s size = %d, want %d", name, goSize, cSize)
	}
}

func checkOffsetEqual(t *testing.T, name string, goOffset, cOffset uintptr) {
	t.Helper()
	if goOffset != cOffset {
		t.Errorf("%s offset = %d, want %d", name, goOffset, cOffset)
	}
}
