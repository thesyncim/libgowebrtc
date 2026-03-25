package pionsend

import (
	"sync"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pioncodec"
	"github.com/thesyncim/libgowebrtc/pkg/track"
)

const defaultAudioPTime = 20 * time.Millisecond

// AudioPublishConfig configures browser-shaped Opus publishing.
type AudioPublishConfig struct {
	TrackID          string
	StreamID         string
	SampleRate       int
	Channels         int
	Bitrate          uint32
	MTU              uint16
	PTime            time.Duration
	CodecPreferences []webrtc.RTPCodecParameters
	Browser          pioncodec.Browser
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

// PublishAudio creates a libgowebrtc-backed local audio track and wires it
// into a Pion RTPSender with browser-like Opus defaults.
func PublishAudio(pc *webrtc.PeerConnection, cfg AudioPublishConfig) (PublishedAudio, error) {
	if pc == nil {
		return nil, ErrNilPeerConnection
	}
	if cfg.TrackID == "" {
		return nil, ErrInvalidConfig
	}
	if cfg.StreamID == "" {
		cfg.StreamID = cfg.TrackID
	}
	if cfg.Browser == "" {
		cfg.Browser = pioncodec.BrowserChrome
	}
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 48000
	}
	if cfg.Channels <= 0 {
		cfg.Channels = 2
	}
	if cfg.Bitrate == 0 {
		cfg.Bitrate = 64000
	}
	if cfg.MTU == 0 {
		cfg.MTU = 1200
	}
	if cfg.PTime <= 0 {
		cfg.PTime = defaultAudioPTime
	}

	codecPreferences := cfg.CodecPreferences
	if len(codecPreferences) == 0 {
		codecPreferences = defaultAudioCodecPreferences(cfg.Browser)
	}
	cfg.CodecPreferences = append([]webrtc.RTPCodecParameters(nil), codecPreferences...)

	audioTrack, err := track.NewAudioTrack(track.AudioTrackConfig{
		ID:               cfg.TrackID,
		StreamID:         cfg.StreamID,
		SampleRate:       cfg.SampleRate,
		Channels:         cfg.Channels,
		Bitrate:          cfg.Bitrate,
		MTU:              cfg.MTU,
		CodecPreferences: codecPreferences,
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
		cfg:    cfg,
		track:  audioTrack,
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

func defaultAudioCodecPreferences(browser pioncodec.Browser) []webrtc.RTPCodecParameters {
	return pioncodec.BrowserPreset(browser, pioncodec.DirectionEncode, pioncodec.PresetModeSupported).AudioCodecs()
}
