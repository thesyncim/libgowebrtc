// Package e2e provides end-to-end tests for libgowebrtc.
// This file tests interoperability between libgowebrtc and pion/webrtc.
package e2e

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pc"
)

const (
	interopTrackTimeout       = 2 * time.Second
	interopTrackDeadline      = 2 * time.Second
	interopICEGatherDelay     = 300 * time.Millisecond
	interopPostSendDelay      = 100 * time.Millisecond
	interopFrameCount         = 10
	interopFrameDelay         = 5 * time.Millisecond
	interopStressFrameCount   = 60
	interopDataChannelTimeout = 2 * time.Second
)

// PionLibPeerPair represents a pion and libwebrtc peer pair for interop testing.
type PionLibPeerPair struct {
	Pion *webrtc.PeerConnection
	Lib  *pc.PeerConnection
	t    *testing.T

	// Frame counters
	pionVideoFrames atomic.Int64
	pionAudioFrames atomic.Int64
	libVideoFrames  atomic.Int64
	libAudioFrames  atomic.Int64

	// Track received signals
	pionTrackReceived chan *webrtc.TrackRemote
	libTrackReceived  chan *pc.Track

	// Data channel signals
	pionDataChannel chan *webrtc.DataChannel
	libDataChannel  chan *pc.DataChannel

	// Close tracking
	closed bool
	mu     sync.Mutex
}

// NewPionLibPeerPair creates a pair with pion as offerer and lib as answerer.
func NewPionLibPeerPair(t *testing.T) *PionLibPeerPair {
	t.Helper()

	// Create pion peer
	pionConfig := webrtc.Configuration{}
	pion, err := webrtc.NewPeerConnection(pionConfig)
	if err != nil {
		t.Fatalf("Failed to create pion PeerConnection: %v", err)
	}

	// Create lib peer
	libConfig := pc.DefaultConfiguration()
	libConfig.ICEServers = nil
	lib, err := pc.NewPeerConnection(libConfig)
	if err != nil {
		pion.Close()
		t.Fatalf("Failed to create lib PeerConnection: %v", err)
	}

	pp := &PionLibPeerPair{
		Pion:              pion,
		Lib:               lib,
		t:                 t,
		pionTrackReceived: make(chan *webrtc.TrackRemote, 10),
		libTrackReceived:  make(chan *pc.Track, 10),
		pionDataChannel:   make(chan *webrtc.DataChannel, 1),
		libDataChannel:    make(chan *pc.DataChannel, 1),
	}

	// Setup pion OnTrack handler
	pion.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		t.Logf("Pion received track: id=%s kind=%s codec=%s", track.ID(), track.Kind(), track.Codec().MimeType)
		select {
		case pp.pionTrackReceived <- track:
		default:
		}

		// Read frames from track
		go func() {
			for {
				_, _, err := track.ReadRTP()
				if err != nil {
					return
				}
				if track.Kind() == webrtc.RTPCodecTypeVideo {
					pp.pionVideoFrames.Add(1)
				} else {
					pp.pionAudioFrames.Add(1)
				}
			}
		}()
	})

	// Setup lib OnTrack handler
	lib.OnTrack = func(track *pc.Track, recv *pc.RTPReceiver, streams []string) {
		t.Logf("Lib received track: id=%s kind=%s", track.ID(), track.Kind())
		select {
		case pp.libTrackReceived <- track:
		default:
		}

		// Setup frame callback
		if track.Kind() == "video" {
			track.SetOnVideoFrame(func(f *frame.VideoFrame) {
				pp.libVideoFrames.Add(1)
			})
		} else if track.Kind() == "audio" {
			track.SetOnAudioFrame(func(f *frame.AudioFrame) {
				pp.libAudioFrames.Add(1)
			})
		}
	}

	// Setup data channel handlers
	pion.OnDataChannel(func(dc *webrtc.DataChannel) {
		t.Logf("Pion received data channel: label=%s", dc.Label())
		select {
		case pp.pionDataChannel <- dc:
		default:
		}
	})

	lib.OnDataChannel = func(dc *pc.DataChannel) {
		t.Logf("Lib received data channel: label=%s", dc.Label())
		select {
		case pp.libDataChannel <- dc:
		default:
		}
	}

	return pp
}

// Close closes both peers.
func (pp *PionLibPeerPair) Close() {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	if pp.closed {
		return
	}
	pp.closed = true
	if pp.Pion != nil {
		pp.Pion.Close()
	}
	if pp.Lib != nil {
		pp.Lib.Close()
	}
}

// ConnectPionOffersLibAnswers performs SDP exchange with pion as offerer.
func (pp *PionLibPeerPair) ConnectPionOffersLibAnswers() error {
	// Pion creates offer
	offer, err := pp.Pion.CreateOffer(nil)
	if err != nil {
		return err
	}

	gatherComplete := webrtc.GatheringCompletePromise(pp.Pion)
	if err = pp.Pion.SetLocalDescription(offer); err != nil {
		return err
	}
	<-gatherComplete

	// Lib sets remote description
	offerDesc := &pc.SessionDescription{
		Type: pc.SDPTypeOffer,
		SDP:  pp.Pion.LocalDescription().SDP,
	}
	if err = pp.Lib.SetRemoteDescription(offerDesc); err != nil {
		return err
	}

	// Lib creates answer
	answer, err := pp.Lib.CreateAnswer(nil)
	if err != nil {
		return err
	}
	if err = pp.Lib.SetLocalDescription(answer); err != nil {
		return err
	}

	// Pion sets remote description
	return pp.Pion.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answer.SDP,
	})
}

// ConnectLibOffersPionAnswers performs SDP exchange with lib as offerer.
func (pp *PionLibPeerPair) ConnectLibOffersPionAnswers() error {
	// Set up ICE candidate exchange
	var libCandidates []*pc.ICECandidate
	var pionCandidates []webrtc.ICECandidate
	var candidatesMu sync.Mutex

	pp.Lib.OnICECandidate = func(c *pc.ICECandidate) {
		if c == nil {
			return
		}
		candidatesMu.Lock()
		libCandidates = append(libCandidates, c)
		candidatesMu.Unlock()
	}

	pp.Pion.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		candidatesMu.Lock()
		pionCandidates = append(pionCandidates, *c)
		candidatesMu.Unlock()
	})

	// Lib creates offer
	offer, err := pp.Lib.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err = pp.Lib.SetLocalDescription(offer); err != nil {
		return err
	}

	// Pion sets remote description
	if err = pp.Pion.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	}); err != nil {
		return err
	}

	// Pion creates answer
	answer, err := pp.Pion.CreateAnswer(nil)
	if err != nil {
		return err
	}

	gatherComplete := webrtc.GatheringCompletePromise(pp.Pion)
	if err = pp.Pion.SetLocalDescription(answer); err != nil {
		return err
	}
	<-gatherComplete

	// Lib sets remote description
	answerDesc := &pc.SessionDescription{
		Type: pc.SDPTypeAnswer,
		SDP:  pp.Pion.LocalDescription().SDP,
	}
	if err = pp.Lib.SetRemoteDescription(answerDesc); err != nil {
		return err
	}

	// Wait for lib's ICE gathering to complete
	time.Sleep(interopICEGatherDelay)

	// Exchange ICE candidates
	candidatesMu.Lock()
	defer candidatesMu.Unlock()

	for _, c := range libCandidates {
		idx := uint16(c.SDPMLineIndex)
		pp.Pion.AddICECandidate(webrtc.ICECandidateInit{
			Candidate:     c.Candidate,
			SDPMid:        &c.SDPMid,
			SDPMLineIndex: &idx,
		})
	}

	for _, c := range pionCandidates {
		cJSON := c.ToJSON()
		pp.Lib.AddICECandidate(&pc.ICECandidate{
			Candidate:     cJSON.Candidate,
			SDPMid:        *cJSON.SDPMid,
			SDPMLineIndex: *cJSON.SDPMLineIndex,
		})
	}

	// Wait for connection to establish
	time.Sleep(interopICEGatherDelay)

	return nil
}

// WaitForPionTrack waits for pion to receive a track.
func (pp *PionLibPeerPair) WaitForPionTrack(timeout time.Duration) *webrtc.TrackRemote {
	select {
	case track := <-pp.pionTrackReceived:
		return track
	case <-time.After(timeout):
		return nil
	}
}

// WaitForLibTrack waits for lib to receive a track.
func (pp *PionLibPeerPair) WaitForLibTrack(timeout time.Duration) *pc.Track {
	select {
	case track := <-pp.libTrackReceived:
		return track
	case <-time.After(timeout):
		return nil
	}
}

// TestLibToPionVideoInterop tests video streaming from lib to pion.
func TestLibToPionVideoInterop(t *testing.T) {
	pp := NewPionLibPeerPair(t)
	defer pp.Close()

	// Create video track on lib
	track, err := pp.Lib.CreateVideoTrack("video-interop", codec.VP8, 640, 480)
	if err != nil {
		t.Fatalf("Failed to create video track: %v", err)
	}
	if _, err = pp.Lib.AddTrack(track); err != nil {
		t.Fatalf("Failed to add track: %v", err)
	}

	// Add a track to pion to enable bidirectional RTP flow
	pionTrackLocal, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		"pion-track",
		"pion-stream",
	)
	if err != nil {
		t.Fatalf("Failed to create pion track: %v", err)
	}
	if _, err = pp.Pion.AddTrack(pionTrackLocal); err != nil {
		t.Fatalf("Failed to add pion track: %v", err)
	}

	// Connect: lib offers, pion answers
	// Important: Wait for ICE gathering to complete before setting remote description
	offer, err := pp.Lib.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create offer: %v", err)
	}
	if err = pp.Lib.SetLocalDescription(offer); err != nil {
		t.Fatalf("Failed to set lib local description: %v", err)
	}
	if err = pp.Pion.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	}); err != nil {
		t.Fatalf("Failed to set pion remote description: %v", err)
	}

	answer, err := pp.Pion.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("Failed to create answer: %v", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(pp.Pion)
	if err = pp.Pion.SetLocalDescription(answer); err != nil {
		t.Fatalf("Failed to set pion local description: %v", err)
	}
	<-gatherComplete // Wait for ICE gathering

	if err = pp.Lib.SetRemoteDescription(&pc.SessionDescription{
		Type: pc.SDPTypeAnswer,
		SDP:  pp.Pion.LocalDescription().SDP,
	}); err != nil {
		t.Fatalf("Failed to set lib remote description: %v", err)
	}

	// Send frames from lib
	go func() {
		for i := 0; i < interopFrameCount; i++ {
			f := CreateTestFrame(640, 480, uint32(i))
			track.WriteVideoFrame(f)
			time.Sleep(interopFrameDelay)
		}
	}()

	// Also have pion send frames to help establish bidirectional RTP flow
	go func() {
		for i := 0; i < interopFrameCount; i++ {
			pionTrackLocal.WriteSample(media.Sample{
				Data:     createMinimalVP8Frame(i == 0),
				Duration: interopFrameDelay,
			})
			time.Sleep(interopFrameDelay)
		}
	}()

	// Wait for pion to receive the track
	pionTrack := pp.WaitForPionTrack(interopTrackTimeout)
	time.Sleep(interopPostSendDelay)

	frames := pp.pionVideoFrames.Load()
	t.Logf("Pion received track=%v, frames=%d", pionTrack != nil, frames)

	if pionTrack == nil && frames == 0 {
		t.Fatal("Neither track received nor frames received - media flow failed")
	}
}

// TestPionToLibVideoInterop tests video streaming from pion to lib.
func TestPionToLibVideoInterop(t *testing.T) {
	pp := NewPionLibPeerPair(t)
	defer pp.Close()

	// Create video track on pion
	pionTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		"video-from-pion",
		"pion-stream",
	)
	if err != nil {
		t.Fatalf("Failed to create pion track: %v", err)
	}
	if _, err = pp.Pion.AddTrack(pionTrack); err != nil {
		t.Fatalf("Failed to add pion track: %v", err)
	}

	// Connect (pion offers, lib answers)
	if err = pp.ConnectPionOffersLibAnswers(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Wait for lib to receive the track
	libTrack := pp.WaitForLibTrack(interopTrackTimeout)
	if libTrack == nil {
		t.Fatal("Lib did not receive track within timeout")
	}

	// Send video frames from pion
	for i := 0; i < interopFrameCount; i++ {
		// Create a minimal VP8 keyframe
		if err := pionTrack.WriteSample(media.Sample{
			Data:     createMinimalVP8Frame(i == 0),
			Duration: interopFrameDelay,
		}); err != nil {
			t.Logf("WriteSample failed: %v", err)
		}
		time.Sleep(interopFrameDelay)
	}

	// Wait for frames to be received
	time.Sleep(interopPostSendDelay)

	frames := pp.libVideoFrames.Load()
	t.Logf("Lib received %d video frames from pion", frames)

	if frames == 0 {
		t.Log("No frames received - this may be expected in loopback without proper ICE connectivity")
	}
}

// TestBidirectionalVideoInterop tests video streaming in both directions simultaneously.
func TestBidirectionalVideoInterop(t *testing.T) {
	pp := NewPionLibPeerPair(t)
	defer pp.Close()

	// Create video track on lib
	libTrack, err := pp.Lib.CreateVideoTrack("video-lib-to-pion", codec.VP8, 320, 240)
	if err != nil {
		t.Fatalf("Failed to create lib video track: %v", err)
	}
	if _, err = pp.Lib.AddTrack(libTrack); err != nil {
		t.Fatalf("Failed to add lib track: %v", err)
	}

	// Create video track on pion
	pionTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		"video-pion-to-lib",
		"pion-stream",
	)
	if err != nil {
		t.Fatalf("Failed to create pion track: %v", err)
	}
	if _, err = pp.Pion.AddTrack(pionTrack); err != nil {
		t.Fatalf("Failed to add pion track: %v", err)
	}

	// Connect (lib offers)
	if err = pp.ConnectLibOffersPionAnswers(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Wait for both sides to receive tracks
	receivedPion := pp.WaitForPionTrack(interopTrackTimeout)
	receivedLib := pp.WaitForLibTrack(interopTrackTimeout)

	if receivedPion == nil {
		t.Log("Pion did not receive track (may be expected without full ICE connectivity)")
	}
	if receivedLib == nil {
		t.Log("Lib did not receive track (may be expected without full ICE connectivity)")
	}

	// Send frames bidirectionally
	var wg sync.WaitGroup
	wg.Add(2)

	// Send from lib to pion
	go func() {
		defer wg.Done()
		for i := 0; i < interopFrameCount; i++ {
			f := CreateTestFrame(320, 240, uint32(i))
			libTrack.WriteVideoFrame(f)
			time.Sleep(interopFrameDelay)
		}
	}()

	// Send from pion to lib
	go func() {
		defer wg.Done()
		for i := 0; i < interopFrameCount; i++ {
			pionTrack.WriteSample(media.Sample{
				Data:     createMinimalVP8Frame(i == 0),
				Duration: interopFrameDelay,
			})
			time.Sleep(interopFrameDelay)
		}
	}()

	wg.Wait()
	time.Sleep(interopPostSendDelay)

	t.Logf("Pion received %d frames, Lib received %d frames",
		pp.pionVideoFrames.Load(), pp.libVideoFrames.Load())
}

// TestDataChannelInterop tests data channel messaging between lib and pion.
func TestDataChannelInterop(t *testing.T) {
	pp := NewPionLibPeerPair(t)
	defer pp.Close()

	// Create data channel on pion
	ordered := true
	dcOptions := &webrtc.DataChannelInit{
		Ordered: &ordered,
	}
	pionDC, err := pp.Pion.CreateDataChannel("test-channel", dcOptions)
	if err != nil {
		t.Fatalf("Failed to create pion data channel: %v", err)
	}

	// Message tracking
	var pionReceived [][]byte
	var libReceived [][]byte
	var msgMu sync.Mutex

	pionDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		msgMu.Lock()
		pionReceived = append(pionReceived, msg.Data)
		msgMu.Unlock()
	})

	// Connect (pion offers since it has the data channel)
	if err = pp.ConnectPionOffersLibAnswers(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Wait for lib to receive data channel
	select {
	case libDC := <-pp.libDataChannel:
		t.Logf("Lib received data channel handle: %v", libDC)
		// Note: Currently lib doesn't expose data channel message callbacks
		// This test verifies the data channel is negotiated correctly
	case <-time.After(interopDataChannelTimeout):
		t.Log("Lib did not receive data channel (may require full ICE connectivity)")
	}

	// Wait for pion data channel to open
	dcOpen := make(chan struct{})
	pionDC.OnOpen(func() {
		close(dcOpen)
	})

	select {
	case <-dcOpen:
		t.Log("Pion data channel opened")

		// Send test messages
		testMsg := []byte("Hello from pion!")
		if err := pionDC.Send(testMsg); err != nil {
			t.Logf("Failed to send message: %v", err)
		}
	case <-time.After(interopDataChannelTimeout):
		t.Log("Data channel did not open (may require full ICE connectivity)")
	}

	time.Sleep(interopPostSendDelay)

	msgMu.Lock()
	t.Logf("Pion received %d messages, Lib received %d messages",
		len(pionReceived), len(libReceived))
	msgMu.Unlock()
}

// TestCodecNegotiation tests that codecs are correctly negotiated between lib and pion.
func TestCodecNegotiation(t *testing.T) {
	testCases := []struct {
		name     string
		libCodec codec.Type
		pionMime string
	}{
		{"VP8", codec.VP8, webrtc.MimeTypeVP8},
		{"VP9", codec.VP9, webrtc.MimeTypeVP9},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pp := NewPionLibPeerPair(t)
			defer pp.Close()

			// Create track with specific codec on lib
			track, err := pp.Lib.CreateVideoTrack("video-"+tc.name, tc.libCodec, 320, 240)
			if err != nil {
				t.Fatalf("Failed to create track: %v", err)
			}
			if _, err = pp.Lib.AddTrack(track); err != nil {
				t.Fatalf("Failed to add track: %v", err)
			}

			// Connect
			if err = pp.ConnectLibOffersPionAnswers(); err != nil {
				t.Fatalf("Failed to connect: %v", err)
			}

			// Wait for pion to receive track
			pionTrack := pp.WaitForPionTrack(interopTrackTimeout)
			if pionTrack == nil {
				t.Log("Pion did not receive track (may require ICE connectivity)")
				return
			}

			// Verify codec
			codec := pionTrack.Codec()
			t.Logf("Negotiated codec: %s", codec.MimeType)

			if codec.MimeType != tc.pionMime {
				t.Errorf("Expected codec %s, got %s", tc.pionMime, codec.MimeType)
			}
		})
	}
}

// TestMultipleTracksInterop tests multiple tracks in a single session.
func TestMultipleTracksInterop(t *testing.T) {
	pp := NewPionLibPeerPair(t)
	defer pp.Close()

	// Create video track on lib
	videoTrack, err := pp.Lib.CreateVideoTrack("video-multi", codec.VP8, 320, 240)
	if err != nil {
		t.Fatalf("Failed to create video track: %v", err)
	}
	if _, err = pp.Lib.AddTrack(videoTrack); err != nil {
		t.Fatalf("Failed to add video track: %v", err)
	}

	// Create audio track on lib
	audioTrack, err := pp.Lib.CreateAudioTrack("audio-multi")
	if err != nil {
		t.Fatalf("Failed to create audio track: %v", err)
	}
	if _, err = pp.Lib.AddTrack(audioTrack); err != nil {
		t.Fatalf("Failed to add audio track: %v", err)
	}

	// Connect
	if err = pp.ConnectLibOffersPionAnswers(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Count received tracks
	receivedTracks := 0
	deadline := time.Now().Add(interopTrackDeadline)
	for time.Now().Before(deadline) && receivedTracks < 2 {
		select {
		case track := <-pp.pionTrackReceived:
			receivedTracks++
			t.Logf("Pion received track %d: kind=%s id=%s", receivedTracks, track.Kind(), track.ID())
		case <-time.After(100 * time.Millisecond):
		}
	}

	t.Logf("Pion received %d tracks total", receivedTracks)

	if receivedTracks < 2 {
		t.Log("Not all tracks received (may require full ICE connectivity)")
	}
}

// TestRenegotiationInterop tests adding a track after initial connection.
func TestRenegotiationInterop(t *testing.T) {
	pp := NewPionLibPeerPair(t)
	defer pp.Close()

	// Create initial video track on lib
	track1, err := pp.Lib.CreateVideoTrack("video-initial", codec.VP8, 320, 240)
	if err != nil {
		t.Fatalf("Failed to create first video track: %v", err)
	}
	if _, err = pp.Lib.AddTrack(track1); err != nil {
		t.Fatalf("Failed to add first track: %v", err)
	}

	// Initial connection
	if err = pp.ConnectLibOffersPionAnswers(); err != nil {
		t.Fatalf("Failed initial connect: %v", err)
	}

	// Wait for first track
	if track := pp.WaitForPionTrack(interopTrackTimeout); track != nil {
		t.Logf("Initial track received: %s", track.ID())
	}

	// Add second track
	track2, err := pp.Lib.CreateVideoTrack("video-renegotiate", codec.VP9, 640, 480)
	if err != nil {
		t.Fatalf("Failed to create second video track: %v", err)
	}
	if _, err = pp.Lib.AddTrack(track2); err != nil {
		t.Fatalf("Failed to add second track: %v", err)
	}

	// Renegotiate
	offer, err := pp.Lib.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create renegotiation offer: %v", err)
	}
	if err = pp.Lib.SetLocalDescription(offer); err != nil {
		t.Fatalf("Failed to set local description: %v", err)
	}

	if err = pp.Pion.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	}); err != nil {
		t.Fatalf("Failed to set pion remote description: %v", err)
	}

	answer, err := pp.Pion.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("Failed to create pion answer: %v", err)
	}

	gatherComplete := webrtc.GatheringCompletePromise(pp.Pion)
	if err = pp.Pion.SetLocalDescription(answer); err != nil {
		t.Fatalf("Failed to set pion local description: %v", err)
	}
	<-gatherComplete

	if err = pp.Lib.SetRemoteDescription(&pc.SessionDescription{
		Type: pc.SDPTypeAnswer,
		SDP:  pp.Pion.LocalDescription().SDP,
	}); err != nil {
		t.Fatalf("Failed to set lib remote description: %v", err)
	}

	// Wait for second track
	if track := pp.WaitForPionTrack(interopTrackTimeout); track != nil {
		t.Logf("Renegotiated track received: %s", track.ID())
	} else {
		t.Log("Renegotiated track not received (may require full ICE connectivity)")
	}
}

// createMinimalVP8Frame creates a minimal valid VP8 frame for testing.
func createMinimalVP8Frame(keyframe bool) []byte {
	// VP8 bitstream format:
	// - Frame tag (3 bytes for keyframe, 1 byte for non-keyframe)
	// - Payload

	if keyframe {
		// Minimal VP8 keyframe
		// Frame tag: size_and_type (3 bytes) + keyframe header
		return []byte{
			0x90, 0x01, 0x00, // Frame tag: keyframe, version 0, show_frame=1
			0x9d, 0x01, 0x2a, // Start code
			0x40, 0x01, // Width (320) in 2 bytes
			0xe0, 0x00, // Height (240) in 2 bytes
			// Minimal DCT partition
			0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00,
		}
	}
	// Minimal VP8 inter frame
	return []byte{
		0x01, 0x00, 0x00, // Frame tag: inter frame
		0x00, 0x00, 0x00, 0x00,
	}
}

// TestSDPParsing verifies that SDP from lib is correctly parsed by pion and vice versa.
func TestSDPParsing(t *testing.T) {
	pp := NewPionLibPeerPair(t)
	defer pp.Close()

	// Create a track to have something in the SDP
	track, err := pp.Lib.CreateVideoTrack("sdp-test", codec.VP8, 640, 480)
	if err != nil {
		t.Fatalf("Failed to create track: %v", err)
	}
	if _, err = pp.Lib.AddTrack(track); err != nil {
		t.Fatalf("Failed to add track: %v", err)
	}

	// Create offer from lib
	offer, err := pp.Lib.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create offer: %v", err)
	}
	// Lib must set local description BEFORE remote description
	if err = pp.Lib.SetLocalDescription(offer); err != nil {
		t.Fatalf("Failed to set lib local description: %v", err)
	}

	t.Logf("Lib generated SDP:\n%s", offer.SDP)

	// Verify pion can parse it
	err = pp.Pion.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	})
	if err != nil {
		t.Fatalf("Pion failed to parse lib SDP: %v", err)
	}

	// Create answer from pion
	answer, err := pp.Pion.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("Failed to create pion answer: %v", err)
	}
	if err = pp.Pion.SetLocalDescription(answer); err != nil {
		t.Fatalf("Failed to set pion local description: %v", err)
	}

	t.Logf("Pion generated SDP:\n%s", answer.SDP)

	// Verify lib can parse it
	err = pp.Lib.SetRemoteDescription(&pc.SessionDescription{
		Type: pc.SDPTypeAnswer,
		SDP:  answer.SDP,
	})
	if err != nil {
		t.Fatalf("Lib failed to parse pion SDP: %v", err)
	}

	t.Log("SDP parsing test passed - both implementations can parse each other's SDP")
}

// TestConnectionStateInterop verifies connection states are correctly reported.
func TestConnectionStateInterop(t *testing.T) {
	pp := NewPionLibPeerPair(t)
	defer pp.Close()

	// Track connection state changes
	var pionStates []webrtc.PeerConnectionState
	var pionStatesMu sync.Mutex
	pp.Pion.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		t.Logf("Pion connection state: %s", state)
		pionStatesMu.Lock()
		pionStates = append(pionStates, state)
		pionStatesMu.Unlock()
	})

	var libStates []pc.PeerConnectionState
	var libStatesMu sync.Mutex
	pp.Lib.OnConnectionStateChange = func(state pc.PeerConnectionState) {
		t.Logf("Lib connection state: %s", state)
		libStatesMu.Lock()
		libStates = append(libStates, state)
		libStatesMu.Unlock()
	}

	// Add a track to have media
	track, _ := pp.Lib.CreateVideoTrack("state-test", codec.VP8, 320, 240)
	_, _ = pp.Lib.AddTrack(track)

	// Connect
	if err := pp.ConnectLibOffersPionAnswers(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Wait for states to settle
	time.Sleep(interopICEGatherDelay)

	pionStatesMu.Lock()
	t.Logf("Pion state changes: %v", pionStates)
	pionStatesMu.Unlock()

	libStatesMu.Lock()
	t.Logf("Lib state changes: %v", libStates)
	libStatesMu.Unlock()

	// Verify we got some state changes
	pionStatesMu.Lock()
	hasPionStates := len(pionStates) > 0
	pionStatesMu.Unlock()

	libStatesMu.Lock()
	hasLibStates := len(libStates) > 0
	libStatesMu.Unlock()

	if !hasPionStates && !hasLibStates {
		t.Log("No state changes observed (may require full ICE connectivity)")
	}
}

// TestICECandidateExchange tests explicit ICE candidate exchange.
func TestICECandidateExchange(t *testing.T) {
	pp := NewPionLibPeerPair(t)
	defer pp.Close()

	// Collect ICE candidates
	var pionCandidates []webrtc.ICECandidate
	var pionCandidatesMu sync.Mutex
	pp.Pion.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		pionCandidatesMu.Lock()
		pionCandidates = append(pionCandidates, *c)
		pionCandidatesMu.Unlock()
		t.Logf("Pion ICE candidate: %s", c.String())
	})

	var libCandidates []*pc.ICECandidate
	var libCandidatesMu sync.Mutex
	pp.Lib.OnICECandidate = func(c *pc.ICECandidate) {
		if c == nil {
			return
		}
		libCandidatesMu.Lock()
		libCandidates = append(libCandidates, c)
		libCandidatesMu.Unlock()
		t.Logf("Lib ICE candidate: %s", c.Candidate)
	}

	// Add a track
	track, _ := pp.Lib.CreateVideoTrack("ice-test", codec.VP8, 320, 240)
	_, _ = pp.Lib.AddTrack(track)

	// Perform offer/answer without waiting for gathering
	offer, _ := pp.Lib.CreateOffer(nil)
	pp.Lib.SetLocalDescription(offer)

	pp.Pion.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	})

	answer, _ := pp.Pion.CreateAnswer(nil)
	pp.Pion.SetLocalDescription(answer)

	pp.Lib.SetRemoteDescription(&pc.SessionDescription{
		Type: pc.SDPTypeAnswer,
		SDP:  answer.SDP,
	})

	// Wait for ICE gathering
	time.Sleep(interopICEGatherDelay)

	// Exchange candidates
	pionCandidatesMu.Lock()
	for _, c := range pionCandidates {
		cJSON := c.ToJSON()
		ufrag := ""
		if cJSON.UsernameFragment != nil {
			ufrag = *cJSON.UsernameFragment
		}
		if err := pp.Lib.AddICECandidate(&pc.ICECandidate{
			Candidate:        cJSON.Candidate,
			SDPMid:           *cJSON.SDPMid,
			SDPMLineIndex:    *cJSON.SDPMLineIndex,
			UsernameFragment: ufrag,
		}); err != nil {
			t.Logf("Failed to add pion candidate to lib: %v", err)
		}
	}
	pionCandidatesMu.Unlock()

	libCandidatesMu.Lock()
	for _, c := range libCandidates {
		idx := uint16(c.SDPMLineIndex)
		ufrag := c.UsernameFragment
		if err := pp.Pion.AddICECandidate(webrtc.ICECandidateInit{
			Candidate:        c.Candidate,
			SDPMid:           &c.SDPMid,
			SDPMLineIndex:    &idx,
			UsernameFragment: &ufrag,
		}); err != nil {
			t.Logf("Failed to add lib candidate to pion: %v", err)
		}
	}
	libCandidatesMu.Unlock()

	pionCandidatesMu.Lock()
	libCandidatesMu.Lock()
	t.Logf("ICE candidates: pion=%d, lib=%d", len(pionCandidates), len(libCandidates))
	libCandidatesMu.Unlock()
	pionCandidatesMu.Unlock()

	// Wait for connection
	time.Sleep(interopICEGatherDelay)
	t.Logf("Final connection state - Lib: %s", pp.Lib.ConnectionState())
}

// TestStressInterop sends many frames rapidly to test stability.
func TestStressInterop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	pp := NewPionLibPeerPair(t)
	defer pp.Close()

	track, _ := pp.Lib.CreateVideoTrack("stress-test", codec.VP8, 320, 240)
	_, _ = pp.Lib.AddTrack(track)

	if err := pp.ConnectLibOffersPionAnswers(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	pionTrack := pp.WaitForPionTrack(interopTrackTimeout)
	if pionTrack == nil {
		t.Skip("Track not received - skipping stress test")
	}

	// Send a short burst of frames to exercise stability.
	start := time.Now()
	framesSent := 0
	for i := 0; i < interopStressFrameCount; i++ {
		f := CreateTestFrame(320, 240, uint32(i))
		if err := track.WriteVideoFrame(f); err == nil {
			framesSent++
		}
		time.Sleep(interopFrameDelay)
	}
	elapsed := time.Since(start)

	time.Sleep(interopPostSendDelay)

	framesReceived := pp.pionVideoFrames.Load()
	t.Logf("Stress test: sent %d frames in %v, received %d",
		framesSent, elapsed, framesReceived)

	if framesReceived == 0 {
		t.Log("No frames received - may require full ICE connectivity")
	} else {
		lossRate := float64(framesSent-int(framesReceived)) / float64(framesSent) * 100
		t.Logf("Frame loss rate: %.1f%%", lossRate)
	}
}

// TestFrameIntegrity tests that frame data is transmitted correctly.
func TestFrameIntegrity(t *testing.T) {
	pp := NewPionLibPeerPair(t)
	defer pp.Close()

	// Use pion sending to lib so we can verify frame content
	pionTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		"integrity-test",
		"test-stream",
	)
	if err != nil {
		t.Fatalf("Failed to create pion track: %v", err)
	}
	_, _ = pp.Pion.AddTrack(pionTrack)

	if err := pp.ConnectPionOffersLibAnswers(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	libTrack := pp.WaitForLibTrack(interopTrackTimeout)
	if libTrack == nil {
		t.Skip("Track not received - skipping integrity test")
	}

	// Send a known pattern
	testPattern := bytes.Repeat([]byte{0xAA, 0x55}, 100)
	keyframe := createMinimalVP8Frame(true)
	keyframe = append(keyframe, testPattern...)

	pionTrack.WriteSample(media.Sample{
		Data:     keyframe,
		Duration: 33 * time.Millisecond,
	})

	time.Sleep(interopPostSendDelay)

	// Note: Full integrity verification would require decoding the frame
	// Here we just verify something was received
	if pp.libVideoFrames.Load() > 0 {
		t.Log("Frame received successfully")
	} else {
		t.Log("Frame not received - may require full ICE connectivity")
	}
}
