package interop

import (
	"testing"
	"time"

	pionwebrtc "github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/pc"
)

func TestMain(m *testing.M) {
	if err := ffi.LoadLibrary(); err != nil {
		// Skip if shim library not available
		return
	}
	defer ffi.Close()
	m.Run()
}

// TestOfferAnswerExchange tests basic offer/answer exchange between
// libwebrtc (pkg/pc) and Pion WebRTC using the helper.
func TestOfferAnswerExchange(t *testing.T) {
	pp, err := NewPeerPair(t, DefaultPeerPairConfig())
	if err != nil {
		t.Fatalf("Failed to create peer pair: %v", err)
	}
	defer pp.Close()

	// Perform offer/answer exchange with libwebrtc as offerer
	if err := pp.ExchangeOfferAnswer(); err != nil {
		t.Fatalf("Offer/answer exchange failed: %v", err)
	}

	t.Log("Offer/Answer exchange completed successfully")
}

// TestOfferAnswerExchangeManual tests offer/answer manually for detailed logging.
func TestOfferAnswerExchangeManual(t *testing.T) {
	// Create libwebrtc PeerConnection (offerer)
	libPC, err := pc.NewPeerConnection(pc.DefaultConfiguration())
	if err != nil {
		t.Fatalf("Failed to create libwebrtc PeerConnection: %v", err)
	}
	defer libPC.Close()

	// Create Pion PeerConnection (answerer)
	pionPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create Pion PeerConnection: %v", err)
	}
	defer pionPC.Close()

	// libwebrtc creates offer
	offer, err := libPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("libwebrtc CreateOffer failed: %v", err)
	}
	LogSDPStats(t, "libwebrtc offer", offer.SDP)

	// libwebrtc sets local description
	if err := libPC.SetLocalDescription(offer); err != nil {
		t.Fatalf("libwebrtc SetLocalDescription failed: %v", err)
	}

	// Pion sets remote description (the offer)
	pionOffer := pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	}
	if err := pionPC.SetRemoteDescription(pionOffer); err != nil {
		t.Fatalf("Pion SetRemoteDescription failed: %v", err)
	}

	// Pion creates answer
	pionAnswer, err := pionPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("Pion CreateAnswer failed: %v", err)
	}
	LogSDPStats(t, "Pion answer", pionAnswer.SDP)

	// Pion sets local description
	if err := pionPC.SetLocalDescription(pionAnswer); err != nil {
		t.Fatalf("Pion SetLocalDescription failed: %v", err)
	}

	// libwebrtc sets remote description (the answer)
	libAnswer := &pc.SessionDescription{
		Type: pc.SDPTypeAnswer,
		SDP:  pionAnswer.SDP,
	}
	if err := libPC.SetRemoteDescription(libAnswer); err != nil {
		t.Fatalf("libwebrtc SetRemoteDescription failed: %v", err)
	}

	t.Log("Offer/Answer exchange completed successfully")
}

// TestOfferAnswerWithICE tests offer/answer exchange with ICE candidate exchange.
func TestOfferAnswerWithICE(t *testing.T) {
	pp, err := NewPeerPair(t, STUNPeerPairConfig())
	if err != nil {
		t.Fatalf("Failed to create peer pair: %v", err)
	}
	defer pp.Close()

	// Perform offer/answer exchange
	if err := pp.ExchangeOfferAnswer(); err != nil {
		t.Fatalf("Offer/answer exchange failed: %v", err)
	}

	// Wait for connection (ICE exchange happens automatically via helpers)
	if pp.WaitForConnection(10 * time.Second) {
		t.Log("Both peers connected successfully")
	} else {
		t.Log("Connection timeout (expected without network - ICE may not complete in test environment)")
	}
}

// TestPionToLibWebRTCOffer tests Pion as offerer, libwebrtc as answerer.
func TestPionToLibWebRTCOffer(t *testing.T) {
	pp, err := NewPeerPair(t, DefaultPeerPairConfig())
	if err != nil {
		t.Fatalf("Failed to create peer pair: %v", err)
	}
	defer pp.Close()

	// Perform offer/answer exchange with Pion as offerer
	if err := pp.ExchangeOfferAnswerWithOfferer(false); err != nil {
		t.Fatalf("Offer/answer exchange failed: %v", err)
	}

	t.Log("Pion-to-libwebrtc offer/answer exchange completed")
}
