// Package pionsend provides browser-shaped Pion publishing helpers backed by
// libgowebrtc local tracks.
package pionsend

import (
	"errors"
	"fmt"
	"sync"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pioncodec"
	"github.com/thesyncim/libgowebrtc/pkg/track"
)

const (
	midRTPHeaderExtensionURI           = "urn:ietf:params:rtp-hdrext:sdes:mid"
	rtpStreamIDHeaderExtensionURI      = "urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id"
	repairedStreamIDHeaderExtensionURI = "urn:ietf:params:rtp-hdrext:sdes:repaired-rtp-stream-id"
)

var (
	ErrNilPeerConnection       = errors.New("pionsend: peer connection is nil")
	ErrInvalidConfig           = errors.New("pionsend: invalid config")
	ErrNilVideoFrame           = errors.New("pionsend: video frame is nil")
	ErrInvalidLayerIndex       = errors.New("pionsend: invalid layer index")
	ErrUnsupportedLayeredCodec = errors.New("pionsend: automatic DD layering requires VP9 or AV1")
)

// VideoPublishConfig configures browser-shaped layered or simulcast publish.
type VideoPublishConfig struct {
	TrackID          string
	StreamID         string
	Width            int
	Height           int
	Bitrate          uint32
	FPS              float64
	MTU              uint16
	CodecPreferences []webrtc.RTPCodecParameters
	SVC              *codec.SVCConfig
	Browser          pioncodec.Browser
}

// PublishedVideo describes an active layered publisher.
type PublishedVideo interface {
	WriteFrame(*frame.VideoFrame, bool) error
	RequestKeyFrame()
	SetLayerActive(index int, active bool) error
	SetLayerBitrate(index int, maxBitrate uint32) error
	Encodings() []PublishedEncoding
	Sender() *webrtc.RTPSender
	Close() error
}

// PublishedEncoding describes a single published encoding.
type PublishedEncoding struct {
	Index   int
	RID     string
	Width   int
	Height  int
	Bitrate uint32
	Track   webrtc.TrackLocal
}

type encodingRuntime struct {
	PublishedEncoding
	videoTrack *track.VideoTrack
	active     bool
	scale      float64
	scaled     *frame.VideoFrame
}

type publishedVideo struct {
	cfg       VideoPublishConfig
	sender    *webrtc.RTPSender
	encodings []*encodingRuntime
	closed    bool
	validated bool
	mu        sync.Mutex
}

// RequiredVideoHeaderExtensionURIs returns the RTP header extensions callers
// should register with their MediaEngine before creating the PeerConnection.
func RequiredVideoHeaderExtensionURIs(cfg VideoPublishConfig) []string {
	uris := []string{midRTPHeaderExtensionURI}
	if cfg.SVC != nil && cfg.SVC.Mode.IsSimulcast() {
		uris = append(uris, rtpStreamIDHeaderExtensionURI, repairedStreamIDHeaderExtensionURI)
	}
	if cfg.SVC != nil && !cfg.SVC.Mode.IsSimulcast() {
		uris = append(uris, track.DependencyDescriptorRTPHeaderExtensionURI)
	}
	return uris
}

// PublishVideo creates one or more libgowebrtc-backed local tracks and wires
// them into a Pion RTPSender with browser-like defaults.
func PublishVideo(pc *webrtc.PeerConnection, cfg VideoPublishConfig) (PublishedVideo, error) {
	if pc == nil {
		return nil, ErrNilPeerConnection
	}
	if cfg.TrackID == "" || cfg.Width <= 0 || cfg.Height <= 0 || cfg.FPS <= 0 {
		return nil, ErrInvalidConfig
	}
	if cfg.StreamID == "" {
		cfg.StreamID = cfg.TrackID
	}
	if cfg.Browser == "" {
		cfg.Browser = pioncodec.BrowserChrome
	}
	if cfg.Bitrate == 0 {
		cfg.Bitrate = codec.DefaultVP8Config(cfg.Width, cfg.Height).Bitrate
	}

	codecPreferences := cfg.CodecPreferences
	if len(codecPreferences) == 0 {
		codecPreferences = defaultCodecPreferences(cfg)
	}
	cfg.CodecPreferences = append([]webrtc.RTPCodecParameters(nil), codecPreferences...)

	layers := deriveEncodingConfigs(cfg)
	if len(layers) == 0 {
		return nil, ErrInvalidConfig
	}

	runtimeEncodings := make([]*encodingRuntime, 0, len(layers))
	for i, layer := range layers {
		videoTrack, err := track.NewVideoTrack(track.VideoTrackConfig{
			ID:               cfg.TrackID,
			StreamID:         cfg.StreamID,
			RID:              layer.RID,
			Codec:            codecFromPreferences(codecPreferences),
			Width:            layer.Width,
			Height:           layer.Height,
			Bitrate:          layer.Bitrate,
			FPS:              cfg.FPS,
			MTU:              cfg.MTU,
			CodecPreferences: codecPreferences,
			SVC:              layer.SVC,
		})
		if err != nil {
			closePublishedTracks(runtimeEncodings)
			return nil, err
		}

		runtimeEncodings = append(runtimeEncodings, &encodingRuntime{
			PublishedEncoding: PublishedEncoding{
				Index:   i,
				RID:     layer.RID,
				Width:   layer.Width,
				Height:  layer.Height,
				Bitrate: layer.Bitrate,
				Track:   videoTrack,
			},
			videoTrack: videoTrack,
			active:     layer.Active,
			scale:      layer.Scale,
			scaled:     layer.AllocScaledFrame(),
		})
	}

	sender, err := pc.AddTrack(runtimeEncodings[0].videoTrack)
	if err != nil {
		closePublishedTracks(runtimeEncodings)
		return nil, err
	}
	for i := 1; i < len(runtimeEncodings); i++ {
		if err := sender.AddEncoding(runtimeEncodings[i].videoTrack); err != nil {
			closePublishedTracks(runtimeEncodings)
			return nil, err
		}
	}

	return &publishedVideo{
		cfg:       cfg,
		sender:    sender,
		encodings: runtimeEncodings,
	}, nil
}

func (p *publishedVideo) WriteFrame(src *frame.VideoFrame, forceKeyframe bool) error {
	if src == nil {
		return ErrNilVideoFrame
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	if err := p.validateNegotiatedLocked(); err != nil {
		return err
	}

	for _, encoding := range p.encodings {
		if !encoding.active {
			continue
		}

		frameForLayer := src
		if encoding.scaled != nil && encoding.scale > 1.0 {
			track.ScaleI420Frame(src, encoding.scaled, encoding.scale)
			encoding.scaled.PTS = src.PTS
			encoding.scaled.Timestamp = src.Timestamp
			encoding.scaled.IsKeyframe = src.IsKeyframe
			frameForLayer = encoding.scaled
		}

		if err := encoding.videoTrack.WriteFrame(frameForLayer, forceKeyframe); err != nil {
			return err
		}
	}

	return nil
}

func (p *publishedVideo) RequestKeyFrame() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, encoding := range p.encodings {
		encoding.videoTrack.RequestKeyFrame()
	}
}

func (p *publishedVideo) SetLayerActive(index int, active bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if index < 0 || index >= len(p.encodings) {
		return ErrInvalidLayerIndex
	}
	p.encodings[index].active = active
	return nil
}

func (p *publishedVideo) SetLayerBitrate(index int, maxBitrate uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if index < 0 || index >= len(p.encodings) {
		return ErrInvalidLayerIndex
	}
	p.encodings[index].Bitrate = maxBitrate
	return p.encodings[index].videoTrack.SetBitrate(maxBitrate)
}

func (p *publishedVideo) Encodings() []PublishedEncoding {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]PublishedEncoding, len(p.encodings))
	for i, encoding := range p.encodings {
		out[i] = encoding.PublishedEncoding
	}
	return out
}

func (p *publishedVideo) Sender() *webrtc.RTPSender {
	return p.sender
}

func (p *publishedVideo) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true
	return closePublishedTracks(p.encodings)
}

func (p *publishedVideo) validateNegotiatedLocked() error {
	if p.validated {
		return nil
	}

	required := RequiredVideoHeaderExtensionURIs(p.cfg)
	var boundContext *track.RTPPacketContext
	for _, encoding := range p.encodings {
		ctx, ok := encoding.videoTrack.RTPContext()
		if ok {
			boundContext = &ctx
			break
		}
	}
	if boundContext == nil {
		return nil
	}

	if p.cfg.SVC != nil && !p.cfg.SVC.Mode.IsSimulcast() {
		selectedCodec, ok := codec.ParseMimeType(boundContext.CodecParameters.MimeType)
		if !ok || (selectedCodec != codec.VP9 && selectedCodec != codec.AV1) {
			return ErrUnsupportedLayeredCodec
		}
	}

	for _, uri := range required {
		if _, ok := boundContext.HeaderExtensionID(uri); !ok {
			return fmt.Errorf("pionsend: negotiated RTP header extension missing: %s", uri)
		}
	}

	p.validated = true
	return nil
}

type encodingConfig struct {
	RID     string
	Width   int
	Height  int
	Bitrate uint32
	Scale   float64
	SVC     *codec.SVCConfig
	Active  bool
}

func (c encodingConfig) AllocScaledFrame() *frame.VideoFrame {
	if c.Scale <= 1.0 {
		return nil
	}
	return frame.NewI420Frame(c.Width, c.Height)
}

func defaultCodecPreferences(cfg VideoPublishConfig) []webrtc.RTPCodecParameters {
	base := pioncodec.BrowserPreset(cfg.Browser, pioncodec.DirectionEncode, pioncodec.PresetModeSupported).VideoCodecs()
	if cfg.SVC == nil || cfg.SVC.Mode.IsSimulcast() {
		return base
	}

	ordered := make([]webrtc.RTPCodecParameters, 0, len(base))
	for _, preferred := range []string{webrtc.MimeTypeVP9, webrtc.MimeTypeAV1, webrtc.MimeTypeH264, webrtc.MimeTypeVP8} {
		for _, candidate := range base {
			if candidate.MimeType == preferred {
				ordered = append(ordered, candidate)
			}
		}
	}
	if len(ordered) == 0 {
		return base
	}
	return ordered
}

func codecFromPreferences(preferred []webrtc.RTPCodecParameters) codec.Type {
	for _, params := range preferred {
		if codecType, ok := codec.ParseMimeType(params.MimeType); ok {
			return codecType
		}
	}
	return codec.VP8
}

func deriveEncodingConfigs(cfg VideoPublishConfig) []encodingConfig {
	if cfg.SVC == nil || !cfg.SVC.Mode.IsSimulcast() {
		return []encodingConfig{{
			Width:   cfg.Width,
			Height:  cfg.Height,
			Bitrate: cfg.Bitrate,
			Scale:   1.0,
			SVC:     cloneSVCConfig(cfg.SVC),
			Active:  true,
		}}
	}

	spatialLayers := cfg.SVC.Mode.SpatialLayers()
	if spatialLayers <= 0 {
		return nil
	}

	weights := defaultLayerWeights(spatialLayers)
	widths := make([]int, spatialLayers)
	heights := make([]int, spatialLayers)
	bitrates := make([]uint32, spatialLayers)
	for i := 0; i < spatialLayers; i++ {
		shift := spatialLayers - i - 1
		widths[i] = evenDimension(maxInt(2, cfg.Width>>shift))
		heights[i] = evenDimension(maxInt(2, cfg.Height>>shift))
		bitrates[i] = weightedBitrate(cfg.Bitrate, weights[i], weights)
	}
	if len(cfg.SVC.Layers) > 0 {
		for i := 0; i < spatialLayers && i < len(cfg.SVC.Layers); i++ {
			layer := cfg.SVC.Layers[i]
			widths[i] = clampPositive(layer.Width, widths[i])
			heights[i] = clampPositive(layer.Height, heights[i])
			bitrates[i] = clampPositiveUint32(layer.Bitrate, bitrates[i])
		}
	}

	rids := defaultRIDs(spatialLayers)
	temporalMode := codec.SVCModeL1T1
	if cfg.SVC.Mode.TemporalLayers() >= 3 {
		temporalMode = codec.SVCModeL1T3
	}

	layers := make([]encodingConfig, 0, spatialLayers)
	for i := 0; i < spatialLayers; i++ {
		scale := float64(cfg.Width) / float64(widths[i])
		active := true
		if len(cfg.SVC.Layers) > 0 && i < len(cfg.SVC.Layers) {
			active = cfg.SVC.Layers[i].Active
		}
		layers = append(layers, encodingConfig{
			RID:     rids[i],
			Width:   widths[i],
			Height:  heights[i],
			Bitrate: bitrates[i],
			Scale:   scale,
			SVC: &codec.SVCConfig{
				Mode: temporalMode,
			},
			Active: active,
		})
	}
	return layers
}

func cloneSVCConfig(in *codec.SVCConfig) *codec.SVCConfig {
	if in == nil {
		return nil
	}
	cloned := *in
	if len(in.Layers) > 0 {
		cloned.Layers = append([]codec.SVCLayerConfig(nil), in.Layers...)
	}
	return &cloned
}

func closePublishedTracks(encodings []*encodingRuntime) error {
	var firstErr error
	for _, encoding := range encodings {
		if encoding == nil || encoding.videoTrack == nil {
			continue
		}
		if err := encoding.videoTrack.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func defaultLayerWeights(count int) []int {
	switch count {
	case 2:
		return []int{1, 2}
	case 3:
		return []int{1, 2, 4}
	default:
		out := make([]int, count)
		for i := range out {
			out[i] = 1
		}
		return out
	}
}

func defaultRIDs(count int) []string {
	switch count {
	case 2:
		return []string{"h", "f"}
	case 3:
		return []string{"q", "h", "f"}
	default:
		return []string{""}
	}
}

func weightedBitrate(total uint32, weight int, weights []int) uint32 {
	sum := 0
	for _, w := range weights {
		sum += w
	}
	if sum == 0 {
		return total
	}
	return uint32((uint64(total) * uint64(weight)) / uint64(sum))
}

func clampPositive(v, fallback int) int {
	if v > 0 {
		return evenDimension(v)
	}
	return evenDimension(fallback)
}

func clampPositiveUint32(v, fallback uint32) uint32 {
	if v > 0 {
		return v
	}
	return fallback
}

func evenDimension(v int) int {
	if v <= 2 {
		return 2
	}
	if v%2 == 0 {
		return v
	}
	return v - 1
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
