package media

import (
	"errors"
	"testing"
	"time"

	"github.com/pion/rtp"
	pioncodecs "github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pionrecv"
)

func TestRemoteStreamRegistryBindPionTrackIntegration(t *testing.T) {
	sender, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection(sender): %v", err)
	}
	defer func() { _ = sender.Close() }()

	receiver, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection(receiver): %v", err)
	}
	defer func() { _ = receiver.Close() }()

	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		},
		"remote-video",
		"remote-stream",
	)
	if err != nil {
		t.Fatalf("NewTrackLocalStaticRTP: %v", err)
	}

	rtpSender, err := sender.AddTrack(videoTrack)
	if err != nil {
		t.Fatalf("sender.AddTrack: %v", err)
	}
	go drainRemoteRegistryRTCP(rtpSender)

	registry := NewRemoteStreamRegistry()
	frameCh := make(chan *frame.VideoFrame, 8)
	eventCh := make(chan PionTrackEvent, 1)
	streamsCh := make(chan []*MediaStream, 1)
	errCh := make(chan error, 1)

	receiver.OnTrack(registry.PionOnTrack(
		func(event PionTrackEvent) {
			video := event.Track.(PionRemoteVideoTrack)
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
			case eventCh <- event:
			default:
			}
			select {
			case streamsCh <- event.Streams:
			default:
			}
		},
		func(err error) {
			select {
			case errCh <- err:
			default:
			}
		},
	))

	connectRemoteRegistryPionPeers(t, sender, receiver)

	for i, pkt := range mustEncodeRemoteRegistryVideoPackets(t, codec.VP8, 96, 0x12345678, []uint32{0, 3000, 6000, 9000}) {
		if err := videoTrack.WriteRTP(pkt); err != nil {
			t.Fatalf("videoTrack.WriteRTP(%d): %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	var event PionTrackEvent
	select {
	case event = <-eventCh:
	case err := <-errCh:
		t.Fatalf("PionOnTrack: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for remote track")
	}
	boundTrack := event.Track

	var streams []*MediaStream
	select {
	case streams = <-streamsCh:
	case err := <-errCh:
		t.Fatalf("PionOnTrack: %v", err)
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

	if boundTrack.ID() != "remote-video" {
		t.Fatalf("boundTrack.ID() = %q, want %q", boundTrack.ID(), "remote-video")
	}
	if boundTrack.StreamID() != "remote-stream" {
		t.Fatalf("boundTrack.StreamID() = %q, want %q", boundTrack.StreamID(), "remote-stream")
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
	if event.TrackRemote == nil {
		t.Fatal("TrackRemote = nil, want original Pion track")
	}
	if event.Receiver == nil {
		t.Fatal("Receiver = nil, want original RTP receiver")
	}

	boundTrack.Stop()
}

func TestRemoteStreamRegistryBindDecodedTrackIntegration(t *testing.T) {
	sender, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection(sender): %v", err)
	}
	defer func() { _ = sender.Close() }()

	receiver, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection(receiver): %v", err)
	}
	defer func() { _ = receiver.Close() }()

	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		},
		"decoded-video",
		"decoded-stream",
	)
	if err != nil {
		t.Fatalf("NewTrackLocalStaticRTP: %v", err)
	}

	rtpSender, err := sender.AddTrack(videoTrack)
	if err != nil {
		t.Fatalf("sender.AddTrack: %v", err)
	}
	go drainRemoteRegistryRTCP(rtpSender)

	decodedCh := make(chan *pionrecv.DecodedTrack, 1)
	errCh := make(chan error, 1)
	receiver.OnTrack(func(remoteTrack *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) {
		decoded, err := pionrecv.BindRemoteTrack(
			remoteTrack,
			rtpReceiver,
			pionrecv.WithRTCPWriter(rtpReceiver.Transport()),
		)
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
			return
		}
		select {
		case decodedCh <- decoded:
		default:
		}
	})

	connectRemoteRegistryPionPeers(t, sender, receiver)

	packets := mustEncodeRemoteRegistryVideoPackets(t, codec.VP8, 96, 0x33445566, []uint32{0, 3000, 6000, 9000})
	if err := videoTrack.WriteRTP(packets[0]); err != nil {
		t.Fatalf("videoTrack.WriteRTP(trigger): %v", err)
	}

	var decoded *pionrecv.DecodedTrack
	select {
	case decoded = <-decodedCh:
	case err := <-errCh:
		t.Fatalf("BindRemoteTrack: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for decoded track")
	}

	registry := NewRemoteStreamRegistry()
	defer registry.Close()

	boundTrack, streams, err := registry.BindDecodedTrack(decoded)
	if err != nil {
		t.Fatalf("BindDecodedTrack: %v", err)
	}

	frameCh := make(chan *frame.VideoFrame, 8)
	video := boundTrack.(PionRemoteVideoTrack)
	if err := video.SetOnVideoFrame(func(f *frame.VideoFrame) {
		select {
		case frameCh <- f:
		default:
		}
	}); err != nil {
		t.Fatalf("SetOnVideoFrame: %v", err)
	}

	for i, pkt := range packets[1:] {
		if err := videoTrack.WriteRTP(pkt); err != nil {
			t.Fatalf("videoTrack.WriteRTP(%d): %v", i+1, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	select {
	case got := <-frameCh:
		if got == nil || got.Width == 0 || got.Height == 0 {
			t.Fatalf("decoded frame = %+v, want non-empty video frame", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for decoded frame")
	}

	if boundTrack.ID() != "decoded-video" {
		t.Fatalf("boundTrack.ID() = %q, want %q", boundTrack.ID(), "decoded-video")
	}
	if boundTrack.StreamID() != "decoded-stream" {
		t.Fatalf("boundTrack.StreamID() = %q, want %q", boundTrack.StreamID(), "decoded-stream")
	}
	if len(streams) != 1 || streams[0].ID() != "decoded-stream" {
		t.Fatalf("streams = %v, want [decoded-stream]", streams)
	}
}

const (
	remoteRegistryTestWidth      = 160
	remoteRegistryTestHeight     = 90
	remoteRegistryTestClockRate  = 90000
	remoteRegistryEncodeRetry    = 3000
	remoteRegistryEncodeAttempts = 5
)

func drainRemoteRegistryRTCP(sender *webrtc.RTPSender) {
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

func connectRemoteRegistryPionPeers(t testing.TB, offerer, answerer *webrtc.PeerConnection) {
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

	waitForRemoteRegistryConnectionState(t, offerer, webrtc.PeerConnectionStateConnected)
	waitForRemoteRegistryConnectionState(t, answerer, webrtc.PeerConnectionStateConnected)
}

func waitForRemoteRegistryConnectionState(t testing.TB, pc *webrtc.PeerConnection, want webrtc.PeerConnectionState) {
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

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		if pc.ConnectionState() != want {
			t.Fatalf("timed out waiting for %s, got %s", want, pc.ConnectionState())
		}
	}
}

func mustEncodeRemoteRegistryVideoPackets(t testing.TB, codecType codec.Type, payloadType webrtc.PayloadType, ssrc webrtc.SSRC, timestamps []uint32) []*rtp.Packet {
	t.Helper()

	enc := mustNewRemoteRegistryVideoEncoder(t, codecType)
	defer func() {
		if err := enc.Close(); err != nil {
			t.Fatalf("encoder.Close: %v", err)
		}
	}()

	packetizer := rtp.NewPacketizer(
		1200,
		uint8(payloadType),
		uint32(ssrc),
		&pioncodecs.VP8Payloader{},
		rtp.NewRandomSequencer(),
		codecType.ClockRate(),
	)
	src := newRemoteRegistryVideoFrame(remoteRegistryTestWidth, remoteRegistryTestHeight)
	encoded := make([]byte, enc.MaxEncodedSize())

	var packets []*rtp.Packet
	for i, ts := range timestamps {
		src.PTS = ts
		result, err := encodeRemoteRegistryVideoUntilOutput(enc, src, encoded, i == 0)
		if err != nil {
			t.Fatalf("encodeRemoteRegistryVideoUntilOutput(%s): %v", codecType, err)
		}
		delta := uint32(remoteRegistryTestClockRate / 30)
		if i+1 < len(timestamps) {
			delta = timestamps[i+1] - timestamps[i]
		}
		packets = append(packets, packetizer.Packetize(encoded[:result.N], delta)...)
	}
	return packets
}

func mustNewRemoteRegistryVideoEncoder(t testing.TB, codecType codec.Type) encoder.VideoEncoder {
	t.Helper()

	switch codecType {
	case codec.VP8:
		enc, err := encoder.NewVP8Encoder(codec.VP8Config{
			Width:   remoteRegistryTestWidth,
			Height:  remoteRegistryTestHeight,
			Bitrate: 300_000,
			FPS:     30,
		})
		if err != nil {
			t.Fatalf("NewVP8Encoder: %v", err)
		}
		return enc
	default:
		t.Fatalf("unsupported integration codec %v", codecType)
		return nil
	}
}

func encodeRemoteRegistryVideoUntilOutput(enc encoder.VideoEncoder, src *frame.VideoFrame, dst []byte, forceKeyframe bool) (encoder.EncodeResult, error) {
	basePTS := src.PTS
	defer func() { src.PTS = basePTS }()

	for i := 0; i < remoteRegistryEncodeAttempts; i++ {
		src.PTS = basePTS + uint32(i*remoteRegistryEncodeRetry)
		result, err := enc.EncodeInto(src, dst, forceKeyframe)
		if err == nil && result.N > 0 {
			return result, nil
		}
		if err != nil && !errors.Is(err, ffi.ErrNeedMoreData) {
			return encoder.EncodeResult{}, err
		}
	}

	return encoder.EncodeResult{}, ffi.ErrNeedMoreData
}

func newRemoteRegistryVideoFrame(width, height int) *frame.VideoFrame {
	f := frame.NewI420Frame(width, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			f.Data[0][y*f.Stride[0]+x] = byte((x + y) % 256)
		}
	}
	uvWidth := (width + 1) / 2
	uvHeight := (height + 1) / 2
	for y := 0; y < uvHeight; y++ {
		for x := 0; x < uvWidth; x++ {
			f.Data[1][y*f.Stride[1]+x] = byte((64 + x + y) % 256)
			f.Data[2][y*f.Stride[2]+x] = byte((192 + x + y) % 256)
		}
	}
	return f
}
