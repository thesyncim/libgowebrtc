package pionrecv

import (
	"errors"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	pioncodecs "github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/decoder"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

const (
	testVideoWidth      = 160
	testVideoHeight     = 90
	testVideoClockRate  = 90000
	testAudioClockRate  = 48000
	testAudioChannels   = 2
	testAudioFrameMs    = 20
	testEncodeRetryStep = 3000
	testEncodeAttempts  = 5
	testTimeout         = 2 * time.Second
)

func TestDecodedTrackDecodeVideoFromPionRTP(t *testing.T) {
	track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, mustCodecParams(t, codec.VP8, 96), 0x10203040)
	packets := mustEncodeVideoPackets(t, codec.VP8, 96, track.SSRC(), []uint32{0, 3000, 6000})
	track.enqueuePackets(packets, track.Codec(), track.PayloadType())
	track.close()

	decoded, err := newDecodedTrack(track, nil, nil)
	if err != nil {
		t.Fatalf("newDecodedTrack: %v", err)
	}

	var (
		gotMu     sync.Mutex
		gotFrames []*frame.VideoFrame
		gotMimes  []string
	)
	if err := decoded.SetOnVideoFrame(func(f *frame.VideoFrame) {
		gotMu.Lock()
		defer gotMu.Unlock()
		gotFrames = append(gotFrames, f)
		gotMimes = append(gotMimes, decoded.CodecParameters().MimeType)
	}); err != nil {
		t.Fatalf("SetOnVideoFrame: %v", err)
	}

	if err := decoded.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	gotMu.Lock()
	defer gotMu.Unlock()

	if len(gotFrames) == 0 {
		t.Fatal("expected at least one decoded video frame")
	}
	last := gotFrames[len(gotFrames)-1]
	if last.Width != testVideoWidth || last.Height != testVideoHeight {
		t.Fatalf("decoded frame size = %dx%d, want %dx%d", last.Width, last.Height, testVideoWidth, testVideoHeight)
	}
	if decoded.Kind() != webrtc.RTPCodecTypeVideo {
		t.Fatalf("Kind() = %v, want video", decoded.Kind())
	}
	if decoded.Codec() != codec.VP8 {
		t.Fatalf("Codec() = %v, want VP8", decoded.Codec())
	}
	if decoded.CodecParameters().MimeType != webrtc.MimeTypeVP8 {
		t.Fatalf("CodecParameters().MimeType = %q, want %q", decoded.CodecParameters().MimeType, webrtc.MimeTypeVP8)
	}
	if decoded.PayloadType() != 96 {
		t.Fatalf("PayloadType() = %d, want 96", decoded.PayloadType())
	}
	if !slices.Contains(gotMimes, webrtc.MimeTypeVP8) {
		t.Fatalf("decoded MIME history = %v, want %q", gotMimes, webrtc.MimeTypeVP8)
	}
}

func TestDecodedTrackDecodeAudioFromPionRTP(t *testing.T) {
	track := newFakeTrackReader(webrtc.RTPCodecTypeAudio, mustCodecParams(t, codec.Opus, 111), 0x01020304)
	packets := mustEncodeAudioPackets(t, 111, track.SSRC(), []uint32{0, 960})
	track.enqueuePackets(packets, track.Codec(), track.PayloadType())
	track.close()

	decoded, err := newDecodedTrack(track, nil, nil)
	if err != nil {
		t.Fatalf("newDecodedTrack: %v", err)
	}

	var got []*frame.AudioFrame
	if err := decoded.SetOnAudioFrame(func(f *frame.AudioFrame) {
		got = append(got, f)
	}); err != nil {
		t.Fatalf("SetOnAudioFrame: %v", err)
	}

	if err := decoded.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(got) == 0 {
		t.Fatal("expected at least one decoded audio frame")
	}
	last := got[len(got)-1]
	if last.SampleRate != testAudioClockRate {
		t.Fatalf("SampleRate = %d, want %d", last.SampleRate, testAudioClockRate)
	}
	if last.Channels != testAudioChannels {
		t.Fatalf("Channels = %d, want %d", last.Channels, testAudioChannels)
	}
	if last.NumSamples == 0 {
		t.Fatal("expected decoded audio samples")
	}
}

func TestNewDecodedTrackValidation(t *testing.T) {
	t.Run("NilTrack", func(t *testing.T) {
		decoded, err := New(nil)
		if !errors.Is(err, ErrNilTrack) {
			t.Fatalf("New(nil) error = %v, want %v", err, ErrNilTrack)
		}
		if decoded != nil {
			t.Fatal("expected nil decoded track")
		}
	})

	t.Run("InvalidMaxVideoDimensions", func(t *testing.T) {
		track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, mustCodecParams(t, codec.VP8, 96), 0x1001)
		decoded, err := newDecodedTrack(track, nil, nil, WithMaxVideoDimensions(0, testVideoHeight))
		if err == nil {
			t.Fatal("expected error for invalid max video dimensions")
		}
		if decoded != nil {
			t.Fatal("expected nil decoded track")
		}
	})

	t.Run("UnsupportedMimeType", func(t *testing.T) {
		track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  "video/red",
				ClockRate: testVideoClockRate,
			},
			PayloadType: 116,
		}, 0x1002)
		decoded, err := newDecodedTrack(track, nil, nil)
		if !errors.Is(err, decoder.ErrUnsupportedCodec) {
			t.Fatalf("newDecodedTrack error = %v, want %v", err, decoder.ErrUnsupportedCodec)
		}
		if decoded != nil {
			t.Fatal("expected nil decoded track")
		}
	})

	for _, codecType := range []codec.Type{codec.PCMU, codec.PCMA} {
		t.Run(codecType.String(), func(t *testing.T) {
			track := newFakeTrackReader(webrtc.RTPCodecTypeAudio, mustCodecParams(t, codecType, 0), 0x1003)
			decoded, err := newDecodedTrack(track, nil, nil)
			if !errors.Is(err, decoder.ErrUnsupportedCodec) {
				t.Fatalf("newDecodedTrack error = %v, want %v", err, decoder.ErrUnsupportedCodec)
			}
			if decoded != nil {
				t.Fatal("expected nil decoded track")
			}
		})
	}
}

func TestDecodedTrackCodecSwitchAndRTCPIntegration(t *testing.T) {
	vp8Params := mustCodecParams(t, codec.VP8, 96)
	h264Params := mustCodecParams(t, codec.H264, 102)
	track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, vp8Params, 0x55667788)

	track.enqueuePackets(mustEncodeVideoPackets(t, codec.VP8, 96, track.SSRC(), []uint32{0, 3000}), vp8Params, 96)
	track.enqueuePackets(mustEncodeVideoPackets(t, codec.H264, 102, track.SSRC(), []uint32{6000, 9000}), h264Params, 102)
	track.close()

	var (
		rtcpMu   sync.Mutex
		rtcpPkts [][]rtcp.Packet
	)
	writeRTCP := func(pkts []rtcp.Packet) error {
		rtcpMu.Lock()
		defer rtcpMu.Unlock()
		cloned := make([]rtcp.Packet, len(pkts))
		copy(cloned, pkts)
		rtcpPkts = append(rtcpPkts, cloned)
		return nil
	}

	decoded, err := newDecodedTrack(track, nil, nil,
		WithWriteRTCP(writeRTCP),
		WithKeyframeRequestGap(time.Nanosecond),
	)
	if err != nil {
		t.Fatalf("newDecodedTrack: %v", err)
	}

	var (
		videoMu     sync.Mutex
		frameMimes  []string
		switchEvent CodecChange
		switches    int
	)
	if err := decoded.SetOnVideoFrame(func(f *frame.VideoFrame) {
		videoMu.Lock()
		defer videoMu.Unlock()
		frameMimes = append(frameMimes, decoded.CodecParameters().MimeType)
	}); err != nil {
		t.Fatalf("SetOnVideoFrame: %v", err)
	}
	decoded.SetOnCodecChange(func(change CodecChange) {
		videoMu.Lock()
		defer videoMu.Unlock()
		switchEvent = change
		switches++
	})

	if err := decoded.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	videoMu.Lock()
	if switches != 1 {
		videoMu.Unlock()
		t.Fatalf("codec switches = %d, want 1", switches)
	}
	if switchEvent.PreviousType != codec.VP8 || switchEvent.CurrentType != codec.H264 {
		videoMu.Unlock()
		t.Fatalf("codec switch types = %v -> %v, want VP8 -> H264", switchEvent.PreviousType, switchEvent.CurrentType)
	}
	if switchEvent.PreviousCodec.MimeType != webrtc.MimeTypeVP8 || switchEvent.CurrentCodec.MimeType != webrtc.MimeTypeH264 {
		videoMu.Unlock()
		t.Fatalf("codec switch MIME types = %q -> %q", switchEvent.PreviousCodec.MimeType, switchEvent.CurrentCodec.MimeType)
	}
	if !slices.Contains(frameMimes, webrtc.MimeTypeVP8) || !slices.Contains(frameMimes, webrtc.MimeTypeH264) {
		videoMu.Unlock()
		t.Fatalf("decoded frame MIME history = %v, want both VP8 and H264", frameMimes)
	}
	videoMu.Unlock()

	if decoded.CodecParameters().MimeType != webrtc.MimeTypeH264 {
		t.Fatalf("final codec MIME = %q, want %q", decoded.CodecParameters().MimeType, webrtc.MimeTypeH264)
	}
	if decoded.PayloadType() != 102 {
		t.Fatalf("final payload type = %d, want 102", decoded.PayloadType())
	}

	rtcpMu.Lock()
	defer rtcpMu.Unlock()
	if len(rtcpPkts) < 2 {
		t.Fatalf("RTCP write count = %d, want at least 2", len(rtcpPkts))
	}
	for _, batch := range rtcpPkts {
		if len(batch) == 0 {
			t.Fatal("expected non-empty RTCP batch")
		}
		pli, ok := batch[0].(*rtcp.PictureLossIndication)
		if !ok {
			t.Fatalf("expected PLI, got %T", batch[0])
		}
		if pli.MediaSSRC != uint32(track.SSRC()) {
			t.Fatalf("PLI MediaSSRC = %d, want %d", pli.MediaSSRC, track.SSRC())
		}
	}
}

func TestDecodedTrackFlushesFinalSampleOnEOF(t *testing.T) {
	track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, mustCodecParams(t, codec.VP8, 96), 0x55660001)
	track.enqueuePackets(mustEncodeVideoPackets(t, codec.VP8, 96, track.SSRC(), []uint32{0}), track.Codec(), track.PayloadType())
	track.close()

	decoded, err := newDecodedTrack(track, nil, nil)
	if err != nil {
		t.Fatalf("newDecodedTrack: %v", err)
	}

	got := make(chan *frame.VideoFrame, 1)
	if err := decoded.SetOnVideoFrame(func(f *frame.VideoFrame) {
		select {
		case got <- f:
		default:
		}
	}); err != nil {
		t.Fatalf("SetOnVideoFrame: %v", err)
	}

	if err := decoded.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	select {
	case f := <-got:
		if f.Width != testVideoWidth || f.Height != testVideoHeight {
			t.Fatalf("decoded frame size = %dx%d, want %dx%d", f.Width, f.Height, testVideoWidth, testVideoHeight)
		}
	default:
		t.Fatal("expected final sample to be flushed on EOF")
	}
}

func TestDecodedTrackPayloadTypeChangeDoesNotEmitCodecChange(t *testing.T) {
	initialParams := mustCodecParams(t, codec.VP8, 96)
	updatedParams := mustCodecParams(t, codec.VP8, 97)
	track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, initialParams, 0x55660002)
	track.enqueuePackets(mustEncodeVideoPackets(t, codec.VP8, 96, track.SSRC(), []uint32{0, 3000}), initialParams, 96)
	track.enqueuePackets(mustEncodeVideoPackets(t, codec.VP8, 97, track.SSRC(), []uint32{6000, 9000}), updatedParams, 97)
	track.close()

	decoded, err := newDecodedTrack(track, nil, nil)
	if err != nil {
		t.Fatalf("newDecodedTrack: %v", err)
	}

	var (
		switches int
		payloads []webrtc.PayloadType
	)
	decoded.SetOnCodecChange(func(change CodecChange) {
		switches++
	})
	if err := decoded.SetOnVideoFrame(func(f *frame.VideoFrame) {
		payloads = append(payloads, decoded.PayloadType())
	}); err != nil {
		t.Fatalf("SetOnVideoFrame: %v", err)
	}

	if err := decoded.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if switches != 0 {
		t.Fatalf("codec switches = %d, want 0", switches)
	}
	if decoded.PayloadType() != 97 {
		t.Fatalf("final payload type = %d, want 97", decoded.PayloadType())
	}
	if !slices.Contains(payloads, webrtc.PayloadType(96)) || !slices.Contains(payloads, webrtc.PayloadType(97)) {
		t.Fatalf("payload history = %v, want both 96 and 97", payloads)
	}
}

func TestDecodedTrackRequestKeyframe(t *testing.T) {
	t.Run("NoWriter", func(t *testing.T) {
		track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, mustCodecParams(t, codec.VP8, 96), 0x55660003)
		decoded, err := newDecodedTrack(track, nil, nil)
		if err != nil {
			t.Fatalf("newDecodedTrack: %v", err)
		}

		if err := decoded.RequestKeyframe(); !errors.Is(err, ErrKeyframeRequesterUnavailable) {
			t.Fatalf("RequestKeyframe error = %v, want %v", err, ErrKeyframeRequesterUnavailable)
		}
	})

	t.Run("WriterError", func(t *testing.T) {
		expected := errors.New("write failed")
		track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, mustCodecParams(t, codec.VP8, 96), 0x55660004)
		decoded, err := newDecodedTrack(track, nil, nil, WithWriteRTCP(func([]rtcp.Packet) error {
			return expected
		}))
		if err != nil {
			t.Fatalf("newDecodedTrack: %v", err)
		}

		if err := decoded.RequestKeyframe(); !errors.Is(err, expected) {
			t.Fatalf("RequestKeyframe error = %v, want %v", err, expected)
		}
	})
}

func TestDecodedTrackAutomaticPLIRateLimiting(t *testing.T) {
	track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, mustCodecParams(t, codec.VP8, 96), 0x55660005)
	track.enqueuePackets([]*rtp.Packet{
		newVP8RTPPacket(1, 0, track.SSRC(), false),
		newVP8RTPPacket(2, 3000, track.SSRC(), false),
	}, track.Codec(), track.PayloadType())
	track.close()

	var requests int
	decoded, err := newDecodedTrack(track, nil, nil,
		WithWriteRTCP(func([]rtcp.Packet) error {
			requests++
			return nil
		}),
		WithKeyframeRequestGap(time.Hour),
	)
	if err != nil {
		t.Fatalf("newDecodedTrack: %v", err)
	}

	if err := decoded.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if requests != 1 {
		t.Fatalf("automatic keyframe requests = %d, want 1", requests)
	}
}

func TestDecodedTrackCloseBeforeRun(t *testing.T) {
	track := newFakeTrackReader(webrtc.RTPCodecTypeVideo, mustCodecParams(t, codec.VP8, 96), 0x55660006)
	decoded, err := newDecodedTrack(track, nil, nil)
	if err != nil {
		t.Fatalf("newDecodedTrack: %v", err)
	}

	if err := decoded.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := decoded.Close(); err != nil {
		t.Fatalf("Close second call: %v", err)
	}
	if err := decoded.Run(); !errors.Is(err, ErrClosed) {
		t.Fatalf("Run error = %v, want %v", err, ErrClosed)
	}
}

func TestDecodedTrackRunTwice(t *testing.T) {
	track := newBlockingTrackReader(webrtc.RTPCodecTypeVideo, mustCodecParams(t, codec.VP8, 96), 0x55660007)
	decoded, err := newDecodedTrack(track, nil, nil)
	if err != nil {
		t.Fatalf("newDecodedTrack: %v", err)
	}

	runDone := make(chan error, 1)
	go func() {
		runDone <- decoded.Run()
	}()

	track.waitForRead(t)

	if err := decoded.Run(); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Run error = %v, want %v", err, ErrAlreadyRunning)
	}

	if err := decoded.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run exit error = %v, want nil", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for blocked Run to exit")
	}
}

func TestDecodedTrackCloseUnblocksRun(t *testing.T) {
	track := newBlockingTrackReader(webrtc.RTPCodecTypeVideo, mustCodecParams(t, codec.VP8, 96), 0x55660008)
	decoded, err := newDecodedTrack(track, nil, nil)
	if err != nil {
		t.Fatalf("newDecodedTrack: %v", err)
	}

	runDone := make(chan error, 1)
	go func() {
		runDone <- decoded.Run()
	}()

	track.waitForRead(t)

	if err := decoded.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run exit error = %v, want nil", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for Run to exit after Close")
	}
}

type fakeTrackReader struct {
	id          string
	streamID    string
	rid         string
	kind        webrtc.RTPCodecType
	ssrc        webrtc.SSRC
	mu          sync.RWMutex
	codecParams webrtc.RTPCodecParameters
	payloadType webrtc.PayloadType
	events      chan fakeTrackEvent
}

type fakeTrackEvent struct {
	packet      *rtp.Packet
	err         error
	codecParams webrtc.RTPCodecParameters
	payloadType webrtc.PayloadType
}

type blockingTrackReader struct {
	*fakeTrackReader
	readStarted chan struct{}
	unblock     chan struct{}
	unblockOnce sync.Once
}

func newFakeTrackReader(kind webrtc.RTPCodecType, params webrtc.RTPCodecParameters, ssrc webrtc.SSRC) *fakeTrackReader {
	return &fakeTrackReader{
		id:          "remote-track",
		streamID:    "remote-stream",
		kind:        kind,
		ssrc:        ssrc,
		codecParams: params,
		payloadType: params.PayloadType,
		events:      make(chan fakeTrackEvent, 256),
	}
}

func newBlockingTrackReader(kind webrtc.RTPCodecType, params webrtc.RTPCodecParameters, ssrc webrtc.SSRC) *blockingTrackReader {
	return &blockingTrackReader{
		fakeTrackReader: newFakeTrackReader(kind, params, ssrc),
		readStarted:     make(chan struct{}, 1),
		unblock:         make(chan struct{}),
	}
}

func (f *fakeTrackReader) ID() string { return f.id }

func (f *fakeTrackReader) StreamID() string { return f.streamID }

func (f *fakeTrackReader) RID() string { return f.rid }

func (f *fakeTrackReader) Kind() webrtc.RTPCodecType { return f.kind }

func (f *fakeTrackReader) Codec() webrtc.RTPCodecParameters {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.codecParams
}

func (f *fakeTrackReader) PayloadType() webrtc.PayloadType {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.payloadType
}

func (f *fakeTrackReader) SSRC() webrtc.SSRC { return f.ssrc }

func (f *fakeTrackReader) ReadRTP() (*rtp.Packet, error) {
	event, ok := <-f.events
	if !ok {
		return nil, io.EOF
	}

	f.mu.Lock()
	f.codecParams = event.codecParams
	f.payloadType = event.payloadType
	f.mu.Unlock()

	return event.packet, event.err
}

func (f *fakeTrackReader) SetReadDeadline(time.Time) error { return nil }

func (f *fakeTrackReader) enqueuePackets(packets []*rtp.Packet, params webrtc.RTPCodecParameters, payloadType webrtc.PayloadType) {
	for _, packet := range packets {
		f.events <- fakeTrackEvent{
			packet:      packet,
			codecParams: params,
			payloadType: payloadType,
		}
	}
}

func (f *fakeTrackReader) close() {
	close(f.events)
}

func (b *blockingTrackReader) ReadRTP() (*rtp.Packet, error) {
	select {
	case b.readStarted <- struct{}{}:
	default:
	}
	<-b.unblock
	return nil, io.EOF
}

func (b *blockingTrackReader) SetReadDeadline(time.Time) error {
	b.unblockOnce.Do(func() {
		close(b.unblock)
	})
	return nil
}

func (b *blockingTrackReader) waitForRead(t testing.TB) {
	t.Helper()
	select {
	case <-b.readStarted:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for ReadRTP to block")
	}
}

func mustCodecParams(t testing.TB, codecType codec.Type, payloadType webrtc.PayloadType) webrtc.RTPCodecParameters {
	t.Helper()

	params := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  codecType.MimeType(),
			ClockRate: codecType.ClockRate(),
		},
		PayloadType: payloadType,
	}
	if codecType == codec.Opus {
		params.Channels = testAudioChannels
	}
	if codecType == codec.H264 {
		params.SDPFmtpLine = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"
	}
	return params
}

func mustEncodeVideoPackets(t testing.TB, codecType codec.Type, payloadType webrtc.PayloadType, ssrc webrtc.SSRC, timestamps []uint32) []*rtp.Packet {
	t.Helper()

	enc := mustNewVideoEncoder(t, codecType)
	defer func() {
		if err := enc.Close(); err != nil {
			t.Fatalf("encoder.Close: %v", err)
		}
	}()

	pkt := mustNewPacketizer(t, codecType, payloadType, ssrc)
	src := newTestVideoFrame(testVideoWidth, testVideoHeight)
	encoded := make([]byte, enc.MaxEncodedSize())

	var packets []*rtp.Packet
	for i, ts := range timestamps {
		src.PTS = ts
		result, err := encodeVideoUntilOutput(enc, src, encoded, i == 0)
		if err != nil {
			t.Fatalf("encodeVideoUntilOutput(%s): %v", codecType, err)
		}
		packets = append(packets, pkt.Packetize(encoded[:result.N], timestampDelta(timestamps, i, testVideoClockRate/30))...)
	}
	return packets
}

func mustEncodeAudioPackets(t testing.TB, payloadType webrtc.PayloadType, ssrc webrtc.SSRC, timestamps []uint32) []*rtp.Packet {
	t.Helper()

	cfg := codec.DefaultOpusConfig()
	enc, err := encoder.NewOpusEncoder(cfg)
	if err != nil {
		t.Fatalf("NewOpusEncoder: %v", err)
	}
	defer func() {
		if err := enc.Close(); err != nil {
			t.Fatalf("encoder.Close: %v", err)
		}
	}()

	pkt := mustNewPacketizer(t, codec.Opus, payloadType, ssrc)
	samplesPerFrame := testAudioClockRate * testAudioFrameMs / 1000
	src := newTestAudioFrame(samplesPerFrame)
	encoded := make([]byte, enc.MaxEncodedSize())

	var packets []*rtp.Packet
	for i, ts := range timestamps {
		src.PTS = ts
		n, err := enc.EncodeInto(src, encoded)
		if err != nil {
			t.Fatalf("EncodeInto(Opus): %v", err)
		}
		packets = append(packets, pkt.Packetize(encoded[:n], timestampDelta(timestamps, i, samplesPerFrame))...)
	}
	return packets
}

func mustNewVideoEncoder(t testing.TB, codecType codec.Type) encoder.VideoEncoder {
	t.Helper()

	switch codecType {
	case codec.H264:
		enc, err := encoder.NewH264Encoder(codec.H264Config{
			Width:    testVideoWidth,
			Height:   testVideoHeight,
			Bitrate:  300_000,
			FPS:      30,
			PreferHW: false,
		})
		if err != nil {
			t.Fatalf("NewH264Encoder: %v", err)
		}
		return enc
	case codec.VP8:
		enc, err := encoder.NewVP8Encoder(codec.VP8Config{
			Width:   testVideoWidth,
			Height:  testVideoHeight,
			Bitrate: 300_000,
			FPS:     30,
		})
		if err != nil {
			t.Fatalf("NewVP8Encoder: %v", err)
		}
		return enc
	default:
		t.Fatalf("unsupported test codec %v", codecType)
		return nil
	}
}

func mustNewPacketizer(t testing.TB, codecType codec.Type, payloadType webrtc.PayloadType, ssrc webrtc.SSRC) rtp.Packetizer {
	t.Helper()

	return rtp.NewPacketizer(
		1200,
		uint8(payloadType),
		uint32(ssrc),
		mustPayloader(t, codecType),
		rtp.NewRandomSequencer(),
		codecType.ClockRate(),
	)
}

func encodeVideoUntilOutput(enc encoder.VideoEncoder, src *frame.VideoFrame, dst []byte, forceKeyframe bool) (encoder.EncodeResult, error) {
	basePTS := src.PTS
	defer func() { src.PTS = basePTS }()

	for i := 0; i < testEncodeAttempts; i++ {
		src.PTS = basePTS + uint32(i*testEncodeRetryStep)
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

func mustPayloader(t testing.TB, codecType codec.Type) rtp.Payloader {
	t.Helper()

	switch codecType {
	case codec.H264:
		return &pioncodecs.H264Payloader{}
	case codec.VP8:
		return &pioncodecs.VP8Payloader{}
	case codec.Opus:
		return &pioncodecs.OpusPayloader{}
	default:
		t.Fatalf("unsupported RTP payloader codec %v", codecType)
		return nil
	}
}

func timestampDelta(timestamps []uint32, index int, fallback int) uint32 {
	if index+1 < len(timestamps) {
		return timestamps[index+1] - timestamps[index]
	}
	return uint32(fallback)
}

func newTestVideoFrame(width, height int) *frame.VideoFrame {
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

func newTestAudioFrame(numSamples int) *frame.AudioFrame {
	f := frame.NewAudioFrameS16(testAudioClockRate, testAudioChannels, numSamples)
	for i := 0; i < numSamples*testAudioChannels; i++ {
		sample := int16((i * 97) % 32767)
		f.Samples[i*2] = byte(sample)
		f.Samples[i*2+1] = byte(sample >> 8)
	}
	return f
}

func newVP8RTPPacket(sequenceNumber uint16, timestamp uint32, ssrc webrtc.SSRC, keyframe bool) *rtp.Packet {
	frameTag := byte(0x01)
	if keyframe {
		frameTag = 0x00
	}

	return &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: sequenceNumber,
			Timestamp:      timestamp,
			SSRC:           uint32(ssrc),
		},
		Payload: []byte{0x10, frameTag, 0x00, 0x00},
	}
}
