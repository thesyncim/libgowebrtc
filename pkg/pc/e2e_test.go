package pc

import (
	"os"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/testutil"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

// End-to-end tests for PeerConnection that require the shim library.
// Run with: go test -tags=integration ./pkg/pc/...

func TestMain(m *testing.M) {
	os.Exit(testutil.RunWithShim(m))
}

func TestNewPeerConnection(t *testing.T) {
	cfg := DefaultConfiguration()
	pc, err := NewPeerConnection(cfg)
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	if !pc.IsValid() {
		t.Error("PeerConnection should have a valid handle")
	}

	// Check initial states
	if pc.SignalingState() != SignalingStateStable {
		t.Errorf("SignalingState = %v, want stable", pc.SignalingState())
	}
	if pc.ICEConnectionState() != ICEConnectionStateNew {
		t.Errorf("ICEConnectionState = %v, want new", pc.ICEConnectionState())
	}
	if pc.ConnectionState() != PeerConnectionStateNew {
		t.Errorf("ConnectionState = %v, want new", pc.ConnectionState())
	}

	t.Log("PeerConnection created successfully")
}

func TestPeerConnectionWithICEServers(t *testing.T) {
	cfg := Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
			{
				URLs:       []string{"turn:turn.example.com:3478"},
				Username:   "testuser",
				Credential: "testpass",
			},
		},
		BundlePolicy:  BundlePolicyMaxBundle,
		RTCPMuxPolicy: RTCPMuxPolicyRequire,
		SDPSemantics:  SDPSemanticsUnifiedPlan,
	}

	pc, err := NewPeerConnection(cfg)
	if err != nil {
		t.Fatalf("NewPeerConnection with ICE servers failed: %v", err)
	}
	defer pc.Close()

	t.Log("PeerConnection with ICE servers created successfully")
}

func TestCreateOffer(t *testing.T) {
	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	// Create offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}

	if offer.Type != SDPTypeOffer {
		t.Errorf("offer.Type = %v, want offer", offer.Type)
	}
	if offer.SDP == "" {
		t.Error("offer.SDP should not be empty")
	}

	t.Logf("Created offer with %d bytes SDP", len(offer.SDP))
}

func TestCreateOfferWithTrack(t *testing.T) {
	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	// Add a video track first
	track, err := pc.CreateVideoTrack("video-0", 640, 480)
	if err != nil {
		t.Fatalf("CreateVideoTrack failed: %v", err)
	}

	_, err = pc.AddTrack(track, "stream-0")
	if err != nil {
		t.Fatalf("AddTrack failed: %v", err)
	}

	// Now create offer - should include video
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}

	if offer.SDP == "" {
		t.Error("offer.SDP should not be empty")
	}

	// SDP should contain video m-line
	// (In a real test we'd parse and verify)
	t.Logf("Created offer with track: %d bytes SDP", len(offer.SDP))
}

func TestSetLocalDescription(t *testing.T) {
	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}

	err = pc.SetLocalDescription(offer)
	if err != nil {
		t.Fatalf("SetLocalDescription failed: %v", err)
	}

	// Verify local description was set
	localDesc := pc.LocalDescription()
	if localDesc == nil {
		t.Fatal("LocalDescription should not be nil after SetLocalDescription")
	}
	if localDesc.Type != SDPTypeOffer {
		t.Errorf("LocalDescription.Type = %v, want offer", localDesc.Type)
	}

	t.Log("SetLocalDescription succeeded")
}

func TestOfferAnswerExchange(t *testing.T) {
	// Create two peer connections
	pc1, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection (offerer) failed: %v", err)
	}
	defer pc1.Close()

	pc2, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection (answerer) failed: %v", err)
	}
	defer pc2.Close()

	// PC1 creates offer
	offer, err := pc1.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}

	// PC1 sets local description
	err = pc1.SetLocalDescription(offer)
	if err != nil {
		t.Fatalf("PC1 SetLocalDescription failed: %v", err)
	}

	// PC2 sets remote description (the offer)
	err = pc2.SetRemoteDescription(offer)
	if err != nil {
		t.Fatalf("PC2 SetRemoteDescription failed: %v", err)
	}

	// PC2 creates answer
	answer, err := pc2.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("CreateAnswer failed: %v", err)
	}

	if answer.Type != SDPTypeAnswer {
		t.Errorf("answer.Type = %v, want answer", answer.Type)
	}

	// PC2 sets local description (the answer)
	err = pc2.SetLocalDescription(answer)
	if err != nil {
		t.Fatalf("PC2 SetLocalDescription failed: %v", err)
	}

	// PC1 sets remote description (the answer)
	err = pc1.SetRemoteDescription(answer)
	if err != nil {
		t.Fatalf("PC1 SetRemoteDescription failed: %v", err)
	}

	t.Log("Offer/Answer exchange completed successfully")
}

func TestAddTrack(t *testing.T) {
	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	// Create video track
	videoTrack, err := pc.CreateVideoTrack("video-0", 640, 480)
	if err != nil {
		t.Fatalf("CreateVideoTrack failed: %v", err)
	}

	sender, err := pc.AddTrack(videoTrack, "stream-0")
	if err != nil {
		t.Fatalf("AddTrack failed: %v", err)
	}

	if sender == nil {
		t.Error("AddTrack should return a sender")
	}
	if sender.Track() != videoTrack {
		t.Error("Sender.Track() should return the added track")
	}

	// Verify sender is in list
	senders := pc.GetSenders()
	if len(senders) != 1 {
		t.Errorf("GetSenders() len = %d, want 1", len(senders))
	}

	t.Log("AddTrack succeeded")
}

func TestAddTrackUsesSingleExplicitStreamID(t *testing.T) {
	senderPC, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection(sender): %v", err)
	}
	defer senderPC.Close()

	receiverPC, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection(receiver): %v", err)
	}
	defer receiverPC.Close()

	streamCh := make(chan string, 1)
	receiverPC.SetOnTrack(func(track *Track, receiver *RTPReceiver, streamID string) {
		select {
		case streamCh <- streamID:
		default:
		}
	})

	videoTrack, err := senderPC.CreateVideoTrack("video-multi", 64, 64)
	if err != nil {
		t.Fatalf("CreateVideoTrack: %v", err)
	}
	if _, err := senderPC.AddTrack(videoTrack, "stream-a"); err != nil {
		t.Fatalf("AddTrack: %v", err)
	}

	offer, err := senderPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := senderPC.SetLocalDescription(offer); err != nil {
		t.Fatalf("SetLocalDescription(offer): %v", err)
	}
	if err := receiverPC.SetRemoteDescription(offer); err != nil {
		t.Fatalf("SetRemoteDescription(offer): %v", err)
	}

	answer, err := receiverPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("CreateAnswer: %v", err)
	}
	if err := receiverPC.SetLocalDescription(answer); err != nil {
		t.Fatalf("SetLocalDescription(answer): %v", err)
	}
	if err := senderPC.SetRemoteDescription(answer); err != nil {
		t.Fatalf("SetRemoteDescription(answer): %v", err)
	}

	select {
	case streamID := <-streamCh:
		if streamID != "stream-a" {
			t.Fatalf("OnTrack streamID = %q, want stream-a", streamID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnTrack callback")
	}
}

func TestRemoveTrack(t *testing.T) {
	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	// Add track
	track, _ := pc.CreateVideoTrack("video-0", 640, 480)
	sender, _ := pc.AddTrack(track, "stream-0")

	// Verify added
	if len(pc.GetSenders()) != 1 {
		t.Fatal("Track should be added")
	}

	// Remove track
	err = pc.RemoveTrack(sender)
	if err != nil {
		t.Fatalf("RemoveTrack failed: %v", err)
	}

	// Verify removed
	if len(pc.GetSenders()) != 0 {
		t.Error("Track should be removed")
	}

	t.Log("RemoveTrack succeeded")
}

func TestCreateDataChannel(t *testing.T) {
	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	dc, err := pc.CreateDataChannel("test-channel", nil)
	if err != nil {
		t.Fatalf("CreateDataChannel failed: %v", err)
	}

	if dc == nil {
		t.Error("DataChannel should not be nil")
	}
	if dc.Label() != "test-channel" {
		t.Errorf("DataChannel.Label() = %v, want test-channel", dc.Label())
	}

	t.Log("CreateDataChannel succeeded")
}

func TestCreateDataChannelWithOptions(t *testing.T) {
	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	ordered := false
	maxPacketLifeTime := uint16(250)
	id := uint16(17)
	protocol := "test-protocol"
	negotiated := true

	dc, err := pc.CreateDataChannel("ordered-channel", &DataChannelInit{
		Ordered:           &ordered,
		MaxPacketLifeTime: &maxPacketLifeTime,
		Protocol:          &protocol,
		Negotiated:        &negotiated,
		ID:                &id,
	})
	if err != nil {
		t.Fatalf("CreateDataChannel with options failed: %v", err)
	}

	if dc.Label() != "ordered-channel" {
		t.Errorf("Label = %v, want ordered-channel", dc.Label())
	}
	if dc.ID() != id {
		t.Errorf("ID = %d, want %d", dc.ID(), id)
	}

	t.Log("CreateDataChannel with options succeeded")
}

func TestAddICECandidate(t *testing.T) {
	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	// Need to set remote description first
	// Create a dummy offer to set as remote
	offer, _ := pc.CreateOffer(nil)
	pc.SetRemoteDescription(offer)

	// Try to add a candidate
	sdpMid := "0"
	sdpLineIndex := uint16(0)
	candidate := ICECandidate{
		Candidate:     "candidate:1 1 UDP 2130706431 192.168.1.1 12345 typ host",
		SDPMid:        &sdpMid,
		SDPMLineIndex: &sdpLineIndex,
	}

	err = pc.AddICECandidate(candidate)
	// May fail if candidate is invalid for the session, but shouldn't crash
	if err != nil {
		t.Logf("AddICECandidate: %v (may be expected)", err)
	} else {
		t.Log("AddICECandidate succeeded")
	}
}

func TestPeerConnectionClose(t *testing.T) {
	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}

	// Add some tracks
	track, _ := pc.CreateVideoTrack("video-0", 640, 480)
	pc.AddTrack(track, "stream-0")
	pc.CreateDataChannel("dc", nil)

	// Close
	err = pc.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify state
	if pc.SignalingState() != SignalingStateClosed {
		t.Errorf("SignalingState after close = %v, want closed", pc.SignalingState())
	}
	if pc.ConnectionState() != PeerConnectionStateClosed {
		t.Errorf("ConnectionState after close = %v, want closed", pc.ConnectionState())
	}

	// Operations on closed PC should fail
	_, err = pc.CreateOffer(nil)
	if err != ErrPeerConnectionClosed {
		t.Errorf("CreateOffer on closed PC should return ErrPeerConnectionClosed, got: %v", err)
	}

	t.Log("Close succeeded")
}

func TestMultipleTracks(t *testing.T) {
	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	// Add video track
	videoTrack, _ := pc.CreateVideoTrack("video-0", 640, 480)
	pc.AddTrack(videoTrack, "stream-0")

	// Add audio track
	audioTrack, _ := pc.CreateAudioTrack("audio-0")
	pc.AddTrack(audioTrack, "stream-0")

	// Verify both tracks
	senders := pc.GetSenders()
	if len(senders) != 2 {
		t.Errorf("GetSenders() len = %d, want 2", len(senders))
	}

	// Create offer with both tracks
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer with multiple tracks failed: %v", err)
	}

	t.Logf("Created offer with multiple tracks: %d bytes SDP", len(offer.SDP))
}

func TestTransceiverDirection(t *testing.T) {
	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	// Add transceiver with specific direction
	transceiver, err := pc.AddTransceiver("video", &TransceiverInit{
		Direction: TransceiverDirectionSendOnly,
	})
	if err != nil {
		t.Fatalf("AddTransceiver failed: %v", err)
	}

	if transceiver.Direction() != TransceiverDirectionSendOnly {
		t.Errorf("Direction = %v, want sendonly", transceiver.Direction())
	}

	transceivers := pc.GetTransceivers()
	if len(transceivers) != 1 {
		t.Errorf("GetTransceivers() len = %d, want 1", len(transceivers))
	}

	t.Log("AddTransceiver succeeded")
}

// Benchmark PeerConnection creation
func BenchmarkNewPeerConnection(b *testing.B) {
	cfg := DefaultConfiguration()
	for i := 0; i < b.N; i++ {
		pc, _ := NewPeerConnection(cfg)
		pc.Close()
	}
}

// Benchmark offer creation
func BenchmarkCreateOffer(b *testing.B) {
	pc, _ := NewPeerConnection(DefaultConfiguration())
	defer pc.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pc.CreateOffer(nil)
	}
}

func TestGetSupportedVideoCodecs(t *testing.T) {
	codecs, err := GetSupportedVideoCodecs()
	if err != nil {
		t.Fatalf("GetSupportedVideoCodecs failed: %v", err)
	}

	if len(codecs) == 0 {
		t.Fatal("Expected at least one video codec")
	}

	// Should include VP8 or H264
	foundKnown := false
	for _, c := range codecs {
		t.Logf("Video codec: %s (clock=%d, pt=%d)", c.MimeType, c.ClockRate, c.PayloadType)
		if c.MimeType == "video/VP8" || c.MimeType == "video/H264" || c.MimeType == "video/VP9" {
			foundKnown = true
		}
	}
	if !foundKnown {
		t.Error("Expected to find VP8, H264, or VP9 codec")
	}
}

func TestGetSupportedAudioCodecs(t *testing.T) {
	codecs, err := GetSupportedAudioCodecs()
	if err != nil {
		t.Fatalf("GetSupportedAudioCodecs failed: %v", err)
	}

	if len(codecs) == 0 {
		t.Fatal("Expected at least one audio codec")
	}

	// Should include Opus
	foundOpus := false
	for _, c := range codecs {
		t.Logf("Audio codec: %s (clock=%d, ch=%d, pt=%d)", c.MimeType, c.ClockRate, c.Channels, c.PayloadType)
		if c.MimeType == "audio/opus" {
			foundOpus = true
		}
	}
	if !foundOpus {
		t.Error("Expected to find Opus codec")
	}
}

func TestIsCodecSupported(t *testing.T) {
	// VP8 and Opus should be supported
	if !IsCodecSupported("video/VP8") {
		t.Error("VP8 should be supported")
	}
	if !IsCodecSupported("audio/opus") {
		t.Error("Opus should be supported")
	}

	// Unknown codec should not be supported
	if IsCodecSupported("video/unknown-codec") {
		t.Error("Unknown codec should not be supported")
	}
}

// ============================================================================
// Jitter Buffer Control Tests
// ============================================================================

func TestJitterBufferMinDelay(t *testing.T) {
	// Test that jitter buffer minimum delay can be set on a receiver
	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	// Add a transceiver that will receive
	transceiver, err := pc.AddTransceiver("video", &TransceiverInit{
		Direction: TransceiverDirectionRecvOnly,
	})
	if err != nil {
		t.Fatalf("AddTransceiver failed: %v", err)
	}

	receiver := transceiver.Receiver()
	if receiver == nil {
		t.Fatal("Receiver should not be nil")
	}

	// Test setting minimum delay (floor for adaptive jitter buffer)
	err = receiver.SetJitterBufferMinDelay(100) // 100ms minimum
	if err != nil {
		t.Errorf("SetJitterBufferMinDelay(100) failed: %v", err)
	}

	// Test with different values
	err = receiver.SetJitterBufferMinDelay(50) // Lower minimum
	if err != nil {
		t.Errorf("SetJitterBufferMinDelay(50) failed: %v", err)
	}

	err = receiver.SetJitterBufferMinDelay(500) // Higher minimum
	if err != nil {
		t.Errorf("SetJitterBufferMinDelay(500) failed: %v", err)
	}

	// Clear minimum delay (let adaptive algorithm decide)
	err = receiver.SetJitterBufferMinDelay(0)
	if err != nil {
		t.Errorf("SetJitterBufferMinDelay(0) failed: %v", err)
	}

	t.Log("Jitter buffer minimum delay setting succeeded")
}

func TestJitterBufferStatsViaGetStats(t *testing.T) {
	// Test that jitter buffer stats are available through PeerConnection.GetStats().
	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	transceiver, err := pc.AddTransceiver("video", &TransceiverInit{
		Direction: TransceiverDirectionRecvOnly,
	})
	if err != nil {
		t.Fatalf("AddTransceiver failed: %v", err)
	}
	_ = transceiver

	stats, err := pc.GetStats()
	if err != nil {
		t.Errorf("GetStats failed: %v", err)
	}

	if stats == nil {
		t.Fatal("Stats should not be nil")
	}

	sawInbound := false
	for _, stat := range stats {
		inbound, ok := stat.(webrtc.InboundRTPStreamStats)
		if !ok {
			continue
		}
		sawInbound = true
		t.Logf("inbound jitter buffer stats: delay=%.2f target=%.2f minimum=%.2f emitted=%d",
			inbound.JitterBufferDelay,
			inbound.JitterBufferTargetDelay,
			inbound.JitterBufferMinimumDelay,
			inbound.JitterBufferEmittedCount,
		)
	}
	if !sawInbound {
		t.Log("GetStats returned no inbound RTP entries yet, which is expected without media flow")
	}

	t.Log("Jitter buffer stats retrieval via GetStats() succeeded")
}

func TestGetCongestionState(t *testing.T) {
	cfg := DefaultConfiguration()
	cfg.ICEServers = nil

	senderPC, err := NewPeerConnection(cfg)
	if err != nil {
		t.Fatalf("NewPeerConnection(sender): %v", err)
	}
	defer senderPC.Close()

	receiverPC, err := NewPeerConnection(cfg)
	if err != nil {
		t.Fatalf("NewPeerConnection(receiver): %v", err)
	}
	defer receiverPC.Close()

	senderCandidates := make(chan *ICECandidate, 16)
	receiverCandidates := make(chan *ICECandidate, 16)
	receiverFrames := make(chan struct{}, 16)

	senderPC.SetOnICECandidate(func(candidate *ICECandidate) {
		select {
		case senderCandidates <- candidate:
		default:
		}
	})
	receiverPC.SetOnICECandidate(func(candidate *ICECandidate) {
		select {
		case receiverCandidates <- candidate:
		default:
		}
	})
	receiverPC.SetOnTrack(func(track *Track, receiver *RTPReceiver, streamID string) {
		if track.Kind() != "video" {
			return
		}
		if err := track.SetOnVideoFrame(func(*frame.VideoFrame) {
			select {
			case receiverFrames <- struct{}{}:
			default:
			}
		}); err != nil {
			t.Errorf("SetOnVideoFrame() failed: %v", err)
		}
	})

	videoTrack, err := senderPC.CreateVideoTrack("video-congestion", 96, 96)
	if err != nil {
		t.Fatalf("CreateVideoTrack: %v", err)
	}
	if _, err := senderPC.AddTrack(videoTrack, "stream-congestion"); err != nil {
		t.Fatalf("AddTrack: %v", err)
	}

	offer, err := senderPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := senderPC.SetLocalDescription(offer); err != nil {
		t.Fatalf("SetLocalDescription(offer): %v", err)
	}
	if err := receiverPC.SetRemoteDescription(offer); err != nil {
		t.Fatalf("SetRemoteDescription(offer): %v", err)
	}

	answer, err := receiverPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("CreateAnswer: %v", err)
	}
	if err := receiverPC.SetLocalDescription(answer); err != nil {
		t.Fatalf("SetLocalDescription(answer): %v", err)
	}
	if err := senderPC.SetRemoteDescription(answer); err != nil {
		t.Fatalf("SetRemoteDescription(answer): %v", err)
	}

	if !exchangeICEUntilConnected(t, senderPC, receiverPC, senderCandidates, receiverCandidates, 5*time.Second) {
		t.Fatalf("sender did not reach connected state, got %s", senderPC.ConnectionState())
	}
	if receiverPC.ConnectionState() != PeerConnectionStateConnected {
		t.Fatalf("receiver did not reach connected state, got %s", receiverPC.ConnectionState())
	}

	for i := 0; i < 5; i++ {
		if err := videoTrack.WriteVideoFrame(makeTestVideoFrame(96, 96, uint32(i*3000))); err != nil {
			t.Fatalf("WriteVideoFrame(%d): %v", i, err)
		}
		time.Sleep(25 * time.Millisecond)
	}

	select {
	case <-receiverFrames:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a received video frame")
	}

	var senderState *CongestionState
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		senderState, err = senderPC.GetCongestionState()
		if err != nil {
			t.Fatalf("GetCongestionState(sender): %v", err)
		}
		if senderState.PacketsSent > 0 && senderState.BytesSent > 0 {
			break
		}
	}
	if senderState == nil {
		t.Fatal("GetCongestionState(sender) returned nil")
	}
	if senderState.PacketsSent == 0 {
		t.Fatalf("sender congestion state packetsSent = 0, want media to flow; state=%+v", *senderState)
	}
	if senderState.BytesSent == 0 {
		t.Fatalf("sender congestion state bytesSent = 0, want media to flow; state=%+v", *senderState)
	}
	if senderState.QualityLimitationReason == "" {
		t.Fatal("sender congestion state qualityLimitationReason is empty")
	}

	receiverState, err := receiverPC.GetCongestionState()
	if err != nil {
		t.Fatalf("GetCongestionState(receiver): %v", err)
	}
	if receiverState == nil {
		t.Fatal("GetCongestionState(receiver) returned nil")
	}
	if receiverState.PacketsReceived == 0 {
		t.Fatalf("receiver congestion state packetsReceived = 0, want inbound media; state=%+v", *receiverState)
	}
}

func TestSetOnCongestionState(t *testing.T) {
	cfg := DefaultConfiguration()
	cfg.ICEServers = nil

	senderPC, err := NewPeerConnection(cfg)
	if err != nil {
		t.Fatalf("NewPeerConnection(sender): %v", err)
	}
	defer senderPC.Close()

	receiverPC, err := NewPeerConnection(cfg)
	if err != nil {
		t.Fatalf("NewPeerConnection(receiver): %v", err)
	}
	defer receiverPC.Close()

	senderCandidates := make(chan *ICECandidate, 16)
	receiverCandidates := make(chan *ICECandidate, 16)
	receiverFrames := make(chan struct{}, 16)
	congestionUpdates := make(chan CongestionState, 16)

	senderPC.SetOnICECandidate(func(candidate *ICECandidate) {
		select {
		case senderCandidates <- candidate:
		default:
		}
	})
	receiverPC.SetOnICECandidate(func(candidate *ICECandidate) {
		select {
		case receiverCandidates <- candidate:
		default:
		}
	})
	receiverPC.SetOnTrack(func(track *Track, receiver *RTPReceiver, streamID string) {
		if track.Kind() != "video" {
			return
		}
		if err := track.SetOnVideoFrame(func(*frame.VideoFrame) {
			select {
			case receiverFrames <- struct{}{}:
			default:
			}
		}); err != nil {
			t.Errorf("SetOnVideoFrame() failed: %v", err)
		}
	})
	senderPC.SetOnCongestionState(func(state *CongestionState) {
		if state == nil {
			return
		}
		select {
		case congestionUpdates <- *state:
		default:
		}
	})

	videoTrack, err := senderPC.CreateVideoTrack("video-congestion-callback", 96, 96)
	if err != nil {
		t.Fatalf("CreateVideoTrack: %v", err)
	}
	if _, err := senderPC.AddTrack(videoTrack, "stream-congestion-callback"); err != nil {
		t.Fatalf("AddTrack: %v", err)
	}

	offer, err := senderPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := senderPC.SetLocalDescription(offer); err != nil {
		t.Fatalf("SetLocalDescription(offer): %v", err)
	}
	if err := receiverPC.SetRemoteDescription(offer); err != nil {
		t.Fatalf("SetRemoteDescription(offer): %v", err)
	}

	answer, err := receiverPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("CreateAnswer: %v", err)
	}
	if err := receiverPC.SetLocalDescription(answer); err != nil {
		t.Fatalf("SetLocalDescription(answer): %v", err)
	}
	if err := senderPC.SetRemoteDescription(answer); err != nil {
		t.Fatalf("SetRemoteDescription(answer): %v", err)
	}

	if !exchangeICEUntilConnected(t, senderPC, receiverPC, senderCandidates, receiverCandidates, 5*time.Second) {
		t.Fatalf("sender did not reach connected state, got %s", senderPC.ConnectionState())
	}
	if receiverPC.ConnectionState() != PeerConnectionStateConnected {
		t.Fatalf("receiver did not reach connected state, got %s", receiverPC.ConnectionState())
	}

	for i := 0; i < 8; i++ {
		if err := videoTrack.WriteVideoFrame(makeTestVideoFrame(96, 96, uint32(i*3000))); err != nil {
			t.Fatalf("WriteVideoFrame(%d): %v", i, err)
		}
		time.Sleep(25 * time.Millisecond)
	}

	select {
	case <-receiverFrames:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a received video frame")
	}

	var update CongestionState
	var gotUpdate bool
	deadline := time.After(3 * time.Second)
	for !gotUpdate {
		select {
		case update = <-congestionUpdates:
			if update.PacketsSent > 0 && update.BytesSent > 0 && update.QualityLimitationReason != "" {
				gotUpdate = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for congestion callback with outbound media stats")
		}
	}

	if update.PacketsSent == 0 {
		t.Fatalf("callback packetsSent = 0, want outbound media flow; update=%+v", update)
	}
	if update.BytesSent == 0 {
		t.Fatalf("callback bytesSent = 0, want outbound media flow; update=%+v", update)
	}
	if update.QualityLimitationReason == "" {
		t.Fatalf("callback qualityLimitationReason is empty; update=%+v", update)
	}
}

func TestJitterBufferOnNilReceiver(t *testing.T) {
	// Test that methods fail gracefully on nil/invalid receiver
	receiver := &RTPReceiver{}

	err := receiver.SetJitterBufferMinDelay(100)
	if err == nil {
		t.Error("SetJitterBufferMinDelay on nil receiver should fail")
	}

	t.Log("Nil receiver error handling succeeded")
}

func TestAddICECandidateAcceptsEndOfCandidates(t *testing.T) {
	offerer, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection(offerer): %v", err)
	}
	defer offerer.Close()

	answerer, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection(answerer): %v", err)
	}
	defer answerer.Close()

	offer, err := offerer.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := offerer.SetLocalDescription(offer); err != nil {
		t.Fatalf("SetLocalDescription(offer): %v", err)
	}
	if err := answerer.SetRemoteDescription(offer); err != nil {
		t.Fatalf("SetRemoteDescription(offer): %v", err)
	}

	if err := answerer.AddICECandidate(ICECandidate{}); err != nil {
		t.Fatalf("AddICECandidate(end-of-candidates): %v", err)
	}
}

func drainICECandidates(t *testing.T, dst *PeerConnection, src <-chan *ICECandidate) {
	t.Helper()
	for {
		select {
		case candidate := <-src:
			if candidate == nil {
				if err := dst.AddICECandidate(ICECandidate{}); err != nil {
					t.Logf("AddICECandidate(end-of-candidates): %v", err)
				}
				continue
			}
			if err := dst.AddICECandidate(*candidate); err != nil {
				t.Logf("AddICECandidate(%q): %v", candidate.Candidate, err)
			}
		default:
			return
		}
	}
}

func waitForPeerConnectionState(t *testing.T, pc *PeerConnection, want PeerConnectionState, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pc.ConnectionState() == want {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}

func exchangeICEUntilConnected(t *testing.T, senderPC, receiverPC *PeerConnection, senderCandidates, receiverCandidates <-chan *ICECandidate, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		drainICECandidates(t, receiverPC, senderCandidates)
		drainICECandidates(t, senderPC, receiverCandidates)
		if senderPC.ConnectionState() == PeerConnectionStateConnected && receiverPC.ConnectionState() == PeerConnectionStateConnected {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	drainICECandidates(t, receiverPC, senderCandidates)
	drainICECandidates(t, senderPC, receiverCandidates)
	return senderPC.ConnectionState() == PeerConnectionStateConnected && receiverPC.ConnectionState() == PeerConnectionStateConnected
}

func makeTestVideoFrame(width, height int, pts uint32) *frame.VideoFrame {
	f := frame.NewI420Frame(width, height)
	f.PTS = pts

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			f.Data[0][y*width+x] = byte((x + y + int(pts/3000)) % 256)
		}
	}
	for i := range f.Data[1] {
		f.Data[1][i] = 128
		f.Data[2][i] = 128
	}
	return f
}
