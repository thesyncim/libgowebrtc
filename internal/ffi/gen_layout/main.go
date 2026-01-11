//go:build ignore

// Code generator for C/Go struct layout tests.
//
// Usage: go run main.go
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
)

type fieldSpec struct {
	CName string
	GoName string
}

type structSpec struct {
	CName  string
	GoType string
	Fields []fieldSpec
}

var structSpecs = []structSpec{
	{
		CName:  "ShimErrorBuffer",
		GoType: "ShimErrorBuffer",
		Fields: []fieldSpec{
			{CName: "message", GoName: "Message"},
		},
	},
	{
		CName:  "ShimVideoEncoderConfig",
		GoType: "VideoEncoderConfig",
		Fields: []fieldSpec{
			{CName: "width", GoName: "Width"},
			{CName: "height", GoName: "Height"},
			{CName: "bitrate_bps", GoName: "BitrateBps"},
			{CName: "framerate", GoName: "Framerate"},
			{CName: "keyframe_interval", GoName: "KeyframeInterval"},
			{CName: "h264_profile", GoName: "H264Profile"},
			{CName: "vp9_profile", GoName: "VP9Profile"},
			{CName: "prefer_hw", GoName: "PreferHW"},
		},
	},
	{
		CName:  "ShimAudioEncoderConfig",
		GoType: "AudioEncoderConfig",
		Fields: []fieldSpec{
			{CName: "sample_rate", GoName: "SampleRate"},
			{CName: "channels", GoName: "Channels"},
			{CName: "bitrate_bps", GoName: "BitrateBps"},
		},
	},
	{
		CName:  "ShimPacketizerConfig",
		GoType: "PacketizerConfig",
		Fields: []fieldSpec{
			{CName: "codec", GoName: "Codec"},
			{CName: "ssrc", GoName: "SSRC"},
			{CName: "payload_type", GoName: "PayloadType"},
			{CName: "mtu", GoName: "MTU"},
			{CName: "clock_rate", GoName: "ClockRate"},
		},
	},
	{
		CName:  "ShimICEServer",
		GoType: "ICEServerConfig",
		Fields: []fieldSpec{
			{CName: "urls", GoName: "URLs"},
			{CName: "url_count", GoName: "URLCount"},
			{CName: "username", GoName: "Username"},
			{CName: "credential", GoName: "Credential"},
		},
	},
	{
		CName:  "ShimPeerConnectionConfig",
		GoType: "PeerConnectionConfig",
		Fields: []fieldSpec{
			{CName: "ice_servers", GoName: "ICEServers"},
			{CName: "ice_server_count", GoName: "ICEServerCount"},
			{CName: "ice_candidate_pool_size", GoName: "ICECandidatePoolSize"},
			{CName: "bundle_policy", GoName: "BundlePolicy"},
			{CName: "rtcp_mux_policy", GoName: "RTCPMuxPolicy"},
			{CName: "sdp_semantics", GoName: "SDPSemantics"},
		},
	},
	{
		CName:  "ShimSessionDescription",
		GoType: "shimSessionDescription",
		Fields: []fieldSpec{
			{CName: "type", GoName: "Type"},
			{CName: "sdp", GoName: "SDP"},
		},
	},
	{
		CName:  "ShimICECandidate",
		GoType: "shimICECandidate",
		Fields: []fieldSpec{
			{CName: "candidate", GoName: "Candidate"},
			{CName: "sdp_mid", GoName: "SDPMid"},
			{CName: "sdp_mline_index", GoName: "SDPMLineIndex"},
		},
	},
	{
		CName:  "ShimRTPEncodingParameters",
		GoType: "RTPEncodingParameters",
		Fields: []fieldSpec{
			{CName: "rid", GoName: "RID"},
			{CName: "max_bitrate_bps", GoName: "MaxBitrateBps"},
			{CName: "min_bitrate_bps", GoName: "MinBitrateBps"},
			{CName: "max_framerate", GoName: "MaxFramerate"},
			{CName: "scale_resolution_down_by", GoName: "ScaleResolutionDownBy"},
			{CName: "active", GoName: "Active"},
			{CName: "scalability_mode", GoName: "ScalabilityMode"},
		},
	},
	{
		CName:  "ShimRTPSendParameters",
		GoType: "RTPSendParameters",
		Fields: []fieldSpec{
			{CName: "encodings", GoName: "Encodings"},
			{CName: "encoding_count", GoName: "EncodingCount"},
			{CName: "transaction_id", GoName: "TransactionID"},
		},
	},
	{
		CName:  "ShimRTCStats",
		GoType: "RTCStats",
		Fields: []fieldSpec{
			{CName: "timestamp_us", GoName: "TimestampUs"},
			{CName: "bytes_sent", GoName: "BytesSent"},
			{CName: "bytes_received", GoName: "BytesReceived"},
			{CName: "packets_sent", GoName: "PacketsSent"},
			{CName: "packets_received", GoName: "PacketsReceived"},
			{CName: "packets_lost", GoName: "PacketsLost"},
			{CName: "round_trip_time_ms", GoName: "RoundTripTimeMs"},
			{CName: "jitter_ms", GoName: "JitterMs"},
			{CName: "available_outgoing_bitrate", GoName: "AvailableOutgoingBitrate"},
			{CName: "available_incoming_bitrate", GoName: "AvailableIncomingBitrate"},
			{CName: "current_rtt_ms", GoName: "CurrentRTTMs"},
			{CName: "total_rtt_ms", GoName: "TotalRTTMs"},
			{CName: "responses_received", GoName: "ResponsesReceived"},
			{CName: "frames_encoded", GoName: "FramesEncoded"},
			{CName: "frames_decoded", GoName: "FramesDecoded"},
			{CName: "frames_dropped", GoName: "FramesDropped"},
			{CName: "key_frames_encoded", GoName: "KeyFramesEncoded"},
			{CName: "key_frames_decoded", GoName: "KeyFramesDecoded"},
			{CName: "nack_count", GoName: "NACKCount"},
			{CName: "pli_count", GoName: "PLICount"},
			{CName: "fir_count", GoName: "FIRCount"},
			{CName: "qp_sum", GoName: "QPSum"},
			{CName: "audio_level", GoName: "AudioLevel"},
			{CName: "total_audio_energy", GoName: "TotalAudioEnergy"},
			{CName: "concealment_events", GoName: "ConcealmentEvents"},
			{CName: "data_channels_opened", GoName: "DataChannelsOpened"},
			{CName: "data_channels_closed", GoName: "DataChannelsClosed"},
			{CName: "messages_sent", GoName: "MessagesSent"},
			{CName: "messages_received", GoName: "MessagesReceived"},
			{CName: "bytes_sent_data_channel", GoName: "BytesSentDataChannel"},
			{CName: "bytes_received_data_channel", GoName: "BytesReceivedDataChannel"},
			{CName: "quality_limitation_reason", GoName: "QualityLimitationReason"},
			{CName: "quality_limitation_duration_ms", GoName: "QualityLimitationDurationMs"},
			{CName: "remote_packets_lost", GoName: "RemotePacketsLost"},
			{CName: "remote_jitter_ms", GoName: "RemoteJitterMs"},
			{CName: "remote_round_trip_time_ms", GoName: "RemoteRoundTripTimeMs"},
			{CName: "jitter_buffer_delay_ms", GoName: "JitterBufferDelayMs"},
			{CName: "jitter_buffer_target_delay_ms", GoName: "JitterBufferTargetDelayMs"},
			{CName: "jitter_buffer_minimum_delay_ms", GoName: "JitterBufferMinimumDelayMs"},
			{CName: "jitter_buffer_emitted_count", GoName: "JitterBufferEmittedCount"},
		},
	},
	{
		CName:  "ShimCodecCapability",
		GoType: "CodecCapability",
		Fields: []fieldSpec{
			{CName: "mime_type", GoName: "MimeType"},
			{CName: "clock_rate", GoName: "ClockRate"},
			{CName: "channels", GoName: "Channels"},
			{CName: "sdp_fmtp_line", GoName: "SDPFmtpLine"},
			{CName: "payload_type", GoName: "PayloadType"},
		},
	},
	{
		CName:  "ShimBandwidthEstimate",
		GoType: "BandwidthEstimate",
		Fields: []fieldSpec{
			{CName: "timestamp_us", GoName: "TimestampUs"},
			{CName: "target_bitrate_bps", GoName: "TargetBitrateBps"},
			{CName: "available_send_bps", GoName: "AvailableSendBps"},
			{CName: "available_recv_bps", GoName: "AvailableRecvBps"},
			{CName: "pacing_rate_bps", GoName: "PacingRateBps"},
			{CName: "congestion_window", GoName: "CongestionWindow"},
			{CName: "loss_rate", GoName: "LossRate"},
		},
	},
	{
		CName:  "ShimDeviceInfo",
		GoType: "shimDeviceInfo",
		Fields: []fieldSpec{
			{CName: "device_id", GoName: "deviceID"},
			{CName: "label", GoName: "label"},
			{CName: "kind", GoName: "kind"},
		},
	},
	{
		CName:  "ShimScreenInfo",
		GoType: "shimScreenInfo",
		Fields: []fieldSpec{
			{CName: "id", GoName: "id"},
			{CName: "title", GoName: "title"},
			{CName: "is_window", GoName: "isWindow"},
		},
	},
}

var testLocalTypes = map[string]string{
	"shimSessionDescription": "type shimSessionDescription struct {\n\tType int32\n\tSDP  *byte\n}\n",
	"shimICECandidate": "type shimICECandidate struct {\n\tCandidate     *byte\n\tSDPMid        *byte\n\tSDPMLineIndex int32\n}\n",
}

func main() {
	outDir := ".."
	if err := writeGoFile(filepath.Join(outDir, "struct_layout_cgo.go"), generateLayoutGo(structSpecs)); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating struct_layout_cgo.go: %v\n", err)
		os.Exit(1)
	}
	if err := writeGoFile(filepath.Join(outDir, "struct_layout_cgo_test.go"), generateLayoutTestGo(structSpecs)); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating struct_layout_cgo_test.go: %v\n", err)
		os.Exit(1)
	}
}

func generateLayoutGo(specs []structSpec) []byte {
	var buf bytes.Buffer

	buf.WriteString(`// Code generated by go generate; DO NOT EDIT.

//go:build ffigo_cgo

package ffi

/*
#cgo CFLAGS: -I${SRCDIR}/../../shim
#include "shim.h"
*/
import "C"

import "unsafe"

type cStructLayout struct {
	size    uintptr
	offsets map[string]uintptr
}

`)

	for _, spec := range specs {
		fmt.Fprintf(&buf, "func %s() cStructLayout {\n", layoutFuncName(spec.CName))
		fmt.Fprintf(&buf, "\tvar cCfg C.%s\n", spec.CName)
		buf.WriteString("\treturn cStructLayout{\n")
		buf.WriteString("\t\tsize: unsafe.Sizeof(cCfg),\n")
		buf.WriteString("\t\toffsets: map[string]uintptr{\n")
		for _, field := range spec.Fields {
			fmt.Fprintf(&buf, "\t\t\t%q: unsafe.Offsetof(cCfg.%s),\n", field.GoName, cgoFieldName(field.CName))
		}
		buf.WriteString("\t\t},\n")
		buf.WriteString("\t}\n")
		buf.WriteString("}\n\n")
	}

	return buf.Bytes()
}

func generateLayoutTestGo(specs []structSpec) []byte {
	var buf bytes.Buffer

	buf.WriteString(`// Code generated by go generate; DO NOT EDIT.

//go:build ffigo_cgo

package ffi

import (
	"testing"
	"unsafe"
)

`)

	usedLocalTypes := collectLocalTypes(specs)
	for _, typeName := range usedLocalTypes {
		buf.WriteString(testLocalTypes[typeName])
		buf.WriteString("\n")
	}

	buf.WriteString("// TestShimStructLayoutCgo compares Go struct layouts against the C shim headers.\n")
	buf.WriteString("func TestShimStructLayoutCgo(t *testing.T) {\n")
	for _, spec := range specs {
		fmt.Fprintf(&buf, "\tt.Run(%q, func(t *testing.T) {\n", spec.CName)
		fmt.Fprintf(&buf, "\t\tvar goCfg %s\n", spec.GoType)
		fmt.Fprintf(&buf, "\t\tlayout := %s()\n", layoutFuncName(spec.CName))
		fmt.Fprintf(&buf, "\t\tcheckSizeEqual(t, %q, unsafe.Sizeof(goCfg), layout.size)\n", spec.CName)
		for _, field := range spec.Fields {
			fmt.Fprintf(&buf, "\t\tcheckOffsetEqual(t, %q, unsafe.Offsetof(goCfg.%s), layout.offsets[%q])\n",
				fmt.Sprintf("%s.%s", spec.CName, field.GoName), field.GoName, field.GoName)
		}
		buf.WriteString("\t})\n\n")
	}
	buf.WriteString("}\n\n")
	buf.WriteString("func checkSizeEqual(t *testing.T, name string, goSize, cSize uintptr) {\n")
	buf.WriteString("\tt.Helper()\n")
	buf.WriteString("\tif goSize != cSize {\n")
	buf.WriteString("\t\tt.Errorf(\"%s size = %d, want %d\", name, goSize, cSize)\n")
	buf.WriteString("\t}\n")
	buf.WriteString("}\n\n")
	buf.WriteString("func checkOffsetEqual(t *testing.T, name string, goOffset, cOffset uintptr) {\n")
	buf.WriteString("\tt.Helper()\n")
	buf.WriteString("\tif goOffset != cOffset {\n")
	buf.WriteString("\t\tt.Errorf(\"%s offset = %d, want %d\", name, goOffset, cOffset)\n")
	buf.WriteString("\t}\n")
	buf.WriteString("}\n")

	return buf.Bytes()
}

func layoutFuncName(cName string) string {
	return "c" + cName + "Layout"
}

func cgoFieldName(cName string) string {
	if cName == "type" {
		return "_type"
	}
	return cName
}

func collectLocalTypes(specs []structSpec) []string {
	seen := make(map[string]struct{})
	for _, spec := range specs {
		if _, ok := testLocalTypes[spec.GoType]; ok {
			seen[spec.GoType] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func writeGoFile(path string, data []byte) error {
	formatted, err := format.Source(data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, formatted, 0644)
}
