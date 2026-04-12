package pionsend

import (
	"sync"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/track"
)

// AudioPublishConfig wires an already-configured local audio track into a sender.
type AudioPublishConfig struct {
	Track *track.AudioTrack
}

// PublishedAudio describes an active audio publisher.
type PublishedAudio interface {
	WriteFrame(*frame.AudioFrame) error
	SetBitrate(uint32) error
	Sender() *webrtc.RTPSender
	Close() error
}

type publishedAudio struct {
	cfg    AudioPublishConfig
	track  *track.AudioTrack
	sender *webrtc.RTPSender

	mu     sync.Mutex
	closed bool
}

// PublishAudio wires an already-configured libgowebrtc audio track into a
// Pion RTPSender.
func PublishAudio(pc *webrtc.PeerConnection, cfg AudioPublishConfig) (PublishedAudio, error) {
	if pc == nil {
		return nil, ErrNilPeerConnection
	}
	if cfg.Track == nil {
		return nil, ErrInvalidConfig
	}

	sender, err := pc.AddTrack(cfg.Track)
	if err != nil {
		return nil, err
	}

	return &publishedAudio{
		cfg:    cfg,
		track:  cfg.Track,
		sender: sender,
	}, nil
}

func (p *publishedAudio) WriteFrame(src *frame.AudioFrame) error {
	if src == nil {
		return ErrNilAudioFrame
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	return p.track.WriteFrame(src)
}

func (p *publishedAudio) SetBitrate(bps uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	return p.track.SetBitrate(bps)
}

func (p *publishedAudio) Sender() *webrtc.RTPSender {
	return p.sender
}

func (p *publishedAudio) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true
	return p.track.Close()
}
