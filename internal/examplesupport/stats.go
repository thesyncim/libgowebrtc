package examplesupport

import "github.com/pion/webrtc/v4"

// PeerConnectionStatsSummary contains the small set of demo-friendly counters
// that the examples surface in logs or simple JSON payloads.
type PeerConnectionStatsSummary struct {
	FramesEncoded   uint32  `json:"framesEncoded,omitempty"`
	BytesSent       uint64  `json:"bytesSent,omitempty"`
	PacketsSent     uint32  `json:"packetsSent,omitempty"`
	RoundTripTimeMs float64 `json:"roundTripTimeMs,omitempty"`
	PacketsLost     int32   `json:"packetsLost,omitempty"`
}

// SummarizePeerConnectionStats collapses a full WebRTC stats report into a
// small aggregate suitable for the example programs.
func SummarizePeerConnectionStats(report webrtc.StatsReport) *PeerConnectionStatsSummary {
	if len(report) == 0 {
		return nil
	}

	summary := &PeerConnectionStatsSummary{}
	var have bool

	for _, stats := range report {
		switch s := stats.(type) {
		case webrtc.OutboundRTPStreamStats:
			summary.FramesEncoded += s.FramesEncoded
			summary.BytesSent += s.BytesSent
			summary.PacketsSent += s.PacketsSent
			have = true
		case webrtc.RemoteInboundRTPStreamStats:
			if ms := s.RoundTripTime * 1000; ms > summary.RoundTripTimeMs {
				summary.RoundTripTimeMs = ms
			}
			summary.PacketsLost += s.PacketsLost
			have = true
		}
	}

	if !have {
		return nil
	}
	return summary
}
