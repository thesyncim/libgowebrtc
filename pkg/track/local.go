// Package track provides Pion-compatible TrackLocal implementations backed by libwebrtc.
package track

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/packetizer"
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
	writer       webrtc.TrackLocalWriter
	codecParams  webrtc.RTPCodecParameters
	ssrc         webrtc.SSRC
	payloadType  webrtc.PayloadType
	rtpTimestamp uint32

	// Pre-allocated buffers for allocation-free encoding
	encBuf     []byte
	packetBuf  []byte
	packetInfo []packetizer.PacketInfo

	mu     sync.Mutex
	closed atomic.Bool
	bound  atomic.Bool
}

// NewVideoTrack creates a new video track backed by libwebrtc encoder.
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

	t := &VideoTrack{
		id:       cfg.ID,
		streamID: cfg.StreamID,
		codec:    cfg.Codec,
		config:   cfg,
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
	if t.bound.Load() {
		return webrtc.RTPCodecParameters{}, ErrAlreadyBound
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Find matching codec from offered codecs
	codecs := ctx.CodecParameters()
	var selected *webrtc.RTPCodecParameters
	targetMime := t.codec.MimeType()

	for i := range codecs {
		if codecs[i].MimeType == targetMime {
			selected = &codecs[i]
			break
		}
	}

	if selected == nil {
		// Use our codec as default if not negotiated
		selected = &webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  targetMime,
				ClockRate: t.codec.ClockRate(),
			},
		}
	}

	// Create encoder
	enc, err := t.createEncoder()
	if err != nil {
		return webrtc.RTPCodecParameters{}, err
	}

	// Create packetizer
	pkt, err := packetizer.New(packetizer.Config{
		Codec:       t.codec,
		SSRC:        uint32(ctx.SSRC()),
		PayloadType: uint8(selected.PayloadType),
		MTU:         t.config.MTU,
		ClockRate:   t.codec.ClockRate(),
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

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.enc == nil || t.pkt == nil || t.writer == nil {
		return ErrNotBound
	}

	// Encode frame
	result, err := t.enc.EncodeInto(f, t.encBuf, forceKeyframe)
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

// Close releases all resources.
func (t *VideoTrack) Close() error {
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

func (t *VideoTrack) createEncoder() (encoder.VideoEncoder, error) {
	switch t.codec {
	case codec.H264:
		return encoder.NewH264Encoder(codec.H264Config{
			Width:   t.config.Width,
			Height:  t.config.Height,
			Bitrate: t.config.Bitrate,
			FPS:     t.config.FPS,
		})
	case codec.VP8:
		return encoder.NewVP8Encoder(codec.VP8Config{
			Width:   t.config.Width,
			Height:  t.config.Height,
			Bitrate: t.config.Bitrate,
			FPS:     t.config.FPS,
		})
	case codec.VP9:
		return encoder.NewVP9Encoder(codec.VP9Config{
			Width:   t.config.Width,
			Height:  t.config.Height,
			Bitrate: t.config.Bitrate,
			FPS:     t.config.FPS,
		})
	case codec.AV1:
		return encoder.NewAV1Encoder(codec.AV1Config{
			Width:   t.config.Width,
			Height:  t.config.Height,
			Bitrate: t.config.Bitrate,
			FPS:     t.config.FPS,
		})
	default:
		return nil, ErrInvalidConfig
	}
}

// AudioTrackConfig configures an audio track.
type AudioTrackConfig struct {
	ID         string
	StreamID   string
	SampleRate int
	Channels   int
	Bitrate    uint32
	MTU        uint16
}

// AudioTrack implements webrtc.TrackLocal using libwebrtc Opus encoder.
type AudioTrack struct {
	id       string
	streamID string

	config AudioTrackConfig
	enc    encoder.AudioEncoder
	pkt    packetizer.Packetizer

	writer       webrtc.TrackLocalWriter
	codecParams  webrtc.RTPCodecParameters
	ssrc         webrtc.SSRC
	payloadType  webrtc.PayloadType
	rtpTimestamp uint32

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
		config:   cfg,
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
	if t.bound.Load() {
		return webrtc.RTPCodecParameters{}, ErrAlreadyBound
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Find Opus codec
	codecs := ctx.CodecParameters()
	var selected *webrtc.RTPCodecParameters
	for i := range codecs {
		if codecs[i].MimeType == codec.Opus.MimeType() {
			selected = &codecs[i]
			break
		}
	}

	if selected == nil {
		selected = &webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  codec.Opus.MimeType(),
				ClockRate: 48000,
				Channels:  2,
			},
		}
	}

	// Create Opus encoder
	enc, err := encoder.NewOpusEncoder(codec.OpusConfig{
		SampleRate: t.config.SampleRate,
		Channels:   t.config.Channels,
		Bitrate:    t.config.Bitrate,
	})
	if err != nil {
		return webrtc.RTPCodecParameters{}, err
	}

	// Create packetizer
	pkt, err := packetizer.New(packetizer.Config{
		Codec:       codec.Opus,
		SSRC:        uint32(ctx.SSRC()),
		PayloadType: uint8(selected.PayloadType),
		MTU:         t.config.MTU,
		ClockRate:   48000,
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
