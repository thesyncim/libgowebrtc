package validate

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/media"
	"github.com/thesyncim/libgowebrtc/pkg/pionrecv"
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
	sendErr   error

	suppressPingAck bool
}

func (f *fakeDataChannel) Label() string { return f.label }
func (f *fakeDataChannel) ID() int       { return f.id }
func (f *fakeDataChannel) ReadyStateString() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}
func (f *fakeDataChannel) SetOnOpen(cb func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onOpen = cb
}
func (f *fakeDataChannel) SetOnClose(cb func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onClose = cb
}
func (f *fakeDataChannel) SetOnMessage(cb func([]byte)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onMessage = cb
}
func (f *fakeDataChannel) SetOnError(cb func(error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onError = cb
}
func (f *fakeDataChannel) SendText(text string) error {
	f.mu.Lock()
	state := f.state
	sendErr := f.sendErr
	onMessage := f.onMessage
	suppressPingAck := f.suppressPingAck
	if state != "closed" && sendErr == nil {
		f.sent = append(f.sent, text)
	}
	f.mu.Unlock()

	if state == "closed" {
		return errors.New("closed")
	}
	if sendErr != nil {
		return sendErr
	}
	if !suppressPingAck && strings.HasPrefix(text, heartbeatPingPrefix) && onMessage != nil {
		token := text[len(heartbeatPingPrefix):]
		onMessage([]byte(heartbeatAckPrefix + token))
	}
	return nil
}
func (f *fakeDataChannel) open() {
	f.mu.Lock()
	f.state = "open"
	onOpen := f.onOpen
	f.mu.Unlock()
	if onOpen != nil {
		onOpen()
	}
}

func (f *fakeDataChannel) close() {
	f.mu.Lock()
	f.state = "closed"
	onClose := f.onClose
	f.mu.Unlock()
	if onClose != nil {
		onClose()
	}
}

func (f *fakeDataChannel) emitError(err error) {
	f.mu.Lock()
	onError := f.onError
	f.mu.Unlock()
	if onError != nil {
		onError(err)
	}
}

func (f *fakeDataChannel) setSendErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sendErr = err
}

func (f *fakeDataChannel) setSuppressPingAck(suppress bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.suppressPingAck = suppress
}

func (f *fakeDataChannel) sentMessages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
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

func TestNewSessionsAndTrackWrappers(t *testing.T) {
	pionPC := mustNewPionPeerConnection(t)
	pionSession := NewPionSession(pionPC, SessionConfig{})
	if pionSession == nil {
		t.Fatal("NewPionSession() = nil")
	}
	if got := pionSession.Snapshot().Browser; got != "chrome" {
		t.Fatalf("pionSession browser = %q, want chrome", got)
	}

	dc, err := pionPC.CreateDataChannel("control", nil)
	if err != nil {
		t.Fatalf("CreateDataChannel: %v", err)
	}
	pionSession.ObservePionDataChannel(dc)
	snapshot := pionSession.Snapshot()
	if len(snapshot.DataChannels) != 1 {
		t.Fatalf("len(DataChannels) = %d, want 1", len(snapshot.DataChannels))
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer waitCancel()
	if err := pionSession.WaitForDataChannelOpen(waitCtx, ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForDataChannelOpen(any) error = %v, want deadline exceeded on unopened pion data channel", err)
	}

	pionSession.PionOnTrack()(nil, nil)
	if failures := pionSession.Snapshot().Failures; len(failures) == 0 || !strings.Contains(failures[0], "nil pion remote track") {
		t.Fatalf("pion failures = %v, want nil-track failure", failures)
	}

	pcSession := NewPCSession(nil, SessionConfig{})
	if pcSession == nil {
		t.Fatal("NewPCSession() = nil")
	}
	pcSession.PCOnTrack()(nil, nil, "")
	if failures := pcSession.Snapshot().Failures; len(failures) == 0 || !strings.Contains(failures[0], "nil pc remote track") {
		t.Fatalf("pc failures = %v, want nil-track failure", failures)
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

	session := newSession(peer, SessionConfig{})
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
	session := newSession(peer, SessionConfig{StatsPollInterval: 10 * time.Millisecond})
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

func TestSessionObserveRemoteTracks(t *testing.T) {
	session := newSession(nil, SessionConfig{
		FreezeThreshold:   5 * time.Millisecond,
		AudioGapThreshold: 5 * time.Millisecond,
		EventHistory:      8,
	})

	video := newFakeRemoteVideoTrack("video-1", "stream-1", "h", codec.VP8, webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		PayloadType:        96,
	})
	audio := newFakeRemoteAudioTrack("audio-1", "stream-1", "", codec.Opus, webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		PayloadType:        111,
	})

	session.ObserveRemoteTrack(nil)
	session.ObserveRemoteTrack(video)
	session.ObserveRemoteTrack(audio)

	video.emitFrame(&frame.VideoFrame{Width: 640, Height: 360, IsKeyframe: true})
	video.emitCodecChange(pionrecv.CodecChange{
		PreviousType:        codec.VP8,
		CurrentType:         codec.H264,
		PreviousCodec:       video.CodecParameters(),
		CurrentCodec:        webrtc.RTPCodecParameters{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, PayloadType: 102},
		PreviousPayloadType: video.PayloadType(),
		CurrentPayloadType:  102,
	})
	audio.emitFrame(frame.NewAudioFrameFromS16([]int16{100, -100, 200, -200}, 48000, 2))
	audio.emitCodecChange(pionrecv.CodecChange{
		PreviousType:        codec.Opus,
		CurrentType:         codec.PCMU,
		PreviousCodec:       audio.CodecParameters(),
		CurrentCodec:        webrtc.RTPCodecParameters{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU}, PayloadType: 0},
		PreviousPayloadType: audio.PayloadType(),
		CurrentPayloadType:  0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.WaitForVideoContinuous(ctx, "video-1"); err != nil {
		t.Fatalf("WaitForVideoContinuous: %v", err)
	}
	if err := session.WaitForAudioContinuous(ctx, "audio-1"); err != nil {
		t.Fatalf("WaitForAudioContinuous: %v", err)
	}

	snapshot := session.Snapshot()
	videoSnap := snapshot.VideoTracks["video-1"]
	if videoSnap.Source != "manual" || videoSnap.FrameCount != 1 || videoSnap.KeyframeCount != 1 {
		t.Fatalf("video snapshot = %+v", videoSnap)
	}
	if videoSnap.CurrentMimeType != webrtc.MimeTypeH264 || videoSnap.CurrentRID != "" {
		t.Fatalf("video mime/rid = %q/%q, want %q/empty", videoSnap.CurrentMimeType, videoSnap.CurrentRID, webrtc.MimeTypeH264)
	}
	audioSnap := snapshot.AudioTracks["audio-1"]
	if audioSnap.Source != "manual" || audioSnap.FrameCount != 1 {
		t.Fatalf("audio snapshot = %+v", audioSnap)
	}
	if audioSnap.CurrentMimeType != webrtc.MimeTypePCMU || audioSnap.CurrentSampleRate != 48000 || audioSnap.CurrentChannels != 2 {
		t.Fatalf("audio snapshot codec/config = %+v", audioSnap)
	}
}

func TestSessionObserveRemoteTrackRecordsCallbackInstallWarnings(t *testing.T) {
	session := newSession(nil, SessionConfig{})
	video := &failingRemoteVideoTrack{
		fakeRemoteVideoTrack: newFakeRemoteVideoTrack("video-1", "stream-1", "", codec.VP8, webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
			PayloadType:        96,
		}),
		err: errors.New("video callback unavailable"),
	}
	audio := &failingRemoteAudioTrack{
		fakeRemoteAudioTrack: newFakeRemoteAudioTrack("audio-1", "stream-1", "", codec.Opus, webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
			PayloadType:        111,
		}),
		err: errors.New("audio callback unavailable"),
	}

	session.ObserveRemoteTrack(video)
	session.ObserveRemoteTrack(audio)

	snapshot := session.Snapshot()
	if len(snapshot.Warnings) != 2 {
		t.Fatalf("warnings = %v, want two callback install warnings", snapshot.Warnings)
	}
	if !strings.Contains(snapshot.Warnings[0], "video frame callback") || !strings.Contains(snapshot.Warnings[1], "audio frame callback") {
		t.Fatalf("warnings = %v, want callback install warnings", snapshot.Warnings)
	}
}

func TestSessionDataChannelHeartbeatsAndScenarioLab(t *testing.T) {
	session := newSession(nil, SessionConfig{
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
	var userMessages []string
	for _, message := range dc.sent {
		if strings.HasPrefix(message, heartbeatPingPrefix) || strings.HasPrefix(message, heartbeatAckPrefix) {
			continue
		}
		userMessages = append(userMessages, message)
	}
	if len(userMessages) != 2 || userMessages[0] != "hello" || userMessages[1] != "world" {
		t.Fatalf("sent user data channel messages = %v (all sent=%v), want hello/world", userMessages, dc.sent)
	}
}

func TestSessionDataChannelReobserveReplacesAdapterAndRestartsHeartbeats(t *testing.T) {
	session := newSession(nil, SessionConfig{
		EnableDataChannelHeartbeats: true,
		HeartbeatInterval:           10 * time.Millisecond,
		HeartbeatTimeout:            50 * time.Millisecond,
	})

	first := &fakeDataChannel{label: "control", id: 7, state: "connecting"}
	session.observeDataChannel(first)
	first.open()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		dc, ok := snapshot.DataChannels[dataChannelKey("control", 7)]
		return ok && dc.HeartbeatAcked > 0
	}); err != nil {
		t.Fatalf("wait for first heartbeat ack: %v", err)
	}

	first.close()
	if err := session.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		dc, ok := snapshot.DataChannels[dataChannelKey("control", 7)]
		return ok && dc.State == "closed"
	}); err != nil {
		t.Fatalf("wait for close state: %v", err)
	}

	second := &fakeDataChannel{label: "control", id: 7, state: "connecting"}
	session.observeDataChannel(second)

	snapshot := session.Snapshot()
	if got := snapshot.DataChannels[dataChannelKey("control", 7)].State; got != "connecting" {
		t.Fatalf("state after re-observe = %q, want connecting", got)
	}

	ackedBefore := snapshot.DataChannels[dataChannelKey("control", 7)].HeartbeatAcked
	second.open()
	if err := session.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		dc, ok := snapshot.DataChannels[dataChannelKey("control", 7)]
		return ok && dc.State == "open" && dc.HeartbeatAcked > ackedBefore
	}); err != nil {
		t.Fatalf("wait for replacement heartbeat ack: %v", err)
	}
}

func TestSessionDataChannelReobserveIgnoresStaleAdapterCallbacks(t *testing.T) {
	session := newSession(nil, SessionConfig{
		EnableDataChannelHeartbeats: true,
		HeartbeatInterval:           10 * time.Millisecond,
		HeartbeatTimeout:            50 * time.Millisecond,
	})

	first := &fakeDataChannel{label: "control", id: 7, state: "connecting"}
	session.observeDataChannel(first)
	first.open()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		dc, ok := snapshot.DataChannels[dataChannelKey("control", 7)]
		return ok && dc.State == "open" && dc.HeartbeatAcked > 0
	}); err != nil {
		t.Fatalf("wait for initial heartbeat ack: %v", err)
	}

	second := &fakeDataChannel{label: "control", id: 7, state: "connecting"}
	session.observeDataChannel(second)
	second.open()

	if err := session.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		dc, ok := snapshot.DataChannels[dataChannelKey("control", 7)]
		return ok && dc.State == "open" && dc.OpenTransitions >= 2
	}); err != nil {
		t.Fatalf("wait for replacement open: %v", err)
	}

	before := session.Snapshot().DataChannels[dataChannelKey("control", 7)]

	first.close()
	first.emitError(errors.New("stale adapter error"))

	snapshot := session.Snapshot().DataChannels[dataChannelKey("control", 7)]
	if snapshot.State != "open" {
		t.Fatalf("state after stale close = %q, want open", snapshot.State)
	}
	if snapshot.CloseTransitions != before.CloseTransitions {
		t.Fatalf("close transitions after stale close = %d, want %d", snapshot.CloseTransitions, before.CloseTransitions)
	}
	if snapshot.LastError != before.LastError {
		t.Fatalf("last error after stale error = %q, want %q", snapshot.LastError, before.LastError)
	}

	if err := session.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		dc, ok := snapshot.DataChannels[dataChannelKey("control", 7)]
		return ok && dc.State == "open" && dc.HeartbeatAcked > before.HeartbeatAcked
	}); err != nil {
		t.Fatalf("wait for replacement heartbeat ack after stale callbacks: %v", err)
	}
}

func TestSessionWaitForAudioContinuousAndDataChannelOpen(t *testing.T) {
	session := newSession(nil, SessionConfig{})
	session.mu.Lock()
	session.audioTracks["audio-1"] = &audioTrackState{id: "audio-1"}
	session.signalLocked()
	session.mu.Unlock()

	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer timeoutCancel()
	if err := session.WaitForAudioContinuous(timeoutCtx, "audio-1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForAudioContinuous(timeout) error = %v, want deadline exceeded", err)
	}

	session.mu.Lock()
	session.audioTracks["audio-1"].frameCount = 1
	session.signalLocked()
	session.mu.Unlock()

	successCtx, successCancel := context.WithTimeout(context.Background(), time.Second)
	defer successCancel()
	if err := session.WaitForAudioContinuous(successCtx, "audio-1"); err != nil {
		t.Fatalf("WaitForAudioContinuous(success): %v", err)
	}

	dc := &fakeDataChannel{label: "chat", id: 9, state: "connecting"}
	session.observeDataChannel(dc)
	go func() {
		time.Sleep(20 * time.Millisecond)
		dc.open()
	}()
	if err := session.WaitForDataChannelOpen(successCtx, "chat"); err != nil {
		t.Fatalf("WaitForDataChannelOpen(chat): %v", err)
	}
	if err := session.WaitForDataChannelOpen(successCtx, ""); err != nil {
		t.Fatalf("WaitForDataChannelOpen(any): %v", err)
	}

	missingCtx, missingCancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer missingCancel()
	if err := session.WaitForDataChannelOpen(missingCtx, "missing"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForDataChannelOpen(missing) error = %v, want deadline exceeded", err)
	}
}

func TestAudioTrackStateObserveFrameLockedRecordsFreezeAndConfigSwitch(t *testing.T) {
	state := &audioTrackState{id: "audio-1"}

	first := frame.NewAudioFrameS16(48000, 2, 960)
	state.observeFrameAtLocked(first, time.Unix(0, 0), time.Nanosecond, sessionDefaultAudioFreezeCap, 8)

	second := frame.NewAudioFrameS16(48000, 2, 480)
	state.observeFrameAtLocked(second, time.Unix(0, 2*int64(time.Millisecond)), time.Nanosecond, sessionDefaultAudioFreezeCap, 8)

	snapshot := state.snapshotLocked()
	if snapshot.FrameCount != 2 {
		t.Fatalf("frame count = %d, want 2", snapshot.FrameCount)
	}
	if snapshot.FreezeCount != 1 {
		t.Fatalf("freeze count = %d, want 1 with a tiny explicit threshold", snapshot.FreezeCount)
	}
	if len(snapshot.FreezeEvents) != 1 || snapshot.FreezeEvents[0].Kind != "frame" {
		t.Fatalf("freeze events = %+v, want one frame freeze event", snapshot.FreezeEvents)
	}
	if len(snapshot.ConfigSwitches) != 1 {
		t.Fatalf("config switches = %d, want 1", len(snapshot.ConfigSwitches))
	}
	if got := snapshot.CurrentPTime; got != second.Duration() {
		t.Fatalf("current ptime = %v, want %v", got, second.Duration())
	}
	if snapshot.Continuous {
		t.Fatalf("snapshot should not report continuous after a tiny-threshold frame gap: %+v", snapshot)
	}
}

func TestVideoTrackStateAdaptiveFreezeThreshold(t *testing.T) {
	state := &videoTrackState{id: "video-1"}
	start := time.Unix(0, 0)

	state.observeFrameAtLocked(&frame.VideoFrame{Width: 640, Height: 360, PTS: 0}, start, 0, sessionDefaultVideoFreezeCap, 8)
	state.observeFrameAtLocked(&frame.VideoFrame{Width: 640, Height: 360, PTS: 9000}, start.Add(100*time.Millisecond), 0, sessionDefaultVideoFreezeCap, 8)

	if got := state.freezeThresholdLocked(0, sessionDefaultVideoFreezeCap); got != 300*time.Millisecond {
		t.Fatalf("video adaptive threshold = %v, want 300ms", got)
	}

	state.observeFrameAtLocked(&frame.VideoFrame{Width: 640, Height: 360, PTS: 18000}, start.Add(380*time.Millisecond), 0, sessionDefaultVideoFreezeCap, 8)
	if state.freezeCount != 0 {
		t.Fatalf("freeze count after sub-threshold gap = %d, want 0", state.freezeCount)
	}

	state.observeFrameAtLocked(&frame.VideoFrame{Width: 640, Height: 360, PTS: 27000}, start.Add(760*time.Millisecond), 0, sessionDefaultVideoFreezeCap, 8)
	if state.freezeCount != 1 {
		t.Fatalf("freeze count after adaptive-threshold breach = %d, want 1", state.freezeCount)
	}
}

func TestAudioTrackStateAdaptiveFreezeThreshold(t *testing.T) {
	state := &audioTrackState{id: "audio-1"}
	start := time.Unix(0, 0)
	frame20ms := frame.NewAudioFrameS16(48000, 2, 960)

	state.observeFrameAtLocked(frame20ms, start, 0, sessionDefaultAudioFreezeCap, 8)
	state.observeFrameAtLocked(frame20ms, start.Add(20*time.Millisecond), 0, sessionDefaultAudioFreezeCap, 8)

	if got := state.freezeThresholdLocked(0, sessionDefaultAudioFreezeCap, frame20ms.Duration()); got != 80*time.Millisecond {
		t.Fatalf("audio adaptive threshold = %v, want 80ms", got)
	}

	state.observeFrameAtLocked(frame20ms, start.Add(90*time.Millisecond), 0, sessionDefaultAudioFreezeCap, 8)
	if state.freezeCount != 0 {
		t.Fatalf("freeze count after sub-threshold audio gap = %d, want 0", state.freezeCount)
	}

	state.observeFrameAtLocked(frame20ms, start.Add(180*time.Millisecond), 0, sessionDefaultAudioFreezeCap, 8)
	if state.freezeCount != 1 {
		t.Fatalf("freeze count after adaptive audio breach = %d, want 1", state.freezeCount)
	}
}

func TestSessionWaitForRespondsToSignalsImmediately(t *testing.T) {
	session := newSession(nil, SessionConfig{})
	session.mu.Lock()
	session.audioTracks["audio-1"] = &audioTrackState{id: "audio-1"}
	session.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer cancel()
		done <- session.WaitForAudioContinuous(ctx, "audio-1")
	}()

	time.Sleep(5 * time.Millisecond)
	session.mu.Lock()
	session.audioTracks["audio-1"].frameCount = 1
	session.signalLocked()
	session.mu.Unlock()

	if err := <-done; err != nil {
		t.Fatalf("WaitForAudioContinuous(signal) error = %v, want nil", err)
	}
}

func TestSessionValidateAggregation(t *testing.T) {
	session := newSession(nil, SessionConfig{EnableDataChannelHeartbeats: true, EventHistory: 8})
	session.recordFailure("boom")

	session.mu.Lock()
	session.appendConnectionStateLocked(time.Now(), webrtc.PeerConnectionStateDisconnected)
	session.appendSignalingStateLocked(time.Now(), webrtc.SignalingStateHaveLocalOffer)
	session.videoTracks["video-1"] = &videoTrackState{id: "video-1"}
	session.audioTracks["audio-1"] = &audioTrackState{id: "audio-1", frameCount: 1, freezeCount: 1}
	session.dataChannels[dataChannelKey("control", 7)] = &dataChannelState{
		label:           "control",
		id:              7,
		state:           "open",
		heartbeatMissed: 2,
		pendingAcks:     make(map[string]time.Time),
	}
	session.recordWarningLocked("warn")
	snapshot := session.snapshotLocked()
	session.mu.Unlock()

	if len(snapshot.Failures) != 1 || snapshot.Failures[0] != "boom" {
		t.Fatalf("snapshot failures = %v, want [boom]", snapshot.Failures)
	}
	if len(snapshot.Warnings) != 1 || snapshot.Warnings[0] != "warn" {
		t.Fatalf("snapshot warnings = %v, want [warn]", snapshot.Warnings)
	}

	err := session.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want aggregated error")
	}
	for _, want := range []string{
		"peer connection state is disconnected",
		"signaling state is have-local-offer",
		`video track "video-1" has no decoded frames`,
		`audio track "audio-1" experienced freezes`,
		`data channel "control" missed 2 heartbeats`,
		"boom",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Validate() error = %q, want substring %q", err.Error(), want)
		}
	}
}

func TestSessionHandleDataChannelMessageBranches(t *testing.T) {
	session := newSession(nil, SessionConfig{})
	dc := &fakeDataChannel{label: "control", id: 7, state: "open"}
	session.observeDataChannel(dc)

	session.mu.Lock()
	state := session.dataChannels[dataChannelKey("control", 7)]
	state.pendingAcks["ack-token"] = time.Now().Add(-10 * time.Millisecond)
	session.mu.Unlock()

	session.handleDataChannelMessage(state, []byte(heartbeatPingPrefix+"ping-token"))
	session.handleDataChannelMessage(state, []byte(heartbeatAckPrefix+"ack-token"))
	session.handleDataChannelMessage(state, []byte("hello"))

	snapshot := session.Snapshot().DataChannels[dataChannelKey("control", 7)]
	if snapshot.HeartbeatReceived != 1 || snapshot.HeartbeatAcked != 1 {
		t.Fatalf("heartbeat counts = %+v, want received=1 acked=1", snapshot)
	}
	if snapshot.UserMessagesReceived != 1 || snapshot.UserBytesReceived != uint64(len("hello")) {
		t.Fatalf("user counts = %+v, want one received message", snapshot)
	}
	if snapshot.LastHeartbeatRTT <= 0 {
		t.Fatalf("LastHeartbeatRTT = %v, want > 0", snapshot.LastHeartbeatRTT)
	}
	sent := dc.sentMessages()
	if len(sent) != 1 || sent[0] != heartbeatAckPrefix+"ping-token" {
		t.Fatalf("sent = %v, want heartbeat ack", sent)
	}
}

func TestSessionHeartbeatMissAndSendError(t *testing.T) {
	session := newSession(nil, SessionConfig{
		EnableDataChannelHeartbeats: true,
		HeartbeatInterval:           10 * time.Millisecond,
		HeartbeatTimeout:            10 * time.Millisecond,
	})
	dc := &fakeDataChannel{label: "control", id: 11, state: "open"}
	dc.setSuppressPingAck(true)
	session.observeDataChannel(dc)
	defer dc.close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		dcSnap, ok := snapshot.DataChannels[dataChannelKey("control", 11)]
		return ok && dcSnap.HeartbeatMissed > 0
	}); err != nil {
		t.Fatalf("wait for heartbeat miss: %v", err)
	}

	dc.setSendErr(errors.New("send failed"))
	if err := session.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		dcSnap, ok := snapshot.DataChannels[dataChannelKey("control", 11)]
		return ok && dcSnap.LastError == "send failed"
	}); err != nil {
		t.Fatalf("wait for heartbeat send error: %v", err)
	}
}

func TestSessionTrackStateHelpersAndAudioLevels(t *testing.T) {
	videoState := &videoTrackState{
		id:            "video-1",
		streamID:      "stream-1",
		rid:           "f",
		source:        "manual",
		currentWidth:  320,
		currentHeight: 180,
		lastFrameAt:   time.Now().Add(-20 * time.Millisecond),
	}
	videoState.observeFrameLocked(&frame.VideoFrame{Width: 640, Height: 360, IsKeyframe: true}, 5*time.Millisecond, sessionDefaultVideoFreezeCap, 4)
	videoState.observeCodecChangeLocked(pionrecv.CodecChange{
		CurrentType:  codec.H264,
		CurrentCodec: webrtc.RTPCodecParameters{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}},
	}, 4)
	videoSnap := videoState.snapshotLocked()
	if videoSnap.FrameCount != 1 || videoSnap.KeyframeCount != 1 || videoSnap.FreezeCount != 1 || videoSnap.ResolutionChanges != 1 {
		t.Fatalf("video snapshot = %+v", videoSnap)
	}
	if videoSnap.CurrentMimeType != webrtc.MimeTypeH264 || videoSnap.Continuous {
		t.Fatalf("video snapshot codec/continuous = %+v", videoSnap)
	}
	videoState.resetLocked()
	if snap := videoState.snapshotLocked(); snap.FrameCount != 0 || snap.CurrentWidth != 0 || snap.CurrentMimeType != webrtc.MimeTypeH264 {
		t.Fatalf("video snapshot after reset = %+v", snap)
	}

	silent := frame.NewAudioFrameFromS16([]int16{0, 0, 0, 0}, 48000, 2)
	clipped := frame.NewAudioFrameFromS16([]int16{32767, -32767}, 44100, 1)
	audioState := &audioTrackState{
		id:       "audio-1",
		streamID: "stream-1",
		source:   "manual",
	}
	audioState.observeFrameLocked(nil, 5*time.Millisecond, sessionDefaultAudioFreezeCap, 4)
	audioState.observeFrameLocked(silent, 5*time.Millisecond, sessionDefaultAudioFreezeCap, 4)
	audioState.lastFrameAt = time.Now().Add(-20 * time.Millisecond)
	audioState.observeFrameLocked(clipped, 5*time.Millisecond, sessionDefaultAudioFreezeCap, 4)
	audioState.observeCodecChangeLocked(pionrecv.CodecChange{
		CurrentType:  codec.PCMU,
		CurrentCodec: webrtc.RTPCodecParameters{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU}},
	}, 4)
	audioSnap := audioState.snapshotLocked()
	if audioSnap.FrameCount != 2 || audioSnap.FreezeCount != 1 || len(audioSnap.ConfigSwitches) != 1 {
		t.Fatalf("audio snapshot = %+v", audioSnap)
	}
	if audioSnap.ActiveFrames != 1 || audioSnap.SilentFrames != 1 || audioSnap.ClippedFrames != 1 {
		t.Fatalf("audio activity snapshot = %+v", audioSnap)
	}
	peak, rms, clippedFlag := audioLevels(clipped)
	if peak <= 0 || rms <= 0 || !clippedFlag {
		t.Fatalf("audioLevels(clipped) = (%v, %v, %v), want positive clipped levels", peak, rms, clippedFlag)
	}
	if peak, rms, clippedFlag = audioLevels(nil); peak != 0 || rms != 0 || clippedFlag {
		t.Fatalf("audioLevels(nil) = (%v, %v, %v), want zeros/false", peak, rms, clippedFlag)
	}
	audioState.resetLocked()
	if snap := audioState.snapshotLocked(); snap.FrameCount != 0 || snap.CurrentSampleRate != 0 || snap.CurrentMimeType != webrtc.MimeTypePCMU {
		t.Fatalf("audio snapshot after reset = %+v", snap)
	}
}

func TestSessionResetUsesCurrentDataChannelReadyState(t *testing.T) {
	session := newSession(nil, SessionConfig{})
	dc := &fakeDataChannel{label: "control", id: 7, state: "open"}
	session.observeDataChannel(dc)

	dc.state = "closed"
	session.Reset()

	snapshot := session.Snapshot()
	dcSnap, ok := snapshot.DataChannels[dataChannelKey("control", 7)]
	if !ok {
		t.Fatal("data channel snapshot missing after reset")
	}
	if dcSnap.State != "closed" {
		t.Fatalf("State after reset = %q, want closed", dcSnap.State)
	}
	if len(dcSnap.StateHistory) != 1 || dcSnap.StateHistory[0].State != "closed" {
		t.Fatalf("StateHistory after reset = %+v, want single closed state", dcSnap.StateHistory)
	}
}

func TestHasOpenDataChannel(t *testing.T) {
	snapshot := SessionSnapshot{
		DataChannels: map[string]DataChannelSnapshot{
			dataChannelKey("control", 7): {Label: "control", ID: 7, State: "closed"},
			dataChannelKey("chat", 9):    {Label: "chat", ID: 9, State: "open"},
		},
	}

	if !hasOpenDataChannel(snapshot, "") {
		t.Fatal("hasOpenDataChannel(empty label) = false, want true when any channel is open")
	}
	if hasOpenDataChannel(snapshot, "control") {
		t.Fatal("hasOpenDataChannel(control) = true, want false for closed channel")
	}
	if !hasOpenDataChannel(snapshot, "chat") {
		t.Fatal("hasOpenDataChannel(chat) = false, want true for open channel")
	}
}

func TestSessionBrowserPolicySkipsUnsupportedAssertions(t *testing.T) {
	session := newSession(nil, SessionConfig{Browser: "safari"})
	session.mu.Lock()
	session.videoTracks["video-1"] = &videoTrackState{id: "video-1"}
	session.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := session.WaitForCodecSwitch(ctx, "video-1", webrtc.MimeTypeH264); err != nil {
		t.Fatalf("WaitForCodecSwitch() error = %v, want nil skip", err)
	}
	if err := session.WaitForVideoRID(ctx, "video-1", "f"); err != nil {
		t.Fatalf("WaitForVideoRID() error = %v, want nil skip", err)
	}
	if err := session.WaitForVideoLayer(ctx, "video-1", 2, 0); err != nil {
		t.Fatalf("WaitForVideoLayer() error = %v, want nil skip", err)
	}

	snapshot := session.Snapshot()
	if len(snapshot.SkippedExpectations) != 3 {
		t.Fatalf("len(SkippedExpectations) = %d, want 3", len(snapshot.SkippedExpectations))
	}
}

func TestSessionVideoSpecificWaiters(t *testing.T) {
	session := newSession(nil, SessionConfig{})
	session.mu.Lock()
	session.videoTracks["video-1"] = &videoTrackState{
		id:              "video-1",
		currentCodec:    codec.VP9,
		currentMime:     webrtc.MimeTypeVP9,
		frameCount:      1,
		currentWidth:    640,
		currentHeight:   360,
		currentRID:      "h",
		hasCurrentLayer: true,
		currentLayer:    pionrecv.VideoLayer{Spatial: 1, Temporal: 0},
	}
	session.signalLocked()
	session.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.WaitForVideoRID(ctx, "video-1", "h"); err != nil {
		t.Fatalf("WaitForVideoRID: %v", err)
	}
	if err := session.WaitForVideoLayer(ctx, "video-1", 1, 0); err != nil {
		t.Fatalf("WaitForVideoLayer: %v", err)
	}
	if err := session.WaitForVideoResolution(ctx, "video-1", 640, 360); err != nil {
		t.Fatalf("WaitForVideoResolution: %v", err)
	}
}

func TestScenarioLabSkipsUnsupportedLayerScenarios(t *testing.T) {
	session := newSession(nil, SessionConfig{Browser: "safari"})
	lab := NewScenarioLab(session, LabConfig{})
	video := &fakePublishedVideo{}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := lab.Run(ctx, ScenarioScript{
		Steps: []ScenarioStep{
			{Action: ScenarioActionSetLayerActive, Video: video, LayerIndex: 1, Active: true},
			{Action: ScenarioActionSetLayerBitrate, Video: video, LayerIndex: 1, Bitrate: 1000},
			{Action: ScenarioActionCodecRenegotiation, Callback: func(context.Context, *Session) error {
				t.Fatal("codec renegotiation callback should have been skipped")
				return nil
			}},
		},
	}); err != nil {
		t.Fatalf("ScenarioLab.Run: %v", err)
	}
	if len(video.layerCalls) != 0 || len(video.bitrateCalls) != 0 {
		t.Fatalf("video calls = %+v %+v, want skipped without invocations", video.layerCalls, video.bitrateCalls)
	}
	snapshot := session.Snapshot()
	if len(snapshot.SkippedExpectations) != 3 {
		t.Fatalf("len(SkippedExpectations) = %d, want 3", len(snapshot.SkippedExpectations))
	}
}
