package pionrecv

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

const integrationWaitTimeout = 10 * time.Second

func TestBindRemoteTrackWithPionOnTrack(t *testing.T) {
	sender, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection(sender): %v", err)
	}
	defer func() {
		_ = sender.Close()
	}()

	receiver, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection(receiver): %v", err)
	}
	defer func() {
		_ = receiver.Close()
	}()

	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: testVideoClockRate,
		},
		"pionrecv-video",
		"pionrecv-stream",
	)
	if err != nil {
		t.Fatalf("NewTrackLocalStaticRTP: %v", err)
	}

	rtpSender, err := sender.AddTrack(videoTrack)
	if err != nil {
		t.Fatalf("sender.AddTrack: %v", err)
	}
	go drainSenderRTCP(rtpSender)

	frames := make(chan *frame.VideoFrame, 8)
	decodedReady := make(chan *DecodedTrack, 1)
	runErr := make(chan error, 1)
	receiver.OnTrack(func(remoteTrack *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) {
		decoded, err := BindRemoteTrack(remoteTrack, rtpReceiver, WithRTCPWriter(rtpReceiver.Transport()))
		if err != nil {
			runErr <- err
			return
		}
		select {
		case decodedReady <- decoded:
		default:
		}
		if err := decoded.SetOnVideoFrame(func(f *frame.VideoFrame) {
			select {
			case frames <- f:
			default:
			}
		}); err != nil {
			runErr <- err
			return
		}
		go func() {
			runErr <- decoded.Run()
		}()
	})

	connectPionPeers(t, sender, receiver)

	packets := mustEncodeVideoPackets(t, codec.VP8, 96, 0x11223344, []uint32{0, 3000, 6000, 9000})
	for i, pkt := range packets {
		if err := videoTrack.WriteRTP(pkt); err != nil {
			t.Fatalf("videoTrack.WriteRTP(%d): %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	var decoded *DecodedTrack
	select {
	case decoded = <-decodedReady:
	case err := <-runErr:
		t.Fatalf("decoded.Run: %v", err)
	case <-time.After(integrationWaitTimeout):
		t.Fatal("timed out waiting for decoded track from Pion OnTrack")
	}

	waitForExpectedVideoFrame(t, frames, runErr, "Pion OnTrack")

	if decoded == nil {
		t.Fatal("expected decoded track")
	}
	if decoded.Kind() != webrtc.RTPCodecTypeVideo {
		t.Fatalf("Kind() = %v, want video", decoded.Kind())
	}
	if decoded.CodecParameters().MimeType != webrtc.MimeTypeVP8 {
		t.Fatalf("CodecParameters().MimeType = %q, want %q", decoded.CodecParameters().MimeType, webrtc.MimeTypeVP8)
	}
	if decoded.PayloadType() != 96 {
		t.Fatalf("PayloadType() = %d, want 96", decoded.PayloadType())
	}
	if err := decoded.Close(); err != nil {
		t.Fatalf("decoded.Close: %v", err)
	}

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("decoded.Run exit error = %v, want nil", err)
		}
	case <-time.After(integrationWaitTimeout):
		t.Fatal("timed out waiting for decoded.Run to exit after Close")
	}
}

func drainSenderRTCP(sender *webrtc.RTPSender) {
	if sender == nil {
		return
	}

	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

func connectPionPeers(t testing.TB, offerer, answerer *webrtc.PeerConnection) {
	t.Helper()

	offerGatheringDone := webrtc.GatheringCompletePromise(offerer)
	offer, err := offerer.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := offerer.SetLocalDescription(offer); err != nil {
		t.Fatalf("SetLocalDescription(offer): %v", err)
	}
	<-offerGatheringDone

	if err := answerer.SetRemoteDescription(*offerer.LocalDescription()); err != nil {
		t.Fatalf("SetRemoteDescription(offer): %v", err)
	}

	answerGatheringDone := webrtc.GatheringCompletePromise(answerer)
	answer, err := answerer.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("CreateAnswer: %v", err)
	}
	if err := answerer.SetLocalDescription(answer); err != nil {
		t.Fatalf("SetLocalDescription(answer): %v", err)
	}
	<-answerGatheringDone

	if err := offerer.SetRemoteDescription(*answerer.LocalDescription()); err != nil {
		t.Fatalf("SetRemoteDescription(answer): %v", err)
	}

	waitForConnectionState(t, offerer, webrtc.PeerConnectionStateConnected)
	waitForConnectionState(t, answerer, webrtc.PeerConnectionStateConnected)
}

func waitForConnectionState(t testing.TB, pc *webrtc.PeerConnection, want webrtc.PeerConnectionState) {
	t.Helper()

	if pc.ConnectionState() == want {
		return
	}

	done := make(chan struct{}, 1)
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == want {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	})

	if pc.ConnectionState() == want {
		return
	}

	select {
	case <-done:
	case <-time.After(integrationWaitTimeout):
		if pc.ConnectionState() == want {
			return
		}
		t.Fatalf("timed out waiting for connection state %s, got %s", want, pc.ConnectionState())
	}
}

func waitForExpectedVideoFrame(t testing.TB, frames <-chan *frame.VideoFrame, runErr <-chan error, context string) *frame.VideoFrame {
	t.Helper()

	timeout := time.NewTimer(integrationWaitTimeout)
	defer timeout.Stop()

	var placeholderFrames int

	for {
		select {
		case got := <-frames:
			if got == nil {
				continue
			}
			if got.Width == 0 || got.Height == 0 {
				placeholderFrames++
				continue
			}
			if got.Width != testVideoWidth || got.Height != testVideoHeight {
				t.Fatalf("%s decoded frame size = %dx%d, want %dx%d", context, got.Width, got.Height, testVideoWidth, testVideoHeight)
			}
			return got
		case err := <-runErr:
			t.Fatalf("decoded.Run: %v", err)
		case <-timeout.C:
			if placeholderFrames > 0 {
				t.Fatalf("timed out waiting for decoded frame from %s after receiving %d placeholder frames", context, placeholderFrames)
			}
			t.Fatalf("timed out waiting for decoded frame from %s", context)
		}
	}
}
