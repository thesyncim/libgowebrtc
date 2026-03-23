// Package track provides Pion-compatible TrackLocal implementations backed by libwebrtc.
package track

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/packetizer"
	"github.com/thesyncim/libgowebrtc/pkg/pioncodec"
)

// Errors
var (
	ErrTrackClosed   = errors.New("track is closed")
	ErrNotBound      = errors.New("track not bound")
	ErrAlreadyBound  = errors.New("track already bound")
	ErrEncodeFailed  = errors.New("encode failed")
	ErrInvalidConfig = errors.New("invalid config")
)

// VideoTrackConfig configures a video track.
type VideoTrackConfig struct {
	ID       string
	StreamID string
	Codec    codec.Type
	Width    int
	Height   int
	Bitrate  uint32
	FPS      float64
	MTU      uint16 // RTP MTU (default 1200)

	// Auto adaptation (all default true for browser-like behavior)
	AutoKeyframe   bool // PLI/FIR → RequestKeyFrame()
	AutoBitrate    bool // BWE → adjust bitrate
	AutoFramerate  bool // BWE → adjust framerate
	AutoResolution bool // BWE → scale resolution

	// Constraints (like browser MediaTrackConstraints)
	MinBitrate   uint32  // Floor for bitrate adaptation
	MaxBitrate   uint32  // Ceiling for bitrate adaptation
	MinFramerate float64 // Floor for framerate
	MaxFramerate float64 // Ceiling for framerate
	MinWidth     int     // Don't scale below this
	MinHeight    int     // Don't scale below this

	// Optional codec preferences used during TrackLocal binding.
	// When provided, Bind selects the best negotiated codec from this list.
	CodecPreferences []webrtc.RTPCodecParameters
}

// BandwidthEstimate contains bandwidth estimation data from the network.
type BandwidthEstimate struct {
	TimestampUs      int64
	TargetBitrateBps int64
	AvailableSendBps int64
	PacingRateBps    int64
	LossRate         float64
}

// BandwidthEstimateSource is a function that returns the current BWE.
type BandwidthEstimateSource func() *BandwidthEstimate

// Parameters mirrors browser RTCRtpEncodingParameters for manual control.
type Parameters struct {
	Active                bool
	MaxBitrate            uint32
	MaxFramerate          float64
	ScaleResolutionDownBy float64
	ScalabilityMode       string // SVC mode (e.g., "L3T3_KEY", "L1T2")
	Priority              string // "very-low", "low", "medium", "high"
}

// adaptationState tracks the current adaptation parameters.
type adaptationState struct {
	currentBitrate   uint32
	currentFramerate float64
	currentScale     float64
}

// VideoTrack implements webrtc.TrackLocal using libwebrtc encoder.
// Call WriteFrame to encode raw video and send RTP packets.
type VideoTrack struct {
	id       string
	streamID string
	codec    codec.Type

	config VideoTrackConfig
	enc    encoder.VideoEncoder
	pkt    packetizer.Packetizer

	// Bound state
	writer      webrtc.TrackLocalWriter
	codecParams webrtc.RTPCodecParameters
	ssrc        webrtc.SSRC
	payloadType webrtc.PayloadType

	// Pre-allocated buffers for allocation-free encoding
	encBuf     []byte
	packetBuf  []byte
	packetInfo []packetizer.PacketInfo

	// Browser-like adaptation
	adaptation   adaptationState
	bweSource    BandwidthEstimateSource
	scaleFactor  float64
	scaledFrame  *frame.VideoFrame // Reusable scaled frame buffer
	paused       atomic.Bool
	adaptStop    chan struct{}
	adaptDone    chan struct{}
	keyframePend atomic.Bool // Pending keyframe request from RTCP

	mu     sync.Mutex
	closed atomic.Bool
	bound  atomic.Bool
}

// NewVideoTrack creates a new video track backed by libwebrtc encoder.
// Auto adaptation features default to true for browser-like behavior.
func NewVideoTrack(cfg VideoTrackConfig) (*VideoTrack, error) {
	if cfg.ID == "" {
		return nil, ErrInvalidConfig
	}
	if cfg.StreamID == "" {
		cfg.StreamID = cfg.ID
	}
	if cfg.MTU == 0 {
		cfg.MTU = 1200
	}
	if cfg.Codec == 0 && len(cfg.CodecPreferences) > 0 {
		if codecType, ok := codec.ParseMimeType(cfg.CodecPreferences[0].MimeType); ok {
			cfg.Codec = codecType
		}
	}

	// Default auto adaptation to true (browser-like behavior)
	// Use explicit false to disable
	if !cfg.AutoKeyframe && cfg.MinBitrate == 0 && cfg.MaxBitrate == 0 {
		cfg.AutoKeyframe = true
	}
	if !cfg.AutoBitrate && cfg.MinBitrate == 0 && cfg.MaxBitrate == 0 {
		cfg.AutoBitrate = true
	}
	if !cfg.AutoFramerate && cfg.MinFramerate == 0 && cfg.MaxFramerate == 0 {
		cfg.AutoFramerate = true
	}
	if !cfg.AutoResolution && cfg.MinWidth == 0 && cfg.MinHeight == 0 {
		cfg.AutoResolution = true
	}

	// Set default constraints if not specified
	if cfg.MaxFramerate == 0 {
		cfg.MaxFramerate = cfg.FPS
	}
	if cfg.MinFramerate == 0 {
		cfg.MinFramerate = 1.0
	}

	t := &VideoTrack{
		id:          cfg.ID,
		streamID:    cfg.StreamID,
		codec:       cfg.Codec,
		config:      cloneVideoTrackConfig(cfg),
		scaleFactor: 1.0, // No scaling by default
		adaptation: adaptationState{
			currentBitrate:   cfg.Bitrate,
			currentFramerate: cfg.FPS,
			currentScale:     1.0,
		},
	}

	return t, nil
}

// ID returns the track ID.
func (t *VideoTrack) ID() string {
	return t.id
}

// RID returns the RTP stream ID (empty for non-simulcast).
func (t *VideoTrack) RID() string {
	return ""
}

// StreamID returns the stream ID.
func (t *VideoTrack) StreamID() string {
	return t.streamID
}

// Kind returns webrtc.RTPCodecTypeVideo.
func (t *VideoTrack) Kind() webrtc.RTPCodecType {
	return webrtc.RTPCodecTypeVideo
}

// Bind is called by Pion when the track is added to a PeerConnection.
func (t *VideoTrack) Bind(ctx webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	if t.closed.Load() {
		return webrtc.RTPCodecParameters{}, ErrTrackClosed
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.bound.Load() {
		return webrtc.RTPCodecParameters{}, ErrAlreadyBound
	}

	selected, codecType, err := t.selectVideoCodec(ctx.CodecParameters())
	if err != nil {
		return webrtc.RTPCodecParameters{}, err
	}
	t.codec = codecType

	// Create encoder
	enc, err := t.createVideoEncoderForSelection(*selected)
	if err != nil {
		return webrtc.RTPCodecParameters{}, err
	}

	// Create packetizer
	clockRate := uint32(selected.ClockRate)
	if clockRate == 0 {
		clockRate = codecType.ClockRate()
	}
	pkt, err := packetizer.New(packetizer.Config{
		Codec:       codecType,
		SSRC:        uint32(ctx.SSRC()),
		PayloadType: uint8(selected.PayloadType),
		MTU:         t.config.MTU,
		ClockRate:   clockRate,
	})
	if err != nil {
		enc.Close()
		return webrtc.RTPCodecParameters{}, err
	}

	// Pre-allocate buffers
	t.encBuf = make([]byte, enc.MaxEncodedSize())

	// Estimate max packets for a keyframe (could be large)
	maxPackets := pkt.MaxPackets(enc.MaxEncodedSize())
	t.packetBuf = make([]byte, maxPackets*pkt.MaxPacketSize())
	t.packetInfo = make([]packetizer.PacketInfo, maxPackets)

	t.enc = enc
	t.pkt = pkt
	t.writer = ctx.WriteStream()
	t.codecParams = *selected
	t.ssrc = ctx.SSRC()
	t.payloadType = webrtc.PayloadType(selected.PayloadType)

	t.bound.Store(true)

	return t.codecParams, nil
}

// Unbind is called when the track is removed from the PeerConnection.
func (t *VideoTrack) Unbind(ctx webrtc.TrackLocalContext) error {
	if !t.bound.CompareAndSwap(true, false) {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.enc != nil {
		t.enc.Close()
		t.enc = nil
	}
	if t.pkt != nil {
		t.pkt.Close()
		t.pkt = nil
	}

	t.writer = nil
	return nil
}

// WriteFrame encodes a video frame and writes RTP packets to the bound peer connection.
func (t *VideoTrack) WriteFrame(f *frame.VideoFrame, forceKeyframe bool) error {
	if t.closed.Load() {
		return ErrTrackClosed
	}
	if !t.bound.Load() {
		return ErrNotBound
	}

	// Check if track is paused (SetParameters with Active=false)
	if t.paused.Load() {
		return nil // Silently drop frames when paused
	}

	// Check for pending keyframe request from RTCP (PLI/FIR)
	if t.keyframePend.CompareAndSwap(true, false) {
		forceKeyframe = true
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.enc == nil || t.pkt == nil || t.writer == nil {
		return ErrNotBound
	}

	// Apply frame scaling if needed
	frameToEncode := f
	if t.scaleFactor > 1.0 && t.scaledFrame != nil {
		ScaleI420Frame(f, t.scaledFrame, t.scaleFactor)
		frameToEncode = t.scaledFrame
		frameToEncode.PTS = f.PTS // Preserve timestamp
	}

	// Encode frame
	result, err := t.enc.EncodeInto(frameToEncode, t.encBuf, forceKeyframe)
	if err != nil {
		return err
	}

	// Convert PTS to RTP timestamp
	rtpTimestamp := uint32(f.PTS)

	// Packetize encoded data
	numPackets, err := t.pkt.PacketizeInto(
		t.encBuf[:result.N],
		rtpTimestamp,
		result.IsKeyframe,
		t.packetBuf,
		t.packetInfo,
	)
	if err != nil {
		return err
	}

	// Write each RTP packet
	for i := 0; i < numPackets; i++ {
		info := t.packetInfo[i]
		pktData := t.packetBuf[info.Offset : info.Offset+info.Size]

		if _, err := t.writer.Write(pktData); err != nil {
			return err
		}
	}

	return nil
}

// WriteEncodedData writes pre-encoded data as RTP packets.
// Useful when you already have encoded H.264/VP8/etc data.
func (t *VideoTrack) WriteEncodedData(data []byte, timestamp uint32, isKeyframe bool) error {
	if t.closed.Load() {
		return ErrTrackClosed
	}
	if !t.bound.Load() {
		return ErrNotBound
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.pkt == nil || t.writer == nil {
		return ErrNotBound
	}

	// Packetize
	numPackets, err := t.pkt.PacketizeInto(
		data,
		timestamp,
		isKeyframe,
		t.packetBuf,
		t.packetInfo,
	)
	if err != nil {
		return err
	}

	// Write each RTP packet
	for i := 0; i < numPackets; i++ {
		info := t.packetInfo[i]
		pktData := t.packetBuf[info.Offset : info.Offset+info.Size]

		if _, err := t.writer.Write(pktData); err != nil {
			return err
		}
	}

	return nil
}

// WriteRTP writes an already-formed RTP packet.
func (t *VideoTrack) WriteRTP(pkt *rtp.Packet) error {
	if t.closed.Load() {
		return ErrTrackClosed
	}
	if !t.bound.Load() {
		return ErrNotBound
	}

	t.mu.Lock()
	writer := t.writer
	t.mu.Unlock()

	if writer == nil {
		return ErrNotBound
	}

	buf, err := pkt.Marshal()
	if err != nil {
		return err
	}

	_, err = writer.Write(buf)
	return err
}

// RequestKeyFrame requests the encoder to generate a keyframe.
func (t *VideoTrack) RequestKeyFrame() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.enc != nil {
		t.enc.RequestKeyFrame()
	}
}

// SetBitrate adjusts encoder bitrate.
func (t *VideoTrack) SetBitrate(bps uint32) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.enc != nil {
		return t.enc.SetBitrate(bps)
	}
	t.config.Bitrate = bps
	return nil
}

// SetFramerate adjusts encoder framerate.
func (t *VideoTrack) SetFramerate(fps float64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.enc != nil {
		return t.enc.SetFramerate(fps)
	}
	t.config.FPS = fps
	return nil
}

// SetParameters allows manual control like browser RTCRtpSender.setParameters().
func (t *VideoTrack) SetParameters(params Parameters) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !params.Active {
		t.paused.Store(true)
		return nil
	}
	t.paused.Store(false)

	if params.MaxBitrate > 0 {
		t.adaptation.currentBitrate = params.MaxBitrate
		if t.enc != nil {
			if err := t.enc.SetBitrate(params.MaxBitrate); err != nil {
				return err
			}
		}
	}
	if params.MaxFramerate > 0 {
		t.adaptation.currentFramerate = params.MaxFramerate
		if t.enc != nil {
			if err := t.enc.SetFramerate(params.MaxFramerate); err != nil {
				return err
			}
		}
	}
	if params.ScaleResolutionDownBy > 0 {
		t.setScaleFactorLocked(params.ScaleResolutionDownBy)
	}
	return nil
}

// SetBWESource sets the bandwidth estimation source for auto adaptation.
// This is typically wired up when the track is added to a PeerConnection.
func (t *VideoTrack) SetBWESource(source BandwidthEstimateSource) {
	t.mu.Lock()
	t.bweSource = source

	// Start adaptation loop if we have a BWE source and any auto feature enabled
	if source != nil && (t.config.AutoBitrate || t.config.AutoFramerate || t.config.AutoResolution) {
		t.startAdaptLoopLocked()
		t.mu.Unlock()
		return
	}
	done := t.stopAdaptLoopLocked()
	t.mu.Unlock()
	if done != nil {
		<-done
	}
}

// HandleRTCPFeedback handles RTCP feedback for browser-like behavior.
// feedbackType: 0=PLI, 1=FIR, 2=NACK
func (t *VideoTrack) HandleRTCPFeedback(feedbackType int, ssrc uint32) {
	if !t.config.AutoKeyframe {
		return
	}

	switch feedbackType {
	case 0, 1: // PLI or FIR
		t.keyframePend.Store(true)
	case 2: // NACK - handled by packetizer/transport layer
		// No action needed here
	}
}

func (t *VideoTrack) startAdaptLoopLocked() {
	if t.adaptStop != nil {
		return // Already running
	}
	t.adaptStop = make(chan struct{})
	t.adaptDone = make(chan struct{})
	go t.adaptLoop(t.adaptStop, t.adaptDone)
}

func (t *VideoTrack) stopAdaptLoopLocked() <-chan struct{} {
	stop := t.adaptStop
	done := t.adaptDone
	t.adaptStop = nil
	t.adaptDone = nil

	if stop != nil {
		close(stop)
	}
	return done
}

func (t *VideoTrack) stopAdaptLoop() {
	t.mu.Lock()
	done := t.stopAdaptLoopLocked()
	t.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (t *VideoTrack) adaptLoop(stop <-chan struct{}, done chan<- struct{}) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	defer close(done)

	for {
		select {
		case <-ticker.C:
			if t.closed.Load() {
				return
			}
			t.mu.Lock()
			bweSource := t.bweSource
			t.mu.Unlock()

			if bweSource == nil {
				continue
			}

			bwe := bweSource()
			if bwe == nil {
				continue
			}

			t.adapt(bwe)

		case <-stop:
			return
		}
	}
}

func (t *VideoTrack) adapt(bwe *BandwidthEstimate) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.enc == nil {
		return
	}

	targetBps := bwe.TargetBitrateBps

	// Apply constraints
	if t.config.MinBitrate > 0 && uint32(targetBps) < t.config.MinBitrate {
		targetBps = int64(t.config.MinBitrate)
	}
	if t.config.MaxBitrate > 0 && uint32(targetBps) > t.config.MaxBitrate {
		targetBps = int64(t.config.MaxBitrate)
	}

	// Bitrate adaptation (errors ignored: best-effort adaptation, encoder continues with previous settings)
	if t.config.AutoBitrate && uint32(targetBps) != t.adaptation.currentBitrate {
		if err := t.enc.SetBitrate(uint32(targetBps)); err == nil {
			t.adaptation.currentBitrate = uint32(targetBps)
		}
	}

	// Resolution adaptation (when bitrate too low for current resolution)
	if t.config.AutoResolution {
		scale := t.calculateScale(targetBps)
		if scale != t.adaptation.currentScale {
			t.setScaleFactorLocked(scale)
		}
	}

	// Framerate adaptation (errors ignored: best-effort adaptation, encoder continues with previous settings)
	if t.config.AutoFramerate {
		fps := t.calculateFramerate(targetBps)
		if fps != t.adaptation.currentFramerate {
			if err := t.enc.SetFramerate(fps); err == nil {
				t.adaptation.currentFramerate = fps
			}
		}
	}
}

// calculateScale determines the resolution scale factor based on available bandwidth.
// Returns 1.0 for full resolution, 2.0 for half, 4.0 for quarter.
func (t *VideoTrack) calculateScale(targetBps int64) float64 {
	// Estimate bits per pixel for current codec at reasonable quality
	// These are rough estimates - adjust based on codec efficiency
	bitsPerPixel := 0.1 // Good quality H264/VP8
	switch t.codec {
	case codec.VP9, codec.AV1:
		bitsPerPixel = 0.07 // More efficient codecs
	}

	currentPixels := float64(t.config.Width * t.config.Height)
	currentFps := t.adaptation.currentFramerate
	if currentFps <= 0 {
		currentFps = 30
	}

	// Required bitrate for full resolution at current fps
	requiredBps := currentPixels * currentFps * bitsPerPixel

	// If we have enough bandwidth, no scaling
	if float64(targetBps) >= requiredBps {
		return 1.0
	}

	// Calculate what scale factor we need
	// scale^2 reduces pixel count, so bandwidth requirement drops by scale^2
	ratio := float64(targetBps) / requiredBps
	scale := 1.0 / ratio

	// Snap to standard scale factors and respect min dimensions
	minScale := 1.0
	if t.config.MinWidth > 0 {
		maxScaleW := float64(t.config.Width) / float64(t.config.MinWidth)
		if maxScaleW > minScale {
			minScale = maxScaleW
		}
	}
	if t.config.MinHeight > 0 {
		maxScaleH := float64(t.config.Height) / float64(t.config.MinHeight)
		if maxScaleH > minScale {
			minScale = maxScaleH
		}
	}

	// Clamp and snap to standard values
	if scale <= 1.0 {
		return 1.0
	} else if scale <= 1.5 {
		return 1.0
	} else if scale <= 2.5 {
		if 2.0 > minScale {
			return minScale
		}
		return 2.0
	}
	if 4.0 > minScale {
		return minScale
	}
	return 4.0
}

// calculateFramerate determines the target framerate based on available bandwidth.
func (t *VideoTrack) calculateFramerate(targetBps int64) float64 {
	// If we have lots of bandwidth, use max framerate
	// If bandwidth is limited, reduce framerate

	maxFps := t.config.MaxFramerate
	minFps := t.config.MinFramerate
	if maxFps <= 0 {
		maxFps = 30
	}
	if minFps <= 0 {
		minFps = 1
	}

	// Estimate bandwidth needed for max fps at current resolution/scale
	currentPixels := float64(t.config.Width*t.config.Height) / (t.scaleFactor * t.scaleFactor)
	bitsPerPixel := 0.1
	requiredForMax := currentPixels * maxFps * bitsPerPixel

	if float64(targetBps) >= requiredForMax {
		return maxFps
	}

	// Scale framerate linearly with available bandwidth
	ratio := float64(targetBps) / requiredForMax
	fps := maxFps * ratio

	// Clamp to range
	if fps < minFps {
		return minFps
	}
	if fps > maxFps {
		return maxFps
	}

	return fps
}

func (t *VideoTrack) setScaleFactorLocked(scale float64) {
	if scale < 1.0 {
		scale = 1.0
	}
	t.scaleFactor = scale
	t.adaptation.currentScale = scale

	// Allocate scaled frame buffer if needed
	if scale > 1.0 {
		newW := int(float64(t.config.Width) / scale)
		newH := int(float64(t.config.Height) / scale)
		if t.scaledFrame == nil || t.scaledFrame.Width != newW || t.scaledFrame.Height != newH {
			t.scaledFrame = frame.NewI420Frame(newW, newH)
		}
	}
}

// Close releases all resources.
func (t *VideoTrack) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}

	// Stop adaptation loop
	t.mu.Lock()
	done := t.stopAdaptLoopLocked()
	t.mu.Unlock()
	if done != nil {
		<-done
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.enc != nil {
		t.enc.Close()
		t.enc = nil
	}
	if t.pkt != nil {
		t.pkt.Close()
		t.pkt = nil
	}

	return nil
}

func (t *VideoTrack) createEncoder() (encoder.VideoEncoder, error) {
	switch t.codec {
	case codec.H264:
		cfg := codec.DefaultH264Config(t.config.Width, t.config.Height)
		if t.config.Bitrate != 0 {
			cfg.Bitrate = t.config.Bitrate
		}
		if t.config.FPS != 0 {
			cfg.FPS = t.config.FPS
		}
		return encoder.NewH264Encoder(cfg)
	case codec.VP8:
		cfg := codec.DefaultVP8Config(t.config.Width, t.config.Height)
		if t.config.Bitrate != 0 {
			cfg.Bitrate = t.config.Bitrate
		}
		if t.config.FPS != 0 {
			cfg.FPS = t.config.FPS
		}
		return encoder.NewVP8Encoder(cfg)
	case codec.VP9:
		cfg := codec.DefaultVP9Config(t.config.Width, t.config.Height)
		if t.config.Bitrate != 0 {
			cfg.Bitrate = t.config.Bitrate
		}
		if t.config.FPS != 0 {
			cfg.FPS = t.config.FPS
		}
		return encoder.NewVP9Encoder(cfg)
	case codec.AV1:
		cfg := codec.DefaultAV1Config(t.config.Width, t.config.Height)
		if t.config.Bitrate != 0 {
			cfg.Bitrate = t.config.Bitrate
		}
		if t.config.FPS != 0 {
			cfg.FPS = t.config.FPS
		}
		return encoder.NewAV1Encoder(cfg)
	default:
		return nil, ErrInvalidConfig
	}
}

func (t *VideoTrack) selectVideoCodec(offered []webrtc.RTPCodecParameters) (*webrtc.RTPCodecParameters, codec.Type, error) {
	if len(t.config.CodecPreferences) > 0 {
		selected, ok := pioncodec.Select(t.config.CodecPreferences, offered)
		if !ok {
			return nil, 0, fmt.Errorf("no matching video codec preference")
		}
		codecType, ok := codec.ParseMimeType(selected.MimeType)
		if !ok {
			return nil, 0, ErrInvalidConfig
		}
		return &selected, codecType, nil
	}

	var selected *webrtc.RTPCodecParameters
	targetMime := t.codec.MimeType()

	for i := range offered {
		if offered[i].MimeType == targetMime {
			selected = &offered[i]
			break
		}
	}
	if selected == nil {
		selected = &webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  targetMime,
				ClockRate: t.codec.ClockRate(),
			},
		}
	}
	return selected, t.codec, nil
}

func (t *VideoTrack) createVideoEncoderForSelection(selected webrtc.RTPCodecParameters) (encoder.VideoEncoder, error) {
	if len(t.config.CodecPreferences) > 0 {
		return pioncodec.NewVideoEncoder(selected, pioncodec.VideoFactoryConfig{
			Width:       t.config.Width,
			Height:      t.config.Height,
			Bitrate:     t.config.Bitrate,
			FPS:         t.config.FPS,
			KeyInterval: int(t.config.FPS * 2),
		})
	}
	return t.createEncoder()
}

func cloneVideoTrackConfig(cfg VideoTrackConfig) VideoTrackConfig {
	cloned := cfg
	if len(cfg.CodecPreferences) > 0 {
		cloned.CodecPreferences = append([]webrtc.RTPCodecParameters(nil), cfg.CodecPreferences...)
	}
	return cloned
}

// AudioTrackConfig configures an audio track.
type AudioTrackConfig struct {
	ID         string
	StreamID   string
	SampleRate int
	Channels   int
	Bitrate    uint32
	MTU        uint16

	// Optional codec preferences used during TrackLocal binding.
	CodecPreferences []webrtc.RTPCodecParameters
}

// AudioTrack implements webrtc.TrackLocal using libwebrtc Opus encoder.
type AudioTrack struct {
	id       string
	streamID string

	config AudioTrackConfig
	enc    encoder.AudioEncoder
	pkt    packetizer.Packetizer

	writer      webrtc.TrackLocalWriter
	codecParams webrtc.RTPCodecParameters
	ssrc        webrtc.SSRC
	payloadType webrtc.PayloadType

	// Pre-allocated buffers
	encBuf     []byte
	packetBuf  []byte
	packetInfo []packetizer.PacketInfo

	mu     sync.Mutex
	closed atomic.Bool
	bound  atomic.Bool
}

// NewAudioTrack creates a new audio track backed by libwebrtc Opus encoder.
func NewAudioTrack(cfg AudioTrackConfig) (*AudioTrack, error) {
	if cfg.ID == "" {
		return nil, ErrInvalidConfig
	}
	if cfg.StreamID == "" {
		cfg.StreamID = cfg.ID
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 48000
	}
	if cfg.Channels == 0 {
		cfg.Channels = 2
	}
	if cfg.Bitrate == 0 {
		cfg.Bitrate = 64000
	}
	if cfg.MTU == 0 {
		cfg.MTU = 1200
	}

	return &AudioTrack{
		id:       cfg.ID,
		streamID: cfg.StreamID,
		config:   cloneAudioTrackConfig(cfg),
	}, nil
}

// ID returns the track ID.
func (t *AudioTrack) ID() string {
	return t.id
}

// RID returns the RTP stream ID (empty for non-simulcast).
func (t *AudioTrack) RID() string {
	return ""
}

// StreamID returns the stream ID.
func (t *AudioTrack) StreamID() string {
	return t.streamID
}

// Kind returns webrtc.RTPCodecTypeAudio.
func (t *AudioTrack) Kind() webrtc.RTPCodecType {
	return webrtc.RTPCodecTypeAudio
}

// Bind is called by Pion when the track is added to a PeerConnection.
func (t *AudioTrack) Bind(ctx webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	if t.closed.Load() {
		return webrtc.RTPCodecParameters{}, ErrTrackClosed
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.bound.Load() {
		return webrtc.RTPCodecParameters{}, ErrAlreadyBound
	}

	selected, err := t.selectAudioCodec(ctx.CodecParameters())
	if err != nil {
		return webrtc.RTPCodecParameters{}, err
	}

	// Create encoder
	enc, err := t.createAudioEncoderForSelection(*selected)
	if err != nil {
		return webrtc.RTPCodecParameters{}, err
	}

	// Create packetizer
	clockRate := uint32(selected.ClockRate)
	if clockRate == 0 {
		clockRate = 48000
	}
	pkt, err := packetizer.New(packetizer.Config{
		Codec:       codec.Opus,
		SSRC:        uint32(ctx.SSRC()),
		PayloadType: uint8(selected.PayloadType),
		MTU:         t.config.MTU,
		ClockRate:   clockRate,
	})
	if err != nil {
		enc.Close()
		return webrtc.RTPCodecParameters{}, err
	}

	// Pre-allocate buffers
	t.encBuf = make([]byte, enc.MaxEncodedSize())
	maxPackets := 10 // Audio frames are small, rarely need many packets
	t.packetBuf = make([]byte, maxPackets*int(t.config.MTU))
	t.packetInfo = make([]packetizer.PacketInfo, maxPackets)

	t.enc = enc
	t.pkt = pkt
	t.writer = ctx.WriteStream()
	t.codecParams = *selected
	t.ssrc = ctx.SSRC()
	t.payloadType = webrtc.PayloadType(selected.PayloadType)

	t.bound.Store(true)

	return t.codecParams, nil
}

func (t *AudioTrack) selectAudioCodec(offered []webrtc.RTPCodecParameters) (*webrtc.RTPCodecParameters, error) {
	if len(t.config.CodecPreferences) > 0 {
		selected, ok := pioncodec.Select(t.config.CodecPreferences, offered)
		if !ok {
			return nil, fmt.Errorf("no matching audio codec preference")
		}
		return &selected, nil
	}

	for i := range offered {
		if offered[i].MimeType == codec.Opus.MimeType() {
			return &offered[i], nil
		}
	}
	return &webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  codec.Opus.MimeType(),
			ClockRate: 48000,
			Channels:  2,
		},
	}, nil
}

func (t *AudioTrack) createAudioEncoderForSelection(selected webrtc.RTPCodecParameters) (encoder.AudioEncoder, error) {
	if len(t.config.CodecPreferences) > 0 {
		return pioncodec.NewAudioEncoder(selected, pioncodec.AudioFactoryConfig{
			SampleRate: t.config.SampleRate,
			Channels:   t.config.Channels,
			Bitrate:    t.config.Bitrate,
		})
	}
	return encoder.NewOpusEncoder(codec.OpusConfig{
		SampleRate: t.config.SampleRate,
		Channels:   t.config.Channels,
		Bitrate:    t.config.Bitrate,
	})
}

func cloneAudioTrackConfig(cfg AudioTrackConfig) AudioTrackConfig {
	cloned := cfg
	if len(cfg.CodecPreferences) > 0 {
		cloned.CodecPreferences = append([]webrtc.RTPCodecParameters(nil), cfg.CodecPreferences...)
	}
	return cloned
}

// NewVideoTrackFromPreset creates a video track using the preset's supported encode subset.
func NewVideoTrackFromPreset(set pioncodec.CodecSet, cfg VideoTrackConfig) (*VideoTrack, error) {
	video := set.SupportedOnly().VideoCodecs()
	if len(video) == 0 {
		return nil, ErrInvalidConfig
	}
	if codecType, ok := codec.ParseMimeType(video[0].MimeType); ok {
		cfg.Codec = codecType
	}
	cfg.CodecPreferences = append([]webrtc.RTPCodecParameters(nil), video...)
	return NewVideoTrack(cfg)
}

// NewAudioTrackFromPreset creates an audio track using the preset's supported encode subset.
func NewAudioTrackFromPreset(set pioncodec.CodecSet, cfg AudioTrackConfig) (*AudioTrack, error) {
	audio := set.SupportedOnly().AudioCodecs()
	if len(audio) == 0 {
		return nil, ErrInvalidConfig
	}
	cfg.CodecPreferences = append([]webrtc.RTPCodecParameters(nil), audio...)
	return NewAudioTrack(cfg)
}

// Unbind is called when the track is removed from the PeerConnection.
func (t *AudioTrack) Unbind(ctx webrtc.TrackLocalContext) error {
	if !t.bound.CompareAndSwap(true, false) {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.enc != nil {
		t.enc.Close()
		t.enc = nil
	}
	if t.pkt != nil {
		t.pkt.Close()
		t.pkt = nil
	}

	t.writer = nil
	return nil
}

// WriteFrame encodes an audio frame and writes RTP packets.
func (t *AudioTrack) WriteFrame(f *frame.AudioFrame) error {
	if t.closed.Load() {
		return ErrTrackClosed
	}
	if !t.bound.Load() {
		return ErrNotBound
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.enc == nil || t.pkt == nil || t.writer == nil {
		return ErrNotBound
	}

	// Encode frame
	n, err := t.enc.EncodeInto(f, t.encBuf)
	if err != nil {
		return err
	}

	// RTP timestamp for audio
	rtpTimestamp := uint32(f.PTS)

	// Packetize (Opus frames are typically single-packet)
	numPackets, err := t.pkt.PacketizeInto(
		t.encBuf[:n],
		rtpTimestamp,
		false, // audio has no keyframes
		t.packetBuf,
		t.packetInfo,
	)
	if err != nil {
		return err
	}

	// Write packets
	for i := 0; i < numPackets; i++ {
		info := t.packetInfo[i]
		pktData := t.packetBuf[info.Offset : info.Offset+info.Size]

		if _, err := t.writer.Write(pktData); err != nil {
			return err
		}
	}

	return nil
}

// WriteEncodedData writes pre-encoded Opus data as RTP packets.
func (t *AudioTrack) WriteEncodedData(data []byte, timestamp uint32) error {
	if t.closed.Load() {
		return ErrTrackClosed
	}
	if !t.bound.Load() {
		return ErrNotBound
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.pkt == nil || t.writer == nil {
		return ErrNotBound
	}

	numPackets, err := t.pkt.PacketizeInto(
		data,
		timestamp,
		false,
		t.packetBuf,
		t.packetInfo,
	)
	if err != nil {
		return err
	}

	for i := 0; i < numPackets; i++ {
		info := t.packetInfo[i]
		pktData := t.packetBuf[info.Offset : info.Offset+info.Size]

		if _, err := t.writer.Write(pktData); err != nil {
			return err
		}
	}

	return nil
}

// SetBitrate adjusts encoder bitrate.
func (t *AudioTrack) SetBitrate(bps uint32) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.enc != nil {
		return t.enc.SetBitrate(bps)
	}
	t.config.Bitrate = bps
	return nil
}

// Close releases all resources.
func (t *AudioTrack) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.enc != nil {
		t.enc.Close()
		t.enc = nil
	}
	if t.pkt != nil {
		t.pkt.Close()
		t.pkt = nil
	}

	return nil
}
