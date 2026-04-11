package validate

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/pionrecv"
)

func TestOutboundSample(t *testing.T) {
	now := time.Now()
	sample := outboundSample(webrtc.OutboundRTPStreamStats{
		Timestamp:               webrtc.StatsTimestamp(now.UnixMilli()),
		Kind:                    "video",
		Mid:                     "video-mid",
		Rid:                     "f",
		TrackID:                 "video-1",
		SSRC:                    7777,
		CodecID:                 "codec-1",
		TransportID:             "transport-1",
		PacketsSent:             13,
		BytesSent:               2048,
		NACKCount:               2,
		PLICount:                3,
		FIRCount:                4,
		FramesEncoded:           5,
		KeyFramesEncoded:        1,
		FrameWidth:              1280,
		FrameHeight:             720,
		FramesPerSecond:         30,
		QualityLimitationReason: webrtc.QualityLimitationReasonBandwidth,
		ScalabilityMode:         "L3T3_KEY",
		Active:                  true,
	})

	if sample.Direction != "outbound" || sample.TrackID != "video-1" || sample.RID != "f" {
		t.Fatalf("outbound sample identity = %+v", sample)
	}
	if sample.FrameWidth != 1280 || sample.FrameHeight != 720 || sample.Frames != 5 || sample.KeyFrames != 1 {
		t.Fatalf("outbound sample frame metrics = %+v", sample)
	}
	if sample.QualityLimitationReason != "bandwidth" || sample.ScalabilityMode != "L3T3_KEY" || !sample.Active {
		t.Fatalf("outbound sample quality/scalability = %+v", sample)
	}
}

func TestEnrichCodecSample(t *testing.T) {
	sample := enrichCodecSample(RTPStatsSample{CodecID: "codec-video"}, map[string]webrtc.CodecStats{
		"codec-video": {
			ID:          "codec-video",
			MimeType:    webrtc.MimeTypeVP8,
			PayloadType: 96,
		},
	})

	if sample.CodecMimeType != webrtc.MimeTypeVP8 || sample.CodecPayloadType != 96 {
		t.Fatalf("enriched sample = %+v", sample)
	}
}

func TestMatchVideoTrackLocked(t *testing.T) {
	t.Run("track_id", func(t *testing.T) {
		session := newSession(nil, SessionConfig{})
		want := &videoTrackState{id: "video-1"}
		session.videoTracks["video-1"] = want
		if got := session.matchVideoTrackLocked(RTPStatsSample{TrackID: "video-1"}); got != want {
			t.Fatalf("match by track id = %p, want %p", got, want)
		}
	})

	t.Run("ssrc", func(t *testing.T) {
		session := newSession(nil, SessionConfig{})
		want := &videoTrackState{id: "video-1", ssrc: 1234}
		session.videoTracks["video-1"] = want
		if got := session.matchVideoTrackLocked(RTPStatsSample{SSRC: 1234}); got != want {
			t.Fatalf("match by ssrc = %p, want %p", got, want)
		}
	})

	t.Run("rid", func(t *testing.T) {
		session := newSession(nil, SessionConfig{})
		want := &videoTrackState{id: "video-1", rid: "f"}
		session.videoTracks["video-1"] = want
		if got := session.matchVideoTrackLocked(RTPStatsSample{RID: "f"}); got != want {
			t.Fatalf("match by rid = %p, want %p", got, want)
		}
	})

	t.Run("current rid", func(t *testing.T) {
		session := newSession(nil, SessionConfig{})
		want := &videoTrackState{id: "video-1", currentRID: "h"}
		session.videoTracks["video-1"] = want
		if got := session.matchVideoTrackLocked(RTPStatsSample{RID: "h"}); got != want {
			t.Fatalf("match by current rid = %p, want %p", got, want)
		}
	})

	t.Run("current mid", func(t *testing.T) {
		session := newSession(nil, SessionConfig{})
		want := &videoTrackState{id: "video-1", currentMID: "mid-1"}
		session.videoTracks["video-1"] = want
		if got := session.matchVideoTrackLocked(RTPStatsSample{MID: "mid-1"}); got != want {
			t.Fatalf("match by current mid = %p, want %p", got, want)
		}
	})

	t.Run("monitor fallback rid", func(t *testing.T) {
		session := newSession(nil, SessionConfig{})
		monitor := pionrecv.NewVideoSubscriberMonitor(pionrecv.VideoSubscriberMonitorConfig{})
		setUnexportedField(t, monitor, "currentRID", "q")
		want := &videoTrackState{id: "video-1", monitor: monitor}
		session.videoTracks["video-1"] = want
		if got := session.matchVideoTrackLocked(RTPStatsSample{RID: "q"}); got != want {
			t.Fatalf("match by monitor rid = %p, want %p", got, want)
		}
	})

	t.Run("monitor fallback mid", func(t *testing.T) {
		session := newSession(nil, SessionConfig{})
		monitor := pionrecv.NewVideoSubscriberMonitor(pionrecv.VideoSubscriberMonitorConfig{})
		setUnexportedField(t, monitor, "currentMID", "mid-wire")
		want := &videoTrackState{id: "video-1", monitor: monitor}
		session.videoTracks["video-1"] = want
		if got := session.matchVideoTrackLocked(RTPStatsSample{MID: "mid-wire"}); got != want {
			t.Fatalf("match by monitor mid = %p, want %p", got, want)
		}
	})

	t.Run("single video track fallback", func(t *testing.T) {
		session := newSession(nil, SessionConfig{})
		want := &videoTrackState{id: "video-1"}
		session.videoTracks["video-1"] = want
		if got := session.matchVideoTrackLocked(RTPStatsSample{Kind: "video"}); got != want {
			t.Fatalf("single-track fallback = %p, want %p", got, want)
		}
		if got := session.matchVideoTrackLocked(RTPStatsSample{Kind: "video", TrackID: "native-track-attachment"}); got != nil {
			t.Fatalf("single-track fallback with explicit id = %p, want nil", got)
		}
	})
}

func TestMatchAudioTrackLocked(t *testing.T) {
	t.Run("track id / ssrc / rid", func(t *testing.T) {
		session := newSession(nil, SessionConfig{})
		track := &audioTrackState{id: "audio-1", ssrc: 4321, rid: "a"}
		session.audioTracks["audio-1"] = track

		if got := session.matchAudioTrackLocked(RTPStatsSample{TrackID: "audio-1"}); got != track {
			t.Fatalf("match by track id = %p, want %p", got, track)
		}
		if got := session.matchAudioTrackLocked(RTPStatsSample{SSRC: 4321}); got != track {
			t.Fatalf("match by ssrc = %p, want %p", got, track)
		}
		if got := session.matchAudioTrackLocked(RTPStatsSample{RID: "a"}); got != track {
			t.Fatalf("match by rid = %p, want %p", got, track)
		}
		if got := session.matchAudioTrackLocked(RTPStatsSample{TrackID: "missing"}); got != nil {
			t.Fatalf("match missing = %p, want nil", got)
		}
	})

	t.Run("current mid", func(t *testing.T) {
		session := newSession(nil, SessionConfig{})
		track := &audioTrackState{id: "audio-1", currentMID: "audio-mid"}
		session.audioTracks["audio-1"] = track
		if got := session.matchAudioTrackLocked(RTPStatsSample{MID: "audio-mid"}); got != track {
			t.Fatalf("match by current mid = %p, want %p", got, track)
		}
	})

	t.Run("single-track fallback", func(t *testing.T) {
		session := newSession(nil, SessionConfig{})
		track := &audioTrackState{id: "audio-1"}
		session.audioTracks["audio-1"] = track
		if got := session.matchAudioTrackLocked(RTPStatsSample{Kind: "audio"}); got != track {
			t.Fatalf("single-track fallback = %p, want %p", got, track)
		}
		if got := session.matchAudioTrackLocked(RTPStatsSample{Kind: "audio", TrackID: "native-track-attachment"}); got != nil {
			t.Fatalf("single-track fallback with explicit id = %p, want nil", got)
		}
	})
}

func TestMatchRTPStatsLocked(t *testing.T) {
	session := newSession(nil, SessionConfig{EventHistory: 8})
	session.videoTracks["video-1"] = &videoTrackState{
		id:            "video-1",
		currentWidth:  320,
		currentHeight: 180,
	}
	session.audioTracks["audio-1"] = &audioTrackState{id: "audio-1"}

	session.matchRTPStatsLocked(RTPStatsSample{
		TrackID:     "video-1",
		MID:         "video-mid",
		RID:         "f",
		FrameWidth:  640,
		FrameHeight: 360,
	})
	videoState := session.videoTracks["video-1"]
	if videoState.currentMID != "video-mid" || videoState.currentRID != "f" {
		t.Fatalf("video current mid/rid = %q/%q", videoState.currentMID, videoState.currentRID)
	}
	if videoState.currentWidth != 640 || videoState.currentHeight != 360 || videoState.resolutionChanges != 1 {
		t.Fatalf("video dimensions/resolution changes = %dx%d/%d", videoState.currentWidth, videoState.currentHeight, videoState.resolutionChanges)
	}
	if len(videoState.stats) != 1 {
		t.Fatalf("len(video stats) = %d, want 1", len(videoState.stats))
	}

	session.matchRTPStatsLocked(RTPStatsSample{TrackID: "audio-1", Kind: "audio"})
	audioState := session.audioTracks["audio-1"]
	if len(audioState.stats) != 1 {
		t.Fatalf("len(audio stats) = %d, want 1", len(audioState.stats))
	}
	if audioState.currentMID != "" {
		t.Fatalf("audio current mid = %q, want empty when sample had no MID", audioState.currentMID)
	}

	session.matchRTPStatsLocked(RTPStatsSample{TrackID: "audio-1", Kind: "audio", MID: "audio-mid"})
	if audioState.currentMID != "audio-mid" {
		t.Fatalf("audio current mid = %q, want %q", audioState.currentMID, "audio-mid")
	}

	session.matchRTPStatsLocked(RTPStatsSample{TrackID: "missing", Kind: "video"})
	if len(session.unmatchedStreams) != 1 {
		t.Fatalf("len(unmatchedStreams) = %d, want 1", len(session.unmatchedStreams))
	}
}

func TestApplyStatsReportLocked(t *testing.T) {
	now := time.Now()
	session := newSession(nil, SessionConfig{EventHistory: 8})
	session.videoTracks["video-1"] = &videoTrackState{id: "video-1"}
	session.audioTracks["audio-1"] = &audioTrackState{id: "audio-1"}
	session.dataChannels[dataChannelKey("control", 3)] = &dataChannelState{
		label:       "control",
		id:          3,
		pendingAcks: make(map[string]time.Time),
	}

	session.applyStatsReportLocked(webrtc.StatsReport{
		"codec-outbound": webrtc.CodecStats{
			ID:          "codec-outbound",
			MimeType:    webrtc.MimeTypeVP8,
			PayloadType: 96,
		},
		"codec-inbound": webrtc.CodecStats{
			ID:          "codec-inbound",
			MimeType:    webrtc.MimeTypeOpus,
			PayloadType: 111,
		},
		"pair": webrtc.ICECandidatePairStats{
			ID:                       "pair",
			CurrentRoundTripTime:     0.021,
			AvailableOutgoingBitrate: 1_000_000,
			AvailableIncomingBitrate: 2_000_000,
		},
		"transport": webrtc.TransportStats{
			ID:                      "transport",
			Timestamp:               webrtc.StatsTimestamp(now.UnixMilli()),
			PacketsSent:             11,
			PacketsReceived:         12,
			BytesSent:               1024,
			BytesReceived:           2048,
			SelectedCandidatePairID: "pair",
		},
		"remote-video": webrtc.RemoteInboundRTPStreamStats{
			ID:            "remote-video",
			RoundTripTime: 0.032,
			FractionLost:  0.125,
		},
		"outbound-video": webrtc.OutboundRTPStreamStats{
			ID:               "outbound-video",
			Timestamp:        webrtc.StatsTimestamp(now.UnixMilli()),
			Kind:             "video",
			Mid:              "video-mid",
			Rid:              "f",
			TrackID:          "video-1",
			SSRC:             9999,
			CodecID:          "codec-outbound",
			RemoteID:         "remote-video",
			PacketsSent:      20,
			BytesSent:        4096,
			FramesEncoded:    6,
			KeyFramesEncoded: 1,
			FrameWidth:       1280,
			FrameHeight:      720,
		},
		"inbound-audio": webrtc.InboundRTPStreamStats{
			ID:              "inbound-audio",
			Timestamp:       webrtc.StatsTimestamp(now.UnixMilli()),
			Kind:            "audio",
			TrackID:         "audio-1",
			SSRC:            3333,
			CodecID:         "codec-inbound",
			PacketsReceived: 18,
			BytesReceived:   512,
			AudioLevel:      0.4,
		},
		"unmatched": webrtc.InboundRTPStreamStats{
			ID:              "unmatched",
			Timestamp:       webrtc.StatsTimestamp(now.UnixMilli()),
			Kind:            "video",
			TrackID:         "missing",
			SSRC:            123,
			PacketsReceived: 1,
		},
		"dc": webrtc.DataChannelStats{
			ID:                    "dc",
			Timestamp:             webrtc.StatsTimestamp(now.UnixMilli()),
			Label:                 "control",
			DataChannelIdentifier: 3,
			TransportID:           "transport",
			State:                 webrtc.DataChannelStateOpen,
			MessagesSent:          2,
			MessagesReceived:      1,
			BytesSent:             20,
			BytesReceived:         10,
		},
	})

	if len(session.transportSamples) != 1 {
		t.Fatalf("len(transportSamples) = %d, want 1", len(session.transportSamples))
	}
	if got := session.transportSamples[0].CurrentRoundTripTime; got != 21*time.Millisecond {
		t.Fatalf("CurrentRoundTripTime = %v, want 21ms", got)
	}
	videoStats := session.videoTracks["video-1"].stats
	if len(videoStats) != 1 || videoStats[0].RemoteRoundTripTime != 32*time.Millisecond || videoStats[0].FractionLost != 0.125 {
		t.Fatalf("video stats = %+v", videoStats)
	}
	if videoStats[0].CodecMimeType != webrtc.MimeTypeVP8 || videoStats[0].CodecPayloadType != 96 {
		t.Fatalf("video codec stats = %+v", videoStats[0])
	}
	audioStats := session.audioTracks["audio-1"].stats
	if len(audioStats) != 1 || audioStats[0].AudioLevel != 0.4 {
		t.Fatalf("audio stats = %+v", audioStats)
	}
	if audioStats[0].CodecMimeType != webrtc.MimeTypeOpus || audioStats[0].CodecPayloadType != 111 {
		t.Fatalf("audio codec stats = %+v", audioStats[0])
	}
	dcState := session.dataChannels[dataChannelKey("control", 3)]
	if len(dcState.stats) != 1 || dcState.state != "open" {
		t.Fatalf("data channel stats/state = %+v / %q", dcState.stats, dcState.state)
	}
	if len(session.unmatchedStreams) != 1 || session.unmatchedStreams[0].TrackID != "missing" {
		t.Fatalf("unmatchedStreams = %+v", session.unmatchedStreams)
	}
}

func TestApplyStatsReportLockedUpdatesCodecSwitchesFromStats(t *testing.T) {
	now := time.Now()
	session := newSession(nil, SessionConfig{EventHistory: 8})
	session.videoTracks["video-1"] = &videoTrackState{id: "video-1"}
	session.audioTracks["audio-1"] = &audioTrackState{id: "audio-1"}

	session.applyStatsReportLocked(webrtc.StatsReport{
		"codec-video-vp8": webrtc.CodecStats{
			ID:          "codec-video-vp8",
			MimeType:    webrtc.MimeTypeVP8,
			PayloadType: 96,
		},
		"codec-audio-opus": webrtc.CodecStats{
			ID:          "codec-audio-opus",
			MimeType:    webrtc.MimeTypeOpus,
			PayloadType: 111,
		},
		"video": webrtc.InboundRTPStreamStats{
			ID:              "video",
			Timestamp:       webrtc.StatsTimestamp(now.UnixMilli()),
			Kind:            "video",
			TrackID:         "video-1",
			CodecID:         "codec-video-vp8",
			PacketsReceived: 4,
			FramesDecoded:   2,
		},
		"audio": webrtc.InboundRTPStreamStats{
			ID:              "audio",
			Timestamp:       webrtc.StatsTimestamp(now.UnixMilli()),
			Kind:            "audio",
			TrackID:         "audio-1",
			CodecID:         "codec-audio-opus",
			PacketsReceived: 4,
		},
	})

	if got := session.videoTracks["video-1"].currentMime; got != webrtc.MimeTypeVP8 {
		t.Fatalf("initial video mime = %q, want %q", got, webrtc.MimeTypeVP8)
	}
	if got := session.audioTracks["audio-1"].currentMime; got != webrtc.MimeTypeOpus {
		t.Fatalf("initial audio mime = %q, want %q", got, webrtc.MimeTypeOpus)
	}
	if got := len(session.videoTracks["video-1"].codecSwitches); got != 0 {
		t.Fatalf("initial video codec switches = %d, want 0", got)
	}
	if got := len(session.audioTracks["audio-1"].codecSwitches); got != 0 {
		t.Fatalf("initial audio codec switches = %d, want 0", got)
	}

	next := now.Add(100 * time.Millisecond)
	session.applyStatsReportLocked(webrtc.StatsReport{
		"codec-video-h264": webrtc.CodecStats{
			ID:          "codec-video-h264",
			MimeType:    webrtc.MimeTypeH264,
			PayloadType: 102,
		},
		"codec-audio-pcmu": webrtc.CodecStats{
			ID:          "codec-audio-pcmu",
			MimeType:    webrtc.MimeTypePCMU,
			PayloadType: 0,
		},
		"video": webrtc.InboundRTPStreamStats{
			ID:              "video",
			Timestamp:       webrtc.StatsTimestamp(next.UnixMilli()),
			Kind:            "video",
			TrackID:         "video-1",
			CodecID:         "codec-video-h264",
			PacketsReceived: 8,
			FramesDecoded:   4,
		},
		"audio": webrtc.InboundRTPStreamStats{
			ID:              "audio",
			Timestamp:       webrtc.StatsTimestamp(next.UnixMilli()),
			Kind:            "audio",
			TrackID:         "audio-1",
			CodecID:         "codec-audio-pcmu",
			PacketsReceived: 8,
		},
	})

	videoState := session.videoTracks["video-1"]
	if videoState.currentMime != webrtc.MimeTypeH264 || videoState.currentCodec != codec.H264 {
		t.Fatalf("video current codec = %v/%q", videoState.currentCodec, videoState.currentMime)
	}
	if got := len(videoState.codecSwitches); got != 1 {
		t.Fatalf("video codec switches = %d, want 1", got)
	}
	if change := videoState.codecSwitches[0].Change; change.PreviousType != codec.VP8 || change.CurrentType != codec.H264 {
		t.Fatalf("video switch change = %+v", change)
	}

	audioState := session.audioTracks["audio-1"]
	if audioState.currentMime != webrtc.MimeTypePCMU || audioState.currentCodec != codec.PCMU {
		t.Fatalf("audio current codec = %v/%q", audioState.currentCodec, audioState.currentMime)
	}
	if got := len(audioState.codecSwitches); got != 1 {
		t.Fatalf("audio codec switches = %d, want 1", got)
	}
	if change := audioState.codecSwitches[0].Change; change.PreviousType != codec.Opus || change.CurrentType != codec.PCMU {
		t.Fatalf("audio switch change = %+v", change)
	}
}
