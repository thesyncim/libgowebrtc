package validate

import (
	"time"

	"github.com/pion/webrtc/v4"
)

func (s *Session) applyStatsReportLocked(report webrtc.StatsReport) {
	remoteInbound := make(map[string]webrtc.RemoteInboundRTPStreamStats)
	candidatePairs := make(map[string]webrtc.ICECandidatePairStats)

	for _, stats := range report {
		switch typed := stats.(type) {
		case webrtc.RemoteInboundRTPStreamStats:
			remoteInbound[typed.ID] = typed
		case webrtc.ICECandidatePairStats:
			candidatePairs[typed.ID] = typed
		}
	}

	for _, stats := range report {
		switch typed := stats.(type) {
		case webrtc.TransportStats:
			sample := TransportSample{
				At:                      typed.Timestamp.Time(),
				PacketsSent:             uint64(typed.PacketsSent),
				PacketsReceived:         uint64(typed.PacketsReceived),
				BytesSent:               typed.BytesSent,
				BytesReceived:           typed.BytesReceived,
				SelectedCandidatePairID: typed.SelectedCandidatePairID,
				ICEState:                typed.ICEState.String(),
				DTLSState:               typed.DTLSState.String(),
			}
			if pair, ok := candidatePairs[typed.SelectedCandidatePairID]; ok {
				sample.CurrentRoundTripTime = durationSeconds(pair.CurrentRoundTripTime)
				sample.AvailableOutgoingBitrate = pair.AvailableOutgoingBitrate
				sample.AvailableIncomingBitrate = pair.AvailableIncomingBitrate
			}
			s.transportSamples = appendLimited(s.transportSamples, sample, s.cfg.EventHistory)

		case webrtc.InboundRTPStreamStats:
			s.matchRTPStatsLocked(inboundSample(typed), "")

		case webrtc.OutboundRTPStreamStats:
			sample := outboundSample(typed)
			if remote, ok := remoteInbound[typed.RemoteID]; ok {
				sample.RemoteRoundTripTime = durationSeconds(remote.RoundTripTime)
				sample.FractionLost = remote.FractionLost
			}
			s.matchRTPStatsLocked(sample, typed.RemoteID)

		case webrtc.DataChannelStats:
			key := dataChannelKey(typed.Label, int(typed.DataChannelIdentifier))
			state, ok := s.dataChannels[key]
			if !ok {
				continue
			}
			state.stats = appendLimited(state.stats, DataChannelStatsSample{
				At:               typed.Timestamp.Time(),
				Label:            typed.Label,
				ID:               int(typed.DataChannelIdentifier),
				State:            typed.State,
				MessagesSent:     uint64(typed.MessagesSent),
				MessagesReceived: uint64(typed.MessagesReceived),
				BytesSent:        typed.BytesSent,
				BytesReceived:    typed.BytesReceived,
				TransportID:      typed.TransportID,
			}, s.cfg.EventHistory)
			if state.state == "" {
				state.state = typed.State.String()
			}
		}
	}
}

func (s *Session) matchRTPStatsLocked(sample RTPStatsSample, remoteID string) {
	if state := s.matchVideoTrackLocked(sample); state != nil {
		state.stats = appendLimited(state.stats, sample, s.cfg.EventHistory)
		return
	}
	if state := s.matchAudioTrackLocked(sample); state != nil {
		state.stats = appendLimited(state.stats, sample, s.cfg.EventHistory)
		return
	}
	s.unmatchedStreams = appendLimited(s.unmatchedStreams, sample, s.cfg.EventHistory)
}

func (s *Session) matchVideoTrackLocked(sample RTPStatsSample) *videoTrackState {
	for _, state := range s.videoTracks {
		if state.ssrc != 0 && state.ssrc == sample.SSRC {
			return state
		}
		if sample.RID != "" && state.rid != "" && sample.RID == state.rid {
			return state
		}
		if state.monitor != nil {
			wire := state.monitor.Snapshot()
			if sample.RID != "" && wire.CurrentRID != "" && sample.RID == wire.CurrentRID {
				return state
			}
			if sample.MID != "" && wire.CurrentMID != "" && sample.MID == wire.CurrentMID {
				return state
			}
		}
	}
	return nil
}

func (s *Session) matchAudioTrackLocked(sample RTPStatsSample) *audioTrackState {
	for _, state := range s.audioTracks {
		if state.ssrc != 0 && state.ssrc == sample.SSRC {
			return state
		}
		if sample.RID != "" && state.rid != "" && sample.RID == state.rid {
			return state
		}
	}
	return nil
}

func inboundSample(stats webrtc.InboundRTPStreamStats) RTPStatsSample {
	return RTPStatsSample{
		At:                       stats.Timestamp.Time(),
		Kind:                     stats.Kind,
		Direction:                "inbound",
		MID:                      stats.Mid,
		TrackID:                  stats.TrackID,
		SSRC:                     uint32(stats.SSRC),
		CodecID:                  stats.CodecID,
		TransportID:              stats.TransportID,
		Packets:                  uint64(stats.PacketsReceived),
		PacketsLost:              int64(stats.PacketsLost),
		PacketsDiscarded:         uint64(stats.PacketsDiscarded),
		Bytes:                    stats.BytesReceived,
		Jitter:                   stats.Jitter,
		NACKCount:                stats.NACKCount,
		PLICount:                 stats.PLICount,
		FIRCount:                 stats.FIRCount,
		Frames:                   uint64(stats.FramesDecoded),
		KeyFrames:                uint64(stats.KeyFramesDecoded),
		FrameWidth:               int(stats.FrameWidth),
		FrameHeight:              int(stats.FrameHeight),
		AudioLevel:               stats.AudioLevel,
		TotalAudioEnergy:         stats.TotalAudioEnergy,
		ConcealedSamples:         stats.ConcealedSamples,
		ConcealmentEvents:        stats.ConcealmentEvents,
		JitterBufferDelay:        durationSeconds(stats.JitterBufferDelay),
		JitterBufferTargetDelay:  durationSeconds(stats.JitterBufferTargetDelay),
		JitterBufferMinimumDelay: durationSeconds(stats.JitterBufferMinimumDelay),
	}
}

func outboundSample(stats webrtc.OutboundRTPStreamStats) RTPStatsSample {
	return RTPStatsSample{
		At:                      stats.Timestamp.Time(),
		Kind:                    stats.Kind,
		Direction:               "outbound",
		MID:                     stats.Mid,
		RID:                     stats.Rid,
		TrackID:                 stats.TrackID,
		SSRC:                    uint32(stats.SSRC),
		CodecID:                 stats.CodecID,
		TransportID:             stats.TransportID,
		Packets:                 uint64(stats.PacketsSent),
		Bytes:                   stats.BytesSent,
		NACKCount:               stats.NACKCount,
		PLICount:                stats.PLICount,
		FIRCount:                stats.FIRCount,
		Frames:                  uint64(stats.FramesEncoded),
		KeyFrames:               uint64(stats.KeyFramesEncoded),
		FrameWidth:              int(stats.FrameWidth),
		FrameHeight:             int(stats.FrameHeight),
		FramesPerSecond:         stats.FramesPerSecond,
		QualityLimitationReason: string(stats.QualityLimitationReason),
		ScalabilityMode:         stats.ScalabilityMode,
		Active:                  stats.Active,
	}
}

func durationSeconds(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
