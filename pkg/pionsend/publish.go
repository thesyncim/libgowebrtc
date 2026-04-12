// Package pionsend provides explicit Pion publishing helpers backed by
// libgowebrtc local tracks.
package pionsend

import (
	"errors"
	"fmt"
	"sync"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
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
	ErrNilAudioFrame           = errors.New("pionsend: audio frame is nil")
	ErrInvalidLayerIndex       = errors.New("pionsend: invalid layer index")
	ErrUnsupportedLayeredCodec = errors.New("pionsend: automatic DD layering requires VP9 or AV1")
)

// VideoPublishConfig configures explicit layered or simulcast publish wiring.
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
// them into a Pion RTPSender using explicit caller-provided publish settings.
func PublishVideo(pc *webrtc.PeerConnection, cfg VideoPublishConfig) (PublishedVideo, error) {
	if pc == nil {
		return nil, ErrNilPeerConnection
	}
	if cfg.TrackID == "" || cfg.StreamID == "" || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Bitrate == 0 || cfg.FPS <= 0 || cfg.MTU == 0 || len(cfg.CodecPreferences) == 0 {
		return nil, ErrInvalidConfig
	}
	selectedCodec, ok := codecFromPreferences(cfg.CodecPreferences)
	if !ok {
		return nil, ErrInvalidConfig
	}
	cfg.CodecPreferences = append([]webrtc.RTPCodecParameters(nil), cfg.CodecPreferences...)

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
			Codec:            selectedCodec,
			Width:            layer.Width,
			Height:           layer.Height,
			Bitrate:          layer.Bitrate,
			FPS:              cfg.FPS,
			MTU:              cfg.MTU,
			AutoKeyframe:     true,
			AutoBitrate:      true,
			AutoFramerate:    true,
			AutoResolution:   true,
			CodecPreferences: cfg.CodecPreferences,
			SVC:              layer.SVC,
		})
		if err != nil {
			if closeErr := closePublishedTracks(runtimeEncodings); closeErr != nil {
				return nil, errors.Join(err, closeErr)
			}
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
		if closeErr := closePublishedTracks(runtimeEncodings); closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
		return nil, err
	}
	for i := 1; i < len(runtimeEncodings); i++ {
		if err := sender.AddEncoding(runtimeEncodings[i].videoTrack); err != nil {
			if closeErr := closePublishedTracks(runtimeEncodings); closeErr != nil {
				return nil, errors.Join(err, closeErr)
			}
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
	if err := encoder.ValidateI420Frame(src); err != nil {
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

func codecFromPreferences(preferred []webrtc.RTPCodecParameters) (codec.Type, bool) {
	for _, params := range preferred {
		if codecType, ok := codec.ParseMimeType(params.MimeType); ok {
			return codecType, true
		}
	}
	return 0, false
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
	if len(cfg.SVC.Layers) != spatialLayers {
		return nil
	}
	temporalMode := codec.SVCModeL1T1
	if cfg.SVC.Mode.TemporalLayers() >= 3 {
		temporalMode = codec.SVCModeL1T3
	}

	layers := make([]encodingConfig, 0, spatialLayers)
	for i := 0; i < spatialLayers; i++ {
		layer := cfg.SVC.Layers[i]
		if layer.RID == "" || layer.Width <= 0 || layer.Height <= 0 || layer.Bitrate == 0 {
			return nil
		}
		if layer.Width > cfg.Width || layer.Height > cfg.Height {
			return nil
		}
		if layer.Width%2 != 0 || layer.Height%2 != 0 {
			return nil
		}
		if cfg.Width*layer.Height != cfg.Height*layer.Width {
			return nil
		}
		scale := 1.0
		if layer.Width != cfg.Width || layer.Height != cfg.Height {
			scale = float64(cfg.Width) / float64(layer.Width)
			if scale <= 1.0 {
				return nil
			}
		}
		layers = append(layers, encodingConfig{
			RID:     layer.RID,
			Width:   layer.Width,
			Height:  layer.Height,
			Bitrate: layer.Bitrate,
			Scale:   scale,
			SVC: &codec.SVCConfig{
				Mode: temporalMode,
			},
			Active: layer.Active,
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
