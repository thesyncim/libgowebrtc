package pionrecv

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/internal/testutil"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/packetizer"
	libtrack "github.com/thesyncim/libgowebrtc/pkg/track"
)

const (
	libTrackSmokeWidth   = testVideoWidth
	libTrackSmokeHeight  = testVideoHeight
	libTrackSmokeFPS     = 30
	libTrackSmokeBitrate = 300_000
	libTrackSmokeSSRC    = webrtc.SSRC(0x66778899)
)

func TestBindRemoteTrackWithLibTrackProducedRTP(t *testing.T) {
	testutil.RequireShim(t)

	testCases := []struct {
		name        string
		codecType   codec.Type
		payloadType webrtc.PayloadType
	}{
		{name: "VP8", codecType: codec.VP8, payloadType: 96},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params, packets := mustCaptureLibTrackPackets(t, tc.codecType, tc.payloadType, libTrackSmokeSSRC)

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

			receiverTransceiver, err := receiver.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
			if err != nil {
				t.Fatalf("receiver.AddTransceiverFromKind: %v", err)
			}
			if err := receiverTransceiver.SetCodecPreferences([]webrtc.RTPCodecParameters{params}); err != nil {
				t.Fatalf("receiver.SetCodecPreferences: %v", err)
			}

			outboundTrack, err := webrtc.NewTrackLocalStaticRTP(
				params.RTPCodecCapability,
				"libtrack-produced-video",
				"libtrack-produced-stream",
			)
			if err != nil {
				t.Fatalf("NewTrackLocalStaticRTP: %v", err)
			}

			senderRTPSender, err := sender.AddTrack(outboundTrack)
			if err != nil {
				t.Fatalf("sender.AddTrack: %v", err)
			}
			go drainSenderRTCP(senderRTPSender)

			senderTransceiver := findSenderTransceiverBySender(sender, senderRTPSender)
			if senderTransceiver == nil {
				t.Fatal("sender transceiver not found")
			}
			if err := senderTransceiver.SetCodecPreferences([]webrtc.RTPCodecParameters{params}); err != nil {
				t.Fatalf("sender.SetCodecPreferences: %v", err)
			}

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

			for i, pkt := range packets {
				if err := outboundTrack.WriteRTP(pkt); err != nil {
					t.Fatalf("outboundTrack.WriteRTP(%d): %v", i, err)
				}
				time.Sleep(10 * time.Millisecond)
			}

			var decoded *DecodedTrack
			select {
			case decoded = <-decodedReady:
			case err := <-runErr:
				t.Fatalf("decoded.Run: %v", err)
			case <-time.After(integrationWaitTimeout):
				t.Fatal("timed out waiting for decoded track from libtrack-produced RTP")
			}

			waitForExpectedVideoFrame(t, frames, runErr, tc.codecType.String()+" libtrack-produced RTP")

			if decoded == nil {
				t.Fatal("expected decoded track")
			}
			if decoded.Codec() != tc.codecType {
				t.Fatalf("decoded.Codec() = %v, want %v", decoded.Codec(), tc.codecType)
			}
			if decoded.CodecParameters().MimeType != tc.codecType.MimeType() {
				t.Fatalf("decoded.CodecParameters().MimeType = %q, want %q", decoded.CodecParameters().MimeType, tc.codecType.MimeType())
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
		})
	}
}

func TestDecodedTrackCodecSwitchWithLibTrackProducedRTP(t *testing.T) {
	testutil.RequireShim(t)

	vp8Params, vp8Packets := mustCaptureLibTrackPackets(t, codec.VP8, 96, libTrackSmokeSSRC)
	h264Params, h264Packets := mustCaptureLibTrackPackets(t, codec.H264, 102, libTrackSmokeSSRC)
	vp9Params, vp9Packets := mustCaptureLibTrackPackets(t, codec.VP9, 98, libTrackSmokeSSRC)

	track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, vp8Params, libTrackSmokeSSRC)
	track.enqueuePackets(vp8Packets, vp8Params, vp8Params.PayloadType)
	track.enqueuePackets(h264Packets, h264Params, h264Params.PayloadType)
	track.enqueuePackets(vp9Packets, vp9Params, vp9Params.PayloadType)
	track.close()

	decoded, err := newDecodedTrack(track, nil, nil)
	if err != nil {
		t.Fatalf("newDecodedTrack: %v", err)
	}

	var (
		gotMu      sync.Mutex
		frameMimes []string
		switches   []CodecChange
	)
	if err := decoded.SetOnVideoFrame(func(_ *frame.VideoFrame) {
		gotMu.Lock()
		defer gotMu.Unlock()
		frameMimes = append(frameMimes, decoded.CodecParameters().MimeType)
	}); err != nil {
		t.Fatalf("SetOnVideoFrame: %v", err)
	}
	decoded.SetOnCodecChange(func(change CodecChange) {
		gotMu.Lock()
		defer gotMu.Unlock()
		switches = append(switches, change)
	})

	if err := decoded.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	gotMu.Lock()
	defer gotMu.Unlock()

	if len(frameMimes) == 0 {
		t.Fatal("expected decoded frames from libtrack-produced RTP")
	}
	for _, mimeType := range []string{webrtc.MimeTypeVP8, webrtc.MimeTypeH264, webrtc.MimeTypeVP9} {
		if !containsString(frameMimes, mimeType) {
			t.Fatalf("decoded MIME history = %v, want %q", frameMimes, mimeType)
		}
	}
	if len(switches) != 2 {
		t.Fatalf("codec switches = %d, want 2", len(switches))
	}
	if switches[0].PreviousType != codec.VP8 || switches[0].CurrentType != codec.H264 {
		t.Fatalf("first codec switch types = %v -> %v, want VP8 -> H264", switches[0].PreviousType, switches[0].CurrentType)
	}
	if switches[1].PreviousType != codec.H264 || switches[1].CurrentType != codec.VP9 {
		t.Fatalf("second codec switch types = %v -> %v, want H264 -> VP9", switches[1].PreviousType, switches[1].CurrentType)
	}
	if decoded.Codec() != codec.VP9 {
		t.Fatalf("final decoded.Codec() = %v, want VP9", decoded.Codec())
	}
	if decoded.CodecParameters().MimeType != webrtc.MimeTypeVP9 {
		t.Fatalf("final decoded.CodecParameters().MimeType = %q, want %q", decoded.CodecParameters().MimeType, webrtc.MimeTypeVP9)
	}
}

func mustCaptureLibTrackPackets(t testing.TB, codecType codec.Type, payloadType webrtc.PayloadType, ssrc webrtc.SSRC) (webrtc.RTPCodecParameters, []*rtp.Packet) {
	t.Helper()

	videoTrack, err := libtrack.NewVideoTrack(libtrack.VideoTrackConfig{
		ID:       "smoke-video",
		StreamID: "smoke-stream",
		Codec:    codecType,
		Width:    libTrackSmokeWidth,
		Height:   libTrackSmokeHeight,
		Bitrate:  libTrackSmokeBitrate,
		FPS:      libTrackSmokeFPS,
	})
	if err != nil {
		t.Fatalf("NewVideoTrack(%s): %v", codecType, err)
	}
	defer func() {
		_ = videoTrack.Close()
	}()

	writer := &capturingTrackWriter{}
	ctx := &fakeTrackLocalContext{
		id: "libtrack-smoke",
		codecs: []webrtc.RTPCodecParameters{
			mustCodecParams(t, codecType, payloadType),
		},
		ssrc:       ssrc,
		writeTrack: writer,
		rtcpReader: noopRTCPReader{},
	}

	params, err := videoTrack.Bind(ctx)
	if err != nil {
		t.Fatalf("videoTrack.Bind(%s): %v", codecType, err)
	}
	defer func() {
		if err := videoTrack.Unbind(ctx); err != nil {
			t.Fatalf("videoTrack.Unbind(%s): %v", codecType, err)
		}
	}()

	src := newTestVideoFrame(libTrackSmokeWidth, libTrackSmokeHeight)
	timestamps := []uint32{0, 3000, 6000, 9000}
	for i, ts := range timestamps {
		src.PTS = ts
		if err := writeFrameUntilRTP(videoTrack, writer, src, i == 0, i); err != nil {
			t.Fatalf("writeFrameUntilRTP(%s, frame %d): %v", codecType, i, err)
		}
	}

	packets := writer.Packets()
	if len(packets) == 0 {
		t.Fatalf("expected RTP packets from %s track", codecType)
	}

	return params, packets
}

func writeFrameUntilRTP(track *libtrack.VideoTrack, writer *capturingTrackWriter, src *frame.VideoFrame, forceKeyframe bool, frameIndex int) error {
	basePTS := src.PTS
	defer func() {
		src.PTS = basePTS
	}()

	packetCount := writer.Len()
	for attempt := 0; attempt < testEncodeAttempts; attempt++ {
		src.PTS = basePTS + uint32(attempt*testEncodeRetryStep)
		mutateTestVideoFrame(src, frameIndex*testEncodeAttempts+attempt)

		err := track.WriteFrame(src, forceKeyframe)
		if err == nil && writer.Len() > packetCount {
			return nil
		}
		if err == nil {
			continue
		}
		if errors.Is(err, ffi.ErrNeedMoreData) || errors.Is(err, packetizer.ErrInvalidData) {
			continue
		}
		return err
	}

	return errors.New("no RTP packets generated")
}

func mutateTestVideoFrame(f *frame.VideoFrame, seed int) {
	for y := 0; y < f.Height; y++ {
		for x := 0; x < f.Width; x++ {
			f.Data[0][y*f.Stride[0]+x] = byte((x + y + seed*11) % 256)
		}
	}

	uvWidth := (f.Width + 1) / 2
	uvHeight := (f.Height + 1) / 2
	for y := 0; y < uvHeight; y++ {
		for x := 0; x < uvWidth; x++ {
			f.Data[1][y*f.Stride[1]+x] = byte((64 + x + seed*7) % 256)
			f.Data[2][y*f.Stride[2]+x] = byte((192 + y + seed*5) % 256)
		}
	}
}

type fakeTrackLocalContext struct {
	id         string
	codecs     []webrtc.RTPCodecParameters
	ssrc       webrtc.SSRC
	writeTrack webrtc.TrackLocalWriter
	rtcpReader interceptor.RTCPReader
}

func (f *fakeTrackLocalContext) CodecParameters() []webrtc.RTPCodecParameters {
	return f.codecs
}

func (f *fakeTrackLocalContext) HeaderExtensions() []webrtc.RTPHeaderExtensionParameter {
	return nil
}

func (f *fakeTrackLocalContext) SSRC() webrtc.SSRC { return f.ssrc }

func (f *fakeTrackLocalContext) SSRCRetransmission() webrtc.SSRC { return 0 }

func (f *fakeTrackLocalContext) SSRCForwardErrorCorrection() webrtc.SSRC { return 0 }

func (f *fakeTrackLocalContext) WriteStream() webrtc.TrackLocalWriter {
	return f.writeTrack
}

func (f *fakeTrackLocalContext) ID() string { return f.id }

func (f *fakeTrackLocalContext) RTCPReader() interceptor.RTCPReader {
	return f.rtcpReader
}

type noopRTCPReader struct{}

func (noopRTCPReader) Read([]byte, interceptor.Attributes) (int, interceptor.Attributes, error) {
	return 0, nil, io.EOF
}

type capturingTrackWriter struct {
	mu      sync.Mutex
	packets []*rtp.Packet
}

func (w *capturingTrackWriter) WriteRTP(header *rtp.Header, payload []byte) (int, error) {
	packet := &rtp.Packet{
		Header:  *header,
		Payload: append([]byte(nil), payload...),
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.packets = append(w.packets, packet)
	return len(payload), nil
}

func (w *capturingTrackWriter) Write(data []byte) (int, error) {
	packet := &rtp.Packet{}
	if err := packet.Unmarshal(data); err != nil {
		return 0, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.packets = append(w.packets, cloneRTPPacket(packet))
	return len(data), nil
}

func (w *capturingTrackWriter) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.packets)
}

func (w *capturingTrackWriter) Packets() []*rtp.Packet {
	w.mu.Lock()
	defer w.mu.Unlock()

	out := make([]*rtp.Packet, 0, len(w.packets))
	for _, packet := range w.packets {
		out = append(out, cloneRTPPacket(packet))
	}
	return out
}

func cloneRTPPacket(packet *rtp.Packet) *rtp.Packet {
	if packet == nil {
		return nil
	}

	cloned := *packet
	cloned.Payload = append([]byte(nil), packet.Payload...)
	cloned.Header.Extensions = append([]rtp.Extension(nil), packet.Header.Extensions...)
	cloned.Raw = append([]byte(nil), packet.Raw...)
	return &cloned
}

func findSenderTransceiverBySender(pc *webrtc.PeerConnection, sender *webrtc.RTPSender) *webrtc.RTPTransceiver {
	for _, transceiver := range pc.GetTransceivers() {
		if transceiver.Sender() == sender {
			return transceiver
		}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
