// Package e2e provides end-to-end tests for libgowebrtc.
// These tests use only libgowebrtc components (no Pion dependency).
package e2e

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pc"
)

const (
	shortFrameCount      = 5
	shortAudioFrameCount = 20
	shortFrameDelay      = 5 * time.Millisecond
	shortAudioFrameDelay = 5 * time.Millisecond
	shortConnectTimeout  = 2 * time.Second
	shortTrackTimeout    = 1 * time.Second
	shortPostSendDelay   = 50 * time.Millisecond
	shortICEGatherDelay  = 50 * time.Millisecond
)

func defaultTestConfig() pc.Configuration {
	cfg := pc.DefaultConfiguration()
	cfg.ICEServers = nil
	return cfg
}

// LibPeerPair represents two libwebrtc PeerConnections for loopback testing.
type LibPeerPair struct {
	Sender   *pc.PeerConnection
	Receiver *pc.PeerConnection
	t        *testing.T

	// ICE candidates collected during negotiation
	senderCandidates   []*pc.ICECandidate
	receiverCandidates []*pc.ICECandidate
	candidatesMu       sync.Mutex

	// Track reception tracking
	ReceivedTracks   []*pc.Track
	receivedTracksMu sync.Mutex
	trackReceived    chan struct{}

	// Frame counting for received tracks
	VideoFrameCounter *FrameCounter
	AudioFrameCounter *FrameCounter
	frameReceived     chan struct{}
}

// NewLibPeerPair creates a pair of connected PeerConnections for testing.
func NewLibPeerPair(t *testing.T) *LibPeerPair {
	t.Helper()

	cfg := defaultTestConfig()

	sender, err := pc.NewPeerConnection(cfg)
	if err != nil {
		t.Fatalf("Failed to create sender PeerConnection: %v", err)
	}

	receiver, err := pc.NewPeerConnection(cfg)
	if err != nil {
		sender.Close()
		t.Fatalf("Failed to create receiver PeerConnection: %v", err)
	}

	pp := &LibPeerPair{
		Sender:            sender,
		Receiver:          receiver,
		t:                 t,
		trackReceived:     make(chan struct{}, 10),
		frameReceived:     make(chan struct{}, 100),
		VideoFrameCounter: &FrameCounter{},
		AudioFrameCounter: &FrameCounter{},
	}

	// Set up ICE candidate handlers
	sender.OnICECandidate = func(c *pc.ICECandidate) {
		if c == nil {
			return
		}
		pp.candidatesMu.Lock()
		pp.senderCandidates = append(pp.senderCandidates, c)
		pp.candidatesMu.Unlock()
	}

	receiver.OnICECandidate = func(c *pc.ICECandidate) {
		if c == nil {
			return
		}
		pp.candidatesMu.Lock()
		pp.receiverCandidates = append(pp.receiverCandidates, c)
		pp.candidatesMu.Unlock()
	}

	// Set up OnTrack handler on receiver
	receiver.OnTrack = func(track *pc.Track, recv *pc.RTPReceiver, streams []string) {
		pp.receivedTracksMu.Lock()
		pp.ReceivedTracks = append(pp.ReceivedTracks, track)
		pp.receivedTracksMu.Unlock()

		// Set up frame callbacks on the received track
		if track.Kind() == "video" {
			if err := track.SetOnVideoFrame(func(f *frame.VideoFrame) {
				pp.VideoFrameCounter.Inc()
				select {
				case pp.frameReceived <- struct{}{}:
				default:
				}
			}); err != nil {
				t.Logf("SetOnVideoFrame failed: %v", err)
			}
		} else if track.Kind() == "audio" {
			if err := track.SetOnAudioFrame(func(f *frame.AudioFrame) {
				pp.AudioFrameCounter.Inc()
				select {
				case pp.frameReceived <- struct{}{}:
				default:
				}
			}); err != nil {
				t.Logf("SetOnAudioFrame failed: %v", err)
			}
		}

		select {
		case pp.trackReceived <- struct{}{}:
		default:
		}
		t.Logf("Receiver got track: id=%s kind=%s", track.ID(), track.Kind())
	}

	return pp
}

// Close closes both PeerConnections.
func (pp *LibPeerPair) Close() {
	if pp.Sender != nil {
		pp.Sender.Close()
	}
	if pp.Receiver != nil {
		pp.Receiver.Close()
	}
}

// ExchangeOfferAnswer performs the SDP exchange between sender and receiver.
func (pp *LibPeerPair) ExchangeOfferAnswer() error {
	// Create offer from sender
	offer, err := pp.Sender.CreateOffer(nil)
	if err != nil {
		return err
	}

	// Set local description on sender
	err = pp.Sender.SetLocalDescription(offer)
	if err != nil {
		return err
	}

	// Set remote description on receiver
	err = pp.Receiver.SetRemoteDescription(offer)
	if err != nil {
		return err
	}

	// Create answer from receiver
	answer, err := pp.Receiver.CreateAnswer(nil)
	if err != nil {
		return err
	}

	// Set local description on receiver
	err = pp.Receiver.SetLocalDescription(answer)
	if err != nil {
		return err
	}

	// Set remote description on sender
	return pp.Sender.SetRemoteDescription(answer)
}

// ExchangeICECandidates exchanges collected ICE candidates between peers.
// Call this after ExchangeOfferAnswer and a brief delay for candidate gathering.
func (pp *LibPeerPair) ExchangeICECandidates() error {
	pp.candidatesMu.Lock()
	senderCandidates := make([]*pc.ICECandidate, len(pp.senderCandidates))
	copy(senderCandidates, pp.senderCandidates)
	receiverCandidates := make([]*pc.ICECandidate, len(pp.receiverCandidates))
	copy(receiverCandidates, pp.receiverCandidates)
	pp.candidatesMu.Unlock()

	// Add sender's candidates to receiver
	for _, c := range senderCandidates {
		if err := pp.Receiver.AddICECandidate(c); err != nil {
			pp.t.Logf("Failed to add sender candidate to receiver: %v", err)
		}
	}

	// Add receiver's candidates to sender
	for _, c := range receiverCandidates {
		if err := pp.Sender.AddICECandidate(c); err != nil {
			pp.t.Logf("Failed to add receiver candidate to sender: %v", err)
		}
	}

	return nil
}

// Connect performs full connection: offer/answer + ICE exchange.
func (pp *LibPeerPair) Connect() error {
	if err := pp.ExchangeOfferAnswer(); err != nil {
		return err
	}

	// Wait a bit for ICE candidates to be gathered
	time.Sleep(shortICEGatherDelay)

	return pp.ExchangeICECandidates()
}

// WaitForTrack waits for at least one track to be received.
func (pp *LibPeerPair) WaitForTrack(timeout time.Duration) bool {
	select {
	case <-pp.trackReceived:
		return true
	case <-time.After(timeout):
		return false
	}
}

// WaitForFrame waits for at least one frame to be received.
func (pp *LibPeerPair) WaitForFrame(timeout time.Duration) bool {
	select {
	case <-pp.frameReceived:
		return true
	case <-time.After(timeout):
		return false
	}
}

// VideoFrameCount returns the number of video frames received.
func (pp *LibPeerPair) VideoFrameCount() int {
	return pp.VideoFrameCounter.Count()
}

// AudioFrameCount returns the number of audio frames received.
func (pp *LibPeerPair) AudioFrameCount() int {
	return pp.AudioFrameCounter.Count()
}

// ReceivedTrackCount returns the number of tracks received.
func (pp *LibPeerPair) ReceivedTrackCount() int {
	pp.receivedTracksMu.Lock()
	defer pp.receivedTracksMu.Unlock()
	return len(pp.ReceivedTracks)
}

// WaitForConnection waits for both peers to reach connected state.
func (pp *LibPeerPair) WaitForConnection(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		senderState := pp.Sender.ConnectionState()
		receiverState := pp.Receiver.ConnectionState()

		if senderState == pc.PeerConnectionStateConnected &&
			receiverState == pc.PeerConnectionStateConnected {
			return true
		}

		time.Sleep(50 * time.Millisecond)
	}

	return false
}

// TestMain handles library loading for all e2e tests.
func TestMain(m *testing.M) {
	if err := ffi.LoadLibrary(); err != nil {
		// Skip tests if library not available
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// FrameCounter tracks received frames with thread-safety.
type FrameCounter struct {
	mu    sync.Mutex
	count int
}

// Inc increments the counter.
func (fc *FrameCounter) Inc() {
	fc.mu.Lock()
	fc.count++
	fc.mu.Unlock()
}

// Count returns the current count.
func (fc *FrameCounter) Count() int {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.count
}

// CreateTestVideoTrack creates a video track for testing.
func CreateTestVideoTrack(t *testing.T, p *pc.PeerConnection, id string, c codec.Type, width, height int) *pc.Track {
	t.Helper()
	track, err := p.CreateVideoTrack(id, c, width, height)
	if err != nil {
		t.Fatalf("CreateVideoTrack failed: %v", err)
	}
	return track
}

// CreateTestAudioTrack creates an audio track for testing.
func CreateTestAudioTrack(t *testing.T, p *pc.PeerConnection, id string) *pc.Track {
	t.Helper()
	track, err := p.CreateAudioTrack(id)
	if err != nil {
		t.Fatalf("CreateAudioTrack failed: %v", err)
	}
	return track
}

// CreateTestFrame creates an I420 test frame with a gradient pattern.
func CreateTestFrame(width, height int, pts uint32) *frame.VideoFrame {
	f := frame.NewI420Frame(width, height)
	f.PTS = pts

	// Fill Y plane with gradient pattern
	yPlane := f.Data[0]
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			yPlane[y*width+x] = byte((x + y + int(pts)) % 256)
		}
	}

	// Fill U/V planes with neutral values
	for i := range f.Data[1] {
		f.Data[1][i] = 128
		f.Data[2][i] = 128
	}

	return f
}

// CreateTestAudioFrame creates a test audio frame with a simple pattern.
func CreateTestAudioFrame(sampleRate, channels, numSamples int, pts uint32) *frame.AudioFrame {
	f := frame.NewAudioFrameS16(sampleRate, channels, numSamples)
	f.PTS = pts

	// Fill with simple sine-like pattern
	totalSamples := numSamples * channels
	for i := 0; i < totalSamples; i++ {
		// Write as little-endian int16
		val := int16((i * 100) % 32767)
		f.Samples[i*2] = byte(val)
		f.Samples[i*2+1] = byte(val >> 8)
	}

	return f
}
