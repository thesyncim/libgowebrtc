// Package e2e provides end-to-end tests for libgowebrtc.
// These tests use only libgowebrtc components (no Pion dependency).
package e2e

import (
	"sync"
	"testing"
	"time"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pc"
)

// LibPeerPair represents two libwebrtc PeerConnections for loopback testing.
type LibPeerPair struct {
	Sender   *pc.PeerConnection
	Receiver *pc.PeerConnection
	t        *testing.T
}

// NewLibPeerPair creates a pair of connected PeerConnections for testing.
func NewLibPeerPair(t *testing.T) *LibPeerPair {
	t.Helper()

	cfg := pc.DefaultConfiguration()

	sender, err := pc.NewPeerConnection(cfg)
	if err != nil {
		t.Fatalf("Failed to create sender PeerConnection: %v", err)
	}

	receiver, err := pc.NewPeerConnection(cfg)
	if err != nil {
		sender.Close()
		t.Fatalf("Failed to create receiver PeerConnection: %v", err)
	}

	return &LibPeerPair{
		Sender:   sender,
		Receiver: receiver,
		t:        t,
	}
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
	if err := pp.Sender.SetLocalDescription(offer); err != nil {
		return err
	}

	// Set remote description on receiver
	if err := pp.Receiver.SetRemoteDescription(offer); err != nil {
		return err
	}

	// Create answer from receiver
	answer, err := pp.Receiver.CreateAnswer(nil)
	if err != nil {
		return err
	}

	// Set local description on receiver
	if err := pp.Receiver.SetLocalDescription(answer); err != nil {
		return err
	}

	// Set remote description on sender
	if err := pp.Sender.SetRemoteDescription(answer); err != nil {
		return err
	}

	return nil
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
		return
	}
	defer ffi.Close()
	m.Run()
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
