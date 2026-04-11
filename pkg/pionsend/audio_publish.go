package pionsend

import (
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/track"
)

// AudioPublishConfig configures explicit audio publishing.
type AudioPublishConfig struct {
	TrackID          string
	StreamID         string
	SampleRate       int
	Channels         int
	Bitrate          uint32
	MTU              uint16
	PTime            time.Duration
	CodecPreferences []webrtc.RTPCodecParameters
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

	samplesPerFrame int

	mu     sync.Mutex
	closed bool
}

// PublishAudio creates a libgowebrtc-backed local audio track and wires it
// into a Pion RTPSender using explicit caller-provided publish settings.
func PublishAudio(pc *webrtc.PeerConnection, cfg AudioPublishConfig) (PublishedAudio, error) {
	if pc == nil {
		return nil, ErrNilPeerConnection
	}
	if cfg.TrackID == "" || cfg.StreamID == "" || cfg.SampleRate <= 0 || cfg.Channels <= 0 || cfg.Bitrate == 0 || cfg.PTime <= 0 || len(cfg.CodecPreferences) == 0 {
		return nil, ErrInvalidConfig
	}
	samplesPerFrame, ok := samplesForPTime(cfg.SampleRate, cfg.PTime)
	if !ok {
		return nil, ErrInvalidConfig
	}

	cfg.CodecPreferences = append([]webrtc.RTPCodecParameters(nil), cfg.CodecPreferences...)

	audioTrack, err := track.NewAudioTrack(track.AudioTrackConfig{
		ID:               cfg.TrackID,
		StreamID:         cfg.StreamID,
		SampleRate:       cfg.SampleRate,
		Channels:         cfg.Channels,
		Bitrate:          cfg.Bitrate,
		MTU:              cfg.MTU,
		CodecPreferences: cfg.CodecPreferences,
	})
	if err != nil {
		return nil, err
	}

	sender, err := pc.AddTrack(audioTrack)
	if err != nil {
		_ = audioTrack.Close()
		return nil, err
	}

	return &publishedAudio{
		cfg:             cfg,
		track:           audioTrack,
		sender:          sender,
		samplesPerFrame: samplesPerFrame,
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
	if err := p.validateFrameLocked(src); err != nil {
		return err
	}
	return p.track.WriteFrame(src)
}

func (p *publishedAudio) SetBitrate(bps uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.cfg.Bitrate = bps
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

func (p *publishedAudio) validateFrameLocked(src *frame.AudioFrame) error {
	if src.SampleRate != p.cfg.SampleRate {
		return fmt.Errorf("pionsend: audio frame sample rate %d does not match configured %d", src.SampleRate, p.cfg.SampleRate)
	}
	if src.Channels != p.cfg.Channels {
		return fmt.Errorf("pionsend: audio frame channels %d do not match configured %d", src.Channels, p.cfg.Channels)
	}
	if p.samplesPerFrame > 0 && src.NumSamples != p.samplesPerFrame {
		return fmt.Errorf("pionsend: audio frame samples %d do not match configured ptime %s (%d samples)", src.NumSamples, p.cfg.PTime, p.samplesPerFrame)
	}
	return nil
}

func samplesForPTime(sampleRate int, ptime time.Duration) (int, bool) {
	if sampleRate <= 0 || ptime <= 0 {
		return 0, false
	}
	total := int64(sampleRate) * ptime.Nanoseconds()
	if total%int64(time.Second) != 0 {
		return 0, false
	}
	samples := total / int64(time.Second)
	if samples <= 0 {
		return 0, false
	}
	return int(samples), true
}
