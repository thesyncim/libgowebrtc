// Example: libgowebrtc publisher/subscriber with a Pion SFU-style relay.
//
// This example demonstrates:
// 1. Producing video with libgowebrtc and switching codecs every 2 seconds
// 2. Negotiating the full codec envelope once, up front
// 3. Relaying raw RTP through a Pion "SFU" without renegotiation
// 4. Consuming and decoding the changing codec stream with libgowebrtc's pionrecv bridge
//
// Run:
//
//	go run ./examples/pion_sfu_codec_switch
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/packetizer"
	"github.com/thesyncim/libgowebrtc/pkg/pioncodec"
	"github.com/thesyncim/libgowebrtc/pkg/pionrecv"
	libtrack "github.com/thesyncim/libgowebrtc/pkg/track"
)

var (
	runDuration    = flag.Duration("duration", 12*time.Second, "how long the example should run")
	switchInterval = flag.Duration("switch-interval", 2*time.Second, "how often the publisher swaps codecs")
	videoWidth     = flag.Int("width", 320, "video width")
	videoHeight    = flag.Int("height", 240, "video height")
	videoFPS       = flag.Int("fps", 10, "video frames per second")
	videoBitrate   = flag.Uint("bitrate", 400_000, "target bitrate in bps")
)

var codecCycle = []codec.Type{
	codec.VP8,
	codec.H264,
	codec.VP9,
}

type exampleConfig struct {
	Duration       time.Duration
	SwitchInterval time.Duration
	Width          int
	Height         int
	FPS            int
	Bitrate        uint32
}

type exampleStats struct {
	PublisherSwitches    int64
	SFUSwitches          int64
	SubscriberSwitches   int64
	DecodedFrames        int64
	PostConnectRenegotes int64
}

func main() {
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	stats, err := runExample(exampleConfig{
		Duration:       *runDuration,
		SwitchInterval: *switchInterval,
		Width:          *videoWidth,
		Height:         *videoHeight,
		FPS:            *videoFPS,
		Bitrate:        uint32(*videoBitrate),
	})
	if err != nil {
		log.Fatalf("example failed: %v", err)
	}

	log.Printf("Summary: publisher codec switches=%d, SFU observed switches=%d, subscriber decoder switches=%d, decoded frames=%d, post-connect renegotiations=%d",
		stats.PublisherSwitches,
		stats.SFUSwitches,
		stats.SubscriberSwitches,
		stats.DecodedFrames,
		stats.PostConnectRenegotes,
	)
}

func runExample(cfg exampleConfig) (exampleStats, error) {
	if cfg.Duration <= 0 {
		return exampleStats{}, errors.New("duration must be > 0")
	}
	if cfg.SwitchInterval <= 0 {
		return exampleStats{}, errors.New("switch interval must be > 0")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return exampleStats{}, errors.New("video dimensions must be > 0")
	}
	if cfg.FPS <= 0 {
		return exampleStats{}, errors.New("fps must be > 0")
	}

	log.Printf("Starting libgowebrtc <-> Pion SFU codec-switch example")
	log.Printf("Codec cycle: %s", joinCodecNames(codecCycle))
	log.Printf("Codec switches reuse the first negotiation only; no renegotiation is performed")
	log.Printf("Subscriber decode envelope is derived from the Chrome browser preset and then filtered to the active cycle")

	errCh := make(chan error, 8)
	reportAsyncError := func(label string, err error) {
		if err == nil || isExpectedClose(err) {
			return
		}
		select {
		case errCh <- fmt.Errorf("%s: %w", label, err):
		default:
		}
	}

	publisherPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return exampleStats{}, fmt.Errorf("create publisher PC: %w", err)
	}
	sfuUpstreamPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		_ = publisherPC.Close()
		return exampleStats{}, fmt.Errorf("create SFU upstream PC: %w", err)
	}
	sfuDownstreamPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		_ = publisherPC.Close()
		_ = sfuUpstreamPC.Close()
		return exampleStats{}, fmt.Errorf("create SFU downstream PC: %w", err)
	}
	subscriberPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		_ = publisherPC.Close()
		_ = sfuUpstreamPC.Close()
		_ = sfuDownstreamPC.Close()
		return exampleStats{}, fmt.Errorf("create subscriber PC: %w", err)
	}

	var (
		publisherRef             atomic.Pointer[codecSwitchingPublisher]
		postConnectRenegotiation atomic.Int64
		negotiationWatchArmed    atomic.Bool
		shutdownOnce             sync.Once
	)
	shutdown := func() {
		shutdownOnce.Do(func() {
			if publisher := publisherRef.Swap(nil); publisher != nil {
				publisher.Close()
			}
			closePeer("publisher", publisherPC)
			closePeer("sfu-upstream", sfuUpstreamPC)
			closePeer("sfu-downstream", sfuDownstreamPC)
			closePeer("subscriber", subscriberPC)
		})
	}
	defer shutdown()

	logConnectionState("publisher", publisherPC)
	logConnectionState("sfu-upstream", sfuUpstreamPC)
	logConnectionState("sfu-downstream", sfuDownstreamPC)
	logConnectionState("subscriber", subscriberPC)

	installNegotiationWatch := func(label string, pc *webrtc.PeerConnection) {
		pc.OnNegotiationNeeded(func() {
			if !negotiationWatchArmed.Load() {
				return
			}
			postConnectRenegotiation.Add(1)
			log.Printf("[%s] unexpected negotiationneeded after initial connect", label)
		})
	}
	installNegotiationWatch("publisher", publisherPC)
	installNegotiationWatch("sfu-upstream", sfuUpstreamPC)
	installNegotiationWatch("sfu-downstream", sfuDownstreamPC)
	installNegotiationWatch("subscriber", subscriberPC)

	upstreamRecv, err := sfuUpstreamPC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	if err != nil {
		return exampleStats{}, fmt.Errorf("add SFU upstream recv transceiver: %w", err)
	}
	if err := upstreamRecv.SetCodecPreferences(codecPreferences(codecCycle)); err != nil {
		return exampleStats{}, fmt.Errorf("set SFU upstream codec preferences: %w", err)
	}

	subscriberRecv, err := subscriberPC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	if err != nil {
		return exampleStats{}, fmt.Errorf("add subscriber recv transceiver: %w", err)
	}
	subscriberPreset := pioncodec.BrowserPreset(pioncodec.BrowserChrome, pioncodec.DirectionDecode, pioncodec.PresetModeSupported)
	if err := subscriberRecv.SetCodecPreferences(filterCodecParametersByTypes(subscriberPreset.VideoCodecs(), codecCycle)); err != nil {
		return exampleStats{}, fmt.Errorf("set subscriber codec preferences: %w", err)
	}

	initialCodec := codecCycle[0]

	publisherTrack := newPassthroughRTPTrack(codecCapabilityFor(initialCodec), "publisher-video", "publisher-stream")
	publisherSender, err := publisherPC.AddTrack(publisherTrack)
	if err != nil {
		return exampleStats{}, fmt.Errorf("publisher AddTrack: %w", err)
	}
	go drainRTCP("publisher", publisherSender)

	publisherTransceiver := findSenderTransceiver(publisherPC, publisherSender)
	if publisherTransceiver == nil {
		return exampleStats{}, errors.New("publisher transceiver not found")
	}
	if err := publisherTransceiver.SetCodecPreferences(codecPreferences(codecCycle)); err != nil {
		return exampleStats{}, fmt.Errorf("set publisher codec preferences: %w", err)
	}

	relayTrack := newPassthroughRTPTrack(codecCapabilityFor(initialCodec), "relay-video", "relay-stream")
	relaySender, err := sfuDownstreamPC.AddTrack(relayTrack)
	if err != nil {
		return exampleStats{}, fmt.Errorf("SFU downstream AddTrack: %w", err)
	}
	go drainRTCP("sfu-downstream", relaySender)

	relayTransceiver := findSenderTransceiver(sfuDownstreamPC, relaySender)
	if relayTransceiver == nil {
		return exampleStats{}, errors.New("SFU downstream transceiver not found")
	}
	if err := relayTransceiver.SetCodecPreferences(codecPreferences(codecCycle)); err != nil {
		return exampleStats{}, fmt.Errorf("set SFU downstream codec preferences: %w", err)
	}

	var (
		sfuSwitches        atomic.Int64
		subscriberFrames   atomic.Int64
		subscriberSwitches atomic.Int64
	)

	sfuUpstreamPC.OnTrack(func(remote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("[sfu] upstream track started: id=%s codec=%s pt=%d ssrc=%d", remote.ID(), remote.Codec().MimeType, remote.PayloadType(), remote.SSRC())

		go func() {
			if err := forwardRemoteRTP(remote, receiver, relayTrack, &sfuSwitches, func() {
				if publisher := publisherRef.Load(); publisher != nil {
					publisher.RequestKeyframe()
				}
			}); err != nil {
				reportAsyncError("SFU forwarder", err)
			}
		}()
	})

	subscriberPC.OnTrack(func(remote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("[subscriber] track started: id=%s codec=%s pt=%d ssrc=%d", remote.ID(), remote.Codec().MimeType, remote.PayloadType(), remote.SSRC())
		if publisher := publisherRef.Load(); publisher != nil {
			publisher.RequestKeyframe()
		}

		decoded, err := pionrecv.BindRemoteTrack(remote, receiver, pionrecv.WithRTCPWriter(receiver.Transport()))
		if err != nil {
			reportAsyncError("bind subscriber track", err)
			return
		}

		var logNextFrame atomic.Bool
		logNextFrame.Store(true)

		decoded.SetOnCodecChange(func(change pionrecv.CodecChange) {
			subscriberSwitches.Add(1)
			logNextFrame.Store(true)
			if publisher := publisherRef.Load(); publisher != nil {
				publisher.RequestKeyframe()
			}
			log.Printf("[subscriber] libgowebrtc decoder switched codec: %s(pt=%d) -> %s(pt=%d)",
				change.PreviousCodec.MimeType,
				change.PreviousPayloadType,
				change.CurrentCodec.MimeType,
				change.CurrentPayloadType,
			)
		})

		if err := decoded.SetOnVideoFrame(func(f *frame.VideoFrame) {
			n := subscriberFrames.Add(1)
			if logNextFrame.CompareAndSwap(true, false) || n%30 == 0 {
				log.Printf("[subscriber] decoded frame #%d via libgowebrtc: %dx%d codec=%s", n, f.Width, f.Height, decoded.Codec().String())
			}
		}); err != nil {
			reportAsyncError("set subscriber frame callback", err)
			return
		}

		go func() {
			reportAsyncError("subscriber decoded.Run", decoded.Run())
		}()
	})

	log.Printf("Negotiating subscriber leg first so the relay is ready before publisher traffic starts")
	if err := connectPeers("sfu-downstream", sfuDownstreamPC, "subscriber", subscriberPC); err != nil {
		return exampleStats{}, fmt.Errorf("connect SFU downstream -> subscriber: %w", err)
	}

	log.Printf("Negotiating publisher leg")
	if err := connectPeers("publisher", publisherPC, "sfu-upstream", sfuUpstreamPC); err != nil {
		return exampleStats{}, fmt.Errorf("connect publisher -> SFU upstream: %w", err)
	}

	negotiationWatchArmed.Store(true)

	publisherCodecs := filterNegotiatedCodecs(publisherSender.GetParameters().Codecs)
	log.Printf("[publisher] negotiated codecs: %s", formatCodecs(publisherCodecs))
	log.Printf("[sfu-downstream] negotiated codecs: %s", formatCodecs(relaySender.GetParameters().Codecs))

	publisher, err := newCodecSwitchingPublisher(codecSwitchingPublisherConfig{
		outbound:         publisherTrack,
		negotiatedCodecs: publisherCodecs,
		initialCodec:     initialCodec,
		trackID:          "publisher-video",
		streamID:         "publisher-stream",
		width:            cfg.Width,
		height:           cfg.Height,
		fps:              cfg.FPS,
		bitrate:          cfg.Bitrate,
	})
	if err != nil {
		return exampleStats{}, fmt.Errorf("create codec-switching publisher: %w", err)
	}
	publisherRef.Store(publisher)

	settleDuration := recommendedSettleDuration(cfg.Duration, cfg.SwitchInterval)
	switchDuration := cfg.Duration - settleDuration
	if switchDuration <= 0 {
		switchDuration = cfg.Duration
	}

	runCtx, runCancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer runCancel()
	switchCtx, switchCancel := context.WithTimeout(runCtx, switchDuration)
	defer switchCancel()

	var loops sync.WaitGroup
	loops.Add(1)
	go func() {
		defer loops.Done()
		reportAsyncError("publisher loop", runPublisher(runCtx, publisher))
	}()

	switchDone := make(chan struct{})
	loops.Add(1)
	go func() {
		defer loops.Done()
		defer close(switchDone)
		reportAsyncError("codec switch loop", runCodecSwitcher(switchCtx, publisher, codecCycle, cfg.SwitchInterval))
	}()

	var runErr error
	select {
	case <-switchCtx.Done():
		<-switchDone
		runErr = waitForSwitchConvergence(runCtx, publisher, &sfuSwitches, &subscriberSwitches)
		if runErr == nil {
			select {
			case <-runCtx.Done():
			case runErr = <-errCh:
			}
		}
	case runErr = <-errCh:
	}

	runCancel()
	shutdown()
	loops.Wait()

	if runErr == nil {
		select {
		case runErr = <-errCh:
		default:
		}
	}
	if runErr != nil {
		return exampleStats{}, runErr
	}

	return exampleStats{
		PublisherSwitches:    publisher.switchCount.Load(),
		SFUSwitches:          sfuSwitches.Load(),
		SubscriberSwitches:   subscriberSwitches.Load(),
		DecodedFrames:        subscriberFrames.Load(),
		PostConnectRenegotes: postConnectRenegotiation.Load(),
	}, nil
}

type codecSwitchingPublisherConfig struct {
	outbound         *passthroughRTPTrack
	negotiatedCodecs []webrtc.RTPCodecParameters
	initialCodec     codec.Type
	trackID          string
	streamID         string
	width            int
	height           int
	fps              int
	bitrate          uint32
}

type codecSwitchingPublisher struct {
	mu                sync.Mutex
	currentTrack      *libtrack.VideoTrack
	currentCodec      codec.Type
	forceNextKeyframe bool

	switchCount atomic.Int64

	ctx      *producerTrackLocalContext
	writer   *passthroughTrackWriter
	trackID  string
	streamID string
	width    int
	height   int
	fps      int
	bitrate  uint32
	nextPTS  uint32
	ptsStep  uint32
}

func newCodecSwitchingPublisher(cfg codecSwitchingPublisherConfig) (*codecSwitchingPublisher, error) {
	writer := &passthroughTrackWriter{
		track:         cfg.outbound,
		timestampStep: uint32(90000 / max(cfg.fps, 1)),
	}
	ctx := &producerTrackLocalContext{
		id:         cfg.trackID,
		codecs:     cfg.negotiatedCodecs,
		ssrc:       0x10203040,
		writeTrack: writer,
		rtcpReader: noopRTCPReader{},
	}

	p := &codecSwitchingPublisher{
		ctx:               ctx,
		writer:            writer,
		currentCodec:      cfg.initialCodec,
		forceNextKeyframe: true,
		trackID:           cfg.trackID,
		streamID:          cfg.streamID,
		width:             cfg.width,
		height:            cfg.height,
		fps:               cfg.fps,
		bitrate:           cfg.bitrate,
		ptsStep:           uint32(90000 / max(cfg.fps, 1)),
	}

	track, err := p.newBoundTrack(cfg.initialCodec)
	if err != nil {
		return nil, err
	}
	p.currentTrack = track

	return p, nil
}

func (p *codecSwitchingPublisher) newBoundTrack(codecType codec.Type) (*libtrack.VideoTrack, error) {
	track, err := newLibVideoTrack(p.trackID, p.streamID, codecType, p.width, p.height, p.bitrate, p.fps)
	if err != nil {
		return nil, err
	}

	if _, err := track.Bind(p.ctx); err != nil {
		_ = track.Close()
		return nil, err
	}

	return track, nil
}

func (p *codecSwitchingPublisher) WriteFrame(f *frame.VideoFrame, forceKeyframe bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.currentTrack == nil {
		return io.ErrClosedPipe
	}

	forceKeyframe = forceKeyframe || p.forceNextKeyframe
	err := p.currentTrack.WriteFrame(f, forceKeyframe)
	if err == nil {
		p.forceNextKeyframe = false
	} else if forceKeyframe && errors.Is(err, packetizer.ErrInvalidData) {
		p.forceNextKeyframe = true
	}
	return err
}

func (p *codecSwitchingPublisher) NextFramePTS() uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()

	pts := p.nextPTS
	p.nextPTS += p.ptsStep
	return pts
}

func (p *codecSwitchingPublisher) SwitchCodec(next codec.Type) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.currentCodec == next {
		return nil
	}

	nextTrack, err := p.newBoundTrack(next)
	if err != nil {
		return err
	}

	oldTrack := p.currentTrack
	oldCodec := p.currentCodec

	log.Printf("[publisher] switching codec without renegotiation: %s -> %s", oldCodec.String(), next.String())
	p.currentTrack = nextTrack
	p.currentCodec = next
	p.forceNextKeyframe = true
	p.nextPTS = 0
	p.switchCount.Add(1)
	p.writer.ResetForNewSource()

	if oldTrack != nil {
		_ = oldTrack.Unbind(p.ctx)
		_ = oldTrack.Close()
	}

	return nil
}

func (p *codecSwitchingPublisher) RequestKeyframe() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.forceNextKeyframe = true
}

func (p *codecSwitchingPublisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.currentTrack == nil {
		return
	}

	_ = p.currentTrack.Unbind(p.ctx)
	_ = p.currentTrack.Close()
	p.currentTrack = nil
}

type passthroughBinding struct {
	id          string
	ssrc        webrtc.SSRC
	writeStream webrtc.TrackLocalWriter
}

type passthroughRTPTrack struct {
	mu       sync.RWMutex
	bindings []passthroughBinding
	codec    webrtc.RTPCodecCapability
	id       string
	streamID string
}

func newPassthroughRTPTrack(codecCapability webrtc.RTPCodecCapability, id, streamID string) *passthroughRTPTrack {
	return &passthroughRTPTrack{
		codec:    codecCapability,
		id:       id,
		streamID: streamID,
	}
}

func (t *passthroughRTPTrack) Bind(ctx webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i := range ctx.CodecParameters() {
		params := ctx.CodecParameters()[i]
		if !strings.EqualFold(params.MimeType, t.codec.MimeType) {
			continue
		}

		t.bindings = append(t.bindings, passthroughBinding{
			id:          ctx.ID(),
			ssrc:        ctx.SSRC(),
			writeStream: ctx.WriteStream(),
		})
		return params, nil
	}

	return webrtc.RTPCodecParameters{}, webrtc.ErrUnsupportedCodec
}

func (t *passthroughRTPTrack) Unbind(ctx webrtc.TrackLocalContext) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i := range t.bindings {
		if t.bindings[i].id == ctx.ID() {
			t.bindings[i] = t.bindings[len(t.bindings)-1]
			t.bindings = t.bindings[:len(t.bindings)-1]
			return nil
		}
	}

	return webrtc.ErrUnbindFailed
}

func (t *passthroughRTPTrack) ID() string { return t.id }

func (t *passthroughRTPTrack) RID() string { return "" }

func (t *passthroughRTPTrack) StreamID() string { return t.streamID }

func (t *passthroughRTPTrack) Kind() webrtc.RTPCodecType {
	if strings.HasPrefix(strings.ToLower(t.codec.MimeType), "audio/") {
		return webrtc.RTPCodecTypeAudio
	}
	return webrtc.RTPCodecTypeVideo
}

func (t *passthroughRTPTrack) WriteRTP(packet *rtp.Packet) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var writeErrs []error
	for _, binding := range t.bindings {
		header := packet.Header
		header.SSRC = uint32(binding.ssrc)
		if packet.PaddingSize != 0 && header.PaddingSize == 0 {
			header.PaddingSize = packet.PaddingSize
		}

		if _, err := binding.writeStream.WriteRTP(&header, packet.Payload); err != nil {
			writeErrs = append(writeErrs, err)
		}
	}

	return errors.Join(writeErrs...)
}

func (t *passthroughRTPTrack) Write(raw []byte) (int, error) {
	packet := &rtp.Packet{}
	if err := packet.Unmarshal(raw); err != nil {
		return 0, err
	}

	return len(raw), t.WriteRTP(packet)
}

type passthroughTrackWriter struct {
	track *passthroughRTPTrack

	mu                 sync.Mutex
	nextSequence       uint16
	lastOutTimestamp   uint32
	timestampOffset    uint32
	timestampStep      uint32
	started            bool
	pendingSourceReset bool
}

func (w *passthroughTrackWriter) WriteRTP(header *rtp.Header, payload []byte) (int, error) {
	packet := &rtp.Packet{
		Header:  *header,
		Payload: append([]byte(nil), payload...),
	}

	w.mu.Lock()
	if !w.started {
		w.timestampOffset = 0
		w.started = true
	} else if w.pendingSourceReset {
		w.timestampOffset = w.lastOutTimestamp + w.timestampStep - packet.Timestamp
		w.pendingSourceReset = false
	}
	packet.SequenceNumber = w.nextSequence
	w.nextSequence++
	packet.Timestamp += w.timestampOffset
	w.lastOutTimestamp = packet.Timestamp
	w.mu.Unlock()

	return len(payload), w.track.WriteRTP(packet)
}

func (w *passthroughTrackWriter) Write(raw []byte) (int, error) {
	packet := &rtp.Packet{}
	if err := packet.Unmarshal(raw); err != nil {
		return 0, err
	}

	if _, err := w.WriteRTP(&packet.Header, packet.Payload); err != nil {
		return 0, err
	}
	return len(raw), nil
}

func (w *passthroughTrackWriter) ResetForNewSource() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pendingSourceReset = true
}

type producerTrackLocalContext struct {
	id         string
	codecs     []webrtc.RTPCodecParameters
	ssrc       webrtc.SSRC
	writeTrack webrtc.TrackLocalWriter
	rtcpReader interceptor.RTCPReader
}

func (c *producerTrackLocalContext) CodecParameters() []webrtc.RTPCodecParameters {
	return c.codecs
}

func (c *producerTrackLocalContext) HeaderExtensions() []webrtc.RTPHeaderExtensionParameter {
	return nil
}

func (c *producerTrackLocalContext) SSRC() webrtc.SSRC { return c.ssrc }

func (c *producerTrackLocalContext) SSRCRetransmission() webrtc.SSRC { return 0 }

func (c *producerTrackLocalContext) SSRCForwardErrorCorrection() webrtc.SSRC { return 0 }

func (c *producerTrackLocalContext) WriteStream() webrtc.TrackLocalWriter { return c.writeTrack }

func (c *producerTrackLocalContext) ID() string { return c.id }

func (c *producerTrackLocalContext) RTCPReader() interceptor.RTCPReader { return c.rtcpReader }

type noopRTCPReader struct{}

func (noopRTCPReader) Read([]byte, interceptor.Attributes) (int, interceptor.Attributes, error) {
	return 0, nil, io.EOF
}

func runPublisher(ctx context.Context, publisher *codecSwitchingPublisher) error {
	f := frame.NewI420Frame(publisher.width, publisher.height)
	ticker := time.NewTicker(time.Second / time.Duration(max(publisher.fps, 1)))
	defer ticker.Stop()

	frameNum := 0
	deadline, hasDeadline := ctx.Deadline()

	for {
		select {
		case <-ctx.Done():
			return nil
		case tickTime := <-ticker.C:
			if hasDeadline && !tickTime.Before(deadline) {
				return nil
			}
			generateTestPattern(f, frameNum)
			f.PTS = publisher.NextFramePTS()
			f.Timestamp = time.Duration(f.PTS) * time.Second / 90000
			// This demo forces keyframes for each synthetic frame so rapid codec
			// swaps stay robust across the whole pipeline.
			forceKeyframe := true
			frameNum++

			if err := publisher.WriteFrame(f, forceKeyframe); err != nil {
				if errors.Is(err, packetizer.ErrInvalidData) {
					continue
				}
				return err
			}
		}
	}
}

func runCodecSwitcher(ctx context.Context, publisher *codecSwitchingPublisher, cycle []codec.Type, interval time.Duration) error {
	if len(cycle) < 2 {
		return nil
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	index := 0
	deadline, hasDeadline := ctx.Deadline()
	for {
		select {
		case <-ctx.Done():
			return nil
		case tickTime := <-ticker.C:
			if hasDeadline && !tickTime.Before(deadline) {
				return nil
			}
			if ctx.Err() != nil {
				return nil
			}
			index = (index + 1) % len(cycle)
			if err := publisher.SwitchCodec(cycle[index]); err != nil {
				return err
			}
		}
	}
}

func forwardRemoteRTP(remote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver, relay *passthroughRTPTrack, switchCount *atomic.Int64, requestKeyframe func()) error {
	_ = receiver

	lastCodec := remote.Codec().RTPCodecCapability
	lastPayloadType := remote.PayloadType()

	for {
		packet, _, err := remote.ReadRTP()
		if err != nil {
			return err
		}

		currentCodec := remote.Codec().RTPCodecCapability
		currentPayloadType := remote.PayloadType()
		if currentPayloadType != lastPayloadType || !sameCodecCapability(lastCodec, currentCodec) {
			switchCount.Add(1)
			log.Printf("[sfu] observed upstream codec switch: %s(pt=%d) -> %s(pt=%d)",
				lastCodec.MimeType,
				lastPayloadType,
				currentCodec.MimeType,
				currentPayloadType,
			)
			if requestKeyframe != nil {
				requestKeyframe()
			}
			lastCodec = currentCodec
			lastPayloadType = currentPayloadType
		}

		if err := relay.WriteRTP(packet); err != nil {
			return err
		}
	}
}

func newLibVideoTrack(trackID, streamID string, codecType codec.Type, width, height int, bitrate uint32, fps int) (*libtrack.VideoTrack, error) {
	return libtrack.NewVideoTrack(libtrack.VideoTrackConfig{
		ID:       trackID,
		StreamID: streamID,
		Codec:    codecType,
		Width:    width,
		Height:   height,
		Bitrate:  bitrate,
		FPS:      float64(max(fps, 1)),
	})
}

func connectPeers(offererLabel string, offerer *webrtc.PeerConnection, answererLabel string, answerer *webrtc.PeerConnection) error {
	offerGatherDone := webrtc.GatheringCompletePromise(offerer)
	offer, err := offerer.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("%s CreateOffer: %w", offererLabel, err)
	}
	if err := offerer.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("%s SetLocalDescription(offer): %w", offererLabel, err)
	}
	<-offerGatherDone

	if err := answerer.SetRemoteDescription(*offerer.LocalDescription()); err != nil {
		return fmt.Errorf("%s SetRemoteDescription(offer): %w", answererLabel, err)
	}

	answerGatherDone := webrtc.GatheringCompletePromise(answerer)
	answer, err := answerer.CreateAnswer(nil)
	if err != nil {
		return fmt.Errorf("%s CreateAnswer: %w", answererLabel, err)
	}
	if err := answerer.SetLocalDescription(answer); err != nil {
		return fmt.Errorf("%s SetLocalDescription(answer): %w", answererLabel, err)
	}
	<-answerGatherDone

	if err := offerer.SetRemoteDescription(*answerer.LocalDescription()); err != nil {
		return fmt.Errorf("%s SetRemoteDescription(answer): %w", offererLabel, err)
	}

	if err := waitForConnected(offererLabel, offerer, 10*time.Second); err != nil {
		return err
	}
	if err := waitForConnected(answererLabel, answerer, 10*time.Second); err != nil {
		return err
	}
	return nil
}

func waitForConnected(label string, pc *webrtc.PeerConnection, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		switch pc.ConnectionState() {
		case webrtc.PeerConnectionStateConnected:
			return nil
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			return fmt.Errorf("%s connection state=%s", label, pc.ConnectionState())
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("%s timed out waiting for connected state (current=%s)", label, pc.ConnectionState())
}

func findSenderTransceiver(pc *webrtc.PeerConnection, sender *webrtc.RTPSender) *webrtc.RTPTransceiver {
	for _, transceiver := range pc.GetTransceivers() {
		if transceiver.Sender() == sender {
			return transceiver
		}
	}
	return nil
}

func filterNegotiatedCodecs(codecs []webrtc.RTPCodecParameters) []webrtc.RTPCodecParameters {
	filtered := make([]webrtc.RTPCodecParameters, 0, len(codecs))
	for _, c := range codecs {
		if strings.EqualFold(c.MimeType, webrtc.MimeTypeRTX) {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered
}

func codecPreferences(cycle []codec.Type) []webrtc.RTPCodecParameters {
	preferences := make([]webrtc.RTPCodecParameters, 0, len(cycle))
	for _, c := range cycle {
		preferences = append(preferences, webrtc.RTPCodecParameters{
			RTPCodecCapability: codecCapabilityFor(c),
		})
	}
	return preferences
}

func filterCodecParametersByTypes(codecs []webrtc.RTPCodecParameters, allowed []codec.Type) []webrtc.RTPCodecParameters {
	filtered := make([]webrtc.RTPCodecParameters, 0, len(codecs))
	for _, candidate := range codecs {
		codecType, ok := codec.ParseMimeType(candidate.MimeType)
		if !ok {
			continue
		}
		for _, allowedType := range allowed {
			if codecType == allowedType {
				filtered = append(filtered, candidate)
				break
			}
		}
	}
	return filtered
}

func codecCapabilityFor(c codec.Type) webrtc.RTPCodecCapability {
	switch c {
	case codec.H264:
		return webrtc.RTPCodecCapability{
			MimeType:    c.MimeType(),
			ClockRate:   uint32(c.ClockRate()),
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=" + string(codec.H264ProfileConstrainedBase),
		}
	default:
		return webrtc.RTPCodecCapability{
			MimeType:  c.MimeType(),
			ClockRate: uint32(c.ClockRate()),
		}
	}
}

func sameCodecCapability(a, b webrtc.RTPCodecCapability) bool {
	return strings.EqualFold(a.MimeType, b.MimeType) &&
		a.ClockRate == b.ClockRate &&
		a.Channels == b.Channels &&
		a.SDPFmtpLine == b.SDPFmtpLine
}

func formatCodecs(codecs []webrtc.RTPCodecParameters) string {
	parts := make([]string, 0, len(codecs))
	for _, c := range codecs {
		if strings.EqualFold(c.MimeType, webrtc.MimeTypeRTX) {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s(pt=%d)", c.MimeType, c.PayloadType))
	}
	if len(parts) == 0 {
		return "<none>"
	}
	return strings.Join(parts, ", ")
}

func joinCodecNames(codecs []codec.Type) string {
	parts := make([]string, 0, len(codecs))
	for _, c := range codecs {
		parts = append(parts, c.String())
	}
	return strings.Join(parts, " -> ")
}

func waitForSwitchConvergence(ctx context.Context, publisher *codecSwitchingPublisher, sfuSwitches, subscriberSwitches *atomic.Int64) error {
	if publisher == nil {
		return nil
	}

	target := publisher.switchCount.Load()
	if target == 0 {
		return nil
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		currentSFU := sfuSwitches.Load()
		currentSubscriber := subscriberSwitches.Load()
		if currentSFU >= target && currentSubscriber >= target {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("codec switch convergence timeout: publisher=%d sfu=%d subscriber=%d", target, currentSFU, currentSubscriber)
		default:
		}
		publisher.RequestKeyframe()
		select {
		case <-ctx.Done():
			return fmt.Errorf("codec switch convergence timeout: publisher=%d sfu=%d subscriber=%d", target, currentSFU, currentSubscriber)
		case <-ticker.C:
		}
	}
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func recommendedSettleDuration(total, interval time.Duration) time.Duration {
	settle := maxDuration(2*time.Second, interval)
	if total <= settle {
		return total / 3
	}
	return settle
}

func drainRTCP(label string, sender *webrtc.RTPSender) {
	if sender == nil {
		return
	}

	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			if !isExpectedClose(err) {
				log.Printf("[%s] RTCP reader stopped: %v", label, err)
			}
			return
		}
	}
}

func logConnectionState(label string, pc *webrtc.PeerConnection) {
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[%s] connection state: %s", label, state.String())
	})
}

func closePeer(label string, pc *webrtc.PeerConnection) {
	if pc == nil {
		return
	}
	if err := pc.Close(); err != nil && !isExpectedClose(err) {
		log.Printf("[%s] close error: %v", label, err)
	}
}

func isExpectedClose(err error) bool {
	return err == nil ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrClosedPipe)
}

func generateTestPattern(f *frame.VideoFrame, frameNum int) {
	y := f.YPlane()
	u := f.UPlane()
	v := f.VPlane()

	w := f.Width
	h := f.Height
	offset := frameNum % 256

	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			value := (col + row + offset) % 256
			barY := (frameNum * 3) % h
			if row >= barY && row < barY+16 {
				value = 255
			}
			y[row*w+col] = uint8(value)
		}
	}

	uvW := w / 2
	uvH := h / 2
	uVal := uint8(128 + 100*math.Sin(float64(frameNum)*0.08))
	vVal := uint8(128 + 100*math.Cos(float64(frameNum)*0.08))

	for row := 0; row < uvH; row++ {
		for col := 0; col < uvW; col++ {
			u[row*uvW+col] = uVal
			v[row*uvW+col] = vVal
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var _ webrtc.TrackLocal = (*passthroughRTPTrack)(nil)
var _ webrtc.TrackLocalWriter = (*passthroughTrackWriter)(nil)
var _ webrtc.TrackLocalContext = (*producerTrackLocalContext)(nil)
var _ interceptor.RTCPReader = (*noopRTCPReader)(nil)
