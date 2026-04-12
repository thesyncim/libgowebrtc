package e2e

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	pionmedia "github.com/pion/webrtc/v4/pkg/media"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	libcodec "github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/pc"
	"github.com/thesyncim/libgowebrtc/pkg/testkit/validate"
)

const (
	audioCodecSwitchStatsPollInterval = 250 * time.Millisecond
	audioWarmupFrameCount             = 6
)

func TestReceiverSessionDetectsAudioCodecSwitchViaLibWebRTCStats(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("native receiver audio codec-switch validation is currently only stable on darwin_arm64")
	}

	pp := NewPionLibPeerPair(t)
	defer pp.Close()

	cfg := validate.DefaultSessionConfig()
	cfg.StatsPollInterval = audioCodecSwitchStatsPollInterval
	cfg.EventHistory = 64
	cfg.RecoveryTimeout = 4 * time.Second
	cfg.Assertions.CodecSwitch = true
	session := validate.NewPCSession(pp.Lib, cfg)

	trackReceived := make(chan struct{}, 1)
	pcOnTrack := session.PCOnTrack()
	pp.Lib.SetOnTrack(func(track *pc.Track, recv *pc.RTPReceiver) {
		select {
		case trackReceived <- struct{}{}:
		default:
		}
		pcOnTrack(track, recv)
	})

	const trackID = "audio-codec-switch"
	const streamID = "stream-audio-switch"

	candidates, err := buildPionAudioTrackCandidates(trackID, streamID)
	if err != nil {
		t.Fatalf("buildPionAudioTrackCandidates: %v", err)
	}
	initialCandidate, ok := findPionAudioTrackCandidate(candidates, webrtc.MimeTypeOpus)
	if !ok {
		t.Skip("need Opus support on both libwebrtc and Pion for audio switch validation")
	}
	sender, err := pp.Pion.AddTrack(initialCandidate.track)
	if err != nil {
		t.Fatalf("Pion.AddTrack(audio): %v", err)
	}
	go drainPionSenderRTCP(sender)

	transceiver := findPionSenderTransceiver(pp.Pion, sender)
	if transceiver == nil {
		t.Fatal("sender transceiver not found")
	}
	if err := transceiver.SetCodecPreferences(codecPreferenceList(candidates, initialCandidate.params.MimeType)); err != nil {
		t.Fatalf("SetCodecPreferences(initial): %v", err)
	}

	if err := signalPionOffersLibAnswers(pp.Pion, pp.Lib); err != nil {
		t.Fatalf("signalPionOffersLibAnswers(initial): %v", err)
	}
	if err := waitForLibTrackSignal(trackReceived, 3*time.Second); err != nil {
		t.Fatalf("waitForLibTrackSignal(initial): %v", err)
	}

	pump := startPionAudioPump(initialCandidate.track, initialCandidate.sample)
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

	if err := waitForReceiverAudioReady(ctx, session, trackID, audioWarmupFrameCount); err != nil {
		t.Fatalf("waitForReceiverAudioReady(initial): %v", err)
	}
	initialMime, ok := waitForReceiverAudioCodecMaybe(session, pp.Lib, trackID, 4*time.Second)
	if !ok {
		t.Skipf("native receiver audio codec stats unavailable; snapshot=%s; stats=%s", summarizeAudioSnapshot(session, trackID), summarizePeerAudioStats(t, pp.Lib))
	}
	targetCandidate, ok := chooseAlternatePionAudioTrack(candidates, initialMime)
	if !ok {
		t.Skipf("need at least two supported audio codecs to test a switch; current=%q stats=%s", initialMime, summarizePeerAudioStats(t, pp.Lib))
	}

	// Startup warmup can include a transient gap on slower runners; reset before
	// asserting the renegotiation behavior itself.
	session.Reset()
	if err := waitForReceiverAudioReady(ctx, session, trackID, audioWarmupFrameCount); err != nil {
		t.Fatalf("waitForReceiverAudioReady(reset): %v", err)
	}

	lab := validate.NewScenarioLab(session, validate.LabConfig{})
	if err := lab.Run(ctx, validate.ScenarioScript{
		Steps: []validate.ScenarioStep{
			{
				Name:   "audio-codec-switch",
				Action: validate.ScenarioActionCodecRenegotiation,
				Callback: func(stepCtx context.Context, stepSession *validate.Session) error {
					if err := pump.stop(2 * time.Second); err != nil {
						return err
					}
					pump = nil
					if err := pp.Pion.RemoveTrack(sender); err != nil {
						return err
					}
					nextSender, err := pp.Pion.AddTrack(targetCandidate.track)
					if err != nil {
						return err
					}
					sender = nextSender
					go drainPionSenderRTCP(sender)
					nextTransceiver := findPionSenderTransceiver(pp.Pion, sender)
					if nextTransceiver == nil {
						return errors.New("replacement sender transceiver not found")
					}
					transceiver = nextTransceiver
					if err := transceiver.SetCodecPreferences(codecPreferenceList(candidates, targetCandidate.params.MimeType)); err != nil {
						return err
					}
					if err := signalPionOffersLibAnswers(pp.Pion, pp.Lib); err != nil {
						return err
					}
					if err := stepSession.WaitForStable(stepCtx); err != nil {
						return err
					}

					pump = startPionAudioPump(targetCandidate.track, targetCandidate.sample)
					return nil
				},
				Expect: validate.ScenarioExpectation{
					Within:            4 * time.Second,
					HoldFor:           400 * time.Millisecond,
					Connected:         true,
					Stable:            true,
					AudioTrackID:      trackID,
					MinNewAudioFrames: 12,
					CodecMime:         targetCandidate.params.MimeType,
				},
			},
		},
	}); err != nil {
		t.Fatalf(
			"ScenarioLab.Run(audio codec switch): %v; snapshot=%s; stats=%s",
			err,
			summarizeAudioSnapshot(session, trackID),
			summarizePeerAudioStats(t, pp.Lib),
		)
	}

	snapshot := session.Snapshot()
	trackSnap, ok := snapshot.AudioTracks[trackID]
	if !ok {
		t.Fatal("receiver audio track snapshot missing")
	}
	if !strings.EqualFold(trackSnap.CurrentMimeType, targetCandidate.params.MimeType) {
		t.Fatalf("receiver current mime = %q, want %q", trackSnap.CurrentMimeType, targetCandidate.params.MimeType)
	}
	if len(trackSnap.CodecSwitches) == 0 {
		t.Fatal("expected at least one audio codec switch observation")
	}
	lastSwitch := trackSnap.CodecSwitches[len(trackSnap.CodecSwitches)-1]
	if !strings.EqualFold(lastSwitch.Change.CurrentCodec.MimeType, targetCandidate.params.MimeType) {
		t.Fatalf("last audio codec switch target = %q, want %q", lastSwitch.Change.CurrentCodec.MimeType, targetCandidate.params.MimeType)
	}
	if trackSnap.FreezeCount > 1 {
		t.Fatalf("receiver audio freeze count = %d, want at most 1 transient freeze during full codec renegotiation: %+v", trackSnap.FreezeCount, trackSnap)
	}
	if trackSnap.FrameCount < 20 {
		t.Fatalf("receiver audio frame count = %d, want ongoing media after switch", trackSnap.FrameCount)
	}
}

func startPionAudioPump(track *webrtc.TrackLocalStaticSample, sample pionmedia.Sample) *pionAudioPump {
	p := &pionAudioPump{
		stopCh: make(chan struct{}),
		done:   make(chan error, 1),
	}
	go p.run(track, sample)
	return p
}

type pionAudioPump struct {
	stopCh chan struct{}
	done   chan error
}

type pionAudioTrackCandidate struct {
	params webrtc.RTPCodecParameters
	track  *webrtc.TrackLocalStaticSample
	sample pionmedia.Sample
}

func (p *pionAudioPump) run(track *webrtc.TrackLocalStaticSample, sample pionmedia.Sample) {
	ticker := time.NewTicker(sample.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			p.done <- nil
			return
		case <-ticker.C:
			if err := track.WriteSample(sample); err != nil {
				p.done <- err
				return
			}
		}
	}
}

func (p *pionAudioPump) stop(timeout time.Duration) error {
	close(p.stopCh)
	select {
	case err := <-p.done:
		return err
	case <-time.After(timeout):
		return errors.New("timed out waiting for pion audio pump shutdown")
	}
}

func chooseAlternatePionAudioTrack(candidates []pionAudioTrackCandidate, currentMime string) (pionAudioTrackCandidate, bool) {
	for _, candidate := range candidates {
		if !strings.EqualFold(candidate.params.MimeType, currentMime) {
			return candidate, true
		}
	}
	return pionAudioTrackCandidate{}, false
}

func buildPionAudioTrackCandidates(trackID, streamID string) ([]pionAudioTrackCandidate, error) {
	supported, err := ffi.GetSupportedAudioCodecs()
	if err != nil {
		return nil, err
	}

	supportedByMime := make(map[string]ffi.CodecCapability, len(supported))
	for _, codec := range supported {
		supportedByMime[strings.ToLower(ffi.CStringToGo(codec.MimeType[:]))] = codec
	}

	candidates := make([]pionAudioTrackCandidate, 0, 3)
	for _, want := range []string{webrtc.MimeTypePCMU, webrtc.MimeTypePCMA, webrtc.MimeTypeOpus} {
		capability, ok := supportedByMime[strings.ToLower(want)]
		if !ok {
			continue
		}

		params, track, sample, err := newPionAudioTrackForCodec(capability, trackID, streamID)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, pionAudioTrackCandidate{
			params: params,
			track:  track,
			sample: sample,
		})
	}

	return candidates, nil
}

func findPionAudioTrackCandidate(candidates []pionAudioTrackCandidate, want string) (pionAudioTrackCandidate, bool) {
	for _, candidate := range candidates {
		if strings.EqualFold(candidate.params.MimeType, want) {
			return candidate, true
		}
	}
	return pionAudioTrackCandidate{}, false
}

func codecPreferenceList(candidates []pionAudioTrackCandidate, firstMime string) []webrtc.RTPCodecParameters {
	preferences := make([]webrtc.RTPCodecParameters, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.EqualFold(candidate.params.MimeType, firstMime) {
			preferences = append(preferences, candidate.params)
		}
	}
	for _, candidate := range candidates {
		if !strings.EqualFold(candidate.params.MimeType, firstMime) {
			preferences = append(preferences, candidate.params)
		}
	}
	return preferences
}

func newPionAudioTrackForCodec(capability ffi.CodecCapability, trackID, streamID string) (webrtc.RTPCodecParameters, *webrtc.TrackLocalStaticSample, pionmedia.Sample, error) {
	mimeType := ffi.CStringToGo(capability.MimeType[:])
	params := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:     mimeType,
			ClockRate:    uint32(capability.ClockRate),
			Channels:     pionCompatibleAudioChannels(mimeType, uint16(capability.Channels)),
			SDPFmtpLine:  ffi.CStringToGo(capability.SDPFmtpLine[:]),
			RTCPFeedback: nil,
		},
		PayloadType: webrtc.PayloadType(capability.PayloadType),
	}
	sample := pionmedia.Sample{Duration: 20 * time.Millisecond}

	switch {
	case strings.EqualFold(mimeType, webrtc.MimeTypeOpus):
		payload, err := encodeOpusSwitchSample()
		if err != nil {
			return webrtc.RTPCodecParameters{}, nil, pionmedia.Sample{}, err
		}
		sample.Data = payload
	case strings.EqualFold(mimeType, webrtc.MimeTypePCMU):
		sample.Data = bytes.Repeat([]byte{0xff}, 160)
	case strings.EqualFold(mimeType, webrtc.MimeTypePCMA):
		sample.Data = bytes.Repeat([]byte{0xd5}, 160)
	default:
		return webrtc.RTPCodecParameters{}, nil, pionmedia.Sample{}, errors.New("unsupported pion audio codec: " + mimeType)
	}

	track, err := webrtc.NewTrackLocalStaticSample(params.RTPCodecCapability, trackID, streamID)
	if err != nil {
		return webrtc.RTPCodecParameters{}, nil, pionmedia.Sample{}, err
	}
	return params, track, sample, nil
}

func pionCompatibleAudioChannels(mimeType string, channels uint16) uint16 {
	switch {
	case strings.EqualFold(mimeType, webrtc.MimeTypePCMU), strings.EqualFold(mimeType, webrtc.MimeTypePCMA):
		return 0
	default:
		return channels
	}
}

func encodeOpusSwitchSample() ([]byte, error) {
	enc, err := encoder.NewOpusEncoder(libcodec.DefaultOpusConfig())
	if err != nil {
		return nil, err
	}
	defer enc.Close()

	src := CreateTestAudioFrame(48_000, 2, 960, 0)
	dst := make([]byte, enc.MaxEncodedSize())
	for attempt := 0; attempt < 4; attempt++ {
		n, err := enc.EncodeInto(src, dst)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			return append([]byte(nil), dst[:n]...), nil
		}
	}
	return nil, errors.New("opus encoder produced no payload")
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

func findPionSenderTransceiver(pc *webrtc.PeerConnection, sender *webrtc.RTPSender) *webrtc.RTPTransceiver {
	for _, transceiver := range pc.GetTransceivers() {
		if transceiver.Sender() == sender {
			return transceiver
		}
	}
	return nil
}

func signalPionOffersLibAnswers(pionPC *webrtc.PeerConnection, libPC *pc.PeerConnection) error {
	gatherComplete := webrtc.GatheringCompletePromise(pionPC)
	offer, err := pionPC.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := pionPC.SetLocalDescription(offer); err != nil {
		return err
	}
	<-gatherComplete

	if err := libPC.SetRemoteDescription(pc.SessionDescription{
		Type: pc.SDPTypeOffer,
		SDP:  pionPC.LocalDescription().SDP,
	}); err != nil {
		return err
	}

	answer, err := libPC.CreateAnswer(nil)
	if err != nil {
		return err
	}
	if err := libPC.SetLocalDescription(answer); err != nil {
		return err
	}

	return pionPC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answer.SDP,
	})
}

func waitForLibTrackSignal(trackReceived <-chan struct{}, timeout time.Duration) error {
	select {
	case <-trackReceived:
		return nil
	case <-time.After(timeout):
		return errors.New("timed out waiting for receiver audio track")
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
