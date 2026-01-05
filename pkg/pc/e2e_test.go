package pc

import (
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

// End-to-end tests for PeerConnection that require the shim library.
// Run with: go test -tags=integration ./pkg/pc/...

func TestMain(m *testing.M) {
	if err := ffi.LoadLibrary(); err != nil {
		// Skip if library not available
		return
	}
	defer ffi.Close()
	m.Run()
}

func TestNewPeerConnection(t *testing.T) {
	cfg := DefaultConfiguration()
	pc, err := NewPeerConnection(cfg)
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	if pc.handle == 0 {
		t.Error("PeerConnection handle should not be 0")
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
		ICEServers: []ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
			{
				URLs:       []string{"turn:turn.example.com:3478"},
				Username:   "testuser",
				Credential: "testpass",
			},
		},
		BundlePolicy:  "max-bundle",
		RTCPMuxPolicy: "require",
		SDPSemantics:  "unified-plan",
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
	track, err := pc.CreateVideoTrack("video-0", codec.H264, 640, 480)
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
	videoTrack, err := pc.CreateVideoTrack("video-0", codec.VP8, 640, 480)
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

func TestRemoveTrack(t *testing.T) {
	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection failed: %v", err)
	}
	defer pc.Close()

	// Add track
	track, _ := pc.CreateVideoTrack("video-0", codec.H264, 640, 480)
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

	ordered := true
	maxRetransmits := uint16(3)

	dc, err := pc.CreateDataChannel("ordered-channel", &DataChannelInit{
		Ordered:        &ordered,
		MaxRetransmits: &maxRetransmits,
		Protocol:       "test-protocol",
	})
	if err != nil {
		t.Fatalf("CreateDataChannel with options failed: %v", err)
	}

	if dc.Label() != "ordered-channel" {
		t.Errorf("Label = %v, want ordered-channel", dc.Label())
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
	candidate := &ICECandidate{
		Candidate:     "candidate:1 1 UDP 2130706431 192.168.1.1 12345 typ host",
		SDPMid:        "0",
		SDPMLineIndex: 0,
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
	track, _ := pc.CreateVideoTrack("video-0", codec.H264, 640, 480)
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
	videoTrack, _ := pc.CreateVideoTrack("video-0", codec.VP9, 640, 480)
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
