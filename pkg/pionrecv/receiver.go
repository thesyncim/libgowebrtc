// Package pionrecv bridges Pion OnTrack pairs into libgowebrtc decoders.
//
// BindRemoteTrack turns a Pion TrackRemote/RTPReceiver pair into decoded
// pkg/frame callbacks while reusing libgowebrtc's video/audio decoders. The
// bridge uses Pion's RTP sample builder, so callers do not need to manually
// read RTP, depacketize, or instantiate decoders inside OnTrack handlers.
package pionrecv

import (
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	pioncodecs "github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	pionmedia "github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/decoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pioncodec"
)

var (
	// ErrNilTrack is returned when a nil TrackRemote is passed to BindRemoteTrack.
	ErrNilTrack = errors.New("nil TrackRemote")
	// ErrNilReceiver is returned when a nil RTPReceiver is passed to BindRemoteTrack.
	ErrNilReceiver = errors.New("nil RTPReceiver")
	// ErrClosed is returned when operating on a closed decoded track.
	ErrClosed = errors.New("decoded track is closed")
	// ErrAlreadyRunning is returned when Run is invoked more than once.
	ErrAlreadyRunning = errors.New("decoded track is already running")
	// ErrUnsupportedTrackKind is returned when the remote track kind is not audio or video.
	ErrUnsupportedTrackKind = errors.New("unsupported track kind")
	// ErrKeyframeRequesterUnavailable is returned when RequestKeyframe is called without a requester configured.
	ErrKeyframeRequesterUnavailable = errors.New("keyframe requester is not configured")
)

const (
	defaultMaxLatePackets            = 50
	defaultMaxTimeDelay              = 500 * time.Millisecond
	defaultKeyframeRequestGap        = 500 * time.Millisecond
	defaultMaxVideoDecodeWidth       = 3840
	defaultMaxVideoDecodeHeight      = 2160
	defaultDefaultOpusChannels       = 2
	h264IDRType                 byte = 5
	h264SPSType                 byte = 7
)

// VideoFrameHandler receives decoded video frames from a remote track.
type VideoFrameHandler func(*frame.VideoFrame)

// AudioFrameHandler receives decoded audio frames from a remote track.
type AudioFrameHandler func(*frame.AudioFrame)

// RTPPacketHandler receives cloned RTP packets before they enter the decode
// pipeline. Handlers must return quickly.
type RTPPacketHandler func(*rtp.Packet)

// CodecChangeHandler is invoked when Pion updates the codec for the same
// TrackRemote while packets are being read.
type CodecChangeHandler func(CodecChange)

// KeyframeRequester requests a keyframe for the provided media SSRC.
// A typical implementation writes a PLI/FIR RTCP packet via Pion.
type KeyframeRequester func(mediaSSRC uint32) error

// WriteRTCPFunc matches callback-style RTCP writer hooks used by higher-level
// receiver abstractions.
type WriteRTCPFunc func([]rtcp.Packet) error

// RTCPWriter is the subset of Pion's PeerConnection/DTLSTransport surface
// needed to send RTCP feedback packets.
type RTCPWriter interface {
	WriteRTCP([]rtcp.Packet) (int, error) // WriteRTCP sends one or more RTCP packets.
}

// CodecChange describes a runtime decoder pipeline reconfiguration for the
// same Pion TrackRemote.
type CodecChange struct {
	PreviousType        codec.Type                // PreviousType is the prior normalized libgowebrtc codec type.
	CurrentType         codec.Type                // CurrentType is the new normalized libgowebrtc codec type.
	PreviousCodec       webrtc.RTPCodecParameters // PreviousCodec is the prior full Pion codec description.
	CurrentCodec        webrtc.RTPCodecParameters // CurrentCodec is the new full Pion codec description.
	PreviousPayloadType webrtc.PayloadType        // PreviousPayloadType is the prior RTP payload type.
	CurrentPayloadType  webrtc.PayloadType        // CurrentPayloadType is the new RTP payload type.
}

type config struct {
	maxLatePackets       uint16
	maxTimeDelay         time.Duration
	maxVideoDecodeWidth  int
	maxVideoDecodeHeight int
	keyframeRequester    KeyframeRequester
	keyframeRequestGap   time.Duration
	rtpPacketHandler     RTPPacketHandler
	videoMonitor         *VideoSubscriberMonitor
	audioMonitor         *AudioSubscriberMonitor
}

// Option configures a decoded Pion remote track.
type Option func(*config)

// WithMaxLatePackets configures the SampleBuilder packet window.
func WithMaxLatePackets(maxLate uint16) Option {
	return func(cfg *config) {
		cfg.maxLatePackets = maxLate
	}
}

// WithMaxTimeDelay configures the maximum buffered media time before old RTP
// packets are purged from the SampleBuilder.
func WithMaxTimeDelay(delay time.Duration) Option {
	return func(cfg *config) {
		cfg.maxTimeDelay = delay
	}
}

// WithMaxVideoDimensions configures the scratch decode buffer size used for
// video decoding before frames are copied into compact output frames.
func WithMaxVideoDimensions(width, height int) Option {
	return func(cfg *config) {
		cfg.maxVideoDecodeWidth = width
		cfg.maxVideoDecodeHeight = height
	}
}

// WithKeyframeRequester configures a callback that requests a keyframe when
// the bridge starts video decoding or detects a codec switch.
func WithKeyframeRequester(requester KeyframeRequester) Option {
	return func(cfg *config) {
		cfg.keyframeRequester = requester
	}
}

// WithRTCPWriter configures automatic PLI keyframe requests using a Pion RTCP
// writer such as *webrtc.PeerConnection or receiver.Transport().
//
// Automatic keyframe requests are best-effort. Explicit RequestKeyframe calls
// still return writer errors directly.
func WithRTCPWriter(writer RTCPWriter) Option {
	return func(cfg *config) {
		cfg.keyframeRequester = PLIRequester(writer)
	}
}

// WithWriteRTCP configures automatic PLI keyframe requests using a callback
// from a higher-level receiver wrapper.
//
// Automatic keyframe requests are best-effort. Explicit RequestKeyframe calls
// still return callback errors directly.
func WithWriteRTCP(writeRTCP WriteRTCPFunc) Option {
	return func(cfg *config) {
		cfg.keyframeRequester = PLIRequesterFunc(writeRTCP)
	}
}

// WithKeyframeRequestGap configures the minimum interval between automatic
// keyframe requests.
func WithKeyframeRequestGap(gap time.Duration) Option {
	return func(cfg *config) {
		cfg.keyframeRequestGap = gap
	}
}

// WithRTPPacketHandler configures a callback that observes cloned RTP packets
// before they are fed into the depacketizer/sample builder.
func WithRTPPacketHandler(handler RTPPacketHandler) Option {
	return func(cfg *config) {
		cfg.rtpPacketHandler = handler
	}
}

// WithVideoSubscriberMonitor configures a passive subscriber-side monitor that
// observes packet continuity, freezes, layer transitions, and codec switches.
func WithVideoSubscriberMonitor(monitor *VideoSubscriberMonitor) Option {
	return func(cfg *config) {
		cfg.videoMonitor = monitor
	}
}

// WithAudioSubscriberMonitor configures a passive subscriber-side monitor that
// observes packet continuity, freezes, decoded audio config changes, and
// activity/silence transitions.
func WithAudioSubscriberMonitor(monitor *AudioSubscriberMonitor) Option {
	return func(cfg *config) {
		cfg.audioMonitor = monitor
	}
}

// PLIRequester builds a KeyframeRequester that sends a PLI via a Pion RTCP
// writer. A nil writer returns nil so callers can treat this as optional.
func PLIRequester(writer RTCPWriter) KeyframeRequester {
	if writer == nil {
		return nil
	}

	return func(mediaSSRC uint32) error {
		_, err := writer.WriteRTCP([]rtcp.Packet{
			&rtcp.PictureLossIndication{
				SenderSSRC: 0,
				MediaSSRC:  mediaSSRC,
			},
		})
		return err
	}
}

// PLIRequesterFunc builds a KeyframeRequester from a simple RTCP callback.
func PLIRequesterFunc(writeRTCP WriteRTCPFunc) KeyframeRequester {
	if writeRTCP == nil {
		return nil
	}

	return func(mediaSSRC uint32) error {
		return writeRTCP([]rtcp.Packet{
			&rtcp.PictureLossIndication{
				SenderSSRC: 0,
				MediaSSRC:  mediaSSRC,
			},
		})
	}
}

type trackReader interface {
	ID() string
	StreamID() string
	RID() string
	Kind() webrtc.RTPCodecType
	Codec() webrtc.RTPCodecParameters
	PayloadType() webrtc.PayloadType
	SSRC() webrtc.SSRC
	ReadRTP() (*rtp.Packet, error)
	SetReadDeadline(time.Time) error
}

type pionTrackAdapter struct {
	track *webrtc.TrackRemote
}

func (a *pionTrackAdapter) ID() string                        { return a.track.ID() }
func (a *pionTrackAdapter) StreamID() string                  { return a.track.StreamID() }
func (a *pionTrackAdapter) RID() string                       { return a.track.RID() }
func (a *pionTrackAdapter) Kind() webrtc.RTPCodecType         { return a.track.Kind() }
func (a *pionTrackAdapter) Codec() webrtc.RTPCodecParameters  { return a.track.Codec() }
func (a *pionTrackAdapter) PayloadType() webrtc.PayloadType   { return a.track.PayloadType() }
func (a *pionTrackAdapter) SSRC() webrtc.SSRC                 { return a.track.SSRC() }
func (a *pionTrackAdapter) SetReadDeadline(t time.Time) error { return a.track.SetReadDeadline(t) }
func (a *pionTrackAdapter) ReadRTP() (*rtp.Packet, error) {
	pkt, _, err := a.track.ReadRTP()
	return pkt, err
}

type pipeline struct {
	codec        codec.Type
	payloadType  webrtc.PayloadType
	clockRate    uint32
	channels     int
	builder      *samplebuilder.SampleBuilder
	videoScratch *frame.VideoFrame
	audioScratch *frame.AudioFrame
}

type sampleMetadata struct {
	keyframe bool
}

// DecodedTrack reads RTP from a Pion TrackRemote, rebuilds encoded samples,
// decodes them with libgowebrtc, and emits pkg/frame callbacks.
type DecodedTrack struct {
	source   *webrtc.TrackRemote
	receiver *webrtc.RTPReceiver
	track    trackReader
	cfg      config

	callbackMu sync.RWMutex
	onVideo    VideoFrameHandler
	onAudio    AudioFrameHandler
	onSwitch   CodecChangeHandler

	stateMu sync.RWMutex
	pipe    *pipeline

	currentCodec       codec.Type
	currentCodecParams webrtc.RTPCodecParameters
	currentPayloadType webrtc.PayloadType
	currentClockRate   uint32
	currentChannels    int
	awaitingKeyframe   bool
	lastKeyframeReq    time.Time

	started atomic.Bool
	closed  atomic.Bool

	videoDecoder *pioncodec.MultiVideoDecoder
	audioDecoder *pioncodec.MultiAudioDecoder
}

// BindRemoteTrack creates a decoded bridge for a Pion OnTrack pair.
// The receiver is required so the bridge stays aligned with Pion's explicit
// OnTrack callback surface and can support receiver-native hooks like
// WithRTCPWriter(receiver.Transport()) without a helper fallback path.
func BindRemoteTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver, opts ...Option) (*DecodedTrack, error) {
	if track == nil {
		return nil, ErrNilTrack
	}
	if receiver == nil {
		return nil, ErrNilReceiver
	}
	return newDecodedTrack(&pionTrackAdapter{track: track}, track, receiver, opts...)
}

func newDecodedTrack(track trackReader, source *webrtc.TrackRemote, receiver *webrtc.RTPReceiver, opts ...Option) (*DecodedTrack, error) {
	cfg := config{
		maxLatePackets:       defaultMaxLatePackets,
		maxTimeDelay:         defaultMaxTimeDelay,
		maxVideoDecodeWidth:  defaultMaxVideoDecodeWidth,
		maxVideoDecodeHeight: defaultMaxVideoDecodeHeight,
		keyframeRequestGap:   defaultKeyframeRequestGap,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.maxVideoDecodeWidth <= 0 || cfg.maxVideoDecodeHeight <= 0 {
		return nil, errors.New("invalid max video decode dimensions")
	}
	if cfg.keyframeRequestGap <= 0 {
		cfg.keyframeRequestGap = defaultKeyframeRequestGap
	}

	codecType, codecParams, payloadType, clockRate, channels, err := currentTrackState(track)
	if err != nil {
		return nil, err
	}
	if err := validateDecodedTrackSupport(track.Kind(), codecType, clockRate, channels); err != nil {
		return nil, err
	}

	d := &DecodedTrack{
		source:             source,
		receiver:           receiver,
		track:              track,
		cfg:                cfg,
		currentCodec:       codecType,
		currentCodecParams: codecParams,
		currentPayloadType: payloadType,
		currentClockRate:   clockRate,
		currentChannels:    channels,
		awaitingKeyframe:   track.Kind() == webrtc.RTPCodecTypeVideo,
	}
	switch track.Kind() {
	case webrtc.RTPCodecTypeVideo:
		d.videoDecoder = pioncodec.NewMultiVideoDecoder()
	case webrtc.RTPCodecTypeAudio:
		d.audioDecoder = pioncodec.NewMultiAudioDecoder()
	}
	if cfg.videoMonitor != nil && track.Kind() == webrtc.RTPCodecTypeVideo {
		cfg.videoMonitor.bind(track, receiver, codecType, codecParams)
	}
	if cfg.audioMonitor != nil && track.Kind() == webrtc.RTPCodecTypeAudio {
		cfg.audioMonitor.bind(track, receiver, codecType, codecParams)
	}
	return d, nil
}

// TrackRemote returns the underlying Pion TrackRemote when the bridge was
// created with New. It returns nil in internal test configurations.
func (d *DecodedTrack) TrackRemote() *webrtc.TrackRemote {
	return d.source
}

// RTPReceiver returns the Pion RTPReceiver passed to BindRemoteTrack.
func (d *DecodedTrack) RTPReceiver() *webrtc.RTPReceiver {
	return d.receiver
}

// ID returns the remote track ID.
func (d *DecodedTrack) ID() string { return d.track.ID() }

// StreamID returns the remote track stream ID.
func (d *DecodedTrack) StreamID() string { return d.track.StreamID() }

// RID returns the remote track RID.
func (d *DecodedTrack) RID() string { return d.track.RID() }

// Kind returns the remote track kind using Pion's RTPCodecType.
func (d *DecodedTrack) Kind() webrtc.RTPCodecType { return d.track.Kind() }

// Codec returns the most recently observed codec mapped into libgowebrtc's
// normalized codec enum.
func (d *DecodedTrack) Codec() codec.Type {
	d.stateMu.RLock()
	defer d.stateMu.RUnlock()
	return d.currentCodec
}

// CodecParameters returns the most recently observed Pion codec parameters.
func (d *DecodedTrack) CodecParameters() webrtc.RTPCodecParameters {
	d.stateMu.RLock()
	defer d.stateMu.RUnlock()
	return d.currentCodecParams
}

// PayloadType returns the most recently observed RTP payload type.
func (d *DecodedTrack) PayloadType() webrtc.PayloadType {
	d.stateMu.RLock()
	defer d.stateMu.RUnlock()
	return d.currentPayloadType
}

// SetOnVideoFrame sets the decoded video callback.
func (d *DecodedTrack) SetOnVideoFrame(handler VideoFrameHandler) error {
	if d.track.Kind() != webrtc.RTPCodecTypeVideo {
		return errors.New("not a video track")
	}
	d.callbackMu.Lock()
	defer d.callbackMu.Unlock()
	d.onVideo = handler
	return nil
}

// SetOnAudioFrame sets the decoded audio callback.
func (d *DecodedTrack) SetOnAudioFrame(handler AudioFrameHandler) error {
	if d.track.Kind() != webrtc.RTPCodecTypeAudio {
		return errors.New("not an audio track")
	}
	d.callbackMu.Lock()
	defer d.callbackMu.Unlock()
	d.onAudio = handler
	return nil
}

// SetOnCodecChange sets a callback for runtime codec switches detected while
// reading packets from the same Pion TrackRemote.
func (d *DecodedTrack) SetOnCodecChange(handler CodecChangeHandler) {
	d.callbackMu.Lock()
	defer d.callbackMu.Unlock()
	d.onSwitch = handler
}

// RequestKeyframe explicitly requests a keyframe via the configured callback.
func (d *DecodedTrack) RequestKeyframe() error {
	if d.track.Kind() != webrtc.RTPCodecTypeVideo {
		return nil
	}
	if d.cfg.keyframeRequester == nil {
		return ErrKeyframeRequesterUnavailable
	}
	if err := d.cfg.keyframeRequester(uint32(d.track.SSRC())); err != nil {
		return err
	}

	d.stateMu.Lock()
	d.lastKeyframeReq = time.Now()
	d.stateMu.Unlock()
	return nil
}

// Run starts reading RTP from the remote track and blocks until the track ends
// or Close is called.
func (d *DecodedTrack) Run() error {
	if d.closed.Load() {
		return ErrClosed
	}
	if !d.started.CompareAndSwap(false, true) {
		return ErrAlreadyRunning
	}

	if err := d.rebuildPipeline(d.currentCodec, d.currentCodecParams, d.currentPayloadType, d.currentClockRate, d.currentChannels); err != nil {
		d.started.Store(false)
		return err
	}
	defer d.shutdownPipeline()
	defer d.shutdownDecoders()
	d.maybeRequestKeyframe(true)

	for {
		pkt, err := d.track.ReadRTP()
		if err != nil {
			if d.closed.Load() || errors.Is(err, io.EOF) {
				if flushErr := d.flushPendingSamples(); flushErr != nil && !errors.Is(flushErr, decoder.ErrNeedMoreData) {
					return flushErr
				}
				return nil
			}
			return err
		}
		if pkt == nil {
			continue
		}
		if d.cfg.rtpPacketHandler != nil {
			d.cfg.rtpPacketHandler(clonePacketForHandler(pkt))
		}
		if d.cfg.videoMonitor != nil {
			d.cfg.videoMonitor.observePacket(pkt)
		}
		if d.cfg.audioMonitor != nil {
			d.cfg.audioMonitor.observePacket(pkt)
		}

		if err := d.refreshPipelineIfNeeded(); err != nil {
			return err
		}

		d.stateMu.RLock()
		pipe := d.pipe
		d.stateMu.RUnlock()
		if pipe == nil {
			return ErrClosed
		}

		pipe.builder.Push(pkt)
		for {
			sample := pipe.builder.Pop()
			if sample == nil {
				break
			}
			if err := d.handleSample(pipe, sample); err != nil {
				if errors.Is(err, decoder.ErrNeedMoreData) {
					continue
				}
				return err
			}
		}
	}
}

func (d *DecodedTrack) flushPendingSamples() error {
	d.stateMu.RLock()
	pipe := d.pipe
	d.stateMu.RUnlock()
	if pipe == nil {
		return nil
	}
	pipe.builder.Flush()

	for {
		sample := pipe.builder.Pop()
		if sample == nil {
			return nil
		}
		if err := d.handleSample(pipe, sample); err != nil && !errors.Is(err, decoder.ErrNeedMoreData) {
			return err
		}
	}
}

// Close stops Run and releases decoder resources.
func (d *DecodedTrack) Close() error {
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}

	_ = d.track.SetReadDeadline(time.Now())
	if !d.started.Load() {
		d.shutdownPipeline()
		d.shutdownDecoders()
	}
	return nil
}

func (d *DecodedTrack) refreshPipelineIfNeeded() error {
	codecType, codecParams, payloadType, clockRate, channels, err := currentTrackState(d.track)
	if err != nil {
		return err
	}

	d.stateMu.RLock()
	currentCodec := d.currentCodec
	currentCodecParams := d.currentCodecParams
	currentPayloadType := d.currentPayloadType
	currentClockRate := d.currentClockRate
	currentChannels := d.currentChannels
	d.stateMu.RUnlock()

	if sameDecoderConfig(currentCodec, currentCodecParams, currentClockRate, currentChannels, codecType, codecParams, clockRate, channels) {
		if payloadType != currentPayloadType || !codecParamsEqual(currentCodecParams, codecParams) {
			d.stateMu.Lock()
			d.currentCodecParams = codecParams
			d.currentPayloadType = payloadType
			d.currentClockRate = clockRate
			d.currentChannels = channels
			d.stateMu.Unlock()
		}
		return nil
	}

	if err := d.flushPendingSamples(); err != nil && !errors.Is(err, decoder.ErrNeedMoreData) {
		return err
	}
	if err := d.rebuildPipeline(codecType, codecParams, payloadType, clockRate, channels); err != nil {
		return err
	}
	d.maybeRequestKeyframe(true)
	d.emitCodecChange(CodecChange{
		PreviousType:        currentCodec,
		CurrentType:         codecType,
		PreviousCodec:       currentCodecParams,
		CurrentCodec:        codecParams,
		PreviousPayloadType: currentPayloadType,
		CurrentPayloadType:  payloadType,
	})
	return nil
}

func (d *DecodedTrack) rebuildPipeline(codecType codec.Type, codecParams webrtc.RTPCodecParameters, payloadType webrtc.PayloadType, clockRate uint32, channels int) error {
	pipe, err := newPipeline(d.cfg, d.track.Kind(), codecType, payloadType, clockRate, channels)
	if err != nil {
		return err
	}

	d.stateMu.Lock()
	old := d.pipe
	d.pipe = pipe
	d.currentCodec = codecType
	d.currentCodecParams = codecParams
	d.currentPayloadType = payloadType
	d.currentClockRate = clockRate
	d.currentChannels = channels
	d.awaitingKeyframe = d.track.Kind() == webrtc.RTPCodecTypeVideo
	d.stateMu.Unlock()

	closePipeline(old)
	return nil
}

func (d *DecodedTrack) shutdownPipeline() {
	d.stateMu.Lock()
	pipe := d.pipe
	d.pipe = nil
	d.stateMu.Unlock()
	closePipeline(pipe)
}

func (d *DecodedTrack) shutdownDecoders() {
	if d.videoDecoder != nil {
		_ = d.videoDecoder.Close()
	}
	if d.audioDecoder != nil {
		_ = d.audioDecoder.Close()
	}
}

func closePipeline(pipe *pipeline) {
	if pipe == nil {
		return
	}
}

func (d *DecodedTrack) handleSample(pipe *pipeline, sample *pionmedia.Sample) error {
	if d.track.Kind() == webrtc.RTPCodecTypeAudio {
		return d.handleAudioSample(pipe, sample)
	}
	return d.handleVideoSample(pipe, sample)
}

func (d *DecodedTrack) handleVideoSample(pipe *pipeline, sample *pionmedia.Sample) error {
	isKeyframe := isVideoSampleKeyframe(pipe.codec, sample)

	err := d.videoDecoder.DecodeInto(pioncodec.EncodedVideoSample{
		Data: sample.Data,
		CodecParameters: webrtc.RTPCodecParameters{
			RTPCodecCapability: d.CodecParameters().RTPCodecCapability,
			PayloadType:        d.PayloadType(),
		},
		PayloadType: d.PayloadType(),
		Timestamp:   sample.PacketTimestamp,
		IsKeyframe:  isKeyframe,
	}, pipe.videoScratch)
	if err != nil {
		if errors.Is(err, decoder.ErrNeedMoreData) {
			if d.isAwaitingKeyframe() {
				d.maybeRequestKeyframe(false)
			}
			return err
		}
		if d.isAwaitingKeyframe() {
			d.maybeRequestKeyframe(false)
			return decoder.ErrNeedMoreData
		}
		return err
	}

	d.stateMu.Lock()
	d.awaitingKeyframe = false
	d.stateMu.Unlock()

	out := cloneVideoFrame(pipe.videoScratch, pipe.clockRate, isKeyframe)
	if d.cfg.videoMonitor != nil {
		d.cfg.videoMonitor.observeFrame(out)
	}
	d.callbackMu.RLock()
	handler := d.onVideo
	d.callbackMu.RUnlock()
	if handler != nil {
		handler(out)
	}
	return nil
}

func (d *DecodedTrack) handleAudioSample(pipe *pipeline, sample *pionmedia.Sample) error {
	numSamples, err := d.audioDecoder.DecodeInto(pioncodec.EncodedAudioSample{
		Data: sample.Data,
		CodecParameters: webrtc.RTPCodecParameters{
			RTPCodecCapability: d.CodecParameters().RTPCodecCapability,
			PayloadType:        d.PayloadType(),
		},
		PayloadType: d.PayloadType(),
	}, pipe.audioScratch)
	if err != nil {
		return err
	}

	out := cloneAudioFrame(pipe.audioScratch, pipe.clockRate, numSamples)
	if d.cfg.audioMonitor != nil {
		d.cfg.audioMonitor.observeFrame(out)
	}
	d.callbackMu.RLock()
	handler := d.onAudio
	d.callbackMu.RUnlock()
	if handler != nil {
		handler(out)
	}
	return nil
}

func (d *DecodedTrack) emitCodecChange(change CodecChange) {
	if d.cfg.videoMonitor != nil {
		d.cfg.videoMonitor.observeCodecChange(change)
	}
	if d.cfg.audioMonitor != nil {
		d.cfg.audioMonitor.observeCodecChange(change)
	}
	d.callbackMu.RLock()
	handler := d.onSwitch
	d.callbackMu.RUnlock()
	if handler != nil && !sameDecoderConfig(
		change.PreviousType,
		change.PreviousCodec,
		uint32(change.PreviousCodec.ClockRate),
		int(change.PreviousCodec.Channels),
		change.CurrentType,
		change.CurrentCodec,
		uint32(change.CurrentCodec.ClockRate),
		int(change.CurrentCodec.Channels),
	) {
		handler(change)
	}
}

func sameDecoderConfig(
	prevType codec.Type,
	prevParams webrtc.RTPCodecParameters,
	prevClockRate uint32,
	prevChannels int,
	nextType codec.Type,
	nextParams webrtc.RTPCodecParameters,
	nextClockRate uint32,
	nextChannels int,
) bool {
	if prevType != nextType {
		return false
	}
	if !strings.EqualFold(prevParams.MimeType, nextParams.MimeType) {
		return false
	}
	if prevParams.SDPFmtpLine != nextParams.SDPFmtpLine {
		return false
	}
	if prevClockRate != nextClockRate {
		return false
	}
	if prevChannels != nextChannels {
		return false
	}
	return true
}

func codecParamsEqual(prev, next webrtc.RTPCodecParameters) bool {
	if prev.PayloadType != next.PayloadType ||
		!strings.EqualFold(prev.MimeType, next.MimeType) ||
		prev.ClockRate != next.ClockRate ||
		prev.Channels != next.Channels ||
		prev.SDPFmtpLine != next.SDPFmtpLine {
		return false
	}
	if len(prev.RTCPFeedback) != len(next.RTCPFeedback) {
		return false
	}
	for i := range prev.RTCPFeedback {
		if prev.RTCPFeedback[i] != next.RTCPFeedback[i] {
			return false
		}
	}
	return true
}

func (d *DecodedTrack) isAwaitingKeyframe() bool {
	d.stateMu.RLock()
	defer d.stateMu.RUnlock()
	return d.awaitingKeyframe
}

func (d *DecodedTrack) maybeRequestKeyframe(force bool) {
	if d.track.Kind() != webrtc.RTPCodecTypeVideo || d.cfg.keyframeRequester == nil {
		return
	}

	now := time.Now()

	d.stateMu.Lock()
	last := d.lastKeyframeReq
	if !force && !last.IsZero() && now.Sub(last) < d.cfg.keyframeRequestGap {
		d.stateMu.Unlock()
		return
	}
	d.lastKeyframeReq = now
	d.stateMu.Unlock()

	_ = d.cfg.keyframeRequester(uint32(d.track.SSRC()))
}

func currentTrackState(track trackReader) (codec.Type, webrtc.RTPCodecParameters, webrtc.PayloadType, uint32, int, error) {
	params := track.Codec()
	params.PayloadType = track.PayloadType()
	codecType, ok := codec.ParseMimeType(params.MimeType)
	if !ok {
		return 0, webrtc.RTPCodecParameters{}, 0, 0, 0, decoder.ErrUnsupportedCodec
	}

	switch track.Kind() {
	case webrtc.RTPCodecTypeVideo:
		if !codecType.IsVideo() {
			return 0, webrtc.RTPCodecParameters{}, 0, 0, 0, ErrUnsupportedTrackKind
		}
	case webrtc.RTPCodecTypeAudio:
		if !codecType.IsAudio() {
			return 0, webrtc.RTPCodecParameters{}, 0, 0, 0, ErrUnsupportedTrackKind
		}
	default:
		return 0, webrtc.RTPCodecParameters{}, 0, 0, 0, ErrUnsupportedTrackKind
	}

	clockRate := uint32(params.ClockRate)
	if clockRate == 0 {
		clockRate = codecType.ClockRate()
	}

	channels := int(params.Channels)
	if channels <= 0 {
		switch codecType {
		case codec.Opus:
			channels = defaultDefaultOpusChannels
		default:
			channels = 1
		}
	}

	params.ClockRate = clockRate
	params.Channels = uint16(channels)
	params.PayloadType = track.PayloadType()

	return codecType, params, track.PayloadType(), clockRate, channels, nil
}

func validateDecodedTrackSupport(kind webrtc.RTPCodecType, codecType codec.Type, clockRate uint32, channels int) error {
	params := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  codecType.MimeType(),
			ClockRate: clockRate,
			Channels:  uint16(channels),
		},
	}
	switch kind {
	case webrtc.RTPCodecTypeVideo:
		dec, err := pioncodec.NewVideoDecoder(params)
		if err != nil {
			return err
		}
		return dec.Close()
	case webrtc.RTPCodecTypeAudio:
		dec, err := pioncodec.NewAudioDecoder(params)
		if err != nil {
			return err
		}
		return dec.Close()
	default:
		return ErrUnsupportedTrackKind
	}
}

func newPipeline(cfg config, kind webrtc.RTPCodecType, codecType codec.Type, payloadType webrtc.PayloadType, clockRate uint32, channels int) (*pipeline, error) {
	builder, err := newSampleBuilder(codecType, clockRate, cfg)
	if err != nil {
		return nil, err
	}

	pipe := &pipeline{
		codec:       codecType,
		payloadType: payloadType,
		clockRate:   clockRate,
		channels:    channels,
		builder:     builder,
	}

	switch kind {
	case webrtc.RTPCodecTypeVideo:
		pipe.videoScratch = frame.NewI420Frame(cfg.maxVideoDecodeWidth, cfg.maxVideoDecodeHeight)
	case webrtc.RTPCodecTypeAudio:
		pipe.audioScratch = frame.NewAudioFrameS16(int(clockRate), channels, 5760)
	default:
		return nil, ErrUnsupportedTrackKind
	}

	return pipe, nil
}

func newSampleBuilder(codecType codec.Type, clockRate uint32, cfg config) (*samplebuilder.SampleBuilder, error) {
	depacketizer, err := newRTPDepacketizer(codecType)
	if err != nil {
		return nil, err
	}

	options := []samplebuilder.Option{
		samplebuilder.WithPacketHeadHandler(func(head any) any {
			return sampleMetadata{keyframe: isDepacketizerKeyframe(codecType, head)}
		}),
	}
	if cfg.maxTimeDelay > 0 {
		options = append(options, samplebuilder.WithMaxTimeDelay(cfg.maxTimeDelay))
	}

	return samplebuilder.New(cfg.maxLatePackets, depacketizer, clockRate, options...), nil
}

func newRTPDepacketizer(codecType codec.Type) (rtp.Depacketizer, error) {
	switch codecType {
	case codec.H264:
		return &pioncodecs.H264Packet{}, nil
	case codec.VP8:
		return &pioncodecs.VP8Packet{}, nil
	case codec.VP9:
		return &pioncodecs.VP9Packet{}, nil
	case codec.AV1:
		return &pioncodecs.AV1Depacketizer{}, nil
	case codec.Opus:
		return &pioncodecs.OpusPacket{}, nil
	default:
		return nil, decoder.ErrUnsupportedCodec
	}
}

func clonePacketForHandler(pkt *rtp.Packet) *rtp.Packet {
	if pkt == nil {
		return nil
	}

	raw, err := pkt.Marshal()
	if err != nil {
		return nil
	}

	clone := &rtp.Packet{}
	if err := clone.Unmarshal(raw); err != nil {
		return nil
	}
	return clone
}

func isDepacketizerKeyframe(codecType codec.Type, head any) bool {
	switch codecType {
	case codec.VP8:
		packet, ok := head.(*pioncodecs.VP8Packet)
		return ok && packet != nil && packet.S == 1 && packet.PID == 0 && isVP8KeyframeBitstream(packet.Payload)
	case codec.VP9:
		packet, ok := head.(*pioncodecs.VP9Packet)
		return ok && packet != nil && packet.B && !packet.P
	case codec.AV1:
		packet, ok := head.(*pioncodecs.AV1Depacketizer)
		return ok && packet != nil && packet.N
	default:
		return false
	}
}

func isVideoSampleKeyframe(codecType codec.Type, sample *pionmedia.Sample) bool {
	if sample == nil {
		return false
	}
	meta, _ := sample.Metadata.(sampleMetadata)
	if meta.keyframe {
		return true
	}

	switch codecType {
	case codec.H264:
		return isH264KeyframeBitstream(sample.Data)
	case codec.VP8:
		return isVP8KeyframeBitstream(sample.Data)
	default:
		return false
	}
}

func isVP8KeyframeBitstream(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	// The least-significant bit of the first payload byte is 0 for keyframes.
	return data[0]&0x01 == 0
}

func isH264KeyframeBitstream(payload []byte) bool {
	if len(payload) == 0 {
		return false
	}

	// Support already-assembled Annex-B samples from Pion's H264 depacketizer.
	for i := 0; i < len(payload); i++ {
		if payload[i] != 0 || i+3 >= len(payload) || payload[i+1] != 0 {
			continue
		}

		start := -1
		switch {
		case payload[i+2] == 1:
			start = i + 3
		case i+4 < len(payload) && payload[i+2] == 0 && payload[i+3] == 1:
			start = i + 4
		}
		if start == -1 || start >= len(payload) {
			continue
		}

		nalType := payload[start] & 0x1F
		if nalType == h264IDRType || nalType == h264SPSType {
			return true
		}
		i = start - 1
	}

	// Fall back to single-NAL detection if the buffer is not Annex-B formatted.
	nalType := payload[0] & 0x1F
	return nalType == h264IDRType || nalType == h264SPSType
}

func cloneVideoFrame(src *frame.VideoFrame, clockRate uint32, isKeyframe bool) *frame.VideoFrame {
	dst := frame.NewI420Frame(src.Width, src.Height)

	copyPlane(dst.Data[0], dst.Stride[0], src.Data[0], src.Stride[0], src.Width, src.Height)
	uvWidth := (src.Width + 1) / 2
	uvHeight := (src.Height + 1) / 2
	copyPlane(dst.Data[1], dst.Stride[1], src.Data[1], src.Stride[1], uvWidth, uvHeight)
	copyPlane(dst.Data[2], dst.Stride[2], src.Data[2], src.Stride[2], uvWidth, uvHeight)

	dst.PTS = src.PTS
	dst.Timestamp = rtpTimestampToDuration(src.PTS, clockRate)
	dst.IsKeyframe = isKeyframe
	return dst
}

func cloneAudioFrame(src *frame.AudioFrame, clockRate uint32, numSamples int) *frame.AudioFrame {
	dst := frame.NewAudioFrameS16(src.SampleRate, src.Channels, numSamples)
	copy(dst.Samples, src.Samples[:numSamples*src.Channels*2])
	dst.NumSamples = numSamples
	dst.PTS = src.PTS
	dst.Timestamp = rtpTimestampToDuration(src.PTS, clockRate)
	return dst
}

func copyPlane(dst []byte, dstStride int, src []byte, srcStride int, width int, height int) {
	for y := 0; y < height; y++ {
		srcOffset := y * srcStride
		dstOffset := y * dstStride
		copy(dst[dstOffset:dstOffset+width], src[srcOffset:srcOffset+width])
	}
}

func rtpTimestampToDuration(timestamp uint32, clockRate uint32) time.Duration {
	if clockRate == 0 {
		return 0
	}
	return time.Duration(timestamp) * time.Second / time.Duration(clockRate)
}
