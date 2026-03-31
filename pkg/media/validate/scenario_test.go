package validate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestScenarioLabRunDelaysAndCancellation(t *testing.T) {
	ctx := context.Background()
	if err := NewScenarioLab(nil, LabConfig{}).Run(ctx, ScenarioScript{}); err == nil || !strings.Contains(err.Error(), "scenario lab session is nil") {
		t.Fatalf("Run(nil session) error = %v, want nil-session error", err)
	}

	session := newSession(nil, nil, SessionConfig{})
	lab := NewScenarioLab(session, LabConfig{StepDelay: 5 * time.Millisecond})
	start := time.Now()
	var calls int
	if err := lab.Run(ctx, ScenarioScript{
		Steps: []ScenarioStep{
			{
				Name:   "after",
				Action: ScenarioActionExternal,
				After:  5 * time.Millisecond,
				Callback: func(context.Context, *Session) error {
					calls++
					return nil
				},
			},
			{
				Name:   "delay",
				Action: ScenarioActionExternal,
				Callback: func(context.Context, *Session) error {
					calls++
					return nil
				},
			},
		},
	}); err != nil {
		t.Fatalf("Run(delayed script): %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Fatalf("elapsed = %v, want at least 10ms", elapsed)
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := lab.Run(cancelCtx, ScenarioScript{
		Steps: []ScenarioStep{{Action: ScenarioActionExternal, After: 20 * time.Millisecond, Callback: func(context.Context, *Session) error { return nil }}},
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run(canceled) error = %v, want context canceled", err)
	}
}

func TestScenarioLabRunStepErrorsAndHelpers(t *testing.T) {
	session := newSession(nil, nil, SessionConfig{})
	lab := NewScenarioLab(session, LabConfig{})
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		step ScenarioStep
		want string
	}{
		{name: "missing video active", step: ScenarioStep{Action: ScenarioActionSetLayerActive}, want: "missing video publisher"},
		{name: "missing video bitrate", step: ScenarioStep{Action: ScenarioActionSetLayerBitrate}, want: "missing video publisher"},
		{name: "missing video keyframe", step: ScenarioStep{Action: ScenarioActionRequestKeyFrame}, want: "missing video publisher"},
		{name: "missing codec callback", step: ScenarioStep{Action: ScenarioActionCodecRenegotiation}, want: "missing external callback"},
		{name: "missing ice callback", step: ScenarioStep{Action: ScenarioActionICERestart}, want: "missing external callback"},
		{name: "missing track add callback", step: ScenarioStep{Action: ScenarioActionTrackAdd}, want: "missing external callback"},
		{name: "missing track remove callback", step: ScenarioStep{Action: ScenarioActionTrackRemove}, want: "missing external callback"},
		{name: "missing external callback", step: ScenarioStep{Action: ScenarioActionExternal}, want: "missing external callback"},
		{name: "missing media track mute", step: ScenarioStep{Action: ScenarioActionMute}, want: "missing media track"},
		{name: "missing media track unmute", step: ScenarioStep{Action: ScenarioActionUnmute}, want: "missing media track"},
		{name: "missing relay apply", step: ScenarioStep{Action: ScenarioActionApplyImpairment}, want: "missing relay"},
		{name: "missing relay clear", step: ScenarioStep{Action: ScenarioActionClearImpairment}, want: "missing relay"},
		{name: "unsupported", step: ScenarioStep{Action: ScenarioAction("nope")}, want: `unsupported scenario action "nope"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := lab.runStep(ctx, tc.step); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("runStep(%s) error = %v, want substring %q", tc.name, err, tc.want)
			}
		})
	}

	missingDCStep := ScenarioStep{Action: ScenarioActionDataChannelBurst, DataChannelLabel: "missing", TextMessages: []string{"hello"}}
	if err := lab.runStep(ctx, missingDCStep); err == nil || !strings.Contains(err.Error(), `data channel "missing" not found`) {
		t.Fatalf("runStep(datachannel missing) error = %v", err)
	}
	if err := lab.runStep(ctx, ScenarioStep{Action: ScenarioActionPauseHeartbeats, DataChannelLabel: "missing"}); err == nil || !strings.Contains(err.Error(), `data channel "missing" not found`) {
		t.Fatalf("runStep(pause missing) error = %v", err)
	}
	if err := lab.runStep(ctx, ScenarioStep{Action: ScenarioActionResumeHeartbeats, DataChannelLabel: "missing"}); err == nil || !strings.Contains(err.Error(), `data channel "missing" not found`) {
		t.Fatalf("runStep(resume missing) error = %v", err)
	}

	dc := &fakeDataChannel{label: "control", id: 7, state: "open"}
	session.observeDataChannel(dc)
	if err := lab.runStep(ctx, ScenarioStep{
		Action:           ScenarioActionDataChannelBurst,
		DataChannelLabel: "control",
		TextMessages:     []string{"one", "two"},
	}); err != nil {
		t.Fatalf("runStep(data burst): %v", err)
	}
	if err := lab.runStep(ctx, ScenarioStep{Action: ScenarioActionPauseHeartbeats, DataChannelLabel: "control"}); err != nil {
		t.Fatalf("runStep(pause): %v", err)
	}
	session.mu.Lock()
	pausedState := session.findDataChannelByLabelLocked("control")
	if pausedState == nil || !pausedState.paused {
		session.mu.Unlock()
		t.Fatalf("paused state = %+v, want paused", pausedState)
	}
	if got := session.findDataChannelByLabelLocked(""); got == nil {
		session.mu.Unlock()
		t.Fatal(`findDataChannelByLabelLocked("") = nil, want first channel`)
	}
	if got := session.findDataChannelByLabelLocked("missing"); got != nil {
		session.mu.Unlock()
		t.Fatalf("findDataChannelByLabelLocked(missing) = %+v, want nil", got)
	}
	session.mu.Unlock()
	if err := lab.runStep(ctx, ScenarioStep{Action: ScenarioActionResumeHeartbeats, DataChannelLabel: "control"}); err != nil {
		t.Fatalf("runStep(resume): %v", err)
	}

	dc.setSendErr(errors.New("boom"))
	if err := lab.runStep(ctx, ScenarioStep{
		Action:           ScenarioActionDataChannelBurst,
		DataChannelLabel: "control",
		TextMessages:     []string{"three"},
	}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("runStep(data burst error) = %v, want send error", err)
	}
	dc.setSendErr(nil)

	relay := &ICEEdgeRelay{}
	profile := ImpairmentProfile{Rules: []NetworkImpairment{{Direction: DirectionBoth, Delay: 10 * time.Millisecond}}}
	if err := lab.runStep(ctx, ScenarioStep{
		Action:     ScenarioActionApplyImpairment,
		Relay:      relay,
		Impairment: profile,
	}); err != nil {
		t.Fatalf("runStep(apply impairment): %v", err)
	}
	if got := relay.impairmentForDirection(DirectionUpstream); got.Delay != 10*time.Millisecond {
		t.Fatalf("relay impairment after apply = %+v", got)
	}
	if err := lab.runStep(ctx, ScenarioStep{
		Action: ScenarioActionClearImpairment,
		Relay:  relay,
	}); err != nil {
		t.Fatalf("runStep(clear impairment): %v", err)
	}
	if got := relay.impairmentForDirection(DirectionUpstream); got.Delay != 0 {
		t.Fatalf("relay impairment after clear = %+v, want zero delay", got)
	}
}
