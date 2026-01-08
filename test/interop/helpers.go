// Package interop provides helper functions for libwebrtc-Pion interoperability tests.
package interop

import (
	"sync"
	"testing"
	"time"

	pionwebrtc "github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/pc"
)

const (
	interopShortTimeout   = 1 * time.Second
	interopMessageTimeout = 500 * time.Millisecond
)

func defaultInteropConfig() pc.Configuration {
	cfg := pc.DefaultConfiguration()
	cfg.ICEServers = nil
	return cfg
}

// PeerPair holds a pair of connected PeerConnections for testing.
type PeerPair struct {
	Lib  *pc.PeerConnection
	Pion *pionwebrtc.PeerConnection

	// ICE candidate channels
	libCandidates  chan *pc.ICECandidate
	pionCandidates chan *pionwebrtc.ICECandidate

	// State tracking
	libConnected  bool
	pionConnected bool
	mu            sync.Mutex

	t *testing.T
}

// PeerPairConfig configures the peer pair.
type PeerPairConfig struct {
	// UseSTUN enables STUN servers for ICE gathering
	UseSTUN bool
	// ExchangeICE enables ICE candidate exchange (requires UseSTUN)
	ExchangeICE bool
}

// DefaultPeerPairConfig returns configuration with no STUN/ICE.
func DefaultPeerPairConfig() PeerPairConfig {
	return PeerPairConfig{
		UseSTUN:     false,
		ExchangeICE: false,
	}
}

// STUNPeerPairConfig returns configuration with STUN and ICE exchange.
func STUNPeerPairConfig() PeerPairConfig {
	return PeerPairConfig{
		UseSTUN:     true,
		ExchangeICE: true,
	}
}

// NewPeerPair creates a pair of PeerConnections for testing.
func NewPeerPair(t *testing.T, cfg PeerPairConfig) (*PeerPair, error) {
	t.Helper()

	// Configure ICE servers
	libConfig := pc.DefaultConfiguration()
	pionConfig := pionwebrtc.Configuration{}

	if cfg.UseSTUN {
		libConfig.ICEServers = []pc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		}
		pionConfig.ICEServers = []pionwebrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		}
	} else {
		libConfig.ICEServers = nil
	}

	// Create libwebrtc PeerConnection
	libPC, err := pc.NewPeerConnection(libConfig)
	if err != nil {
		return nil, err
	}

	// Create Pion PeerConnection
	pionPC, err := pionwebrtc.NewPeerConnection(pionConfig)
	if err != nil {
		libPC.Close()
		return nil, err
	}

	pp := &PeerPair{
		Lib:            libPC,
		Pion:           pionPC,
		libCandidates:  make(chan *pc.ICECandidate, 20),
		pionCandidates: make(chan *pionwebrtc.ICECandidate, 20),
		t:              t,
	}

	// Set up ICE callbacks if enabled
	if cfg.ExchangeICE {
		pp.setupICECallbacks()
	}

	return pp, nil
}

// setupICECallbacks sets up ICE candidate exchange between peers.
func (pp *PeerPair) setupICECallbacks() {
	pp.Lib.OnICECandidate = func(candidate *pc.ICECandidate) {
		if candidate != nil {
			pp.libCandidates <- candidate
		} else {
			close(pp.libCandidates)
		}
	}

	pp.Pion.OnICECandidate(func(candidate *pionwebrtc.ICECandidate) {
		if candidate != nil {
			pp.pionCandidates <- candidate
		} else {
			close(pp.pionCandidates)
		}
	})

	// Start ICE exchange goroutines
	go pp.forwardLibToPion()
	go pp.forwardPionToLib()
}

func (pp *PeerPair) forwardLibToPion() {
	for candidate := range pp.libCandidates {
		pp.Pion.AddICECandidate(pionwebrtc.ICECandidateInit{
			Candidate:     candidate.Candidate,
			SDPMid:        &candidate.SDPMid,
			SDPMLineIndex: &candidate.SDPMLineIndex,
		})
	}
}

func (pp *PeerPair) forwardPionToLib() {
	for candidate := range pp.pionCandidates {
		if candidate != nil {
			init := candidate.ToJSON()
			pp.Lib.AddICECandidate(&pc.ICECandidate{
				Candidate:     init.Candidate,
				SDPMid:        *init.SDPMid,
				SDPMLineIndex: uint16(*init.SDPMLineIndex),
			})
		}
	}
}

// ExchangeOfferAnswer performs offer/answer exchange with lib as offerer.
func (pp *PeerPair) ExchangeOfferAnswer() error {
	return pp.ExchangeOfferAnswerWithOfferer(true)
}

// ExchangeOfferAnswerWithOfferer performs offer/answer exchange.
// If libOffers is true, libwebrtc creates the offer; otherwise Pion does.
func (pp *PeerPair) ExchangeOfferAnswerWithOfferer(libOffers bool) error {
	if libOffers {
		return pp.libOffers()
	}
	return pp.pionOffers()
}

func (pp *PeerPair) libOffers() error {
	offer, err := pp.Lib.CreateOffer(nil)
	if err != nil {
		return err
	}

	if err := pp.Lib.SetLocalDescription(offer); err != nil {
		return err
	}

	if err := pp.Pion.SetRemoteDescription(pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	}); err != nil {
		return err
	}

	answer, err := pp.Pion.CreateAnswer(nil)
	if err != nil {
		return err
	}

	if err := pp.Pion.SetLocalDescription(answer); err != nil {
		return err
	}

	if err := pp.Lib.SetRemoteDescription(&pc.SessionDescription{
		Type: pc.SDPTypeAnswer,
		SDP:  answer.SDP,
	}); err != nil {
		return err
	}

	return nil
}

func (pp *PeerPair) pionOffers() error {
	offer, err := pp.Pion.CreateOffer(nil)
	if err != nil {
		return err
	}

	if err := pp.Pion.SetLocalDescription(offer); err != nil {
		return err
	}

	if err := pp.Lib.SetRemoteDescription(&pc.SessionDescription{
		Type: pc.SDPTypeOffer,
		SDP:  offer.SDP,
	}); err != nil {
		return err
	}

	answer, err := pp.Lib.CreateAnswer(nil)
	if err != nil {
		return err
	}

	if err := pp.Lib.SetLocalDescription(answer); err != nil {
		return err
	}

	if err := pp.Pion.SetRemoteDescription(pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeAnswer,
		SDP:  answer.SDP,
	}); err != nil {
		return err
	}

	return nil
}

// WaitForConnection waits for both peers to reach connected state.
func (pp *PeerPair) WaitForConnection(timeout time.Duration) bool {
	var wg sync.WaitGroup
	wg.Add(2)

	pp.Lib.OnConnectionStateChange = func(state pc.PeerConnectionState) {
		pp.t.Logf("libwebrtc connection state: %v", state)
		pp.mu.Lock()
		if state == pc.PeerConnectionStateConnected && !pp.libConnected {
			pp.libConnected = true
			wg.Done()
		}
		pp.mu.Unlock()
	}

	pp.Pion.OnConnectionStateChange(func(state pionwebrtc.PeerConnectionState) {
		pp.t.Logf("Pion connection state: %v", state)
		pp.mu.Lock()
		if state == pionwebrtc.PeerConnectionStateConnected && !pp.pionConnected {
			pp.pionConnected = true
			wg.Done()
		}
		pp.mu.Unlock()
	})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// Close closes both PeerConnections.
func (pp *PeerPair) Close() {
	if pp.Lib != nil {
		pp.Lib.Close()
	}
	if pp.Pion != nil {
		pp.Pion.Close()
	}
}

// DataChannelPair holds a pair of data channels for testing.
type DataChannelPair struct {
	Lib  *pc.DataChannel
	Pion *pionwebrtc.DataChannel

	// Message tracking
	libMessages  [][]byte
	pionMessages [][]byte
	messagesMu   sync.Mutex

	// State channels
	pionDCReceived chan struct{}
	pionDCOpen     chan struct{}

	t *testing.T
}

// CreateDataChannelPair creates a data channel pair with lib as creator.
func (pp *PeerPair) CreateDataChannelPair(label string, ordered bool) (*DataChannelPair, error) {
	dcp := &DataChannelPair{
		pionDCReceived: make(chan struct{}),
		pionDCOpen:     make(chan struct{}),
		t:              pp.t,
	}

	// Create DC on libwebrtc side
	libDC, err := pp.Lib.CreateDataChannel(label, &pc.DataChannelInit{
		Ordered: &ordered,
	})
	if err != nil {
		return nil, err
	}
	dcp.Lib = libDC

	// Set up libDC message handler
	libDC.SetOnMessage(func(data []byte) {
		dcp.messagesMu.Lock()
		dcp.libMessages = append(dcp.libMessages, data)
		dcp.messagesMu.Unlock()
	})

	// Set up Pion to receive DC
	pp.Pion.OnDataChannel(func(dc *pionwebrtc.DataChannel) {
		dcp.Pion = dc
		close(dcp.pionDCReceived)

		dc.OnOpen(func() {
			close(dcp.pionDCOpen)
		})

		dc.OnMessage(func(msg pionwebrtc.DataChannelMessage) {
			dcp.messagesMu.Lock()
			dcp.pionMessages = append(dcp.pionMessages, msg.Data)
			dcp.messagesMu.Unlock()
		})
	})

	return dcp, nil
}

// WaitForOpen waits for both data channels to be open.
func (dcp *DataChannelPair) WaitForOpen(timeout time.Duration) bool {
	select {
	case <-dcp.pionDCOpen:
		return true
	case <-time.After(timeout):
		return false
	}
}

// WaitForReceived waits for Pion to receive the data channel.
func (dcp *DataChannelPair) WaitForReceived(timeout time.Duration) bool {
	select {
	case <-dcp.pionDCReceived:
		return true
	case <-time.After(timeout):
		return false
	}
}

// SendFromLib sends a message from libwebrtc to Pion.
func (dcp *DataChannelPair) SendFromLib(data []byte) error {
	return dcp.Lib.Send(data)
}

// SendFromPion sends a message from Pion to libwebrtc.
func (dcp *DataChannelPair) SendFromPion(data []byte) error {
	return dcp.Pion.Send(data)
}

// GetLibMessages returns messages received by libwebrtc.
func (dcp *DataChannelPair) GetLibMessages() [][]byte {
	dcp.messagesMu.Lock()
	defer dcp.messagesMu.Unlock()
	result := make([][]byte, len(dcp.libMessages))
	copy(result, dcp.libMessages)
	return result
}

// GetPionMessages returns messages received by Pion.
func (dcp *DataChannelPair) GetPionMessages() [][]byte {
	dcp.messagesMu.Lock()
	defer dcp.messagesMu.Unlock()
	result := make([][]byte, len(dcp.pionMessages))
	copy(result, dcp.pionMessages)
	return result
}

// Close closes both data channels.
func (dcp *DataChannelPair) Close() {
	if dcp.Lib != nil {
		dcp.Lib.Close()
	}
	if dcp.Pion != nil {
		dcp.Pion.Close()
	}
}

// containsMediaLine checks if SDP contains a specific media type.
func containsMediaLine(sdp, mediaType string) bool {
	// Simple check for "m=video" or "m=audio" line
	target := "m=" + mediaType
	for i := 0; i <= len(sdp)-len(target); i++ {
		if sdp[i:i+len(target)] == target {
			return true
		}
	}
	return false
}

// MustContain fails the test if the SDP doesn't contain the expected media line.
func MustContain(t *testing.T, sdp, mediaType string) {
	t.Helper()
	if !containsMediaLine(sdp, mediaType) {
		t.Errorf("SDP should contain %s m-line", mediaType)
	}
}

// LogSDPStats logs SDP statistics for debugging.
func LogSDPStats(t *testing.T, name, sdp string) {
	t.Helper()
	t.Logf("%s SDP: %d bytes", name, len(sdp))
	t.Logf("  Has video: %v", containsMediaLine(sdp, "video"))
	t.Logf("  Has audio: %v", containsMediaLine(sdp, "audio"))
	t.Logf("  Has application (DC): %v", containsMediaLine(sdp, "application"))
}
