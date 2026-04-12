package validate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/media"
	"github.com/thesyncim/libgowebrtc/pkg/pionrecv"
	"github.com/thesyncim/libgowebrtc/pkg/pionsend"
)

// ScenarioAction identifies a built-in scenario step kind.
type ScenarioAction string

// Built-in scenario actions.
const (
	ScenarioActionSetLayerActive     ScenarioAction = "set_layer_active"
	ScenarioActionSetLayerBitrate    ScenarioAction = "set_layer_bitrate"
	ScenarioActionRequestKeyFrame    ScenarioAction = "request_keyframe"
	ScenarioActionCodecRenegotiation ScenarioAction = "codec_renegotiation"
	ScenarioActionICERestart         ScenarioAction = "ice_restart"
	ScenarioActionTrackAdd           ScenarioAction = "track_add"
	ScenarioActionTrackRemove        ScenarioAction = "track_remove"
	ScenarioActionMute               ScenarioAction = "mute"
	ScenarioActionUnmute             ScenarioAction = "unmute"
	ScenarioActionDataChannelBurst   ScenarioAction = "datachannel_burst"
	ScenarioActionPauseHeartbeats    ScenarioAction = "pause_heartbeats"
	ScenarioActionResumeHeartbeats   ScenarioAction = "resume_heartbeats"
	ScenarioActionApplyImpairment    ScenarioAction = "apply_impairment"
	ScenarioActionClearImpairment    ScenarioAction = "clear_impairment"
	ScenarioActionExternal           ScenarioAction = "external"
)

// ScenarioScript is a sequence of validation-driving steps.
type ScenarioScript struct {
	Steps []ScenarioStep
}

// ScenarioStep is a single scenario action.
type ScenarioStep struct {
	Name       string
	Action     ScenarioAction
	After      time.Duration
	Video      pionsend.PublishedVideo
	Audio      pionsend.PublishedAudio
	MediaTrack media.MediaStreamTrack

	LayerIndex int
	Active     bool
	Bitrate    uint32

	DataChannelLabel string
	TextMessages     []string

	Relay      *ICEEdgeRelay
	Impairment ImpairmentProfile

	Callback func(context.Context, *Session) error
	Expect   ScenarioExpectation
}

// ScenarioExpectation describes the subscriber-visible outcome that should be
// observed after a scenario step completes.
type ScenarioExpectation struct {
	Within  time.Duration
	HoldFor time.Duration

	Connected   bool
	Stable      bool
	Reconnected bool

	TrackID   string
	CodecMime string

	VideoTrackID      string
	VideoContinuous   bool
	NoNewVideoFreezes bool
	MinNewVideoFrames uint64
	VideoRID          string
	VideoWidth        int
	VideoHeight       int
	HasVideoLayer     bool
	VideoLayer        pionrecv.VideoLayer

	AudioTrackID      string
	AudioContinuous   bool
	NoNewAudioFreezes bool
	MinNewAudioFrames uint64

	DataChannelLabel         string
	DataChannelOpen          bool
	NoNewHeartbeatMisses     bool
	NoNewDataChannelClosures bool
}

// LabConfig configures scenario execution behavior.
type LabConfig struct {
	StepDelay time.Duration
}

// ScenarioLab executes validation scenario scripts against a Session.
type ScenarioLab struct {
	session *Session
	cfg     LabConfig
}

// NewScenarioLab constructs a scenario runner.
func NewScenarioLab(session *Session, cfg LabConfig) *ScenarioLab {
	return &ScenarioLab{
		session: session,
		cfg:     cfg,
	}
}

// Run executes the scenario script serially.
func (l *ScenarioLab) Run(ctx context.Context, script ScenarioScript) error {
	if l == nil || l.session == nil {
		return errors.New("validate: scenario lab session is nil")
	}

	for i, step := range script.Steps {
		if step.After > 0 {
			timer := time.NewTimer(step.After)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		} else if l.cfg.StepDelay > 0 {
			timer := time.NewTimer(l.cfg.StepDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}

		if err := l.runStep(ctx, step); err != nil {
			name := step.Name
			if name == "" {
				name = fmt.Sprintf("step-%d", i)
			}
			return fmt.Errorf("validate: scenario %s failed: %w", name, err)
		}
	}

	return nil
}

func (l *ScenarioLab) runStep(ctx context.Context, step ScenarioStep) error {
	baseline := SessionSnapshot{}
	if step.Expect.enabled() {
		baseline = l.session.Snapshot()
	}

	var actionErr error
	switch step.Action {
	case ScenarioActionSetLayerActive:
		if !l.session.policy.SupportsSimulcast && !l.session.policy.SupportsLayeredVP9 && !l.session.policy.SupportsLayeredAV1 {
			l.session.recordSkip(fmt.Sprintf("layer activation scenarios are not guaranteed for validation profile %q", l.session.policy.Profile))
			return nil
		}
		if step.Video == nil {
			return errors.New("missing video publisher")
		}
		actionErr = step.Video.SetLayerActive(step.LayerIndex, step.Active)

	case ScenarioActionSetLayerBitrate:
		if !l.session.policy.SupportsSimulcast && !l.session.policy.SupportsLayeredVP9 && !l.session.policy.SupportsLayeredAV1 {
			l.session.recordSkip(fmt.Sprintf("layer bitrate scenarios are not guaranteed for validation profile %q", l.session.policy.Profile))
			return nil
		}
		if step.Video == nil {
			return errors.New("missing video publisher")
		}
		actionErr = step.Video.SetLayerBitrate(step.LayerIndex, step.Bitrate)

	case ScenarioActionRequestKeyFrame:
		if step.Video == nil {
			return errors.New("missing video publisher")
		}
		step.Video.RequestKeyFrame()

	case ScenarioActionCodecRenegotiation, ScenarioActionICERestart, ScenarioActionTrackAdd, ScenarioActionTrackRemove, ScenarioActionExternal:
		if step.Action == ScenarioActionCodecRenegotiation && !l.session.policy.SupportsCodecSwitchAssertions {
			l.session.recordSkip(fmt.Sprintf("codec renegotiation assertions are not guaranteed for validation profile %q", l.session.policy.Profile))
			return nil
		}
		if step.Callback == nil {
			return errors.New("missing external callback")
		}
		actionErr = step.Callback(ctx, l.session)

	case ScenarioActionMute:
		if step.MediaTrack == nil {
			return errors.New("missing media track")
		}
		step.MediaTrack.SetEnabled(false)

	case ScenarioActionUnmute:
		if step.MediaTrack == nil {
			return errors.New("missing media track")
		}
		step.MediaTrack.SetEnabled(true)

	case ScenarioActionDataChannelBurst:
		if len(step.TextMessages) == 0 {
			break
		}
		for _, msg := range step.TextMessages {
			if err := l.session.sendDataChannelText(step.DataChannelLabel, msg); err != nil {
				return err
			}
		}

	case ScenarioActionPauseHeartbeats:
		actionErr = l.session.setHeartbeatPaused(step.DataChannelLabel, true)

	case ScenarioActionResumeHeartbeats:
		actionErr = l.session.setHeartbeatPaused(step.DataChannelLabel, false)

	case ScenarioActionApplyImpairment:
		if step.Relay == nil {
			return errors.New("missing relay")
		}
		step.Relay.SetImpairment(step.Impairment)

	case ScenarioActionClearImpairment:
		if step.Relay == nil {
			return errors.New("missing relay")
		}
		step.Relay.ClearImpairment()

	default:
		return fmt.Errorf("unsupported scenario action %q", step.Action)
	}

	if actionErr != nil {
		return actionErr
	}

	return l.awaitExpectation(ctx, step.Expect, baseline)
}

func (l *ScenarioLab) awaitExpectation(ctx context.Context, expect ScenarioExpectation, baseline SessionSnapshot) error {
	expect = l.normalizeExpectation(expect)
	if err := expect.validate(); err != nil {
		return err
	}
	if !expect.enabled() {
		return nil
	}

	timeout := expect.timeout(l.session.cfg.SwitchRecoveryThreshold)
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var (
		satisfiedAt time.Time
		lastUnmet   []string
	)

	for {
		snapshot := l.session.Snapshot()
		unmet := expect.unmet(baseline, snapshot)
		if len(unmet) == 0 {
			if expect.HoldFor <= 0 {
				return nil
			}
			if satisfiedAt.IsZero() {
				satisfiedAt = time.Now()
			}
			if time.Since(satisfiedAt) >= expect.HoldFor {
				return nil
			}
		} else {
			satisfiedAt = time.Time{}
			lastUnmet = unmet
		}

		waitInterval, changed := l.session.nextWaitSignal(expect, satisfiedAt)
		timer := time.NewTimer(waitInterval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			current := l.session.Snapshot()
			if len(lastUnmet) == 0 {
				lastUnmet = expect.unmet(baseline, current)
			}
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				message := fmt.Sprintf("scenario expectation not met within %s: %s", timeout, strings.Join(lastUnmet, ", "))
				l.session.recordFailure(message)
				return errors.New(message)
			}
			return waitCtx.Err()
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (l *ScenarioLab) normalizeExpectation(expect ScenarioExpectation) ScenarioExpectation {
	if expect.CodecMime != "" && !l.session.policy.SupportsCodecSwitchAssertions {
		l.session.recordSkip(fmt.Sprintf("codec switch assertions are not guaranteed for validation profile %q", l.session.policy.Profile))
		expect.CodecMime = ""
	}
	if expect.VideoRID != "" && !l.session.policy.SupportsRID {
		l.session.recordSkip(fmt.Sprintf("RID assertions are not guaranteed for validation profile %q", l.session.policy.Profile))
		expect.VideoRID = ""
	}
	if expect.HasVideoLayer && !l.session.policy.SupportsDependencyDescriptor {
		l.session.recordSkip(fmt.Sprintf("dependency-descriptor layer assertions are not guaranteed for validation profile %q", l.session.policy.Profile))
		expect.HasVideoLayer = false
		expect.VideoLayer = pionrecv.VideoLayer{}
	}
	return expect
}

func (e ScenarioExpectation) enabled() bool {
	return e.Connected ||
		e.Stable ||
		e.Reconnected ||
		e.CodecMime != "" ||
		e.VideoContinuous ||
		e.NoNewVideoFreezes ||
		e.MinNewVideoFrames > 0 ||
		e.VideoRID != "" ||
		e.VideoWidth > 0 ||
		e.VideoHeight > 0 ||
		e.HasVideoLayer ||
		e.AudioContinuous ||
		e.NoNewAudioFreezes ||
		e.MinNewAudioFrames > 0 ||
		e.DataChannelOpen ||
		e.NoNewHeartbeatMisses ||
		e.NoNewDataChannelClosures
}

func (e ScenarioExpectation) validate() error {
	if !e.enabled() {
		return nil
	}
	if e.CodecMime != "" && e.codecTrackID() == "" {
		return errors.New("scenario expectation missing track id for codec check")
	}
	if (e.VideoContinuous || e.NoNewVideoFreezes || e.VideoRID != "" || e.VideoWidth > 0 || e.VideoHeight > 0 || e.HasVideoLayer) && e.VideoTrackID == "" {
		return errors.New("scenario expectation missing video track id")
	}
	if (e.AudioContinuous || e.NoNewAudioFreezes) && e.AudioTrackID == "" {
		return errors.New("scenario expectation missing audio track id")
	}
	if e.MinNewVideoFrames > 0 && e.VideoTrackID == "" {
		return errors.New("scenario expectation missing video track id for frame delta check")
	}
	if e.MinNewAudioFrames > 0 && e.AudioTrackID == "" {
		return errors.New("scenario expectation missing audio track id for frame delta check")
	}
	if (e.NoNewHeartbeatMisses || e.NoNewDataChannelClosures) && e.DataChannelLabel == "" {
		return errors.New("scenario expectation missing data channel label for continuity delta check")
	}
	return nil
}

func (e ScenarioExpectation) timeout(defaultTimeout time.Duration) time.Duration {
	if e.Within > 0 {
		return e.Within
	}
	if defaultTimeout > 0 {
		return defaultTimeout
	}
	return time.Second
}

func (e ScenarioExpectation) codecTrackID() string {
	if e.TrackID != "" {
		return e.TrackID
	}
	if e.VideoTrackID != "" {
		return e.VideoTrackID
	}
	return e.AudioTrackID
}

func (e ScenarioExpectation) unmet(baseline, current SessionSnapshot) []string {
	failures := make([]string, 0, 8)

	if e.Connected {
		if len(current.ConnectionStates) == 0 || current.ConnectionStates[len(current.ConnectionStates)-1].State != webrtc.PeerConnectionStateConnected {
			failures = append(failures, "peer connection is not connected")
		}
	}
	if e.Stable {
		if len(current.SignalingStates) == 0 || current.SignalingStates[len(current.SignalingStates)-1].State != webrtc.SignalingStateStable {
			failures = append(failures, "signaling state is not stable")
		}
	}
	if e.Reconnected && !snapshotSawReconnected(current) {
		failures = append(failures, "peer connection has not reconnected")
	}

	if e.CodecMime != "" {
		trackID := e.codecTrackID()
		if track, ok := current.VideoTracks[trackID]; ok {
			if !strings.EqualFold(track.CurrentMimeType, e.CodecMime) {
				failures = append(failures, fmt.Sprintf("track %q codec is %q", trackID, track.CurrentMimeType))
			}
		} else if track, ok := current.AudioTracks[trackID]; ok {
			if !strings.EqualFold(track.CurrentMimeType, e.CodecMime) {
				failures = append(failures, fmt.Sprintf("track %q codec is %q", trackID, track.CurrentMimeType))
			}
		} else {
			failures = append(failures, fmt.Sprintf("track %q not found for codec expectation", trackID))
		}
	}

	if e.VideoTrackID != "" {
		track, ok := current.VideoTracks[e.VideoTrackID]
		if !ok {
			failures = append(failures, fmt.Sprintf("video track %q not found", e.VideoTrackID))
		} else {
			if e.VideoContinuous && !track.Continuous {
				failures = append(failures, fmt.Sprintf("video track %q is not continuous", e.VideoTrackID))
			}
			if e.NoNewVideoFreezes {
				base := baselineFreezeCountVideo(baseline, e.VideoTrackID)
				if track.FreezeCount > base {
					failures = append(failures, fmt.Sprintf("video track %q freeze count increased from %d to %d", e.VideoTrackID, base, track.FreezeCount))
				}
			}
			if e.MinNewVideoFrames > 0 {
				base := baselineVideoFrameCount(baseline, e.VideoTrackID)
				if track.FrameCount < base+e.MinNewVideoFrames {
					failures = append(failures, fmt.Sprintf("video track %q frame count is %d, need at least %d", e.VideoTrackID, track.FrameCount, base+e.MinNewVideoFrames))
				}
			}
			if e.VideoRID != "" && track.CurrentRID != e.VideoRID {
				failures = append(failures, fmt.Sprintf("video track %q RID is %q", e.VideoTrackID, track.CurrentRID))
			}
			if e.VideoWidth > 0 && track.CurrentWidth != e.VideoWidth {
				failures = append(failures, fmt.Sprintf("video track %q width is %d", e.VideoTrackID, track.CurrentWidth))
			}
			if e.VideoHeight > 0 && track.CurrentHeight != e.VideoHeight {
				failures = append(failures, fmt.Sprintf("video track %q height is %d", e.VideoTrackID, track.CurrentHeight))
			}
			if e.HasVideoLayer {
				if !track.HasCurrentLayer {
					failures = append(failures, fmt.Sprintf("video track %q has no current layer", e.VideoTrackID))
				} else if track.CurrentLayer != e.VideoLayer {
					failures = append(failures, fmt.Sprintf("video track %q layer is S%dT%d", e.VideoTrackID, track.CurrentLayer.Spatial, track.CurrentLayer.Temporal))
				}
			}
		}
	}

	if e.AudioTrackID != "" {
		track, ok := current.AudioTracks[e.AudioTrackID]
		if !ok {
			failures = append(failures, fmt.Sprintf("audio track %q not found", e.AudioTrackID))
		} else {
			if e.AudioContinuous && !track.Continuous {
				failures = append(failures, fmt.Sprintf("audio track %q is not continuous", e.AudioTrackID))
			}
			if e.NoNewAudioFreezes {
				base := baselineFreezeCountAudio(baseline, e.AudioTrackID)
				if track.FreezeCount > base {
					failures = append(failures, fmt.Sprintf("audio track %q freeze count increased from %d to %d", e.AudioTrackID, base, track.FreezeCount))
				}
			}
			if e.MinNewAudioFrames > 0 {
				base := baselineAudioFrameCount(baseline, e.AudioTrackID)
				if track.FrameCount < base+e.MinNewAudioFrames {
					failures = append(failures, fmt.Sprintf("audio track %q frame count is %d, need at least %d", e.AudioTrackID, track.FrameCount, base+e.MinNewAudioFrames))
				}
			}
		}
	}

	if e.DataChannelOpen && !hasOpenDataChannel(current, e.DataChannelLabel) {
		if e.DataChannelLabel == "" {
			failures = append(failures, "no open data channel observed")
		} else {
			failures = append(failures, fmt.Sprintf("data channel %q is not open", e.DataChannelLabel))
		}
	}
	if e.NoNewHeartbeatMisses {
		base := baselineHeartbeatMisses(baseline, e.DataChannelLabel)
		if currentMisses := currentHeartbeatMisses(current, e.DataChannelLabel); currentMisses > base {
			failures = append(failures, fmt.Sprintf("data channel %q heartbeat misses increased from %d to %d", e.DataChannelLabel, base, currentMisses))
		}
	}
	if e.NoNewDataChannelClosures {
		base := baselineCloseTransitions(baseline, e.DataChannelLabel)
		if currentCloses := currentCloseTransitions(current, e.DataChannelLabel); currentCloses > base {
			failures = append(failures, fmt.Sprintf("data channel %q close transitions increased from %d to %d", e.DataChannelLabel, base, currentCloses))
		}
	}

	return failures
}

func baselineVideoFrameCount(snapshot SessionSnapshot, trackID string) uint64 {
	if track, ok := snapshot.VideoTracks[trackID]; ok {
		return track.FrameCount
	}
	return 0
}

func baselineFreezeCountVideo(snapshot SessionSnapshot, trackID string) uint64 {
	if track, ok := snapshot.VideoTracks[trackID]; ok {
		return track.FreezeCount
	}
	return 0
}

func baselineAudioFrameCount(snapshot SessionSnapshot, trackID string) uint64 {
	if track, ok := snapshot.AudioTracks[trackID]; ok {
		return track.FrameCount
	}
	return 0
}

func baselineFreezeCountAudio(snapshot SessionSnapshot, trackID string) uint64 {
	if track, ok := snapshot.AudioTracks[trackID]; ok {
		return track.FreezeCount
	}
	return 0
}

func baselineHeartbeatMisses(snapshot SessionSnapshot, label string) uint64 {
	if dc, ok := findDataChannelSnapshot(snapshot, label); ok {
		return dc.HeartbeatMissed
	}
	return 0
}

func currentHeartbeatMisses(snapshot SessionSnapshot, label string) uint64 {
	if dc, ok := findDataChannelSnapshot(snapshot, label); ok {
		return dc.HeartbeatMissed
	}
	return 0
}

func baselineCloseTransitions(snapshot SessionSnapshot, label string) uint64 {
	if dc, ok := findDataChannelSnapshot(snapshot, label); ok {
		return dc.CloseTransitions
	}
	return 0
}

func currentCloseTransitions(snapshot SessionSnapshot, label string) uint64 {
	if dc, ok := findDataChannelSnapshot(snapshot, label); ok {
		return dc.CloseTransitions
	}
	return 0
}

func findDataChannelSnapshot(snapshot SessionSnapshot, label string) (DataChannelSnapshot, bool) {
	for _, dc := range snapshot.DataChannels {
		if dc.Label == label {
			return dc, true
		}
	}
	return DataChannelSnapshot{}, false
}

func snapshotSawReconnected(snapshot SessionSnapshot) bool {
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
}

func (s *Session) setHeartbeatPaused(label string, paused bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.resolveDataChannelForActionLocked(label)
	if err != nil {
		return err
	}
	state.paused = paused
	s.signalLocked()
	return nil
}

func (s *Session) sendDataChannelText(label, text string) error {
	s.mu.Lock()
	state, err := s.resolveDataChannelForActionLocked(label)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	if err := state.adapter.SendText(text); err != nil {
		s.mu.Lock()
		state.lastError = errString(err)
		s.signalLocked()
		s.mu.Unlock()
		return err
	}

	s.mu.Lock()
	state.userMessagesSent++
	state.userBytesSent += uint64(len(text))
	s.signalLocked()
	s.mu.Unlock()
	return nil
}

func (s *Session) resolveDataChannelForActionLocked(label string) (*dataChannelState, error) {
	if label != "" {
		state := s.findDataChannelByLabelLocked(label)
		if state == nil {
			return nil, fmt.Errorf("validate: data channel %q not found", label)
		}
		return state, nil
	}

	var only *dataChannelState
	for _, state := range s.dataChannels {
		if only != nil {
			return nil, errors.New("validate: multiple data channels present; label is required")
		}
		only = state
	}
	if only == nil {
		return nil, fmt.Errorf("validate: data channel %q not found", label)
	}
	return only, nil
}

func (s *Session) findDataChannelByLabelLocked(label string) *dataChannelState {
	if label == "" {
		for _, state := range s.dataChannels {
			return state
		}
		return nil
	}
	for _, state := range s.dataChannels {
		if state.label == label {
			return state
		}
	}
	return nil
}

func (s *Session) nextWaitSignal(expect ScenarioExpectation, satisfiedAt time.Time) (waitInterval time.Duration, changed <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	waitInterval = sessionChangePollInterval
	if s.cfg.StatsPollInterval > 0 && s.cfg.StatsPollInterval < waitInterval {
		waitInterval = s.cfg.StatsPollInterval
	}
	if !satisfiedAt.IsZero() && expect.HoldFor > 0 {
		remaining := expect.HoldFor - time.Since(satisfiedAt)
		if remaining < waitInterval {
			waitInterval = remaining
		}
	}
	if waitInterval <= 0 {
		waitInterval = time.Millisecond
	}
	changed = s.changed
	return waitInterval, changed
}
