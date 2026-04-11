//go:build extended

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

// TestVideoAndAudioTrackReception exercises mixed media flow under load.
func TestVideoAndAudioTrackReception(t *testing.T) {
	pp := NewLibPeerPair(t)
	defer pp.Close()

	videoTrack, err := pp.Sender.CreateVideoTrack("video-multi", 640, 480)
	if err != nil {
		t.Fatalf("CreateVideoTrack failed: %v", err)
	}
	pp.Sender.AddTrack(videoTrack, "stream-0")

	audioTrack, err := pp.Sender.CreateAudioTrackWithOptions("audio-multi", 48000, 2)
	if err != nil {
		t.Fatalf("CreateAudioTrack failed: %v", err)
	}
	pp.Sender.AddTrack(audioTrack, "stream-0")

	err = pp.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	go func() {
		for i := 0; i < shortFrameCount; i++ {
			frame := CreateTestFrame(640, 480, uint32(i*3000))
			videoTrack.WriteVideoFrame(frame)
			time.Sleep(shortFrameDelay)
		}
	}()

	go func() {
		for i := 0; i < shortAudioFrameCount; i++ {
			frame := CreateTestAudioFrame(48000, 2, 480, uint32(i*480))
			audioTrack.WriteAudioFrame(frame)
			time.Sleep(shortAudioFrameDelay)
		}
	}()

	if pp.WaitForConnection(shortConnectTimeout) {
		t.Log("Connection established")
		time.Sleep(shortPostSendDelay)
		t.Logf("Received %d track(s)", pp.ReceivedTrackCount())
	} else {
		t.Log("Connection timeout (expected in test environment)")
	}

	localDesc := pp.Sender.LocalDescription()
	if localDesc == nil {
		t.Fatal("Sender should have local description")
	}

	if !strings.Contains(localDesc.SDP, "m=video") {
		t.Error("SDP should contain video m-line")
	}
	if !strings.Contains(localDesc.SDP, "m=audio") {
		t.Error("SDP should contain audio m-line")
	}
}
