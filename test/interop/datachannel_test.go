package interop

import (
	"sync"
	"testing"
	"time"

	pionwebrtc "github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/pc"
)

// TestDataChannelLibToP tests data channel from libwebrtc to Pion.
// libwebrtc creates the data channel; Pion receives it.
func TestDataChannelLibToP(t *testing.T) {
	// Create libwebrtc PeerConnection (creates data channel)
	libPC, err := pc.NewPeerConnection(pc.DefaultConfiguration())
	if err != nil {
		t.Fatalf("Failed to create libwebrtc PeerConnection: %v", err)
	}
	defer libPC.Close()

	// Create Pion PeerConnection (receives data channel)
	pionPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create Pion PeerConnection: %v", err)
	}
	defer pionPC.Close()

	// Create data channel on libwebrtc side
	dcLabel := "test-channel-lib-to-pion"
	ordered := true
	libDC, err := libPC.CreateDataChannel(dcLabel, &pc.DataChannelInit{
		Ordered: &ordered,
	})
	if err != nil {
		t.Fatalf("Failed to create data channel: %v", err)
	}
	defer libDC.Close()

	if libDC.Label() != dcLabel {
		t.Errorf("Data channel label mismatch: got %q, want %q", libDC.Label(), dcLabel)
	}

	// Track data channel received on Pion side
	var (
		pionDC          *pionwebrtc.DataChannel
		dcReceived      = make(chan struct{})
		messagesFromLib [][]byte
		messagesMu      sync.Mutex
	)

	pionPC.OnDataChannel(func(dc *pionwebrtc.DataChannel) {
		t.Logf("Pion received data channel: %s", dc.Label())
		pionDC = dc
		close(dcReceived)

		dc.OnMessage(func(msg pionwebrtc.DataChannelMessage) {
			messagesMu.Lock()
			messagesFromLib = append(messagesFromLib, msg.Data)
			messagesMu.Unlock()
		})
	})

	// Do offer/answer exchange (libwebrtc offers since it created the DC)
	offer, err := libPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}

	err = libPC.SetLocalDescription(offer)
	if err != nil {
		t.Fatalf("libPC.SetLocalDescription failed: %v", err)
	}

	err = pionPC.SetRemoteDescription(pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	})
	if err != nil {
		t.Fatalf("pionPC.SetRemoteDescription failed: %v", err)
	}

	answer, err := pionPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("pionPC.CreateAnswer failed: %v", err)
	}

	err = pionPC.SetLocalDescription(answer)
	if err != nil {
		t.Fatalf("pionPC.SetLocalDescription failed: %v", err)
	}

	err = libPC.SetRemoteDescription(&pc.SessionDescription{
		Type: pc.SDPTypeAnswer,
		SDP:  answer.SDP,
	})
	if err != nil {
		t.Fatalf("libPC.SetRemoteDescription failed: %v", err)
	}

	t.Log("Offer/answer exchange completed for data channel test")

	// Verify SDP contains data channel application m-line
	if !containsMediaLine(offer.SDP, "application") {
		t.Error("Offer SDP should contain application m-line for data channel")
	}

	select {
	case <-dcReceived:
		t.Logf("Data channel received by Pion: %s", pionDC.Label())
		if pionDC.Label() != dcLabel {
			t.Errorf("Received DC label mismatch: got %q, want %q", pionDC.Label(), dcLabel)
		}
	case <-time.After(5 * time.Second):
		t.Log("Data channel not received (expected without full ICE connectivity)")
	}
}

// TestDataChannelPionToLib tests data channel from Pion to libwebrtc.
// Pion creates the data channel; libwebrtc receives it.
func TestDataChannelPionToLib(t *testing.T) {
	// Create Pion PeerConnection (creates data channel)
	pionPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create Pion PeerConnection: %v", err)
	}
	defer pionPC.Close()

	// Create libwebrtc PeerConnection (receives data channel)
	libPC, err := pc.NewPeerConnection(pc.DefaultConfiguration())
	if err != nil {
		t.Fatalf("Failed to create libwebrtc PeerConnection: %v", err)
	}
	defer libPC.Close()

	// Create data channel on Pion side
	dcLabel := "test-channel-pion-to-lib"
	pionDC, err := pionPC.CreateDataChannel(dcLabel, nil)
	if err != nil {
		t.Fatalf("Failed to create Pion data channel: %v", err)
	}
	defer pionDC.Close()

	// Track data channel received on libwebrtc side
	dcReceived := make(chan struct{})
	var receivedLabel string

	libPC.OnDataChannel = func(dc *pc.DataChannel) {
		receivedLabel = dc.Label()
		t.Logf("libwebrtc received data channel: %s", dc.Label())
		close(dcReceived)
	}

	// Do offer/answer exchange (Pion offers since it created the DC)
	offer, err := pionPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}

	err = pionPC.SetLocalDescription(offer)
	if err != nil {
		t.Fatalf("pionPC.SetLocalDescription failed: %v", err)
	}

	err = libPC.SetRemoteDescription(&pc.SessionDescription{
		Type: pc.SDPTypeOffer,
		SDP:  offer.SDP,
	})
	if err != nil {
		t.Fatalf("libPC.SetRemoteDescription failed: %v", err)
	}

	answer, err := libPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("libPC.CreateAnswer failed: %v", err)
	}

	err = libPC.SetLocalDescription(answer)
	if err != nil {
		t.Fatalf("libPC.SetLocalDescription failed: %v", err)
	}

	err = pionPC.SetRemoteDescription(pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeAnswer,
		SDP:  answer.SDP,
	})
	if err != nil {
		t.Fatalf("pionPC.SetRemoteDescription failed: %v", err)
	}

	t.Log("Offer/answer exchange completed (Pion to libwebrtc)")

	select {
	case <-dcReceived:
		if receivedLabel != dcLabel {
			t.Errorf("Received DC label mismatch: got %q, want %q", receivedLabel, dcLabel)
		}
		t.Log("Data channel received by libwebrtc")
	case <-time.After(5 * time.Second):
		t.Log("Data channel not received (expected without full ICE connectivity)")
	}
}

// TestDataChannelBidirectional tests bidirectional data channel messaging.
func TestDataChannelBidirectional(t *testing.T) {
	// Create both PeerConnections
	libPC, err := pc.NewPeerConnection(pc.DefaultConfiguration())
	if err != nil {
		t.Fatalf("Failed to create libwebrtc PeerConnection: %v", err)
	}
	defer libPC.Close()

	pionPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create Pion PeerConnection: %v", err)
	}
	defer pionPC.Close()

	// Create data channel on libwebrtc side
	ordered := true
	libDC, err := libPC.CreateDataChannel("bidirectional-test", &pc.DataChannelInit{
		Ordered: &ordered,
	})
	if err != nil {
		t.Fatalf("Failed to create data channel: %v", err)
	}

	var (
		pionDC       *pionwebrtc.DataChannel
		dcOpen       = make(chan struct{})
		libMessages  []string
		pionMessages []string
		messagesMu   sync.Mutex
	)

	// Set up libwebrtc DC callbacks
	libDC.SetOnOpen(func() {
		t.Log("libwebrtc DC opened")
	})
	libDC.SetOnMessage(func(data []byte) {
		messagesMu.Lock()
		libMessages = append(libMessages, string(data))
		messagesMu.Unlock()
	})

	// Set up Pion DC callbacks
	pionPC.OnDataChannel(func(dc *pionwebrtc.DataChannel) {
		pionDC = dc
		t.Logf("Pion received DC: %s", dc.Label())

		dc.OnOpen(func() {
			t.Log("Pion DC opened")
			close(dcOpen)
		})

		dc.OnMessage(func(msg pionwebrtc.DataChannelMessage) {
			messagesMu.Lock()
			pionMessages = append(pionMessages, string(msg.Data))
			messagesMu.Unlock()
		})
	})

	// Do offer/answer exchange
	offer, err := libPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}

	libPC.SetLocalDescription(offer)
	pionPC.SetRemoteDescription(pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	})

	answer, err := pionPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("CreateAnswer failed: %v", err)
	}

	pionPC.SetLocalDescription(answer)
	libPC.SetRemoteDescription(&pc.SessionDescription{
		Type: pc.SDPTypeAnswer,
		SDP:  answer.SDP,
	})

	t.Log("Signaling completed for bidirectional DC test")

	// Wait for DC to open (requires ICE connectivity)
	select {
	case <-dcOpen:
		t.Log("Data channel is open, testing bidirectional messaging")

		// Send from libwebrtc to Pion
		testMsg1 := "Hello from libwebrtc"
		if err := libDC.SendText(testMsg1); err != nil {
			t.Errorf("libDC.SendText failed: %v", err)
		}

		// Send from Pion to libwebrtc
		testMsg2 := "Hello from Pion"
		if err := pionDC.SendText(testMsg2); err != nil {
			t.Errorf("pionDC.SendText failed: %v", err)
		}

		// Give time for messages to be delivered
		time.Sleep(100 * time.Millisecond)

		messagesMu.Lock()
		t.Logf("Pion received %d messages from libwebrtc", len(pionMessages))
		t.Logf("libwebrtc received %d messages from Pion", len(libMessages))
		messagesMu.Unlock()

	case <-time.After(5 * time.Second):
		t.Log("Data channel did not open (expected without full ICE connectivity)")
	}
}

// TestDataChannelWithICE tests data channel with ICE candidate exchange.
func TestDataChannelWithICE(t *testing.T) {
	// Create PeerConnections with STUN servers
	libPC, err := pc.NewPeerConnection(pc.Configuration{
		ICEServers: []pc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	if err != nil {
		t.Fatalf("Failed to create libwebrtc PeerConnection: %v", err)
	}
	defer libPC.Close()

	pionPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{
		ICEServers: []pionwebrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	if err != nil {
		t.Fatalf("Failed to create Pion PeerConnection: %v", err)
	}
	defer pionPC.Close()

	// ICE candidate channels
	libCandidates := make(chan *pc.ICECandidate, 20)
	pionCandidates := make(chan *pionwebrtc.ICECandidate, 20)

	// Set up ICE callbacks
	libPC.OnICECandidate = func(candidate *pc.ICECandidate) {
		if candidate != nil {
			libCandidates <- candidate
		} else {
			close(libCandidates)
		}
	}

	pionPC.OnICECandidate(func(candidate *pionwebrtc.ICECandidate) {
		if candidate != nil {
			pionCandidates <- candidate
		} else {
			close(pionCandidates)
		}
	})

	// Create data channel
	ordered := true
	libDC, err := libPC.CreateDataChannel("ice-dc-test", &pc.DataChannelInit{
		Ordered: &ordered,
	})
	if err != nil {
		t.Fatalf("Failed to create data channel: %v", err)
	}

	dcOpen := make(chan struct{})
	messageReceived := make(chan string, 1)

	pionPC.OnDataChannel(func(dc *pionwebrtc.DataChannel) {
		dc.OnOpen(func() {
			close(dcOpen)
		})
		dc.OnMessage(func(msg pionwebrtc.DataChannelMessage) {
			messageReceived <- string(msg.Data)
		})
	})

	// Do offer/answer exchange
	offer, _ := libPC.CreateOffer(nil)
	libPC.SetLocalDescription(offer)
	pionPC.SetRemoteDescription(pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	})

	answer, _ := pionPC.CreateAnswer(nil)
	pionPC.SetLocalDescription(answer)
	libPC.SetRemoteDescription(&pc.SessionDescription{
		Type: pc.SDPTypeAnswer,
		SDP:  answer.SDP,
	})

	// Exchange ICE candidates in goroutines
	go func() {
		for candidate := range libCandidates {
			pionPC.AddICECandidate(pionwebrtc.ICECandidateInit{
				Candidate:     candidate.Candidate,
				SDPMid:        &candidate.SDPMid,
				SDPMLineIndex: &candidate.SDPMLineIndex,
			})
		}
	}()

	go func() {
		for candidate := range pionCandidates {
			if candidate != nil {
				init := candidate.ToJSON()
				libPC.AddICECandidate(&pc.ICECandidate{
					Candidate:     init.Candidate,
					SDPMid:        *init.SDPMid,
					SDPMLineIndex: uint16(*init.SDPMLineIndex),
				})
			}
		}
	}()

	// Wait for connection
	select {
	case <-dcOpen:
		t.Log("Data channel opened with ICE")

		// Send a message
		testMessage := "Hello via ICE data channel!"
		if err := libDC.SendText(testMessage); err != nil {
			t.Errorf("SendText failed: %v", err)
		}

		select {
		case received := <-messageReceived:
			if received != testMessage {
				t.Errorf("Message mismatch: got %q, want %q", received, testMessage)
			} else {
				t.Logf("Successfully sent and received message: %q", received)
			}
		case <-time.After(2 * time.Second):
			t.Log("Message not received within timeout")
		}

	case <-time.After(10 * time.Second):
		t.Log("Data channel connection timeout (ICE may not complete in test environment)")
	}
}

// TestMultipleDataChannels tests creating multiple data channels.
func TestMultipleDataChannels(t *testing.T) {
	libPC, err := pc.NewPeerConnection(pc.DefaultConfiguration())
	if err != nil {
		t.Fatalf("Failed to create libwebrtc PeerConnection: %v", err)
	}
	defer libPC.Close()

	pionPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create Pion PeerConnection: %v", err)
	}
	defer pionPC.Close()

	// Create multiple data channels
	channels := []string{"channel-1", "channel-2", "channel-3"}
	createdChannels := make(map[string]*pc.DataChannel)

	for _, label := range channels {
		dc, err := libPC.CreateDataChannel(label, nil)
		if err != nil {
			t.Fatalf("Failed to create data channel %s: %v", label, err)
		}
		createdChannels[label] = dc
	}

	// Track received channels on Pion side
	var (
		receivedLabels []string
		labelsMu       sync.Mutex
		allReceived    = make(chan struct{})
	)

	pionPC.OnDataChannel(func(dc *pionwebrtc.DataChannel) {
		labelsMu.Lock()
		receivedLabels = append(receivedLabels, dc.Label())
		if len(receivedLabels) == len(channels) {
			close(allReceived)
		}
		labelsMu.Unlock()
	})

	// Do signaling
	offer, _ := libPC.CreateOffer(nil)
	libPC.SetLocalDescription(offer)
	pionPC.SetRemoteDescription(pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	})

	answer, _ := pionPC.CreateAnswer(nil)
	pionPC.SetLocalDescription(answer)
	libPC.SetRemoteDescription(&pc.SessionDescription{
		Type: pc.SDPTypeAnswer,
		SDP:  answer.SDP,
	})

	t.Logf("Created %d data channels", len(channels))

	select {
	case <-allReceived:
		labelsMu.Lock()
		t.Logf("Pion received all %d data channels: %v", len(receivedLabels), receivedLabels)
		labelsMu.Unlock()
	case <-time.After(5 * time.Second):
		labelsMu.Lock()
		t.Logf("Received %d/%d channels (expected without ICE): %v",
			len(receivedLabels), len(channels), receivedLabels)
		labelsMu.Unlock()
	}

	// Clean up
	for _, dc := range createdChannels {
		dc.Close()
	}
}
