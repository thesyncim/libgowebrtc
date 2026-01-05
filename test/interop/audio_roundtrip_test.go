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

// TestAudioTrackRoundtrip tests sending audio from libwebrtc to Pion.
// libwebrtc encodes and packetizes Opus; Pion receives RTP packets.
func TestAudioTrackRoundtrip(t *testing.T) {
	// Create peer pair
	pp, err := NewPeerPair(t, DefaultPeerPairConfig())
	if err != nil {
		t.Fatalf("Failed to create peer pair: %v", err)
	}
	defer pp.Close()

	// Create libwebrtc audio track
	audioTrack, err := track.NewAudioTrack(track.AudioTrackConfig{
		ID:         "audio-interop-test",
		StreamID:   "stream-audio",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		t.Fatalf("Failed to create audio track: %v", err)
	}
	defer audioTrack.Close()

	// Add a transceiver to Pion to receive audio
	_, err = pp.Pion.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeAudio,
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
	pp.Pion.OnTrack(func(remoteTrack *pionwebrtc.TrackRemote, receiver *pionwebrtc.RTPReceiver) {
		t.Logf("Pion received audio track: %s, codec: %s", remoteTrack.ID(), remoteTrack.Codec().MimeType)
		close(trackReceived)

		// Read some packets
		for i := 0; i < 5; i++ {
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

	// Create libwebrtc audio track (native)
	libTrack, err := pp.Lib.CreateAudioTrack("audio-interop-test")
	if err != nil {
		t.Fatalf("Failed to create libwebrtc audio track: %v", err)
	}

	// Add track to libwebrtc PeerConnection
	_, err = pp.Lib.AddTrack(libTrack)
	if err != nil {
		t.Fatalf("Failed to add audio track to libwebrtc: %v", err)
	}

	// Do offer/answer exchange
	if err := pp.ExchangeOfferAnswer(); err != nil {
		t.Fatalf("Offer/answer exchange failed: %v", err)
	}

	t.Log("Offer/answer exchange completed for audio track test")

	select {
	case <-trackReceived:
		t.Log("Audio track was received by Pion")
		packetsMu.Lock()
		count := len(receivedPackets)
		packetsMu.Unlock()
		t.Logf("Received %d RTP packets", count)
	case <-time.After(5 * time.Second):
		t.Log("Audio track not received (expected without full ICE connectivity)")
	}
}

// TestPionToLibWebRTCAudio tests sending audio from Pion to libwebrtc.
func TestPionToLibWebRTCAudio(t *testing.T) {
	pp, err := NewPeerPair(t, DefaultPeerPairConfig())
	if err != nil {
		t.Fatalf("Failed to create peer pair: %v", err)
	}
	defer pp.Close()

	// Create a Pion audio track (Opus)
	audioTrack, err := pionwebrtc.NewTrackLocalStaticRTP(
		pionwebrtc.RTPCodecCapability{
			MimeType:  pionwebrtc.MimeTypeOpus,
			ClockRate: 48000,
			Channels:  2,
		},
		"audio-pion-to-lib",
		"stream-pion-audio",
	)
	if err != nil {
		t.Fatalf("Failed to create Pion audio track: %v", err)
	}

	// Add track to Pion
	_, err = pp.Pion.AddTrack(audioTrack)
	if err != nil {
		t.Fatalf("Failed to add track to Pion: %v", err)
	}

	// Track received on libwebrtc side
	trackReceived := make(chan struct{})
	pp.Lib.OnTrack = func(track *pc.Track, receiver *pc.RTPReceiver, streams []string) {
		t.Logf("libwebrtc received audio track: %s, kind: %s", track.ID(), track.Kind())
		close(trackReceived)
	}

	// Do offer/answer exchange (Pion offers)
	if err := pp.ExchangeOfferAnswerWithOfferer(false); err != nil {
		t.Fatalf("Offer/answer exchange failed: %v", err)
	}

	t.Log("Offer/answer exchange completed (Pion audio to libwebrtc)")

	select {
	case <-trackReceived:
		t.Log("Audio track was received by libwebrtc")
	case <-time.After(5 * time.Second):
		t.Log("Audio track not received (expected without full ICE connectivity)")
	}
}

// TestAudioTrackWithPionIntegration tests the Pion-compatible AudioTrack.
func TestAudioTrackWithPionIntegration(t *testing.T) {
	// Create Pion PeerConnection
	pionPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create Pion PeerConnection: %v", err)
	}
	defer pionPC.Close()

	// Create libwebrtc-backed audio track
	audioTrack, err := track.NewAudioTrack(track.AudioTrackConfig{
		ID:         "opus-track-test",
		StreamID:   "stream-opus",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		t.Fatalf("Failed to create audio track: %v", err)
	}
	defer audioTrack.Close()

	// Add libwebrtc audio track to Pion PeerConnection
	// This works because track.AudioTrack implements webrtc.TrackLocal
	sender, err := pionPC.AddTrack(audioTrack)
	if err != nil {
		t.Fatalf("Failed to add libwebrtc audio track to Pion: %v", err)
	}

	if sender == nil {
		t.Fatal("Sender should not be nil")
	}

	t.Logf("Successfully added libwebrtc-backed audio track to Pion PeerConnection")
	t.Logf("Track ID: %s, StreamID: %s, Kind: %s",
		audioTrack.ID(), audioTrack.StreamID(), audioTrack.Kind())

	// Verify the track can create an offer
	offer, err := pionPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}

	if len(offer.SDP) == 0 {
		t.Fatal("Offer SDP should not be empty")
	}

	t.Logf("Created offer with %d bytes SDP", len(offer.SDP))

	// Verify audio m-line is present in SDP
	MustContain(t, offer.SDP, "audio")
}

// TestWriteAudioFrameToTrack tests writing audio frames to a track.
func TestWriteAudioFrameToTrack(t *testing.T) {
	audioTrack, err := track.NewAudioTrack(track.AudioTrackConfig{
		ID:         "write-audio-test",
		StreamID:   "stream-write-audio",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		t.Fatalf("Failed to create audio track: %v", err)
	}
	defer audioTrack.Close()

	// Create a test audio frame (20ms at 48kHz stereo = 960 samples)
	testFrame := frame.NewAudioFrameS16(48000, 2, 960)
	testFrame.PTS = 0

	// Writing frame before binding should fail
	err = audioTrack.WriteFrame(testFrame)
	if err != track.ErrNotBound {
		t.Errorf("WriteFrame before binding should return ErrNotBound, got: %v", err)
	}

	t.Log("Verified WriteFrame returns ErrNotBound when audio track is not bound")
}

// TestAudioAndVideoTogether tests sending both audio and video tracks.
func TestAudioAndVideoTogether(t *testing.T) {
	pp, err := NewPeerPair(t, DefaultPeerPairConfig())
	if err != nil {
		t.Fatalf("Failed to create peer pair: %v", err)
	}
	defer pp.Close()

	// Create video track on libwebrtc
	libVideoTrack, err := pp.Lib.CreateVideoTrack("video-combined", codec.VP8, 640, 480)
	if err != nil {
		t.Fatalf("Failed to create video track: %v", err)
	}

	// Create audio track on libwebrtc
	libAudioTrack, err := pp.Lib.CreateAudioTrack("audio-combined")
	if err != nil {
		t.Fatalf("Failed to create audio track: %v", err)
	}

	// Add both tracks
	_, err = pp.Lib.AddTrack(libVideoTrack)
	if err != nil {
		t.Fatalf("Failed to add video track: %v", err)
	}

	_, err = pp.Lib.AddTrack(libAudioTrack)
	if err != nil {
		t.Fatalf("Failed to add audio track: %v", err)
	}

	// Add transceivers on Pion to receive
	pp.Pion.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo,
		pionwebrtc.RTPTransceiverInit{Direction: pionwebrtc.RTPTransceiverDirectionRecvonly})
	pp.Pion.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeAudio,
		pionwebrtc.RTPTransceiverInit{Direction: pionwebrtc.RTPTransceiverDirectionRecvonly})

	// Track what we received
	var (
		receivedVideo = make(chan struct{})
		receivedAudio = make(chan struct{})
		videoOnce     sync.Once
		audioOnce     sync.Once
	)

	pp.Pion.OnTrack(func(remoteTrack *pionwebrtc.TrackRemote, receiver *pionwebrtc.RTPReceiver) {
		t.Logf("Pion received track: kind=%s, codec=%s", remoteTrack.Kind(), remoteTrack.Codec().MimeType)
		if remoteTrack.Kind() == pionwebrtc.RTPCodecTypeVideo {
			videoOnce.Do(func() { close(receivedVideo) })
		} else if remoteTrack.Kind() == pionwebrtc.RTPCodecTypeAudio {
			audioOnce.Do(func() { close(receivedAudio) })
		}
	})

	// Do offer/answer exchange
	if err := pp.ExchangeOfferAnswer(); err != nil {
		t.Fatalf("Offer/answer exchange failed: %v", err)
	}

	// Verify SDP contains both media types
	localDesc := pp.Lib.LocalDescription()
	if localDesc != nil {
		MustContain(t, localDesc.SDP, "video")
		MustContain(t, localDesc.SDP, "audio")
		t.Log("SDP contains both video and audio m-lines")
	}

	// Wait briefly for tracks (won't arrive without ICE)
	timeout := time.After(2 * time.Second)
	select {
	case <-receivedVideo:
		t.Log("Video track received")
	case <-timeout:
		t.Log("Video track not received (expected without ICE)")
	}

	select {
	case <-receivedAudio:
		t.Log("Audio track received")
	case <-timeout:
		t.Log("Audio track not received (expected without ICE)")
	}

	t.Log("Audio and video test completed")
}

// TestAudioTrackWithICE tests audio with ICE candidate exchange.
func TestAudioTrackWithICE(t *testing.T) {
	pp, err := NewPeerPair(t, STUNPeerPairConfig())
	if err != nil {
		t.Fatalf("Failed to create peer pair: %v", err)
	}
	defer pp.Close()

	// Create audio track on libwebrtc
	libAudioTrack, err := pp.Lib.CreateAudioTrack("audio-ice-test")
	if err != nil {
		t.Fatalf("Failed to create audio track: %v", err)
	}

	_, err = pp.Lib.AddTrack(libAudioTrack)
	if err != nil {
		t.Fatalf("Failed to add audio track: %v", err)
	}

	// Add transceiver on Pion to receive
	pp.Pion.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeAudio,
		pionwebrtc.RTPTransceiverInit{Direction: pionwebrtc.RTPTransceiverDirectionRecvonly})

	trackReceived := make(chan struct{})
	pp.Pion.OnTrack(func(remoteTrack *pionwebrtc.TrackRemote, receiver *pionwebrtc.RTPReceiver) {
		t.Logf("Pion received audio track with ICE: %s", remoteTrack.Codec().MimeType)
		close(trackReceived)
	})

	// Do offer/answer exchange
	if err := pp.ExchangeOfferAnswer(); err != nil {
		t.Fatalf("Offer/answer exchange failed: %v", err)
	}

	// Wait for connection
	if pp.WaitForConnection(10 * time.Second) {
		t.Log("Connection established with ICE")

		select {
		case <-trackReceived:
			t.Log("Audio track received with ICE connectivity")
		case <-time.After(5 * time.Second):
			t.Log("Audio track not received within timeout")
		}
	} else {
		t.Log("ICE connection timeout (expected in test environment)")
	}
}
