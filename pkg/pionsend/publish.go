// Package pionsend provides explicit Pion publishing helpers backed by
// libgowebrtc local tracks.
package pionsend

import (
	"errors"
	"fmt"
	"sync"

	"github.com/pion/webrtc/v4"

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
	ErrNilPeerConnection = errors.New("pionsend: peer connection is nil")
	ErrInvalidConfig     = errors.New("pionsend: invalid config")
	ErrNilVideoFrame     = errors.New("pionsend: video frame is nil")
	ErrNilAudioFrame     = errors.New("pionsend: audio frame is nil")
	ErrInvalidLayerIndex = errors.New("pionsend: invalid layer index")
)

// VideoPublishConfig wires already-configured local tracks into a single sender.
type VideoPublishConfig struct {
	Encodings                []VideoPublishEncoding
	RequiredHeaderExtensions []string
}

// VideoPublishEncoding describes one sender encoding backed by a concrete local track.
type VideoPublishEncoding struct {
	Track                 *track.VideoTrack
	Width                 int
	Height                int
	Bitrate               uint32
	ScaleResolutionDownBy float64
	Active                bool
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
	Track   *track.VideoTrack
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

// PublishVideo wires one or more already-configured local tracks into a single
// Pion RTPSender. Track construction, codec policy, and simulcast layout all
// live with the caller.
func PublishVideo(pc *webrtc.PeerConnection, cfg VideoPublishConfig) (PublishedVideo, error) {
	if pc == nil {
		return nil, ErrNilPeerConnection
	}
	if len(cfg.Encodings) == 0 {
		return nil, ErrInvalidConfig
	}
	if !validVideoPublishEncodings(cfg.Encodings) {
		return nil, ErrInvalidConfig
	}
	cfg.RequiredHeaderExtensions = append([]string(nil), cfg.RequiredHeaderExtensions...)
	runtimeEncodings := make([]*encodingRuntime, 0, len(cfg.Encodings))
	for i, encoding := range cfg.Encodings {
		runtimeEncodings = append(runtimeEncodings, &encodingRuntime{
			PublishedEncoding: PublishedEncoding{
				Index:   i,
				RID:     encoding.Track.RID(),
				Width:   encoding.Width,
				Height:  encoding.Height,
				Bitrate: encoding.Bitrate,
				Track:   encoding.Track,
			},
			videoTrack: encoding.Track,
			active:     encoding.Active,
			scale:      encoding.ScaleResolutionDownBy,
			scaled:     allocScaledFrame(encoding),
		})
	}

	cfg.Encodings = nil

	sender, err := pc.AddTrack(runtimeEncodings[0].videoTrack)
	if err != nil {
		return nil, err
	}
	for i := 1; i < len(runtimeEncodings); i++ {
		if err := sender.AddEncoding(runtimeEncodings[i].videoTrack); err != nil {
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

	for _, uri := range p.cfg.RequiredHeaderExtensions {
		if _, ok := boundContext.HeaderExtensionID(uri); !ok {
			return fmt.Errorf("pionsend: negotiated RTP header extension missing: %s", uri)
		}
	}

	p.validated = true
	return nil
}

func validVideoPublishEncoding(encoding VideoPublishEncoding) bool {
	if encoding.Track == nil {
		return false
	}
	if encoding.Width <= 0 || encoding.Height <= 0 || encoding.Bitrate == 0 {
		return false
	}
	if encoding.Width%2 != 0 || encoding.Height%2 != 0 {
		return false
	}
	if encoding.ScaleResolutionDownBy < 1.0 {
		return false
	}
	return true
}

func validVideoPublishEncodings(encodings []VideoPublishEncoding) bool {
	if len(encodings) == 0 || !validVideoPublishEncoding(encodings[0]) {
		return false
	}
	baseID := encodings[0].Track.ID()
	baseStreamID := encodings[0].Track.StreamID()
	for i := 1; i < len(encodings); i++ {
		encoding := encodings[i]
		if !validVideoPublishEncoding(encoding) {
			return false
		}
		if encoding.Track.ID() != baseID || encoding.Track.StreamID() != baseStreamID {
			return false
		}
	}
	return true
}

func allocScaledFrame(encoding VideoPublishEncoding) *frame.VideoFrame {
	if encoding.ScaleResolutionDownBy <= 1.0 {
		return nil
	}
	return frame.NewI420Frame(encoding.Width, encoding.Height)
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
