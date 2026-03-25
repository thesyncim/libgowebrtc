package validate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/thesyncim/libgowebrtc/pkg/media"
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
}

// LabConfig configures scenario execution behavior.
type LabConfig struct {
	StepDelay time.Duration
}

// ScenarioLab executes browser-style scenario scripts against a Session.
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
	switch step.Action {
	case ScenarioActionSetLayerActive:
		if step.Video == nil {
			return errors.New("missing video publisher")
		}
		return step.Video.SetLayerActive(step.LayerIndex, step.Active)

	case ScenarioActionSetLayerBitrate:
		if step.Video == nil {
			return errors.New("missing video publisher")
		}
		return step.Video.SetLayerBitrate(step.LayerIndex, step.Bitrate)

	case ScenarioActionRequestKeyFrame:
		if step.Video == nil {
			return errors.New("missing video publisher")
		}
		step.Video.RequestKeyFrame()
		return nil

	case ScenarioActionCodecRenegotiation, ScenarioActionICERestart, ScenarioActionTrackAdd, ScenarioActionTrackRemove, ScenarioActionExternal:
		if step.Callback == nil {
			return errors.New("missing external callback")
		}
		return step.Callback(ctx, l.session)

	case ScenarioActionMute:
		if step.MediaTrack == nil {
			return errors.New("missing media track")
		}
		step.MediaTrack.SetEnabled(false)
		return nil

	case ScenarioActionUnmute:
		if step.MediaTrack == nil {
			return errors.New("missing media track")
		}
		step.MediaTrack.SetEnabled(true)
		return nil

	case ScenarioActionDataChannelBurst:
		if len(step.TextMessages) == 0 {
			return nil
		}
		for _, msg := range step.TextMessages {
			if err := l.session.sendDataChannelText(step.DataChannelLabel, msg); err != nil {
				return err
			}
		}
		return nil

	case ScenarioActionPauseHeartbeats:
		return l.session.setHeartbeatPaused(step.DataChannelLabel, true)

	case ScenarioActionResumeHeartbeats:
		return l.session.setHeartbeatPaused(step.DataChannelLabel, false)

	case ScenarioActionApplyImpairment:
		if step.Relay == nil {
			return errors.New("missing relay")
		}
		step.Relay.SetImpairment(step.Impairment)
		return nil

	case ScenarioActionClearImpairment:
		if step.Relay == nil {
			return errors.New("missing relay")
		}
		step.Relay.ClearImpairment()
		return nil

	default:
		return fmt.Errorf("unsupported scenario action %q", step.Action)
	}
}

func (s *Session) setHeartbeatPaused(label string, paused bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.findDataChannelByLabelLocked(label)
	if state == nil {
		return fmt.Errorf("validate: data channel %q not found", label)
	}
	state.paused = paused
	s.signalLocked()
	return nil
}

func (s *Session) sendDataChannelText(label, text string) error {
	s.mu.Lock()
	state := s.findDataChannelByLabelLocked(label)
	if state == nil {
		s.mu.Unlock()
		return fmt.Errorf("validate: data channel %q not found", label)
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
