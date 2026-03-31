package validate

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/media"
	"github.com/thesyncim/libgowebrtc/pkg/pc"
	"github.com/thesyncim/libgowebrtc/pkg/pionrecv"
)

const (
	sessionChangePollInterval    = 50 * time.Millisecond
	heartbeatPingPrefix          = "__libgowebrtc_validate_hb_ping__:"
	heartbeatAckPrefix           = "__libgowebrtc_validate_hb_ack__:"
	sessionAudioSilenceThreshold = 0.002
	sessionDefaultVideoFreezeCap = 450 * time.Millisecond
	sessionMinVideoFreeze        = 250 * time.Millisecond
	sessionDefaultAudioFreezeCap = 150 * time.Millisecond
	sessionMinAudioFreeze        = 80 * time.Millisecond
)

type peerAdapter interface {
	GetStats() (webrtc.StatsReport, error)
	ConnectionState() webrtc.PeerConnectionState
	ICEConnectionState() webrtc.ICEConnectionState
	SignalingState() webrtc.SignalingState
}

type pionPeerAdapter struct {
	pc *webrtc.PeerConnection
}

func (p *pionPeerAdapter) GetStats() (webrtc.StatsReport, error) { return p.pc.GetStats(), nil }
func (p *pionPeerAdapter) ConnectionState() webrtc.PeerConnectionState {
	return p.pc.ConnectionState()
}
func (p *pionPeerAdapter) ICEConnectionState() webrtc.ICEConnectionState {
	return p.pc.ICEConnectionState()
}
func (p *pionPeerAdapter) SignalingState() webrtc.SignalingState {
	return p.pc.SignalingState()
}

type nativePeerAdapter struct {
	pc *pc.PeerConnection
}

func (n *nativePeerAdapter) GetStats() (webrtc.StatsReport, error) { return n.pc.GetStats() }
func (n *nativePeerAdapter) ConnectionState() webrtc.PeerConnectionState {
	return n.pc.ConnectionState()
}
func (n *nativePeerAdapter) ICEConnectionState() webrtc.ICEConnectionState {
	return n.pc.ICEConnectionState()
}
func (n *nativePeerAdapter) SignalingState() webrtc.SignalingState {
	return n.pc.SignalingState()
}

type dataChannelAdapter interface {
	Label() string
	ID() int
	ReadyStateString() string
	SetOnOpen(func())
	SetOnClose(func())
	SetOnMessage(func([]byte))
	SetOnError(func(error))
	SendText(string) error
}

type pionDataChannelAdapter struct {
	dc *webrtc.DataChannel
}

func (p *pionDataChannelAdapter) Label() string { return p.dc.Label() }
func (p *pionDataChannelAdapter) ID() int {
	if id := p.dc.ID(); id != nil {
		return int(*id)
	}
	return -1
}
func (p *pionDataChannelAdapter) ReadyStateString() string { return p.dc.ReadyState().String() }
func (p *pionDataChannelAdapter) SetOnOpen(cb func())      { p.dc.OnOpen(cb) }
func (p *pionDataChannelAdapter) SetOnClose(cb func())     { p.dc.OnClose(cb) }
func (p *pionDataChannelAdapter) SetOnMessage(cb func([]byte)) {
	p.dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		cb(append([]byte(nil), msg.Data...))
	})
}
func (p *pionDataChannelAdapter) SetOnError(cb func(error))  { p.dc.OnError(cb) }
func (p *pionDataChannelAdapter) SendText(text string) error { return p.dc.SendText(text) }

type nativeDataChannelAdapter struct {
	dc *pc.DataChannel
}

func (n *nativeDataChannelAdapter) Label() string            { return n.dc.Label() }
func (n *nativeDataChannelAdapter) ID() int                  { return int(n.dc.ID()) }
func (n *nativeDataChannelAdapter) ReadyStateString() string { return n.dc.ReadyState().String() }
func (n *nativeDataChannelAdapter) SetOnOpen(cb func())      { n.dc.SetOnOpen(cb) }
func (n *nativeDataChannelAdapter) SetOnClose(cb func())     { n.dc.SetOnClose(cb) }
func (n *nativeDataChannelAdapter) SetOnMessage(cb func([]byte)) {
	n.dc.SetOnMessage(func(data []byte) {
		cb(append([]byte(nil), data...))
	})
}
func (n *nativeDataChannelAdapter) SetOnError(cb func(error))  { n.dc.SetOnError(cb) }
func (n *nativeDataChannelAdapter) SendText(text string) error { return n.dc.SendText(text) }

type videoTrackState struct {
	id       string
	streamID string
	rid      string
	source   string
	ssrc     uint32

	currentCodec codec.Type
	currentMime  string
	currentPT    webrtc.PayloadType

	startedAt         time.Time
	lastFrameAt       time.Time
	frameCount        uint64
	keyframeCount     uint64
	freezeCount       uint64
	lastFramePTS      uint32
	hasLastFramePTS   bool
	estimatedStep     time.Duration
	currentWidth      int
	currentHeight     int
	resolutionChanges uint64
	currentMID        string
	currentRID        string
	hasCurrentLayer   bool
	currentLayer      pionrecv.VideoLayer
	hasMaxActiveLayer bool
	maxActiveLayer    pionrecv.VideoLayer

	codecSwitches []pionrecv.CodecSwitchObservation
	freezeEvents  []pionrecv.FreezeEvent
	stats         []RTPStatsSample
	monitor       *pionrecv.VideoSubscriberMonitor
}

type audioTrackState struct {
	id       string
	streamID string
	rid      string
	source   string
	ssrc     uint32

	currentCodec codec.Type
	currentMime  string
	currentPT    webrtc.PayloadType
	currentMID   string

	startedAt         time.Time
	lastFrameAt       time.Time
	frameCount        uint64
	freezeCount       uint64
	estimatedStep     time.Duration
	currentSampleRate int
	currentChannels   int
	currentNumSamples int
	currentPTime      time.Duration
	peakLevel         float64
	rmsLevel          float64
	activeFrames      uint64
	silentFrames      uint64
	clippedFrames     uint64

	codecSwitches  []pionrecv.CodecSwitchObservation
	configSwitches []pionrecv.AudioConfigSwitch
	freezeEvents   []pionrecv.FreezeEvent
	stats          []RTPStatsSample
	monitor        *pionrecv.AudioSubscriberMonitor
}

type dataChannelState struct {
	adapter dataChannelAdapter
	gen     uint64

	label string
	id    int

	openedAt         time.Time
	closedAt         time.Time
	openTransitions  uint64
	closeTransitions uint64

	userMessagesSent     uint64
	userMessagesReceived uint64
	userBytesSent        uint64
	userBytesReceived    uint64

	heartbeatSent      uint64
	heartbeatReceived  uint64
	heartbeatAcked     uint64
	heartbeatMissed    uint64
	lastHeartbeatRTT   time.Duration
	lastHeartbeatAt    time.Time
	lastHeartbeatAckAt time.Time
	lastError          string
	state              string

	stateHistory []DataChannelStateEvent
	stats        []DataChannelStatsSample
	pendingAcks  map[string]time.Time
	paused       bool
	heartbeatOn  bool
}

// Session is the browser-style validation surface for media, data channels,
// and transport state.
type Session struct {
	cfg      SessionConfig
	policy   BrowserPolicy
	peer     peerAdapter
	registry *media.RemoteStreamRegistry

	mu sync.Mutex

	connectionStates    []PeerConnectionStateEvent
	iceConnectionStates []ICEConnectionStateEvent
	signalingStates     []SignalingStateEvent

	videoTracks  map[string]*videoTrackState
	audioTracks  map[string]*audioTrackState
	dataChannels map[string]*dataChannelState

	transportSamples []TransportSample
	unmatchedStreams []RTPStatsSample
	lastStatsPoll    time.Time

	failures []string
	warnings []string
	skipped  []string

	changed chan struct{}
}

// NewPionSession creates a validation session around a Pion PeerConnection.
func NewPionSession(pc *webrtc.PeerConnection, registry *media.RemoteStreamRegistry, cfg SessionConfig) *Session {
	if pc == nil {
		return newSession(nil, registry, cfg)
	}
	return newSession(&pionPeerAdapter{pc: pc}, registry, cfg)
}

// NewPCSession creates a validation session around a native pkg/pc PeerConnection.
func NewPCSession(pc *pc.PeerConnection, registry *media.RemoteStreamRegistry, cfg SessionConfig) *Session {
	if pc == nil {
		return newSession(nil, registry, cfg)
	}
	return newSession(&nativePeerAdapter{pc: pc}, registry, cfg)
}

func newSession(peer peerAdapter, registry *media.RemoteStreamRegistry, cfg SessionConfig) *Session {
	cfg, policy := normalizeSessionConfig(cfg)
	if registry == nil {
		registry = media.NewRemoteStreamRegistry()
	}

	s := &Session{
		cfg:          cfg,
		policy:       policy,
		peer:         peer,
		registry:     registry,
		videoTracks:  make(map[string]*videoTrackState),
		audioTracks:  make(map[string]*audioTrackState),
		dataChannels: make(map[string]*dataChannelState),
		changed:      make(chan struct{}),
	}
	s.refresh(time.Now())
	return s
}

// PionOnTrack returns an OnTrack handler that binds browser-shaped remote
// tracks and passive subscriber monitors into the session.
func (s *Session) PionOnTrack() func(*webrtc.TrackRemote, *webrtc.RTPReceiver) {
	return func(trackRemote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		if trackRemote == nil {
			s.recordFailure("validate: nil pion remote track")
			return
		}

		opts := make([]pionrecv.Option, 0, 3)
		if receiver != nil && receiver.Transport() != nil {
			opts = append(opts, pionrecv.WithRTCPWriter(receiver.Transport()))
		}

		var (
			videoMonitor *pionrecv.VideoSubscriberMonitor
			audioMonitor *pionrecv.AudioSubscriberMonitor
		)
		switch trackRemote.Kind() {
		case webrtc.RTPCodecTypeVideo:
			videoMonitor = pionrecv.NewVideoSubscriberMonitor(pionrecv.VideoSubscriberMonitorConfig{
				FreezeThreshold:    s.cfg.FreezeThreshold,
				PacketGapThreshold: s.cfg.PacketGapThreshold,
				EventHistory:       s.cfg.EventHistory,
			})
			opts = append(opts, pionrecv.WithVideoSubscriberMonitor(videoMonitor))
		case webrtc.RTPCodecTypeAudio:
			audioMonitor = pionrecv.NewAudioSubscriberMonitor(pionrecv.AudioSubscriberMonitorConfig{
				FreezeThreshold:    s.cfg.AudioGapThreshold,
				PacketGapThreshold: s.cfg.PacketGapThreshold,
				EventHistory:       s.cfg.EventHistory,
			})
			opts = append(opts, pionrecv.WithAudioSubscriberMonitor(audioMonitor))
		}

		remoteTrack, _, err := s.registry.BindPionTrack(trackRemote, receiver, opts...)
		if err != nil {
			s.recordFailure(fmt.Sprintf("validate: bind pion track %q: %v", trackRemote.ID(), err))
			return
		}
		s.observeRemoteTrackInternal(remoteTrack, "pion", videoMonitor, audioMonitor)
	}
}

// PCOnTrack returns an OnTrack handler that binds browser-shaped remote tracks
// into the session.
func (s *Session) PCOnTrack() func(*pc.Track, *pc.RTPReceiver, []string) {
	return func(trackRemote *pc.Track, receiver *pc.RTPReceiver, streams []string) {
		if trackRemote == nil {
			s.recordFailure("validate: nil pc remote track")
			return
		}
		remoteTrack, _, err := s.registry.BindPCTrack(trackRemote, receiver, streams)
		if err != nil {
			s.recordFailure(fmt.Sprintf("validate: bind pc track %q: %v", trackRemote.ID(), err))
			return
		}
		s.observeRemoteTrackInternal(remoteTrack, "pc", nil, nil)
	}
}

// ObserveRemoteTrack adds a previously-bound browser-style remote track to the
// session. This is best-effort when a track already has caller-installed frame
// callbacks.
func (s *Session) ObserveRemoteTrack(track media.RemoteTrack) {
	s.observeRemoteTrackInternal(track, "manual", nil, nil)
}

func (s *Session) observeRemoteTrackInternal(track media.RemoteTrack, source string, videoMonitor *pionrecv.VideoSubscriberMonitor, audioMonitor *pionrecv.AudioSubscriberMonitor) {
	if track == nil {
		return
	}

	switch typed := track.(type) {
	case media.RemoteVideoTrack:
		s.observeVideoTrack(typed, source, videoMonitor)
	case media.RemoteAudioTrack:
		s.observeAudioTrack(typed, source, audioMonitor)
	}
}

func (s *Session) observeVideoTrack(track media.RemoteVideoTrack, source string, monitor *pionrecv.VideoSubscriberMonitor) {
	s.mu.Lock()
	state := s.ensureVideoTrackStateLocked(track, source, monitor)
	s.signalLocked()
	s.mu.Unlock()

	if monitor != nil {
		return
	}

	if err := track.SetOnVideoFrame(func(f *frame.VideoFrame) {
		s.mu.Lock()
		defer s.mu.Unlock()
		state.observeFrameLocked(f, s.cfg.FreezeThreshold, s.policy.DefaultFreezeThreshold, s.cfg.EventHistory)
		s.signalLocked()
	}); err != nil {
		s.recordWarning(fmt.Sprintf("validate: install video frame callback for %q: %v", track.ID(), err))
	}
	if codecTrack, ok := track.(media.PionRemoteVideoTrack); ok {
		codecTrack.SetOnCodecChange(func(change pionrecv.CodecChange) {
			s.mu.Lock()
			defer s.mu.Unlock()
			state.observeCodecChangeLocked(change, s.cfg.EventHistory)
			s.signalLocked()
		})
	}
}

func (s *Session) observeAudioTrack(track media.RemoteAudioTrack, source string, monitor *pionrecv.AudioSubscriberMonitor) {
	s.mu.Lock()
	state := s.ensureAudioTrackStateLocked(track, source, monitor)
	s.signalLocked()
	s.mu.Unlock()

	if monitor != nil {
		return
	}

	if err := track.SetOnAudioFrame(func(f *frame.AudioFrame) {
		s.mu.Lock()
		defer s.mu.Unlock()
		state.observeFrameLocked(f, s.cfg.AudioGapThreshold, s.policy.DefaultAudioGapThreshold, s.cfg.EventHistory)
		s.signalLocked()
	}); err != nil {
		s.recordWarning(fmt.Sprintf("validate: install audio frame callback for %q: %v", track.ID(), err))
	}
	if codecTrack, ok := track.(media.PionRemoteAudioTrack); ok {
		codecTrack.SetOnCodecChange(func(change pionrecv.CodecChange) {
			s.mu.Lock()
			defer s.mu.Unlock()
			state.observeCodecChangeLocked(change, s.cfg.EventHistory)
			s.signalLocked()
		})
	}
}

func (s *Session) ensureVideoTrackStateLocked(track media.RemoteVideoTrack, source string, monitor *pionrecv.VideoSubscriberMonitor) *videoTrackState {
	state, ok := s.videoTracks[track.ID()]
	if !ok {
		state = &videoTrackState{
			id:       track.ID(),
			streamID: track.StreamID(),
			rid:      track.RID(),
			source:   source,
		}
		s.videoTracks[track.ID()] = state
	}
	if source != "" {
		state.source = source
	}
	if state.streamID == "" {
		state.streamID = track.StreamID()
	}
	if state.rid == "" {
		state.rid = track.RID()
	}
	if monitor != nil {
		state.monitor = monitor
	}
	if codecTrack, ok := track.(media.RemoteCodecTrack); ok {
		state.currentCodec = codecTrack.Codec()
		state.currentMime = codecTrack.CodecParameters().MimeType
		state.currentPT = codecTrack.PayloadType()
	}
	if pionTrack, ok := track.(media.PionRemoteVideoTrack); ok {
		if decoded := pionTrack.DecodedTrack(); decoded != nil && decoded.TrackRemote() != nil {
			state.ssrc = uint32(decoded.TrackRemote().SSRC())
		}
	}
	return state
}

func (s *Session) ensureAudioTrackStateLocked(track media.RemoteAudioTrack, source string, monitor *pionrecv.AudioSubscriberMonitor) *audioTrackState {
	state, ok := s.audioTracks[track.ID()]
	if !ok {
		state = &audioTrackState{
			id:       track.ID(),
			streamID: track.StreamID(),
			rid:      track.RID(),
			source:   source,
		}
		s.audioTracks[track.ID()] = state
	}
	if source != "" {
		state.source = source
	}
	if state.streamID == "" {
		state.streamID = track.StreamID()
	}
	if state.rid == "" {
		state.rid = track.RID()
	}
	if monitor != nil {
		state.monitor = monitor
	}
	if codecTrack, ok := track.(media.RemoteCodecTrack); ok {
		state.currentCodec = codecTrack.Codec()
		state.currentMime = codecTrack.CodecParameters().MimeType
		state.currentPT = codecTrack.PayloadType()
	}
	if pionTrack, ok := track.(media.PionRemoteAudioTrack); ok {
		if decoded := pionTrack.DecodedTrack(); decoded != nil && decoded.TrackRemote() != nil {
			state.ssrc = uint32(decoded.TrackRemote().SSRC())
		}
	}
	return state
}

// ObservePionDataChannel adds a Pion data channel to the session.
func (s *Session) ObservePionDataChannel(dc *webrtc.DataChannel) {
	if dc == nil {
		return
	}
	s.observeDataChannel(&pionDataChannelAdapter{dc: dc})
}

// ObservePCDataChannel adds a native pkg/pc data channel to the session.
func (s *Session) ObservePCDataChannel(dc *pc.DataChannel) {
	if dc == nil {
		return
	}
	s.observeDataChannel(&nativeDataChannelAdapter{dc: dc})
}

func (s *Session) observeDataChannel(adapter dataChannelAdapter) {
	if adapter == nil {
		return
	}

	key := dataChannelKey(adapter.Label(), adapter.ID())
	readyState := adapter.ReadyStateString()

	s.mu.Lock()
	state, ok := s.dataChannels[key]
	if !ok {
		state = &dataChannelState{
			adapter:     adapter,
			gen:         1,
			label:       adapter.Label(),
			id:          adapter.ID(),
			state:       readyState,
			pendingAcks: make(map[string]time.Time),
		}
		state.appendStateLocked(state.state, s.cfg.EventHistory)
		s.dataChannels[key] = state
	} else {
		state.gen++
		state.adapter = adapter
		state.label = adapter.Label()
		state.id = adapter.ID()
		if readyState == "" {
			readyState = state.state
		}
		if state.state != readyState || len(state.stateHistory) == 0 {
			state.state = readyState
			state.appendStateLocked(readyState, s.cfg.EventHistory)
		}
	}
	s.signalLocked()
	generation := state.gen
	s.mu.Unlock()

	adapter.SetOnOpen(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if state.gen != generation {
			return
		}
		state.state = "open"
		state.openTransitions++
		if state.openedAt.IsZero() {
			state.openedAt = time.Now()
		}
		clear(state.pendingAcks)
		state.appendStateLocked("open", s.cfg.EventHistory)
		s.signalLocked()
		if s.cfg.EnableDataChannelHeartbeats && !state.heartbeatOn {
			state.heartbeatOn = true
			go s.runHeartbeatLoop(state)
		}
	})
	adapter.SetOnClose(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if state.gen != generation {
			return
		}
		state.state = "closed"
		state.closeTransitions++
		state.closedAt = time.Now()
		state.heartbeatOn = false
		clear(state.pendingAcks)
		state.appendStateLocked("closed", s.cfg.EventHistory)
		s.signalLocked()
	})
	adapter.SetOnMessage(func(data []byte) {
		s.mu.Lock()
		if state.gen != generation {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		s.handleDataChannelMessage(state, data)
	})
	adapter.SetOnError(func(err error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if state.gen != generation {
			return
		}
		state.lastError = errString(err)
		s.signalLocked()
	})

	if adapter.ReadyStateString() == "open" && s.cfg.EnableDataChannelHeartbeats {
		s.mu.Lock()
		if !state.heartbeatOn {
			state.heartbeatOn = true
			go s.runHeartbeatLoop(state)
		}
		s.mu.Unlock()
	}
}

func (s *Session) handleDataChannelMessage(state *dataChannelState, data []byte) {
	now := time.Now()
	text := string(data)

	switch {
	case strings.HasPrefix(text, heartbeatPingPrefix):
		token := strings.TrimPrefix(text, heartbeatPingPrefix)
		s.mu.Lock()
		state.heartbeatReceived++
		state.lastHeartbeatAt = now
		s.signalLocked()
		s.mu.Unlock()
		_ = state.adapter.SendText(heartbeatAckPrefix + token)
	case strings.HasPrefix(text, heartbeatAckPrefix):
		token := strings.TrimPrefix(text, heartbeatAckPrefix)
		s.mu.Lock()
		defer s.mu.Unlock()
		state.heartbeatAcked++
		state.lastHeartbeatAckAt = now
		if sentAt, ok := state.pendingAcks[token]; ok {
			state.lastHeartbeatRTT = now.Sub(sentAt)
			delete(state.pendingAcks, token)
		}
		s.signalLocked()
	default:
		s.mu.Lock()
		defer s.mu.Unlock()
		state.userMessagesReceived++
		state.userBytesReceived += uint64(len(data))
		s.signalLocked()
	}
}

func (s *Session) runHeartbeatLoop(state *dataChannelState) {
	ticker := time.NewTicker(s.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		if state.state == "closed" {
			state.heartbeatOn = false
			s.mu.Unlock()
			return
		}
		if state.paused {
			s.mu.Unlock()
			continue
		}

		now := time.Now()
		for token, sentAt := range state.pendingAcks {
			if now.Sub(sentAt) > s.cfg.HeartbeatTimeout {
				state.heartbeatMissed++
				delete(state.pendingAcks, token)
			}
		}

		token := fmt.Sprintf("%d", now.UnixNano())
		state.pendingAcks[token] = now
		state.heartbeatSent++
		state.lastHeartbeatAt = now
		s.signalLocked()
		s.mu.Unlock()

		if err := state.adapter.SendText(heartbeatPingPrefix + token); err != nil {
			s.mu.Lock()
			state.lastError = errString(err)
			s.signalLocked()
			s.mu.Unlock()
		}
	}
}

// Snapshot returns the current validation snapshot.
func (s *Session) Snapshot() SessionSnapshot {
	s.refresh(time.Now())
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

// Reset clears accumulated observations while preserving active bindings.
func (s *Session) Reset() {
	s.mu.Lock()
	s.connectionStates = nil
	s.iceConnectionStates = nil
	s.signalingStates = nil
	s.transportSamples = nil
	s.unmatchedStreams = nil
	s.failures = nil
	s.warnings = nil
	s.skipped = nil
	s.lastStatsPoll = time.Time{}

	for _, state := range s.videoTracks {
		state.resetLocked()
	}
	for _, state := range s.audioTracks {
		state.resetLocked()
	}
	for _, state := range s.dataChannels {
		state.resetLocked(s.cfg.EventHistory)
	}
	s.signalLocked()
	s.mu.Unlock()

	s.refresh(time.Now())
}

// Validate returns an aggregated validation error when the session is not in a
// healthy browser-visible state.
func (s *Session) Validate() error {
	snap := s.Snapshot()
	var problems []string

	if len(snap.ConnectionStates) > 0 {
		last := snap.ConnectionStates[len(snap.ConnectionStates)-1].State
		if last != webrtc.PeerConnectionStateConnected && last != webrtc.PeerConnectionStateConnecting {
			problems = append(problems, fmt.Sprintf("peer connection state is %s", last.String()))
		}
	}
	if len(snap.SignalingStates) > 0 {
		last := snap.SignalingStates[len(snap.SignalingStates)-1].State
		if last != webrtc.SignalingStateStable {
			problems = append(problems, fmt.Sprintf("signaling state is %s", last.String()))
		}
	}

	for _, track := range snap.VideoTracks {
		if track.FrameCount == 0 {
			problems = append(problems, fmt.Sprintf("video track %q has no decoded frames", track.TrackID))
			continue
		}
		if !track.Continuous {
			problems = append(problems, fmt.Sprintf("video track %q experienced freezes", track.TrackID))
		}
	}
	for _, track := range snap.AudioTracks {
		if track.FrameCount == 0 {
			problems = append(problems, fmt.Sprintf("audio track %q has no decoded frames", track.TrackID))
			continue
		}
		if !track.Continuous {
			problems = append(problems, fmt.Sprintf("audio track %q experienced freezes", track.TrackID))
		}
	}
	for _, dc := range snap.DataChannels {
		if s.cfg.EnableDataChannelHeartbeats && dc.HeartbeatMissed > 0 {
			problems = append(problems, fmt.Sprintf("data channel %q missed %d heartbeats", dc.Label, dc.HeartbeatMissed))
		}
	}
	problems = append(problems, snap.Failures...)
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
}

// WaitForConnected waits until the session reaches a connected state.
func (s *Session) WaitForConnected(ctx context.Context) error {
	return s.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		if len(snapshot.ConnectionStates) == 0 {
			return false
		}
		state := snapshot.ConnectionStates[len(snapshot.ConnectionStates)-1].State
		return state == webrtc.PeerConnectionStateConnected
	})
}

// WaitForStable waits until the session returns to stable signaling.
func (s *Session) WaitForStable(ctx context.Context) error {
	return s.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		if len(snapshot.SignalingStates) == 0 {
			return false
		}
		return snapshot.SignalingStates[len(snapshot.SignalingStates)-1].State == webrtc.SignalingStateStable
	})
}

// WaitForVideoContinuous waits until the target video track is continuously rendering.
func (s *Session) WaitForVideoContinuous(ctx context.Context, trackID string) error {
	return s.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		track, ok := snapshot.VideoTracks[trackID]
		return ok && track.Continuous
	})
}

// WaitForAudioContinuous waits until the target audio track is continuously rendering.
func (s *Session) WaitForAudioContinuous(ctx context.Context, trackID string) error {
	return s.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		track, ok := snapshot.AudioTracks[trackID]
		return ok && track.Continuous
	})
}

// WaitForCodecSwitch waits until the target track reflects the requested MIME type.
func (s *Session) WaitForCodecSwitch(ctx context.Context, trackID, mime string) error {
	if !s.policy.SupportsCodecSwitchAssertions {
		s.recordSkip(fmt.Sprintf("codec switch assertions are not guaranteed for browser profile %q", s.policy.Browser))
		return nil
	}
	return s.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		if track, ok := snapshot.VideoTracks[trackID]; ok {
			return strings.EqualFold(track.CurrentMimeType, mime)
		}
		if track, ok := snapshot.AudioTracks[trackID]; ok {
			return strings.EqualFold(track.CurrentMimeType, mime)
		}
		return false
	})
}

// WaitForVideoLayer waits until the target video track reports the desired
// spatial/temporal layer through the subscriber-visible monitor.
func (s *Session) WaitForVideoLayer(ctx context.Context, trackID string, spatial, temporal int) error {
	if !s.policy.SupportsDependencyDescriptor {
		s.recordSkip(fmt.Sprintf("dependency-descriptor layer assertions are not guaranteed for browser profile %q", s.policy.Browser))
		return nil
	}
	return s.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		track, ok := snapshot.VideoTracks[trackID]
		return ok && track.HasCurrentLayer && track.CurrentLayer == (pionrecv.VideoLayer{Spatial: spatial, Temporal: temporal})
	})
}

// WaitForVideoRID waits until the target video track reports the desired RID.
func (s *Session) WaitForVideoRID(ctx context.Context, trackID, rid string) error {
	if !s.policy.SupportsRID {
		s.recordSkip(fmt.Sprintf("RID assertions are not guaranteed for browser profile %q", s.policy.Browser))
		return nil
	}
	return s.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		track, ok := snapshot.VideoTracks[trackID]
		return ok && track.CurrentRID == rid
	})
}

// WaitForVideoResolution waits until the target video track reports the desired resolution.
func (s *Session) WaitForVideoResolution(ctx context.Context, trackID string, width, height int) error {
	return s.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		track, ok := snapshot.VideoTracks[trackID]
		return ok && track.CurrentWidth == width && track.CurrentHeight == height
	})
}

// WaitForDataChannelOpen waits until the named data channel reports an open state.
func (s *Session) WaitForDataChannelOpen(ctx context.Context, label string) error {
	return s.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		return hasOpenDataChannel(snapshot, label)
	})
}

// WaitForReconnected waits until the session has re-entered the connected state
// after first becoming disconnected or failed.
func (s *Session) WaitForReconnected(ctx context.Context) error {
	return s.waitFor(ctx, func(snapshot SessionSnapshot) bool {
		var sawBreak bool
		for _, event := range snapshot.ConnectionStates {
			switch event.State {
			case webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateFailed:
				sawBreak = true
			case webrtc.PeerConnectionStateConnected:
				if sawBreak {
					return true
				}
			}
		}
		return false
	})
}

func (s *Session) waitFor(ctx context.Context, predicate func(SessionSnapshot) bool) error {
	for {
		s.mu.Lock()
		changed := s.changed
		waitInterval := sessionChangePollInterval
		if s.cfg.StatsPollInterval > 0 && s.cfg.StatsPollInterval < waitInterval {
			waitInterval = s.cfg.StatsPollInterval
		}
		s.mu.Unlock()

		snapshot := s.Snapshot()
		if predicate(snapshot) {
			return nil
		}

		timer := time.NewTimer(waitInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (s *Session) refresh(now time.Time) {
	if s.peer == nil {
		return
	}

	connectionState := s.peer.ConnectionState()
	iceState := s.peer.ICEConnectionState()
	signalingState := s.peer.SignalingState()

	var shouldPollStats bool
	s.mu.Lock()
	s.appendConnectionStateLocked(now, connectionState)
	s.appendICEConnectionStateLocked(now, iceState)
	s.appendSignalingStateLocked(now, signalingState)

	if !s.lastStatsPoll.IsZero() && now.Sub(s.lastStatsPoll) < s.cfg.StatsPollInterval {
		s.mu.Unlock()
		return
	}
	s.lastStatsPoll = now
	shouldPollStats = true
	s.mu.Unlock()

	if !shouldPollStats {
		return
	}

	report, err := s.peer.GetStats()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendConnectionStateLocked(now, connectionState)
	s.appendICEConnectionStateLocked(now, iceState)
	s.appendSignalingStateLocked(now, signalingState)
	if err != nil {
		s.recordWarningLocked(fmt.Sprintf("validate: get stats: %v", err))
		return
	}
	s.applyStatsReportLocked(report)
}

func (s *Session) snapshotLocked() SessionSnapshot {
	videoTracks := make(map[string]VideoTrackSnapshot, len(s.videoTracks))
	for id, state := range s.videoTracks {
		videoTracks[id] = state.snapshotLocked()
	}
	audioTracks := make(map[string]AudioTrackSnapshot, len(s.audioTracks))
	for id, state := range s.audioTracks {
		audioTracks[id] = state.snapshotLocked()
	}
	dataChannels := make(map[string]DataChannelSnapshot, len(s.dataChannels))
	for id, state := range s.dataChannels {
		dataChannels[id] = state.snapshotLocked()
	}

	return SessionSnapshot{
		Browser:             s.cfg.Browser,
		Policy:              s.policy,
		ConnectionStates:    append([]PeerConnectionStateEvent(nil), s.connectionStates...),
		ICEConnectionStates: append([]ICEConnectionStateEvent(nil), s.iceConnectionStates...),
		SignalingStates:     append([]SignalingStateEvent(nil), s.signalingStates...),
		VideoTracks:         videoTracks,
		AudioTracks:         audioTracks,
		DataChannels:        dataChannels,
		Transport: TransportSnapshot{
			Samples:          append([]TransportSample(nil), s.transportSamples...),
			UnmatchedStreams: append([]RTPStatsSample(nil), s.unmatchedStreams...),
			DataChannels:     collectDataChannelStatsLocked(s.dataChannels),
		},
		Failures:            append([]string(nil), s.failures...),
		Warnings:            append([]string(nil), s.warnings...),
		SkippedExpectations: append([]string(nil), s.skipped...),
	}
}

func (s *Session) appendConnectionStateLocked(at time.Time, state webrtc.PeerConnectionState) {
	if len(s.connectionStates) > 0 && s.connectionStates[len(s.connectionStates)-1].State == state {
		return
	}
	s.connectionStates = appendLimited(s.connectionStates, PeerConnectionStateEvent{At: at, State: state}, s.cfg.EventHistory)
}

func (s *Session) appendICEConnectionStateLocked(at time.Time, state webrtc.ICEConnectionState) {
	if len(s.iceConnectionStates) > 0 && s.iceConnectionStates[len(s.iceConnectionStates)-1].State == state {
		return
	}
	s.iceConnectionStates = appendLimited(s.iceConnectionStates, ICEConnectionStateEvent{At: at, State: state}, s.cfg.EventHistory)
}

func (s *Session) appendSignalingStateLocked(at time.Time, state webrtc.SignalingState) {
	if len(s.signalingStates) > 0 && s.signalingStates[len(s.signalingStates)-1].State == state {
		return
	}
	s.signalingStates = appendLimited(s.signalingStates, SignalingStateEvent{At: at, State: state}, s.cfg.EventHistory)
}

func (s *Session) recordFailure(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordFailureLocked(message)
}

func (s *Session) recordWarning(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordWarningLocked(message)
}

func (s *Session) recordSkip(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordSkipLocked(message)
}

func (s *Session) recordFailureLocked(message string) {
	s.failures = appendUniqueLimited(s.failures, message, s.cfg.EventHistory)
	s.signalLocked()
}

func (s *Session) recordWarningLocked(message string) {
	s.warnings = appendUniqueLimited(s.warnings, message, s.cfg.EventHistory)
	s.signalLocked()
}

func (s *Session) recordSkipLocked(message string) {
	s.skipped = appendUniqueLimited(s.skipped, message, s.cfg.EventHistory)
	s.signalLocked()
}

func (s *Session) signalLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

func (t *videoTrackState) observeFrameLocked(f *frame.VideoFrame, configuredThreshold, defaultThreshold time.Duration, history int) {
	t.observeFrameAtLocked(f, time.Now(), configuredThreshold, defaultThreshold, history)
}

func (t *videoTrackState) observeFrameAtLocked(f *frame.VideoFrame, now time.Time, configuredThreshold, defaultThreshold time.Duration, history int) {
	if f == nil {
		return
	}
	if t.startedAt.IsZero() {
		t.startedAt = now
	}
	if !t.lastFrameAt.IsZero() {
		gap := now.Sub(t.lastFrameAt)
		if gap > t.freezeThresholdLocked(configuredThreshold, defaultThreshold) {
			t.freezeCount++
			t.freezeEvents = appendLimited(t.freezeEvents, pionrecv.FreezeEvent{
				At:   now,
				Gap:  gap,
				Kind: "frame",
			}, history)
		} else if gap > 0 {
			t.estimatedStep = gap
		}
	}
	if t.hasLastFramePTS && f.PTS > t.lastFramePTS {
		step := time.Duration(f.PTS-t.lastFramePTS) * time.Second / 90000
		if step > 0 {
			t.estimatedStep = step
		}
	}
	t.lastFramePTS = f.PTS
	t.hasLastFramePTS = true
	t.lastFrameAt = now
	t.frameCount++
	if f.IsKeyframe {
		t.keyframeCount++
	}
	if t.currentWidth > 0 && t.currentHeight > 0 && (t.currentWidth != f.Width || t.currentHeight != f.Height) {
		t.resolutionChanges++
	}
	t.currentWidth = f.Width
	t.currentHeight = f.Height
}

func (t *videoTrackState) freezeThresholdLocked(configuredThreshold, defaultThreshold time.Duration) time.Duration {
	if configuredThreshold > 0 {
		return configuredThreshold
	}
	threshold := defaultThreshold
	if threshold <= 0 {
		threshold = sessionDefaultVideoFreezeCap
	}
	if t.estimatedStep > 0 {
		adaptive := 3 * t.estimatedStep
		if adaptive < sessionMinVideoFreeze {
			adaptive = sessionMinVideoFreeze
		}
		if adaptive < threshold {
			threshold = adaptive
		}
	}
	return threshold
}

func (t *videoTrackState) observeCodecChangeLocked(change pionrecv.CodecChange, history int) {
	t.currentCodec = change.CurrentType
	t.currentMime = change.CurrentCodec.MimeType
	t.currentPT = change.CurrentPayloadType
	t.codecSwitches = appendLimited(t.codecSwitches, pionrecv.CodecSwitchObservation{
		At:     time.Now(),
		Change: change,
	}, history)
}

func (t *videoTrackState) observeStatsCodecLocked(sample RTPStatsSample, history int) {
	observeStatsCodecChangeLocked("video", &t.currentCodec, &t.currentMime, &t.currentPT, &t.codecSwitches, sample, history)
}

func (t *videoTrackState) snapshotLocked() VideoTrackSnapshot {
	if t.monitor != nil {
		wire := t.monitor.Snapshot()
		out := VideoTrackSnapshot{
			TrackID:           t.id,
			StreamID:          t.streamID,
			RID:               t.rid,
			Source:            t.source,
			SSRC:              t.ssrc,
			CurrentCodec:      wire.CurrentCodec,
			CurrentMimeType:   wire.CurrentCodecParameters.MimeType,
			StartedAt:         wire.StartedAt,
			LastFrameAt:       wire.LastFrameAt,
			FrameCount:        wire.FrameCount,
			KeyframeCount:     wire.KeyframeCount,
			FreezeCount:       wire.FreezeCount,
			CurrentWidth:      wire.CurrentWidth,
			CurrentHeight:     wire.CurrentHeight,
			ResolutionChanges: wire.ResolutionChanges,
			CurrentMID:        wire.CurrentMID,
			CurrentRID:        wire.CurrentRID,
			HasCurrentLayer:   wire.HasCurrentLayer,
			CurrentLayer:      wire.CurrentLayer,
			HasMaxActiveLayer: wire.HasMaxActiveLayer,
			MaxActiveLayer:    wire.MaxActiveLayer,
			Continuous:        wire.Continuous(),
			CodecSwitches:     append([]pionrecv.CodecSwitchObservation(nil), wire.CodecSwitches...),
			FreezeEvents:      append([]pionrecv.FreezeEvent(nil), wire.FreezeEvents...),
			Stats:             append([]RTPStatsSample(nil), t.stats...),
		}
		wireCopy := wire
		out.Wire = &wireCopy
		return out
	}
	return VideoTrackSnapshot{
		TrackID:           t.id,
		StreamID:          t.streamID,
		RID:               t.rid,
		Source:            t.source,
		SSRC:              t.ssrc,
		CurrentCodec:      t.currentCodec,
		CurrentMimeType:   t.currentMime,
		StartedAt:         t.startedAt,
		LastFrameAt:       t.lastFrameAt,
		FrameCount:        t.frameCount,
		KeyframeCount:     t.keyframeCount,
		FreezeCount:       t.freezeCount,
		CurrentWidth:      t.currentWidth,
		CurrentHeight:     t.currentHeight,
		ResolutionChanges: t.resolutionChanges,
		CurrentMID:        t.currentMID,
		CurrentRID:        t.currentRID,
		HasCurrentLayer:   t.hasCurrentLayer,
		CurrentLayer:      t.currentLayer,
		HasMaxActiveLayer: t.hasMaxActiveLayer,
		MaxActiveLayer:    t.maxActiveLayer,
		Continuous:        t.frameCount > 0 && t.freezeCount == 0,
		CodecSwitches:     append([]pionrecv.CodecSwitchObservation(nil), t.codecSwitches...),
		FreezeEvents:      append([]pionrecv.FreezeEvent(nil), t.freezeEvents...),
		Stats:             append([]RTPStatsSample(nil), t.stats...),
	}
}

func (t *videoTrackState) resetLocked() {
	t.startedAt = time.Time{}
	t.lastFrameAt = time.Time{}
	t.frameCount = 0
	t.keyframeCount = 0
	t.freezeCount = 0
	t.lastFramePTS = 0
	t.hasLastFramePTS = false
	t.estimatedStep = 0
	t.currentWidth = 0
	t.currentHeight = 0
	t.resolutionChanges = 0
	t.currentMID = ""
	t.currentRID = ""
	t.hasCurrentLayer = false
	t.currentLayer = pionrecv.VideoLayer{}
	t.hasMaxActiveLayer = false
	t.maxActiveLayer = pionrecv.VideoLayer{}
	t.codecSwitches = nil
	t.freezeEvents = nil
	t.stats = nil
}

func (t *audioTrackState) observeFrameLocked(f *frame.AudioFrame, configuredThreshold, defaultThreshold time.Duration, history int) {
	t.observeFrameAtLocked(f, time.Now(), configuredThreshold, defaultThreshold, history)
}

func (t *audioTrackState) observeFrameAtLocked(f *frame.AudioFrame, now time.Time, configuredThreshold, defaultThreshold time.Duration, history int) {
	if f == nil {
		return
	}
	if t.startedAt.IsZero() {
		t.startedAt = now
	}
	ptime := f.Duration()
	if !t.lastFrameAt.IsZero() {
		gap := now.Sub(t.lastFrameAt)
		if gap > t.freezeThresholdLocked(configuredThreshold, defaultThreshold, ptime) {
			t.freezeCount++
			t.freezeEvents = appendLimited(t.freezeEvents, pionrecv.FreezeEvent{
				At:   now,
				Gap:  gap,
				Kind: "frame",
			}, history)
		} else if gap > 0 && ptime <= 0 {
			t.estimatedStep = gap
		}
	}
	t.lastFrameAt = now
	t.frameCount++

	if ptime > 0 {
		t.estimatedStep = ptime
	}
	peak, rms, clipped := audioLevels(f)
	silent := rms <= sessionAudioSilenceThreshold

	if clipped {
		t.clippedFrames++
	}
	if silent {
		t.silentFrames++
	} else {
		t.activeFrames++
	}

	if t.currentSampleRate != 0 &&
		(t.currentSampleRate != f.SampleRate ||
			t.currentChannels != f.Channels ||
			t.currentNumSamples != f.NumSamples ||
			t.currentPTime != ptime) {
		t.configSwitches = appendLimited(t.configSwitches, pionrecv.AudioConfigSwitch{
			At:                 now,
			PreviousSampleRate: t.currentSampleRate,
			PreviousChannels:   t.currentChannels,
			PreviousNumSamples: t.currentNumSamples,
			PreviousPTime:      t.currentPTime,
			CurrentSampleRate:  f.SampleRate,
			CurrentChannels:    f.Channels,
			CurrentNumSamples:  f.NumSamples,
			CurrentPTime:       ptime,
		}, history)
	}

	t.currentSampleRate = f.SampleRate
	t.currentChannels = f.Channels
	t.currentNumSamples = f.NumSamples
	t.currentPTime = ptime
	t.peakLevel = peak
	t.rmsLevel = rms
}

func (t *audioTrackState) freezeThresholdLocked(configuredThreshold, defaultThreshold, ptime time.Duration) time.Duration {
	if configuredThreshold > 0 {
		return configuredThreshold
	}
	threshold := defaultThreshold
	if threshold <= 0 {
		threshold = sessionDefaultAudioFreezeCap
	}
	step := t.estimatedStep
	if ptime > 0 {
		step = ptime
	}
	if step > 0 {
		adaptive := 3 * step
		if adaptive < sessionMinAudioFreeze {
			adaptive = sessionMinAudioFreeze
		}
		if adaptive < threshold {
			threshold = adaptive
		}
	}
	return threshold
}

func (t *audioTrackState) observeCodecChangeLocked(change pionrecv.CodecChange, history int) {
	t.currentCodec = change.CurrentType
	t.currentMime = change.CurrentCodec.MimeType
	t.currentPT = change.CurrentPayloadType
	t.codecSwitches = appendLimited(t.codecSwitches, pionrecv.CodecSwitchObservation{
		At:     time.Now(),
		Change: change,
	}, history)
}

func (t *audioTrackState) observeStatsCodecLocked(sample RTPStatsSample, history int) {
	observeStatsCodecChangeLocked("audio", &t.currentCodec, &t.currentMime, &t.currentPT, &t.codecSwitches, sample, history)
}

func (t *audioTrackState) snapshotLocked() AudioTrackSnapshot {
	if t.monitor != nil {
		wire := t.monitor.Snapshot()
		out := AudioTrackSnapshot{
			TrackID:           t.id,
			StreamID:          t.streamID,
			RID:               t.rid,
			Source:            t.source,
			SSRC:              t.ssrc,
			CurrentMID:        t.currentMID,
			CurrentCodec:      wire.CurrentCodec,
			CurrentMimeType:   wire.CurrentCodecParameters.MimeType,
			StartedAt:         wire.StartedAt,
			LastFrameAt:       wire.LastFrameAt,
			FrameCount:        wire.FrameCount,
			FreezeCount:       wire.FreezeCount,
			CurrentSampleRate: wire.CurrentSampleRate,
			CurrentChannels:   wire.CurrentChannels,
			CurrentNumSamples: wire.CurrentNumSamples,
			CurrentPTime:      wire.CurrentPTime,
			PeakLevel:         wire.PeakLevel,
			RMSLevel:          wire.RMSLevel,
			ActiveFrames:      wire.ActiveFrames,
			SilentFrames:      wire.SilentFrames,
			ClippedFrames:     wire.ClippedFrames,
			Continuous:        wire.Continuous(),
			CodecSwitches:     append([]pionrecv.CodecSwitchObservation(nil), wire.CodecSwitches...),
			ConfigSwitches:    append([]pionrecv.AudioConfigSwitch(nil), wire.ConfigSwitches...),
			FreezeEvents:      append([]pionrecv.FreezeEvent(nil), wire.FreezeEvents...),
			Stats:             append([]RTPStatsSample(nil), t.stats...),
		}
		wireCopy := wire
		out.Wire = &wireCopy
		return out
	}
	return AudioTrackSnapshot{
		TrackID:           t.id,
		StreamID:          t.streamID,
		RID:               t.rid,
		Source:            t.source,
		SSRC:              t.ssrc,
		CurrentMID:        t.currentMID,
		CurrentCodec:      t.currentCodec,
		CurrentMimeType:   t.currentMime,
		StartedAt:         t.startedAt,
		LastFrameAt:       t.lastFrameAt,
		FrameCount:        t.frameCount,
		FreezeCount:       t.freezeCount,
		CurrentSampleRate: t.currentSampleRate,
		CurrentChannels:   t.currentChannels,
		CurrentNumSamples: t.currentNumSamples,
		CurrentPTime:      t.currentPTime,
		PeakLevel:         t.peakLevel,
		RMSLevel:          t.rmsLevel,
		ActiveFrames:      t.activeFrames,
		SilentFrames:      t.silentFrames,
		ClippedFrames:     t.clippedFrames,
		Continuous:        t.frameCount > 0 && t.freezeCount == 0,
		CodecSwitches:     append([]pionrecv.CodecSwitchObservation(nil), t.codecSwitches...),
		ConfigSwitches:    append([]pionrecv.AudioConfigSwitch(nil), t.configSwitches...),
		FreezeEvents:      append([]pionrecv.FreezeEvent(nil), t.freezeEvents...),
		Stats:             append([]RTPStatsSample(nil), t.stats...),
	}
}

func (t *audioTrackState) resetLocked() {
	t.startedAt = time.Time{}
	t.lastFrameAt = time.Time{}
	t.frameCount = 0
	t.freezeCount = 0
	t.estimatedStep = 0
	t.currentMID = ""
	t.currentSampleRate = 0
	t.currentChannels = 0
	t.currentNumSamples = 0
	t.currentPTime = 0
	t.peakLevel = 0
	t.rmsLevel = 0
	t.activeFrames = 0
	t.silentFrames = 0
	t.clippedFrames = 0
	t.codecSwitches = nil
	t.configSwitches = nil
	t.freezeEvents = nil
	t.stats = nil
}

func observeStatsCodecChangeLocked(
	kind string,
	currentCodec *codec.Type,
	currentMime *string,
	currentPT *webrtc.PayloadType,
	codecSwitches *[]pionrecv.CodecSwitchObservation,
	sample RTPStatsSample,
	history int,
) {
	if strings.TrimSpace(sample.CodecMimeType) == "" {
		return
	}

	nextCodec, nextCodecOK := codec.ParseMimeType(sample.CodecMimeType)
	if *currentMime != "" && !strings.EqualFold(*currentMime, sample.CodecMimeType) {
		previousCodec := *currentCodec
		if parsedPrevious, ok := codec.ParseMimeType(*currentMime); ok {
			previousCodec = parsedPrevious
		}
		change := pionrecv.CodecChange{
			PreviousType:        previousCodec,
			CurrentType:         previousCodec,
			PreviousCodec:       codecParametersForStatsSample(kind, *currentMime, *currentPT),
			CurrentCodec:        codecParametersForStatsSample(kind, sample.CodecMimeType, sample.CodecPayloadType),
			PreviousPayloadType: *currentPT,
			CurrentPayloadType:  sample.CodecPayloadType,
		}
		if nextCodecOK {
			change.CurrentType = nextCodec
		}
		at := sample.At
		if at.IsZero() {
			at = time.Now()
		}
		*codecSwitches = appendLimited(*codecSwitches, pionrecv.CodecSwitchObservation{
			At:     at,
			Change: change,
		}, history)
	}

	if nextCodecOK {
		*currentCodec = nextCodec
	}
	*currentMime = sample.CodecMimeType
	*currentPT = sample.CodecPayloadType
}

func (d *dataChannelState) appendStateLocked(state string, history int) {
	d.stateHistory = appendLimited(d.stateHistory, DataChannelStateEvent{At: time.Now(), State: state}, history)
}

func (d *dataChannelState) snapshotLocked() DataChannelSnapshot {
	return DataChannelSnapshot{
		Label:                d.label,
		ID:                   d.id,
		State:                d.state,
		OpenedAt:             d.openedAt,
		ClosedAt:             d.closedAt,
		OpenTransitions:      d.openTransitions,
		CloseTransitions:     d.closeTransitions,
		UserMessagesSent:     d.userMessagesSent,
		UserMessagesReceived: d.userMessagesReceived,
		UserBytesSent:        d.userBytesSent,
		UserBytesReceived:    d.userBytesReceived,
		HeartbeatSent:        d.heartbeatSent,
		HeartbeatReceived:    d.heartbeatReceived,
		HeartbeatAcked:       d.heartbeatAcked,
		HeartbeatMissed:      d.heartbeatMissed,
		LastHeartbeatRTT:     d.lastHeartbeatRTT,
		LastHeartbeatAt:      d.lastHeartbeatAt,
		LastHeartbeatAckAt:   d.lastHeartbeatAckAt,
		LastError:            d.lastError,
		StateHistory:         append([]DataChannelStateEvent(nil), d.stateHistory...),
		Stats:                append([]DataChannelStatsSample(nil), d.stats...),
	}
}

func (d *dataChannelState) resetLocked(history int) {
	d.state = d.adapter.ReadyStateString()
	d.openedAt = time.Time{}
	d.closedAt = time.Time{}
	d.openTransitions = 0
	d.closeTransitions = 0
	d.userMessagesSent = 0
	d.userMessagesReceived = 0
	d.userBytesSent = 0
	d.userBytesReceived = 0
	d.heartbeatSent = 0
	d.heartbeatReceived = 0
	d.heartbeatAcked = 0
	d.heartbeatMissed = 0
	d.lastHeartbeatRTT = 0
	d.lastHeartbeatAt = time.Time{}
	d.lastHeartbeatAckAt = time.Time{}
	d.lastError = ""
	d.stats = nil
	if d.state == "closed" {
		d.heartbeatOn = false
	}
	clear(d.pendingAcks)
	d.stateHistory = nil
	d.appendStateLocked(d.state, history)
}

func audioLevels(f *frame.AudioFrame) (peakLevel, rmsLevel float64, clipped bool) {
	if f == nil {
		return 0, 0, false
	}
	samples := f.SamplesS16()
	if len(samples) == 0 {
		return 0, 0, false
	}

	var (
		peak float64
		sum  float64
	)
	for _, sample := range samples {
		value := math.Abs(float64(sample))
		if value > peak {
			peak = value
		}
		if value >= math.MaxInt16-7 {
			clipped = true
		}
		normalized := value / math.MaxInt16
		sum += normalized * normalized
	}

	return peak / math.MaxInt16, math.Sqrt(sum / float64(len(samples))), clipped
}

func collectDataChannelStatsLocked(states map[string]*dataChannelState) []DataChannelStatsSample {
	out := make([]DataChannelStatsSample, 0, len(states))
	for _, state := range states {
		if len(state.stats) == 0 {
			continue
		}
		out = append(out, state.stats[len(state.stats)-1])
	}
	return out
}

func appendUniqueLimited(slice []string, value string, limit int) []string {
	for _, existing := range slice {
		if existing == value {
			return slice
		}
	}
	return appendLimited(slice, value, limit)
}

func appendLimited[T any](slice []T, value T, limit int) []T {
	slice = append(slice, value)
	if limit > 0 && len(slice) > limit {
		slice = append([]T(nil), slice[len(slice)-limit:]...)
	}
	return slice
}

func dataChannelKey(label string, id int) string {
	return fmt.Sprintf("%s:%d", label, id)
}

func hasOpenDataChannel(snapshot SessionSnapshot, label string) bool {
	if label != "" {
		for _, dc := range snapshot.DataChannels {
			if dc.Label == label {
				return dc.State == "open"
			}
		}
		return false
	}

	for _, dc := range snapshot.DataChannels {
		if dc.State == "open" {
			return true
		}
	}
	return false
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
