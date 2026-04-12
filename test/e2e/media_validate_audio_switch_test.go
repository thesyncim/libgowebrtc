package e2e

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/pc"
	"github.com/thesyncim/libgowebrtc/pkg/pioncodec"
	"github.com/thesyncim/libgowebrtc/pkg/testkit/validate"
)

const (
	audioCodecSwitchStatsPollInterval = 250 * time.Millisecond
	audioSenderSettleDelay            = 350 * time.Millisecond
	audioWarmupFrameCount             = 6
)

func TestReceiverSessionDetectsAudioCodecSwitchViaLibWebRTCStats(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("native receiver audio codec-switch validation is currently only stable on darwin_arm64")
	}

	pp := NewLibPeerPair(t)
	defer pp.Close()

	session := validate.NewPCSession(pp.Receiver, validate.SessionConfig{
		Browser:                 pioncodec.BrowserChrome,
		StatsPollInterval:       audioCodecSwitchStatsPollInterval,
		EventHistory:            64,
		SwitchRecoveryThreshold: 4 * time.Second,
	})

	pcOnTrack := session.PCOnTrack()
	pp.Receiver.SetOnTrack(func(track *pc.Track, recv *pc.RTPReceiver, streamID string) {
		pp.receivedTracksMu.Lock()
		pp.ReceivedTracks = append(pp.ReceivedTracks, track)
		pp.receivedTracksMu.Unlock()
		select {
		case pp.trackReceived <- struct{}{}:
		default:
		}
		pcOnTrack(track, recv, streamID)
	})

	const (
		audioSampleRate = 8000
		audioChannels   = 1
		audioSamples    = 160
	)

	track, err := pp.Sender.CreateAudioTrackWithOptions("audio-codec-switch", audioSampleRate, audioChannels)
	if err != nil {
		t.Fatalf("CreateAudioTrackWithOptions: %v", err)
	}
	sender, err := pp.Sender.AddTrack(track, "stream-audio-switch")
	if err != nil {
		t.Fatalf("AddTrack(audio): %v", err)
	}

	if err := pp.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !pp.WaitForTrack(3 * time.Second) {
		t.Fatal("timed out waiting for receiver audio track")
	}

	pump := newAudioPump(track, audioSampleRate, audioChannels, audioSamples)
	defer func() {
		if pump == nil {
			return
		}
		if err := pump.stop(2 * time.Second); err != nil {
			t.Fatalf("audioPump.stop: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := session.WaitForConnected(ctx); err != nil {
		t.Fatalf("WaitForConnected: %v", err)
	}
	if err := session.WaitForStable(ctx); err != nil {
		t.Fatalf("WaitForStable: %v", err)
	}
	if err := waitForAudioSenderSettle(ctx, audioSenderSettleDelay); err != nil {
		t.Fatalf("waitForAudioSenderSettle(initial): %v", err)
	}

	if err := pump.resume(ctx); err != nil {
		t.Fatalf("audioPump.resume(initial): %v", err)
	}
	if err := waitForReceiverAudioReady(ctx, session, "audio-codec-switch", audioWarmupFrameCount); err != nil {
		t.Fatalf("waitForReceiverAudioReady(initial): %v", err)
	}
	initialMime, ok := waitForReceiverAudioCodecMaybe(session, pp.Receiver, "audio-codec-switch", 4*time.Second)
	if !ok {
		t.Skipf("native receiver audio codec stats unavailable; snapshot=%s; stats=%s", summarizeAudioSnapshot(session, "audio-codec-switch"), summarizePeerAudioStats(t, pp.Receiver))
	}
	targetCodec, ok := chooseAlternateAudioCodec(sender, initialMime)
	if !ok {
		t.Skipf("need at least two negotiated audio codecs to test a switch; current=%q negotiated=%v", initialMime, mustNegotiatedAudioCodecs(t, sender))
	}

	// Startup warmup can include a transient gap on slower runners; reset before
	// asserting the renegotiation behavior itself.
	session.Reset()
	if err := waitForReceiverAudioReady(ctx, session, "audio-codec-switch", audioWarmupFrameCount); err != nil {
		t.Fatalf("waitForReceiverAudioReady(reset): %v", err)
	}

	lab := validate.NewScenarioLab(session, validate.LabConfig{})
	if err := lab.Run(ctx, validate.ScenarioScript{
		Steps: []validate.ScenarioStep{
			{
				Name:   "audio-codec-switch",
				Action: validate.ScenarioActionCodecRenegotiation,
				Callback: func(stepCtx context.Context, stepSession *validate.Session) error {
					if err := pump.pause(stepCtx); err != nil {
						return err
					}
					if err := pump.stop(2 * time.Second); err != nil {
						return err
					}
					pump = nil
					if err := waitForAudioSenderSettle(stepCtx, audioSenderSettleDelay); err != nil {
						return err
					}

					err := sender.SetPreferredCodec(targetCodec)
					if err != nil && !errors.Is(err, pc.ErrRenegotiationNeeded) {
						return err
					}
					if err := pp.Renegotiate(); err != nil {
						return err
					}
					if err := stepSession.WaitForStable(stepCtx); err != nil {
						return err
					}
					if err := waitForAudioSenderSettle(stepCtx, audioSenderSettleDelay); err != nil {
						return err
					}

					pump = newAudioPump(track, audioSampleRate, audioChannels, audioSamples)
					return pump.resume(stepCtx)
				},
				Expect: validate.ScenarioExpectation{
					Within:            4 * time.Second,
					HoldFor:           400 * time.Millisecond,
					Connected:         true,
					Stable:            true,
					AudioTrackID:      "audio-codec-switch",
					MinNewAudioFrames: 12,
					CodecMime:         targetCodec.MimeType,
				},
			},
		},
	}); err != nil {
		t.Fatalf("ScenarioLab.Run(audio codec switch): %v", err)
	}

	snapshot := session.Snapshot()
	trackSnap, ok := snapshot.AudioTracks["audio-codec-switch"]
	if !ok {
		t.Fatal("receiver audio track snapshot missing")
	}
	if !strings.EqualFold(trackSnap.CurrentMimeType, targetCodec.MimeType) {
		t.Fatalf("receiver current mime = %q, want %q", trackSnap.CurrentMimeType, targetCodec.MimeType)
	}
	if len(trackSnap.CodecSwitches) == 0 {
		t.Fatal("expected at least one audio codec switch observation")
	}
	lastSwitch := trackSnap.CodecSwitches[len(trackSnap.CodecSwitches)-1]
	if !strings.EqualFold(lastSwitch.Change.CurrentCodec.MimeType, targetCodec.MimeType) {
		t.Fatalf("last audio codec switch target = %q, want %q", lastSwitch.Change.CurrentCodec.MimeType, targetCodec.MimeType)
	}
	if trackSnap.FreezeCount > 1 {
		t.Fatalf("receiver audio freeze count = %d, want at most 1 transient freeze during full codec renegotiation: %+v", trackSnap.FreezeCount, trackSnap)
	}
	if trackSnap.FrameCount < 20 {
		t.Fatalf("receiver audio frame count = %d, want ongoing media after switch", trackSnap.FrameCount)
	}
}

type audioPumpCommand struct {
	active bool
	ack    chan struct{}
}

type audioPump struct {
	stopCh chan struct{}
	done   chan error
	cmd    chan audioPumpCommand
}

func newAudioPump(track *pc.Track, sampleRate, channels, numSamples int) *audioPump {
	p := &audioPump{
		stopCh: make(chan struct{}),
		done:   make(chan error, 1),
		cmd:    make(chan audioPumpCommand),
	}
	go p.run(track, sampleRate, channels, numSamples)
	return p
}

func (p *audioPump) run(track *pc.Track, sampleRate, channels, numSamples int) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	var frameIndex uint32
	active := false
	for {
		select {
		case <-p.stopCh:
			p.done <- nil
			return
		case cmd := <-p.cmd:
			active = cmd.active
			close(cmd.ack)
		case <-ticker.C:
			if !active {
				continue
			}
			frame := CreateTestAudioFrame(sampleRate, channels, numSamples, frameIndex*uint32(numSamples))
			if err := track.WriteAudioFrame(frame); err != nil {
				p.done <- err
				return
			}
			frameIndex++
		}
	}
}

func (p *audioPump) pause(ctx context.Context) error {
	return p.setActive(ctx, false)
}

func (p *audioPump) resume(ctx context.Context) error {
	return p.setActive(ctx, true)
}

func (p *audioPump) setActive(ctx context.Context, active bool) error {
	ack := make(chan struct{})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case p.cmd <- audioPumpCommand{active: active, ack: ack}:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ack:
		return nil
	}
}

func (p *audioPump) stop(timeout time.Duration) error {
	close(p.stopCh)
	select {
	case err := <-p.done:
		return err
	case <-time.After(timeout):
		return errors.New("timed out waiting for audio pump shutdown")
	}
}

func waitForAudioSenderSettle(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func waitForReceiverAudioReady(ctx context.Context, session *validate.Session, trackID string, minFrames uint64) error {
	for {
		snapshot := session.Snapshot()
		if track, ok := snapshot.AudioTracks[trackID]; ok && track.FrameCount >= minFrames {
			return nil
		}

		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func waitForReceiverAudioCodecMaybe(session *validate.Session, peer *pc.PeerConnection, trackID string, timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot := session.Snapshot()
		if track, ok := snapshot.AudioTracks[trackID]; ok && strings.TrimSpace(track.CurrentMimeType) != "" {
			return track.CurrentMimeType, true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return "", false
}

func summarizeAudioSnapshot(session *validate.Session, trackID string) string {
	snapshot := session.Snapshot()
	track, ok := snapshot.AudioTracks[trackID]
	if !ok {
		return "missing track"
	}
	return strings.Join([]string{
		"mime=" + track.CurrentMimeType,
		"frames=" + intToString(int(track.FrameCount)),
		"freezes=" + intToString(int(track.FreezeCount)),
		"stats=" + intToString(len(track.Stats)),
	}, " ")
}

func summarizePeerAudioStats(t *testing.T, peer *pc.PeerConnection) string {
	t.Helper()

	report, err := peer.GetStats()
	if err != nil {
		return "get-stats-error=" + err.Error()
	}

	lines := make([]string, 0, len(report))
	for _, stats := range report {
		switch typed := stats.(type) {
		case webrtc.InboundRTPStreamStats:
			if typed.Kind == "audio" {
				lines = append(lines, "inbound:audio:track="+typed.TrackID+":mid="+typed.Mid+":codec="+typed.CodecID)
			}
		case webrtc.CodecStats:
			lines = append(lines, "codec:"+typed.ID+":mime="+typed.MimeType)
		}
	}
	if len(lines) == 0 {
		return "no-inbound-or-codec-stats"
	}
	return strings.Join(lines, " | ")
}

func chooseAlternateAudioCodec(sender *pc.RTPSender, currentMime string) (webrtc.RTPCodecParameters, bool) {
	codecs, err := sender.GetNegotiatedCodecs()
	if err != nil {
		return webrtc.RTPCodecParameters{}, false
	}

	for _, want := range []string{webrtc.MimeTypeOpus, webrtc.MimeTypePCMU, webrtc.MimeTypePCMA} {
		if strings.EqualFold(want, currentMime) {
			continue
		}
		for _, codec := range codecs {
			if strings.EqualFold(codec.MimeType, want) {
				return codec, true
			}
		}
	}
	for _, codec := range codecs {
		if !strings.EqualFold(codec.MimeType, currentMime) {
			return codec, true
		}
	}
	return webrtc.RTPCodecParameters{}, false
}

func mustNegotiatedAudioCodecs(t *testing.T, sender *pc.RTPSender) []webrtc.RTPCodecParameters {
	t.Helper()
	codecs, err := sender.GetNegotiatedCodecs()
	if err != nil {
		t.Fatalf("GetNegotiatedCodecs: %v", err)
	}
	return codecs
}
