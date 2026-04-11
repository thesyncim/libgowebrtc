package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/thesyncim/libgowebrtc/pkg/pc"
	"github.com/thesyncim/libgowebrtc/pkg/pioncodec"
	"github.com/thesyncim/libgowebrtc/pkg/testkit/validate"
)

func TestReceiverSessionDetectsAudioCodecSwitchViaLibWebRTCStats(t *testing.T) {
	pp := NewPionLibPeerPair(t)
	defer pp.Close()

	session := validate.NewPCSession(pp.Lib, validate.SessionConfig{
		Browser:                 pioncodec.BrowserChrome,
		StatsPollInterval:       50 * time.Millisecond,
		EventHistory:            64,
		SwitchRecoveryThreshold: 4 * time.Second,
	})

	trackReceived := make(chan struct{}, 1)
	pcOnTrack := session.PCOnTrack()
	pp.Lib.SetOnTrack(func(track *pc.Track, recv *pc.RTPReceiver, streamID string) {
		select {
		case trackReceived <- struct{}{}:
		default:
		}
		pcOnTrack(track, recv, streamID)
	})

	const trackID = "audio-codec-switch"
	const streamID = "stream-audio-switch"
	const audioPTime = 20 * time.Millisecond

	initialCodec := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: 48000,
			Channels:  2,
		},
		PayloadType: 111,
	}
	targetCodec := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: 8000,
			Channels:  0,
		},
		PayloadType: 0,
	}

	initialTrack, err := newPionAudioSampleTrack(initialCodec, trackID, streamID)
	if err != nil {
		t.Fatalf("newPionAudioSampleTrack(initial): %v", err)
	}
	replacementTrack, err := newPionAudioSampleTrack(targetCodec, trackID, streamID)
	if err != nil {
		t.Fatalf("newPionAudioSampleTrack(replacement): %v", err)
	}

	sender, err := pp.Pion.AddTrack(initialTrack)
	if err != nil {
		t.Fatalf("Pion.AddTrack(audio): %v", err)
	}
	go drainPionSenderRTCP(sender)

	var transceiver *webrtc.RTPTransceiver

	if err := connectPionOffersLibAnswersWithICE(pp.Pion, pp.Lib); err != nil {
		t.Fatalf("connectPionOffersLibAnswersWithICE: %v", err)
	}

	startPump := func() (chan struct{}, chan error) {
		stop := make(chan struct{})
		done := make(chan error, 1)
		go pumpPionAudioSamples(initialTrack, opusSilenceAccessUnit(), audioPTime, stop, done)
		return stop, done
	}
	stopPump := func(stop chan struct{}, done chan error) {
		close(stop)
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("pumpAudioFrames: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for audio pump shutdown")
		}
	}

	var pumpStop chan struct{}
	var pumpDone chan error
	defer func() {
		if pumpStop != nil {
			stopPump(pumpStop, pumpDone)
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

	pumpStop, pumpDone = startPump()
	select {
	case <-trackReceived:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for receiver audio track")
	}
	if err := session.WaitForAudioContinuous(ctx, "audio-codec-switch"); err != nil {
		t.Fatalf("WaitForAudioContinuous(initial): %v", err)
	}
	initialMime := waitForReceiverAudioCodec(t, session, pp.Lib, trackID, 4*time.Second)
	if !strings.EqualFold(initialMime, initialCodec.MimeType) {
		t.Fatalf("initial receiver mime = %q, want %q", initialMime, initialCodec.MimeType)
	}

	lab := validate.NewScenarioLab(session, validate.LabConfig{})
	if err := lab.Run(ctx, validate.ScenarioScript{
		Steps: []validate.ScenarioStep{
			{
				Name:   "audio-codec-switch",
				Action: validate.ScenarioActionCodecRenegotiation,
				Callback: func(stepCtx context.Context, stepSession *validate.Session) error {
					stopPump(pumpStop, pumpDone)
					pumpStop, pumpDone = nil, nil

					if err := pp.Pion.RemoveTrack(sender); err != nil {
						return fmt.Errorf("remove old sender: %w", err)
					}
					var err error
					sender, err = pp.Pion.AddTrack(replacementTrack)
					if err != nil {
						return fmt.Errorf("add replacement track: %w", err)
					}
					go drainPionSenderRTCP(sender)

					transceiver = findPionSenderTransceiver(pp.Pion, sender)
					if transceiver == nil {
						return errors.New("replacement sender transceiver not found")
					}
					if err := transceiver.SetCodecPreferences([]webrtc.RTPCodecParameters{targetCodec}); err != nil {
						return fmt.Errorf("set replacement codec prefs: %w", err)
					}
					if err := renegotiatePionOffersLibAnswers(pp.Pion, pp.Lib); err != nil {
						return fmt.Errorf("renegotiate replacement sender: %w", err)
					}
					if err := stepSession.WaitForStable(stepCtx); err != nil {
						return fmt.Errorf("wait for stable after replacement: %w", err)
					}

					pumpStop = make(chan struct{})
					pumpDone = make(chan error, 1)
					go pumpPionAudioSamples(replacementTrack, pcmuSilenceAccessUnit(), audioPTime, pumpStop, pumpDone)
					return nil
				},
				Expect: validate.ScenarioExpectation{
					Within:            4 * time.Second,
					HoldFor:           400 * time.Millisecond,
					Connected:         true,
					Stable:            true,
					AudioTrackID:      trackID,
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
	if trackSnap.FreezeCount > maxTransientCodecRenegotiationFreezes {
		t.Fatalf("receiver audio freeze count = %d, want at most %d transient freezes during full codec renegotiation: %+v", trackSnap.FreezeCount, maxTransientCodecRenegotiationFreezes, trackSnap)
	}
	if trackSnap.FrameCount < 20 {
		t.Fatalf("receiver audio frame count = %d, want ongoing media after switch", trackSnap.FrameCount)
	}
}

func newPionAudioSampleTrack(codecParams webrtc.RTPCodecParameters, trackID, streamID string) (*webrtc.TrackLocalStaticSample, error) {
	return webrtc.NewTrackLocalStaticSample(codecParams.RTPCodecCapability, trackID, streamID)
}

func pumpPionAudioSamples(track *webrtc.TrackLocalStaticSample, payload []byte, frameDuration time.Duration, stop <-chan struct{}, done chan<- error) {
	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			done <- nil
			return
		case <-ticker.C:
			if err := track.WriteSample(media.Sample{
				Data:     append([]byte(nil), payload...),
				Duration: frameDuration,
			}); err != nil {
				done <- err
				return
			}
		}
	}
}

func opusSilenceAccessUnit() []byte {
	return []byte{0xF8, 0xFF, 0xFE}
}

func pcmuSilenceAccessUnit() []byte {
	payload := make([]byte, 160)
	for i := range payload {
		payload[i] = 0xFF
	}
	return payload
}

func waitForReceiverAudioCodec(t *testing.T, session *validate.Session, peer *pc.PeerConnection, trackID string, timeout time.Duration) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot := session.Snapshot()
		if track, ok := snapshot.AudioTracks[trackID]; ok && strings.TrimSpace(track.CurrentMimeType) != "" {
			return track.CurrentMimeType
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for receiver audio codec on %q; snapshot=%s; stats=%s", trackID, summarizeAudioSnapshot(session, trackID), summarizePeerAudioStats(t, peer))
	return ""
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

func connectPionOffersLibAnswersWithICE(pion *webrtc.PeerConnection, lib *pc.PeerConnection) error {
	var (
		pionCandidates []*webrtc.ICECandidate
		libCandidates  []*pc.ICECandidate
		mu             sync.Mutex
	)

	pion.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		pionCandidates = append(pionCandidates, candidate)
	})
	lib.SetOnICECandidate(func(candidate *pc.ICECandidate) {
		if candidate == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		libCandidates = append(libCandidates, candidate)
	})

	if err := renegotiatePionOffersLibAnswers(pion, lib); err != nil {
		return err
	}

	time.Sleep(interopICEGatherDelay)

	mu.Lock()
	defer mu.Unlock()

	for _, candidate := range pionCandidates {
		init := candidate.ToJSON()
		if err := lib.AddICECandidate(pc.ICECandidate(init)); err != nil {
			return err
		}
	}
	for _, candidate := range libCandidates {
		if err := pion.AddICECandidate(webrtc.ICECandidateInit{
			Candidate:        candidate.Candidate,
			SDPMid:           candidate.SDPMid,
			SDPMLineIndex:    candidate.SDPMLineIndex,
			UsernameFragment: candidate.UsernameFragment,
		}); err != nil {
			return err
		}
	}

	time.Sleep(interopICEGatherDelay)
	return nil
}

func renegotiatePionOffersLibAnswers(pion *webrtc.PeerConnection, lib *pc.PeerConnection) error {
	gatherComplete := webrtc.GatheringCompletePromise(pion)

	offer, err := pion.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := pion.SetLocalDescription(offer); err != nil {
		return err
	}
	<-gatherComplete

	if err := lib.SetRemoteDescription(pc.SessionDescription{
		Type: pc.SDPTypeOffer,
		SDP:  pion.LocalDescription().SDP,
	}); err != nil {
		return err
	}

	answer, err := lib.CreateAnswer(nil)
	if err != nil {
		return err
	}
	if err := lib.SetLocalDescription(answer); err != nil {
		return err
	}

	return pion.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answer.SDP,
	})
}

func findPionSenderTransceiver(pc *webrtc.PeerConnection, sender *webrtc.RTPSender) *webrtc.RTPTransceiver {
	for _, transceiver := range pc.GetTransceivers() {
		if transceiver.Sender() == sender {
			return transceiver
		}
	}
	return nil
}

func drainPionSenderRTCP(sender *webrtc.RTPSender) {
	if sender == nil {
		return
	}

	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}
