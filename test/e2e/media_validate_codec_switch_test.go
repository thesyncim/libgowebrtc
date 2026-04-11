package e2e

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/pc"
	"github.com/thesyncim/libgowebrtc/pkg/pioncodec"
	"github.com/thesyncim/libgowebrtc/pkg/testkit/validate"
)

func TestReceiverSessionDetectsCodecSwitchViaLibWebRTCStats(t *testing.T) {
	pp := NewLibPeerPair(t)
	defer pp.Close()

	session := validate.NewPCSession(pp.Receiver, validate.SessionConfig{
		Browser:                 pioncodec.BrowserChrome,
		StatsPollInterval:       50 * time.Millisecond,
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

	track, err := pp.Sender.CreateVideoTrack("video-codec-switch", 640, 360)
	if err != nil {
		t.Fatalf("CreateVideoTrack: %v", err)
	}
	sender, err := pp.Sender.AddTrack(track, "stream-codec-switch")
	if err != nil {
		t.Fatalf("AddTrack: %v", err)
	}

	if err := pp.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !pp.WaitForTrack(3 * time.Second) {
		t.Fatal("timed out waiting for receiver track")
	}

	stopPump := make(chan struct{})
	pumpDone := make(chan error, 1)
	go pumpVideoFrames(track, stopPump, pumpDone)
	defer func() {
		close(stopPump)
		select {
		case err := <-pumpDone:
			if err != nil {
				t.Fatalf("pumpVideoFrames: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for frame pump shutdown")
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	if err := session.WaitForConnected(ctx); err != nil {
		t.Fatalf("WaitForConnected: %v", err)
	}
	if err := session.WaitForStable(ctx); err != nil {
		t.Fatalf("WaitForStable: %v", err)
	}
	if err := session.WaitForVideoContinuous(ctx, "video-codec-switch"); err != nil {
		t.Fatalf("WaitForVideoContinuous(initial): %v", err)
	}

	initialMime := waitForReceiverVideoCodec(t, session, pp.Receiver, "video-codec-switch", 4*time.Second)
	targetCodec, ok := chooseAlternateCodec(sender, initialMime)
	if !ok {
		t.Skipf("need at least two negotiated video codecs to test a switch; current=%q negotiated=%v", initialMime, mustNegotiatedCodecs(t, sender))
	}

	lab := validate.NewScenarioLab(session, validate.LabConfig{})
	if err := lab.Run(ctx, validate.ScenarioScript{
		Steps: []validate.ScenarioStep{
			{
				Name:   "codec-switch",
				Action: validate.ScenarioActionCodecRenegotiation,
				Callback: func(context.Context, *validate.Session) error {
					err := sender.SetPreferredCodec(targetCodec)
					if err != nil && !errors.Is(err, pc.ErrRenegotiationNeeded) {
						return err
					}
					return pp.Renegotiate()
				},
				Expect: validate.ScenarioExpectation{
					Within:            4 * time.Second,
					HoldFor:           400 * time.Millisecond,
					Connected:         true,
					Stable:            true,
					VideoTrackID:      "video-codec-switch",
					MinNewVideoFrames: 12,
					CodecMime:         targetCodec.MimeType,
				},
			},
		},
	}); err != nil {
		t.Fatalf("ScenarioLab.Run(codec switch): %v", err)
	}

	snapshot := session.Snapshot()
	trackSnap, ok := snapshot.VideoTracks["video-codec-switch"]
	if !ok {
		t.Fatal("receiver video track snapshot missing")
	}
	if !strings.EqualFold(trackSnap.CurrentMimeType, targetCodec.MimeType) {
		t.Fatalf("receiver current mime = %q, want %q", trackSnap.CurrentMimeType, targetCodec.MimeType)
	}
	if len(trackSnap.CodecSwitches) == 0 {
		t.Fatal("expected at least one codec switch observation")
	}
	lastSwitch := trackSnap.CodecSwitches[len(trackSnap.CodecSwitches)-1]
	if !strings.EqualFold(lastSwitch.Change.CurrentCodec.MimeType, targetCodec.MimeType) {
		t.Fatalf("last codec switch target = %q, want %q", lastSwitch.Change.CurrentCodec.MimeType, targetCodec.MimeType)
	}
	if trackSnap.FreezeCount > 1 {
		t.Fatalf("receiver video freeze count = %d, want at most 1 transient freeze during full codec renegotiation: %+v", trackSnap.FreezeCount, trackSnap)
	}
	if trackSnap.FrameCount < 20 {
		t.Fatalf("receiver frame count = %d, want ongoing media after switch", trackSnap.FrameCount)
	}
}

func pumpVideoFrames(track *pc.Track, stop <-chan struct{}, done chan<- error) {
	ticker := time.NewTicker(33 * time.Millisecond)
	defer ticker.Stop()

	var frameIndex uint32
	for {
		select {
		case <-stop:
			done <- nil
			return
		case <-ticker.C:
			if err := track.WriteVideoFrame(CreateTestFrame(640, 360, frameIndex*3000)); err != nil {
				done <- err
				return
			}
			frameIndex++
		}
	}
}

func waitForReceiverVideoCodec(t *testing.T, session *validate.Session, peer *pc.PeerConnection, trackID string, timeout time.Duration) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot := session.Snapshot()
		if track, ok := snapshot.VideoTracks[trackID]; ok && strings.TrimSpace(track.CurrentMimeType) != "" {
			return track.CurrentMimeType
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for receiver codec on %q; snapshot=%s; stats=%s", trackID, summarizeVideoSnapshot(session, trackID), summarizePeerStats(t, peer))
	return ""
}

func summarizeVideoSnapshot(session *validate.Session, trackID string) string {
	snapshot := session.Snapshot()
	track, ok := snapshot.VideoTracks[trackID]
	if !ok {
		return "missing track"
	}
	return strings.Join([]string{
		"mime=" + track.CurrentMimeType,
		"mid=" + track.CurrentMID,
		"rid=" + track.CurrentRID,
		"frames=" + intToString(int(track.FrameCount)),
		"stats=" + intToString(len(track.Stats)),
	}, " ")
}

func summarizePeerStats(t *testing.T, peer *pc.PeerConnection) string {
	t.Helper()

	report, err := peer.GetStats()
	if err != nil {
		return "get-stats-error=" + err.Error()
	}

	lines := make([]string, 0, len(report))
	for _, stats := range report {
		switch typed := stats.(type) {
		case webrtc.InboundRTPStreamStats:
			lines = append(lines, "inbound:"+typed.Kind+":track="+typed.TrackID+":mid="+typed.Mid+":codec="+typed.CodecID)
		case webrtc.CodecStats:
			lines = append(lines, "codec:"+typed.ID+":mime="+typed.MimeType)
		}
	}
	if len(lines) == 0 {
		return "no-inbound-or-codec-stats"
	}
	return strings.Join(lines, " | ")
}

func intToString(v int) string {
	return strconv.Itoa(v)
}

func chooseAlternateCodec(sender *pc.RTPSender, currentMime string) (webrtc.RTPCodecParameters, bool) {
	preferredOrder := []string{
		webrtc.MimeTypeH264,
		webrtc.MimeTypeVP8,
		webrtc.MimeTypeVP9,
		webrtc.MimeTypeAV1,
	}

	codecs, err := sender.GetNegotiatedCodecs()
	if err != nil {
		return webrtc.RTPCodecParameters{}, false
	}

	for _, want := range preferredOrder {
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

func mustNegotiatedCodecs(t *testing.T, sender *pc.RTPSender) []webrtc.RTPCodecParameters {
	t.Helper()
	codecs, err := sender.GetNegotiatedCodecs()
	if err != nil {
		t.Fatalf("GetNegotiatedCodecs: %v", err)
	}
	return codecs
}
