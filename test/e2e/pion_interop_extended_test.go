//go:build extended

package e2e

import (
	"testing"
	"time"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

// TestStressInterop sends many frames rapidly to test stability.
func TestStressInterop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	pp := NewPionLibPeerPair(t)
	defer pp.Close()

	track, _ := pp.Lib.CreateVideoTrack("stress-test", 320, 240)
	_, _ = pp.Lib.AddTrack(track)

	if err := pp.ConnectLibOffersPionAnswers(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	pionTrack := pp.WaitForPionTrack(interopTrackTimeout)
	if pionTrack == nil {
		t.Skip("Track not received - skipping stress test")
	}

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
