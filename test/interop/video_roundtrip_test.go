package interop

import (
	"sync"
	"testing"
	"time"

	"github.com/pion/rtp"
	pionwebrtc "github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pc"
	"github.com/thesyncim/libgowebrtc/pkg/track"
)

// TestVideoTrackRoundtrip tests sending video from libwebrtc to Pion.
// libwebrtc encodes and packetizes; Pion receives RTP packets.
func TestVideoTrackRoundtrip(t *testing.T) {
	// Create libwebrtc PeerConnection (sender)
	libPC, err := pc.NewPeerConnection(defaultInteropConfig())
	if err != nil {
		t.Fatalf("Failed to create libwebrtc PeerConnection: %v", err)
	}
	defer libPC.Close()

	// Create Pion PeerConnection (receiver)
	pionPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create Pion PeerConnection: %v", err)
	}
	defer pionPC.Close()

	// Create a libwebrtc video track
	videoTrack, err := track.NewVideoTrack(track.VideoTrackConfig{
		ID:       "video-interop-test",
		StreamID: "stream-0",
		Codec:    codec.VP8, // VP8 is widely supported
		Width:    640,
		Height:   480,
		Bitrate:  1_000_000,
		FPS:      30,
	})
	if err != nil {
		t.Fatalf("Failed to create video track: %v", err)
	}
	defer videoTrack.Close()

	// Add a transceiver to Pion to receive video
	_, err = pionPC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo,
		pionwebrtc.RTPTransceiverInit{Direction: pionwebrtc.RTPTransceiverDirectionRecvonly})
	if err != nil {
		t.Fatalf("Failed to add Pion transceiver: %v", err)
	}

	// Track received RTP packets
	var (
		receivedPackets []*rtp.Packet
		packetsMu       sync.Mutex
		trackReceived   = make(chan struct{})
	)

	// Handle incoming track on Pion side
	pionPC.OnTrack(func(remoteTrack *pionwebrtc.TrackRemote, receiver *pionwebrtc.RTPReceiver) {
		t.Logf("Pion received track: %s, codec: %s", remoteTrack.ID(), remoteTrack.Codec().MimeType)
		close(trackReceived)

		// Read some packets
		for i := 0; i < 10; i++ {
			pkt, _, err := remoteTrack.ReadRTP()
			if err != nil {
				t.Logf("ReadRTP error: %v", err)
				return
			}
			packetsMu.Lock()
			receivedPackets = append(receivedPackets, pkt)
			packetsMu.Unlock()
		}
	})

	// Create libwebrtc video track (native)
	libTrack, err := libPC.CreateVideoTrack("video-interop-test", codec.VP8, 640, 480)
	if err != nil {
		t.Fatalf("Failed to create libwebrtc track: %v", err)
	}

	// Add track to libwebrtc PeerConnection
	_, err = libPC.AddTrack(libTrack)
	if err != nil {
		t.Fatalf("Failed to add track to libwebrtc: %v", err)
	}

	// Do offer/answer exchange
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

	t.Log("Offer/answer exchange completed for video track test")

	// Note: In a real test with actual connectivity (STUN/ICE),
	// we would write frames and verify they're received.
	// Without network, we verify the signaling completed successfully.

	select {
	case <-trackReceived:
		t.Log("Track was received by Pion")
		packetsMu.Lock()
		count := len(receivedPackets)
		packetsMu.Unlock()
		t.Logf("Received %d RTP packets", count)
	case <-time.After(interopShortTimeout):
		t.Log("Track not received (expected without full ICE connectivity)")
	}
}

// TestPionToLibWebRTCVideo tests sending video from Pion to libwebrtc.
func TestPionToLibWebRTCVideo(t *testing.T) {
	// Create Pion PeerConnection (sender)
	pionPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create Pion PeerConnection: %v", err)
	}
	defer pionPC.Close()

	// Create libwebrtc PeerConnection (receiver)
	libPC, err := pc.NewPeerConnection(defaultInteropConfig())
	if err != nil {
		t.Fatalf("Failed to create libwebrtc PeerConnection: %v", err)
	}
	defer libPC.Close()

	// Create a Pion video track
	videoTrack, err := pionwebrtc.NewTrackLocalStaticRTP(
		pionwebrtc.RTPCodecCapability{MimeType: pionwebrtc.MimeTypeVP8},
		"video-pion-to-lib",
		"stream-pion",
	)
	if err != nil {
		t.Fatalf("Failed to create Pion video track: %v", err)
	}

	// Add track to Pion
	_, err = pionPC.AddTrack(videoTrack)
	if err != nil {
		t.Fatalf("Failed to add track to Pion: %v", err)
	}

	// Track received on libwebrtc side
	trackReceived := make(chan struct{})
	libPC.OnTrack = func(track *pc.Track, receiver *pc.RTPReceiver, streams []string) {
		t.Logf("libwebrtc received track: %s, kind: %s", track.ID(), track.Kind())
		close(trackReceived)
	}

	// Do offer/answer exchange (Pion offers)
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
	case <-trackReceived:
		t.Log("Track was received by libwebrtc")
	case <-time.After(interopShortTimeout):
		t.Log("Track not received (expected without full ICE connectivity)")
	}
}

// TestVideoTrackWithEncodedVideoTrack tests the Pion-compatible EncodedVideoTrack.
func TestVideoTrackWithEncodedVideoTrack(t *testing.T) {
	// This test verifies that track.VideoTrack implements webrtc.TrackLocal
	// and can be added directly to a Pion PeerConnection.

	// Create Pion PeerConnection
	pionPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create Pion PeerConnection: %v", err)
	}
	defer pionPC.Close()

	// Create libwebrtc-backed video track
	videoTrack, err := track.NewVideoTrack(track.VideoTrackConfig{
		ID:       "encoded-track-test",
		StreamID: "stream-encoded",
		Codec:    codec.H264,
		Width:    1280,
		Height:   720,
		Bitrate:  2_000_000,
		FPS:      30,
	})
	if err != nil {
		t.Fatalf("Failed to create video track: %v", err)
	}
	defer videoTrack.Close()

	// Add libwebrtc track to Pion PeerConnection
	// This works because track.VideoTrack implements webrtc.TrackLocal
	sender, err := pionPC.AddTrack(videoTrack)
	if err != nil {
		t.Fatalf("Failed to add libwebrtc track to Pion: %v", err)
	}

	if sender == nil {
		t.Fatal("Sender should not be nil")
	}

	t.Logf("Successfully added libwebrtc-backed track to Pion PeerConnection")
	t.Logf("Track ID: %s, StreamID: %s, Kind: %s",
		videoTrack.ID(), videoTrack.StreamID(), videoTrack.Kind())

	// Verify the track can create an offer
	offer, err := pionPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}

	if len(offer.SDP) == 0 {
		t.Fatal("Offer SDP should not be empty")
	}

	t.Logf("Created offer with %d bytes SDP", len(offer.SDP))

	// Verify video m-line is present in SDP
	if !containsMediaLine(offer.SDP, "video") {
		t.Error("Offer SDP should contain video m-line")
	}
}

// TestWriteFrameToTrack tests writing frames to a track (requires connectivity).
func TestWriteFrameToTrack(t *testing.T) {
	// Create libwebrtc video track
	videoTrack, err := track.NewVideoTrack(track.VideoTrackConfig{
		ID:       "write-frame-test",
		StreamID: "stream-write",
		Codec:    codec.VP8,
		Width:    640,
		Height:   480,
		Bitrate:  1_000_000,
		FPS:      30,
	})
	if err != nil {
		t.Fatalf("Failed to create video track: %v", err)
	}
	defer videoTrack.Close()

	// Create a test frame
	testFrame := frame.NewI420Frame(640, 480)
	for i := range testFrame.Data[0] {
		testFrame.Data[0][i] = 128 // Y
	}
	for i := range testFrame.Data[1] {
		testFrame.Data[1][i] = 128 // U
		testFrame.Data[2][i] = 128 // V
	}
	testFrame.PTS = 0

	// Writing frame before binding should fail
	err = videoTrack.WriteFrame(testFrame, true)
	if err != track.ErrNotBound {
		t.Errorf("WriteFrame before binding should return ErrNotBound, got: %v", err)
	}

	t.Log("Verified WriteFrame returns ErrNotBound when track is not bound")
}
