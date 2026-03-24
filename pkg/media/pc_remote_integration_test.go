package media

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/thesyncim/libgowebrtc/internal/testutil"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pc"
)

func TestRemoteStreamRegistryBindPCTrackIntegration(t *testing.T) {
	testutil.RequireShim(t)

	sender, err := newMediaRemotePeerConnection()
	if err != nil {
		t.Fatalf("newMediaRemotePeerConnection(sender): %v", err)
	}
	defer func() { _ = sender.Close() }()

	receiver, err := newMediaRemotePeerConnection()
	if err != nil {
		t.Fatalf("newMediaRemotePeerConnection(receiver): %v", err)
	}
	defer func() { _ = receiver.Close() }()

	videoTrack, err := sender.CreateVideoTrack("remote-video", codec.VP8, 96, 64)
	if err != nil {
		t.Fatalf("sender.CreateVideoTrack: %v", err)
	}
	if _, err := sender.AddTrack(videoTrack, "remote-stream"); err != nil {
		t.Fatalf("sender.AddTrack: %v", err)
	}

	registry := NewRemoteStreamRegistry()
	defer registry.Close()

	frameCh := make(chan *frame.VideoFrame, 8)
	trackCh := make(chan RemoteTrack, 1)
	streamsCh := make(chan []*MediaStream, 1)
	errCh := make(chan error, 1)

	receiver.SetOnTrack(func(track *pc.Track, rtpReceiver *pc.RTPReceiver, streams []string) {
		boundTrack, mediaStreams, err := registry.BindPCTrack(track, rtpReceiver, streams)
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
			return
		}

		video := boundTrack.(PCRemoteVideoTrack)
		if err := video.SetOnVideoFrame(func(f *frame.VideoFrame) {
			select {
			case frameCh <- f:
			default:
			}
		}); err != nil {
			select {
			case errCh <- err:
			default:
			}
			return
		}

		select {
		case trackCh <- boundTrack:
		default:
		}
		select {
		case streamsCh <- mediaStreams:
		default:
		}
	})

	if err := connectMediaRemotePeers(sender, receiver); err != nil {
		t.Fatalf("connectMediaRemotePeers: %v", err)
	}

	for i := 0; i < 6; i++ {
		frameToSend := testutil.CreateTestVideoFrame(96, 64)
		frameToSend.PTS = uint32(i * 3000)
		if err := videoTrack.WriteVideoFrame(frameToSend); err != nil {
			t.Fatalf("videoTrack.WriteVideoFrame(%d): %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	var boundTrack RemoteTrack
	select {
	case boundTrack = <-trackCh:
	case err := <-errCh:
		t.Fatalf("BindPCTrack: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for bound remote track")
	}

	var streams []*MediaStream
	select {
	case streams = <-streamsCh:
	case err := <-errCh:
		t.Fatalf("BindPCTrack: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for remote streams")
	}

	select {
	case got := <-frameCh:
		if got == nil || got.Width == 0 || got.Height == 0 {
			t.Fatalf("decoded frame = %+v, want non-empty video frame", got)
		}
	case err := <-errCh:
		t.Fatalf("remote frame callback error = %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for decoded frame")
	}

	video := boundTrack.(PCRemoteVideoTrack)
	if boundTrack.ID() != "remote-video" {
		t.Fatalf("boundTrack.ID() = %q, want %q", boundTrack.ID(), "remote-video")
	}
	if boundTrack.StreamID() != "remote-stream" {
		t.Fatalf(
			"boundTrack.ID()=%q StreamID()=%q StreamIDs()=%v, want remote-stream\nremote SDP:\n%s",
			boundTrack.ID(),
			boundTrack.StreamID(),
			boundTrack.StreamIDs(),
			receiver.RemoteDescription().SDP,
		)
	}
	if got := boundTrack.StreamIDs(); len(got) != 1 || got[0] != "remote-stream" {
		t.Fatalf("boundTrack.StreamIDs() = %v, want [remote-stream]", got)
	}
	if len(streams) != 1 {
		t.Fatalf("streams len = %d, want 1", len(streams))
	}
	if streams[0].ID() != "remote-stream" {
		t.Fatalf("stream ID = %q, want %q", streams[0].ID(), "remote-stream")
	}
	if got := streams[0].GetTrackByID(boundTrack.ID()); got == nil {
		t.Fatal("expected bound track to be present in returned MediaStream")
	}
	if video.PCTrack() == nil {
		t.Fatal("PCTrack() = nil, want native libwebrtc track")
	}
	if video.PCReceiver() == nil {
		t.Fatal("PCReceiver() = nil, want native RTP receiver")
	}

	boundTrack.Stop()
}

func newMediaRemotePeerConnection() (*pc.PeerConnection, error) {
	cfg := pc.DefaultConfiguration()
	cfg.ICEServers = nil
	return pc.NewPeerConnection(cfg)
}

func connectMediaRemotePeers(offerer, answerer *pc.PeerConnection) error {
	var (
		mu                 sync.Mutex
		offererCandidates  []*pc.ICECandidate
		answererCandidates []*pc.ICECandidate
	)

	offerer.SetOnICECandidate(func(c *pc.ICECandidate) {
		if c == nil {
			return
		}
		mu.Lock()
		offererCandidates = append(offererCandidates, c)
		mu.Unlock()
	})
	answerer.SetOnICECandidate(func(c *pc.ICECandidate) {
		if c == nil {
			return
		}
		mu.Lock()
		answererCandidates = append(answererCandidates, c)
		mu.Unlock()
	})

	offer, err := offerer.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := offerer.SetLocalDescription(offer); err != nil {
		return err
	}
	if err := answerer.SetRemoteDescription(offer); err != nil {
		return err
	}

	answer, err := answerer.CreateAnswer(nil)
	if err != nil {
		return err
	}
	if err := answerer.SetLocalDescription(answer); err != nil {
		return err
	}
	if err := offerer.SetRemoteDescription(answer); err != nil {
		return err
	}

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	offererCandidatesCopy := append([]*pc.ICECandidate(nil), offererCandidates...)
	answererCandidatesCopy := append([]*pc.ICECandidate(nil), answererCandidates...)
	mu.Unlock()
	for _, candidate := range offererCandidatesCopy {
		if err := answerer.AddICECandidate(candidate); err != nil {
			return err
		}
	}
	for _, candidate := range answererCandidatesCopy {
		if err := offerer.AddICECandidate(candidate); err != nil {
			return err
		}
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if offerer.ConnectionState() == pc.PeerConnectionStateConnected &&
			answerer.ConnectionState() == pc.PeerConnectionStateConnected {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errMediaRemoteConnectTimeout
}

var errMediaRemoteConnectTimeout = errors.New("timed out waiting for libwebrtc peers to connect")
