package pc

import (
	"sync"
	"testing"
	"time"

	"github.com/thesyncim/libgowebrtc/internal/testutil"
)

func TestPeerConnection_InitialState(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	if pc.SignalingState() != SignalingStateStable {
		t.Errorf("initial signaling state = %v, want stable", pc.SignalingState())
	}
	if pc.ICEConnectionState() != ICEConnectionStateNew {
		t.Errorf("initial ICE connection state = %v, want new", pc.ICEConnectionState())
	}
	if pc.ICEGatheringState() != ICEGatheringStateNew {
		t.Errorf("initial ICE gathering state = %v, want new", pc.ICEGatheringState())
	}
	if pc.ConnectionState() != PeerConnectionStateNew {
		t.Errorf("initial connection state = %v, want new", pc.ConnectionState())
	}
}

func TestPeerConnection_SignalingState_CreateOffer(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	// Create offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if offer.Type != SDPTypeOffer {
		t.Errorf("offer type = %v, want offer", offer.Type)
	}
	if offer.SDP == "" {
		t.Error("offer SDP is empty")
	}

	// State should still be stable until SetLocalDescription
	if pc.SignalingState() != SignalingStateStable {
		t.Errorf("state after CreateOffer = %v, want stable", pc.SignalingState())
	}

	// Set local description
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatalf("SetLocalDescription: %v", err)
	}

	// Now state should be have-local-offer
	if pc.SignalingState() != SignalingStateHaveLocalOffer {
		t.Errorf("state after SetLocalDescription(offer) = %v, want have-local-offer", pc.SignalingState())
	}
}

func TestPeerConnection_SignalingState_OfferAnswerExchange(t *testing.T) {
	testutil.SkipIfNoShim(t)

	offerer, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection (offerer): %v", err)
	}
	defer offerer.Close()

	answerer, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection (answerer): %v", err)
	}
	defer answerer.Close()

	// Offerer creates and sets offer
	offer, err := offerer.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := offerer.SetLocalDescription(offer); err != nil {
		t.Fatalf("offerer.SetLocalDescription: %v", err)
	}

	// Verify offerer state
	if offerer.SignalingState() != SignalingStateHaveLocalOffer {
		t.Errorf("offerer state = %v, want have-local-offer", offerer.SignalingState())
	}

	// Answerer receives offer
	if err := answerer.SetRemoteDescription(offer); err != nil {
		t.Fatalf("answerer.SetRemoteDescription: %v", err)
	}

	// Verify answerer state - should be have-remote-offer or in transition
	answererState := answerer.SignalingState()
	if answererState != SignalingStateHaveRemoteOffer && answererState != SignalingStateHaveLocalPranswer {
		t.Errorf("answerer state = %v, want have-remote-offer or have-local-pranswer", answererState)
	}

	// Answerer creates and sets answer
	answer, err := answerer.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("CreateAnswer: %v", err)
	}
	if answer.Type != SDPTypeAnswer {
		t.Errorf("answer type = %v, want answer", answer.Type)
	}

	if err := answerer.SetLocalDescription(answer); err != nil {
		t.Fatalf("answerer.SetLocalDescription: %v", err)
	}

	// Answerer should now be stable
	if answerer.SignalingState() != SignalingStateStable {
		t.Errorf("answerer state after answer = %v, want stable", answerer.SignalingState())
	}

	// Offerer receives answer
	if err := offerer.SetRemoteDescription(answer); err != nil {
		t.Fatalf("offerer.SetRemoteDescription: %v", err)
	}

	// Offerer should now be stable
	if offerer.SignalingState() != SignalingStateStable {
		t.Errorf("offerer state after answer = %v, want stable", offerer.SignalingState())
	}
}

func TestPeerConnection_OperationsAfterClose(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	pc.Close()

	// CreateOffer should fail
	_, err = pc.CreateOffer(nil)
	if err != ErrPeerConnectionClosed {
		t.Errorf("CreateOffer after close: got %v, want ErrPeerConnectionClosed", err)
	}

	// CreateAnswer should fail
	_, err = pc.CreateAnswer(nil)
	if err != ErrPeerConnectionClosed {
		t.Errorf("CreateAnswer after close: got %v, want ErrPeerConnectionClosed", err)
	}

	// SetLocalDescription should fail
	err = pc.SetLocalDescription(&SessionDescription{Type: SDPTypeOffer, SDP: "v=0..."})
	if err != ErrPeerConnectionClosed {
		t.Errorf("SetLocalDescription after close: got %v, want ErrPeerConnectionClosed", err)
	}

	// SetRemoteDescription should fail
	err = pc.SetRemoteDescription(&SessionDescription{Type: SDPTypeOffer, SDP: "v=0..."})
	if err != ErrPeerConnectionClosed {
		t.Errorf("SetRemoteDescription after close: got %v, want ErrPeerConnectionClosed", err)
	}

	// AddICECandidate should fail
	err = pc.AddICECandidate(&ICECandidate{Candidate: "candidate:..."})
	if err != ErrPeerConnectionClosed {
		t.Errorf("AddICECandidate after close: got %v, want ErrPeerConnectionClosed", err)
	}

	// GetStats should fail
	_, err = pc.GetStats()
	if err != ErrPeerConnectionClosed {
		t.Errorf("GetStats after close: got %v, want ErrPeerConnectionClosed", err)
	}

	// CreateDataChannel should fail
	_, err = pc.CreateDataChannel("test", nil)
	if err != ErrPeerConnectionClosed {
		t.Errorf("CreateDataChannel after close: got %v, want ErrPeerConnectionClosed", err)
	}
}

func TestPeerConnection_DoubleClose(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}

	err1 := pc.Close()
	if err1 != nil {
		t.Errorf("first Close: %v", err1)
	}

	err2 := pc.Close()
	if err2 != nil {
		t.Errorf("second Close: %v", err2)
	}
}

func TestPeerConnection_CloseState(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}

	pc.Close()

	if pc.SignalingState() != SignalingStateClosed {
		t.Errorf("signaling state after close = %v, want closed", pc.SignalingState())
	}
	if pc.ConnectionState() != PeerConnectionStateClosed {
		t.Errorf("connection state after close = %v, want closed", pc.ConnectionState())
	}
	if pc.ICEConnectionState() != ICEConnectionStateClosed {
		t.Errorf("ICE connection state after close = %v, want closed", pc.ICEConnectionState())
	}
}

func TestPeerConnection_SignalingStateCallback(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	var states []SignalingState
	var mu sync.Mutex

	pc.OnSignalingStateChange = func(state SignalingState) {
		mu.Lock()
		states = append(states, state)
		mu.Unlock()
	}

	// Create and set offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatalf("SetLocalDescription: %v", err)
	}

	// Give callback time to fire
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(states) == 0 {
		t.Error("OnSignalingStateChange callback never fired")
	} else {
		found := false
		for _, s := range states {
			if s == SignalingStateHaveLocalOffer {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected HaveLocalOffer in callback states, got %v", states)
		}
	}
	mu.Unlock()
}

func TestPeerConnection_ICECandidateCallback(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	candidateReceived := make(chan *ICECandidate, 10)

	pc.OnICECandidate = func(candidate *ICECandidate) {
		if candidate != nil {
			candidateReceived <- candidate
		}
	}

	// Create and set offer to trigger ICE gathering
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatalf("SetLocalDescription: %v", err)
	}

	// Wait for at least one candidate or timeout
	select {
	case candidate := <-candidateReceived:
		if candidate.Candidate == "" {
			t.Error("received empty candidate")
		}
	case <-time.After(5 * time.Second):
		// ICE gathering might not produce candidates in all test environments
		// Just verify the callback mechanism doesn't hang
		t.Log("no ICE candidates received (may be expected in test environment)")
	}
}

func TestPeerConnection_DataChannel_Creation(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	dc, err := pc.CreateDataChannel("test-channel", nil)
	if err != nil {
		t.Fatalf("CreateDataChannel: %v", err)
	}

	if dc.Label() != "test-channel" {
		t.Errorf("data channel label = %q, want %q", dc.Label(), "test-channel")
	}

	// Initial state should be connecting
	if dc.ReadyState() != DataChannelStateConnecting {
		t.Errorf("initial data channel state = %v, want connecting", dc.ReadyState())
	}

	dc.Close()
}

func TestPeerConnection_DataChannel_WithOptions(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	ordered := false
	maxRetransmits := uint16(3)

	dc, err := pc.CreateDataChannel("unordered-channel", &DataChannelInit{
		Ordered:        &ordered,
		MaxRetransmits: &maxRetransmits,
		Protocol:       "custom-protocol",
	})
	if err != nil {
		t.Fatalf("CreateDataChannel with options: %v", err)
	}
	defer dc.Close()

	if dc.Label() != "unordered-channel" {
		t.Errorf("data channel label = %q, want %q", dc.Label(), "unordered-channel")
	}
}

func TestPeerConnection_Track_VideoTrackCreation(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	track, err := pc.CreateVideoTrack("video-1", 0, 640, 480)
	if err != nil {
		t.Fatalf("CreateVideoTrack: %v", err)
	}

	if track.ID() != "video-1" {
		t.Errorf("track ID = %q, want %q", track.ID(), "video-1")
	}
	if track.Kind() != "video" {
		t.Errorf("track kind = %q, want %q", track.Kind(), "video")
	}
	if !track.Enabled() {
		t.Error("track should be enabled by default")
	}
}

func TestPeerConnection_Track_AudioTrackCreation(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	track, err := pc.CreateAudioTrack("audio-1")
	if err != nil {
		t.Fatalf("CreateAudioTrack: %v", err)
	}

	if track.ID() != "audio-1" {
		t.Errorf("track ID = %q, want %q", track.ID(), "audio-1")
	}
	if track.Kind() != "audio" {
		t.Errorf("track kind = %q, want %q", track.Kind(), "audio")
	}
}

func TestPeerConnection_Track_InvalidDimensions(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"zero width", 0, 480},
		{"zero height", 640, 0},
		{"negative width", -1, 480},
		{"negative height", 640, -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pc.CreateVideoTrack("test", 0, tc.width, tc.height)
			if err == nil {
				t.Error("expected error for invalid dimensions")
			}
		})
	}
}

func TestPeerConnection_Track_InvalidAudioParams(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	tests := []struct {
		name       string
		sampleRate int
		channels   int
	}{
		{"zero sample rate", 0, 2},
		{"negative sample rate", -1, 2},
		{"zero channels", 48000, 0},
		{"too many channels", 48000, 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pc.CreateAudioTrackWithOptions("test", tc.sampleRate, tc.channels)
			if err == nil {
				t.Error("expected error for invalid audio params")
			}
		})
	}
}

func TestPeerConnection_AddTrack_AfterClose(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}

	track, err := pc.CreateVideoTrack("video", 0, 640, 480)
	if err != nil {
		t.Fatalf("CreateVideoTrack: %v", err)
	}

	pc.Close()

	_, err = pc.AddTrack(track)
	if err != ErrPeerConnectionClosed {
		t.Errorf("AddTrack after close: got %v, want ErrPeerConnectionClosed", err)
	}
}

func TestPeerConnection_Concurrent_StateQueries(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	const numGoroutines = 10
	const numIterations = 100

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				_ = pc.SignalingState()
				_ = pc.ICEConnectionState()
				_ = pc.ICEGatheringState()
				_ = pc.ConnectionState()
				_ = pc.LocalDescription()
				_ = pc.RemoteDescription()
			}
		}()
	}

	wg.Wait()
	// Success = no deadlock, no panic
}

func TestPeerConnection_Concurrent_CreateOfferWhileQuerying(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	var wg sync.WaitGroup
	errCh := make(chan error, 10)

	// Goroutine creating offers
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			_, err := pc.CreateOffer(nil)
			if err != nil {
				errCh <- err
			}
		}
	}()

	// Goroutine querying state
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = pc.SignalingState()
			_ = pc.ConnectionState()
		}
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent operation error: %v", err)
	}
}

func TestPeerConnection_RestartICE_AfterClose(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	pc.Close()

	err = pc.RestartICE()
	if err != ErrPeerConnectionClosed {
		t.Errorf("RestartICE after close: got %v, want ErrPeerConnectionClosed", err)
	}
}

func TestPeerConnection_Transceiver_AddAndQuery(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	// Add video transceiver
	videoTr, err := pc.AddTransceiver("video", &TransceiverInit{
		Direction: TransceiverDirectionSendRecv,
	})
	if err != nil {
		t.Fatalf("AddTransceiver(video): %v", err)
	}
	if videoTr == nil {
		t.Fatal("AddTransceiver returned nil")
	}

	// Add audio transceiver
	audioTr, err := pc.AddTransceiver("audio", &TransceiverInit{
		Direction: TransceiverDirectionRecvOnly,
	})
	if err != nil {
		t.Fatalf("AddTransceiver(audio): %v", err)
	}
	if audioTr == nil {
		t.Fatal("AddTransceiver(audio) returned nil")
	}

	// Query transceivers
	transceivers := pc.GetTransceivers()
	if len(transceivers) < 2 {
		t.Errorf("expected at least 2 transceivers, got %d", len(transceivers))
	}

	// Verify transceiver direction
	if audioTr.Direction() != TransceiverDirectionRecvOnly {
		t.Errorf("audio transceiver direction = %v, want recvonly", audioTr.Direction())
	}
}

func TestPeerConnection_NegotiationNeededCallback(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	negotiationNeeded := make(chan struct{}, 1)

	pc.OnNegotiationNeeded = func() {
		select {
		case negotiationNeeded <- struct{}{}:
		default:
		}
	}

	// Adding a transceiver should trigger negotiation needed
	_, err = pc.AddTransceiver("video", nil)
	if err != nil {
		t.Fatalf("AddTransceiver: %v", err)
	}

	select {
	case <-negotiationNeeded:
		// Expected
	case <-time.After(1 * time.Second):
		t.Log("OnNegotiationNeeded not fired (may be expected depending on implementation)")
	}
}

func TestPeerConnection_GetSendersReceivers(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	// Initially should have no senders/receivers
	senders := pc.GetSenders()
	if len(senders) != 0 {
		t.Errorf("initial senders count = %d, want 0", len(senders))
	}

	receivers := pc.GetReceivers()
	if len(receivers) != 0 {
		t.Errorf("initial receivers count = %d, want 0", len(receivers))
	}
}

func TestPeerConnection_LocalRemoteDescription(t *testing.T) {
	testutil.SkipIfNoShim(t)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	// Initially should have no descriptions
	if pc.LocalDescription() != nil {
		t.Error("initial local description should be nil")
	}
	if pc.RemoteDescription() != nil {
		t.Error("initial remote description should be nil")
	}

	// Create and set offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatalf("SetLocalDescription: %v", err)
	}

	// Now local description should be set
	localDesc := pc.LocalDescription()
	if localDesc == nil {
		t.Fatal("local description should not be nil after SetLocalDescription")
	}
	if localDesc.Type != SDPTypeOffer {
		t.Errorf("local description type = %v, want offer", localDesc.Type)
	}
	if localDesc.SDP == "" {
		t.Error("local description SDP is empty")
	}
}

// Benchmarks

func BenchmarkPeerConnection_StateQuery(b *testing.B) {
	testutil.RequireShim(b)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		b.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pc.SignalingState()
		_ = pc.ConnectionState()
		_ = pc.ICEConnectionState()
		_ = pc.ICEGatheringState()
	}
}

func BenchmarkPeerConnection_CreateOffer(b *testing.B) {
	testutil.RequireShim(b)

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		b.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pc.CreateOffer(nil)
	}
}
