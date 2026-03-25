package validate

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/media"
	"github.com/thesyncim/libgowebrtc/pkg/pionsend"
)

type fakePeer struct {
	mu        sync.Mutex
	report    webrtc.StatsReport
	conn      webrtc.PeerConnectionState
	ice       webrtc.ICEConnectionState
	signaling webrtc.SignalingState
}

func (f *fakePeer) GetStats() (webrtc.StatsReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.report, nil
}

func (f *fakePeer) ConnectionState() webrtc.PeerConnectionState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.conn
}

func (f *fakePeer) ICEConnectionState() webrtc.ICEConnectionState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ice
}

func (f *fakePeer) SignalingState() webrtc.SignalingState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.signaling
}

type fakeDataChannel struct {
	label string
	id    int
	state string

	mu        sync.Mutex
	onOpen    func()
	onClose   func()
	onMessage func([]byte)
	onError   func(error)
	sent      []string
}

func (f *fakeDataChannel) Label() string            { return f.label }
func (f *fakeDataChannel) ID() int                  { return f.id }
func (f *fakeDataChannel) ReadyStateString() string { return f.state }
func (f *fakeDataChannel) SetOnOpen(cb func())      { f.onOpen = cb }
func (f *fakeDataChannel) SetOnClose(cb func())     { f.onClose = cb }
func (f *fakeDataChannel) SetOnMessage(cb func([]byte)) {
	f.onMessage = cb
}
func (f *fakeDataChannel) SetOnError(cb func(error)) { f.onError = cb }
func (f *fakeDataChannel) SendText(text string) error {
	f.mu.Lock()
	f.sent = append(f.sent, text)
	f.mu.Unlock()
	if len(text) > len(heartbeatPingPrefix) && text[:len(heartbeatPingPrefix)] == heartbeatPingPrefix && f.onMessage != nil {
		token := text[len(heartbeatPingPrefix):]
		f.onMessage([]byte(heartbeatAckPrefix + token))
	}
	return nil
}
func (f *fakeDataChannel) open() {
	f.state = "open"
	if f.onOpen != nil {
		f.onOpen()
	}
}

type fakePublishedVideo struct {
	layerCalls []struct {
		index  int
		active bool
	}
	bitrateCalls []struct {
		index   int
		bitrate uint32
	}
	keyframes int
}

func (f *fakePublishedVideo) WriteFrame(*frame.VideoFrame, bool) error { return nil }
func (f *fakePublishedVideo) RequestKeyFrame()                         { f.keyframes++ }
func (f *fakePublishedVideo) SetLayerActive(index int, active bool) error {
	f.layerCalls = append(f.layerCalls, struct {
		index  int
		active bool
	}{index: index, active: active})
	return nil
}
func (f *fakePublishedVideo) SetLayerBitrate(index int, maxBitrate uint32) error {
	f.bitrateCalls = append(f.bitrateCalls, struct {
		index   int
		bitrate uint32
	}{index: index, bitrate: maxBitrate})
	return nil
}
func (f *fakePublishedVideo) Encodings() []pionsend.PublishedEncoding { return nil }
func (f *fakePublishedVideo) Sender() *webrtc.RTPSender               { return nil }
func (f *fakePublishedVideo) Close() error                            { return nil }

type fakeMediaTrack struct {
	enabled bool
}

func (f *fakeMediaTrack) ID() string                    { return "track-1" }
func (f *fakeMediaTrack) Kind() string                  { return "audio" }
func (f *fakeMediaTrack) Label() string                 { return "fake" }
func (f *fakeMediaTrack) Enabled() bool                 { return f.enabled }
func (f *fakeMediaTrack) SetEnabled(enabled bool)       { f.enabled = enabled }
func (f *fakeMediaTrack) Muted() bool                   { return false }
func (f *fakeMediaTrack) ReadyState() string            { return "live" }
func (f *fakeMediaTrack) Stop()                         {}
func (f *fakeMediaTrack) Clone() media.MediaStreamTrack { return &fakeMediaTrack{enabled: f.enabled} }

func TestPolicyForBrowser(t *testing.T) {
	chrome := PolicyForBrowser("chrome")
	if !chrome.SupportsSimulcast || !chrome.SupportsDependencyDescriptor {
		t.Fatalf("chrome policy = %+v, want simulcast and DD support", chrome)
	}

	safari := PolicyForBrowser("safari")
	if safari.SupportsDependencyDescriptor || safari.SupportsLayeredVP9 {
		t.Fatalf("safari policy = %+v, want conservative layered support flags disabled", safari)
	}
}

func TestSessionStatsCorrelationAndValidation(t *testing.T) {
	peer := &fakePeer{
		conn:      webrtc.PeerConnectionStateConnected,
		ice:       webrtc.ICEConnectionStateConnected,
		signaling: webrtc.SignalingStateStable,
		report: webrtc.StatsReport{
			"pair": webrtc.ICECandidatePairStats{
				ID:                       "pair",
				CurrentRoundTripTime:     0.045,
				AvailableOutgoingBitrate: 1_500_000,
				AvailableIncomingBitrate: 2_000_000,
			},
			"transport": webrtc.TransportStats{
				ID:                      "transport",
				Timestamp:               webrtc.StatsTimestamp(time.Now().UnixMilli()),
				PacketsSent:             10,
				PacketsReceived:         11,
				BytesSent:               1000,
				BytesReceived:           2000,
				SelectedCandidatePairID: "pair",
			},
			"inbound-video": webrtc.InboundRTPStreamStats{
				ID:               "inbound-video",
				Timestamp:        webrtc.StatsTimestamp(time.Now().UnixMilli()),
				SSRC:             1111,
				Kind:             "video",
				PacketsReceived:  20,
				FramesDecoded:    5,
				KeyFramesDecoded: 1,
				FrameWidth:       1280,
				FrameHeight:      720,
			},
		},
	}

	session := newSession(peer, nil, SessionConfig{})
	session.mu.Lock()
	session.videoTracks["video-1"] = &videoTrackState{
		id:           "video-1",
		ssrc:         1111,
		currentCodec: codec.VP8,
		currentMime:  webrtc.MimeTypeVP8,
		frameCount:   5,
		lastFrameAt:  time.Now(),
		startedAt:    time.Now(),
	}
	session.lastStatsPoll = time.Time{}
	session.mu.Unlock()

	snapshot := session.Snapshot()
	if len(snapshot.Transport.Samples) == 0 {
		t.Fatal("Transport.Samples = 0, want transport stats")
	}
	video := snapshot.VideoTracks["video-1"]
	if len(video.Stats) != 1 {
		t.Fatalf("len(video.Stats) = %d, want 1", len(video.Stats))
	}
	if video.Stats[0].FrameWidth != 1280 || video.Stats[0].FrameHeight != 720 {
		t.Fatalf("video stats frame size = %dx%d, want 1280x720", video.Stats[0].FrameWidth, video.Stats[0].FrameHeight)
	}
	if snapshot.Transport.Samples[0].CurrentRoundTripTime <= 0 {
		t.Fatalf("CurrentRoundTripTime = %v, want > 0", snapshot.Transport.Samples[0].CurrentRoundTripTime)
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}

	session.mu.Lock()
	session.videoTracks["video-1"].freezeCount = 1
	session.mu.Unlock()
	if err := session.Validate(); err == nil {
		t.Fatal("Validate() = nil, want freeze failure")
	}
}

func TestSessionWaitersAndCodecSwitch(t *testing.T) {
	peer := &fakePeer{
		conn:      webrtc.PeerConnectionStateConnecting,
		ice:       webrtc.ICEConnectionStateChecking,
		signaling: webrtc.SignalingStateHaveLocalOffer,
	}
	session := newSession(peer, nil, SessionConfig{StatsPollInterval: 10 * time.Millisecond})
	session.mu.Lock()
	session.videoTracks["video-1"] = &videoTrackState{
		id:           "video-1",
		currentCodec: codec.VP8,
		currentMime:  webrtc.MimeTypeVP8,
	}
	session.mu.Unlock()

	go func() {
		time.Sleep(40 * time.Millisecond)
		peer.mu.Lock()
		peer.conn = webrtc.PeerConnectionStateConnected
		peer.ice = webrtc.ICEConnectionStateConnected
		peer.signaling = webrtc.SignalingStateStable
		peer.mu.Unlock()

		session.mu.Lock()
		session.videoTracks["video-1"].frameCount = 2
		session.videoTracks["video-1"].currentMime = webrtc.MimeTypeH264
		session.videoTracks["video-1"].currentCodec = codec.H264
		session.signalLocked()
		session.mu.Unlock()

		time.Sleep(40 * time.Millisecond)
		peer.mu.Lock()
		peer.conn = webrtc.PeerConnectionStateDisconnected
		peer.mu.Unlock()

		time.Sleep(40 * time.Millisecond)
		peer.mu.Lock()
		peer.conn = webrtc.PeerConnectionStateConnected
		peer.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := session.WaitForConnected(ctx); err != nil {
		t.Fatalf("WaitForConnected: %v", err)
	}
	if err := session.WaitForStable(ctx); err != nil {
		t.Fatalf("WaitForStable: %v", err)
	}
	if err := session.WaitForVideoContinuous(ctx, "video-1"); err != nil {
		t.Fatalf("WaitForVideoContinuous: %v", err)
	}
	if err := session.WaitForCodecSwitch(ctx, "video-1", webrtc.MimeTypeH264); err != nil {
		t.Fatalf("WaitForCodecSwitch: %v", err)
	}
	if err := session.WaitForReconnected(ctx); err != nil {
		t.Fatalf("WaitForReconnected: %v", err)
	}
}

func TestSessionDataChannelHeartbeatsAndScenarioLab(t *testing.T) {
	session := newSession(nil, nil, SessionConfig{
		EnableDataChannelHeartbeats: true,
		HeartbeatInterval:           10 * time.Millisecond,
		HeartbeatTimeout:            50 * time.Millisecond,
	})
	dc := &fakeDataChannel{label: "control", id: 7, state: "connecting"}
	session.observeDataChannel(dc)
	dc.open()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		dcSnap, ok := snapshot.DataChannels[dataChannelKey("control", 7)]
		return ok && dcSnap.HeartbeatAcked > 0
	}); err != nil {
		t.Fatalf("wait for heartbeat ack: %v", err)
	}

	video := &fakePublishedVideo{}
	track := &fakeMediaTrack{enabled: true}
	var externalCalls int

	lab := NewScenarioLab(session, LabConfig{})
	if err := lab.Run(ctx, ScenarioScript{
		Steps: []ScenarioStep{
			{Action: ScenarioActionSetLayerActive, Video: video, LayerIndex: 1, Active: false},
			{Action: ScenarioActionSetLayerBitrate, Video: video, LayerIndex: 2, Bitrate: 123456},
			{Action: ScenarioActionRequestKeyFrame, Video: video},
			{Action: ScenarioActionMute, MediaTrack: track},
			{Action: ScenarioActionUnmute, MediaTrack: track},
			{Action: ScenarioActionDataChannelBurst, DataChannelLabel: "control", TextMessages: []string{"hello", "world"}},
			{Action: ScenarioActionPauseHeartbeats, DataChannelLabel: "control"},
			{Action: ScenarioActionResumeHeartbeats, DataChannelLabel: "control"},
			{Action: ScenarioActionExternal, Callback: func(context.Context, *Session) error {
				externalCalls++
				return nil
			}},
		},
	}); err != nil {
		t.Fatalf("ScenarioLab.Run: %v", err)
	}

	if len(video.layerCalls) != 1 || video.layerCalls[0].index != 1 || video.layerCalls[0].active {
		t.Fatalf("layerCalls = %+v, want one disable call", video.layerCalls)
	}
	if len(video.bitrateCalls) != 1 || video.bitrateCalls[0].bitrate != 123456 {
		t.Fatalf("bitrateCalls = %+v, want bitrate update", video.bitrateCalls)
	}
	if video.keyframes != 1 {
		t.Fatalf("keyframes = %d, want 1", video.keyframes)
	}
	if !track.enabled {
		t.Fatal("track.enabled = false, want true after mute/unmute")
	}
	if externalCalls != 1 {
		t.Fatalf("externalCalls = %d, want 1", externalCalls)
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()
	if len(dc.sent) < 2 || dc.sent[len(dc.sent)-2] != "hello" || dc.sent[len(dc.sent)-1] != "world" {
		t.Fatalf("sent data channel messages = %v, want hello/world", dc.sent)
	}
}
