// Package media provides a browser-like API for media capture and track management.
// This mirrors the Web APIs like getUserMedia, MediaStreamTrack, and addTrack.
package media

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/track"
)

// Errors
var (
	ErrInvalidConstraints = errors.New("invalid constraints")
	ErrTrackNotFound      = errors.New("track not found")
	ErrStreamClosed       = errors.New("stream closed")
)

// VideoConstraints mirrors browser's MediaTrackConstraints for video.
type VideoConstraints struct {
	Width      int              // Desired width (0 = any)
	Height     int              // Desired height (0 = any)
	FrameRate  float64          // Desired framerate (0 = any)
	FacingMode string           // "user" or "environment" (for camera selection)
	DeviceID   string           // Specific device ID
	Codec      codec.Type       // Preferred codec (default: H264)
	Bitrate    uint32           // Target bitrate (0 = auto)
	SVC        *codec.SVCConfig // SVC configuration (nil = none)
}

// AudioConstraints mirrors browser's MediaTrackConstraints for audio.
type AudioConstraints struct {
	SampleRate       int    // Desired sample rate (0 = 48000)
	ChannelCount     int    // Desired channels (0 = 2)
	EchoCancellation bool   // Enable echo cancellation
	NoiseSuppression bool   // Enable noise suppression
	AutoGainControl  bool   // Enable auto gain control
	DeviceID         string // Specific device ID
	Bitrate          uint32 // Target bitrate (0 = 64000)
}

// Constraints mirrors browser's MediaStreamConstraints.
type Constraints struct {
	Video *VideoConstraints // nil = no video
	Audio *AudioConstraints // nil = no audio
}

// MediaStreamTrack mirrors browser's MediaStreamTrack interface.
type MediaStreamTrack interface {
	// ID returns the track's unique identifier.
	ID() string

	// Kind returns "video" or "audio".
	Kind() string

	// Label returns a human-readable label.
	Label() string

	// Enabled returns/sets whether the track is enabled.
	Enabled() bool
	SetEnabled(enabled bool)

	// Muted returns whether the track is muted.
	Muted() bool

	// ReadyState returns "live" or "ended".
	ReadyState() string

	// Stop stops the track.
	Stop()

	// Clone creates a clone of this track.
	Clone() MediaStreamTrack

	// GetConstraints returns the current constraints.
	GetConstraints() interface{}

	// ApplyConstraints applies new constraints.
	ApplyConstraints(constraints interface{}) error

	// GetSettings returns current settings.
	GetSettings() interface{}

	// Internal: Get the underlying Pion track for addTrack
	PionTrack() webrtc.TrackLocal
}

// MediaStream mirrors browser's MediaStream interface.
type MediaStream struct {
	id          string
	videoTracks []MediaStreamTrack
	audioTracks []MediaStreamTrack
	mu          sync.RWMutex
	closed      atomic.Bool
}

// NewMediaStream creates a new empty MediaStream.
func NewMediaStream() *MediaStream {
	return &MediaStream{
		id:          generateID(),
		videoTracks: make([]MediaStreamTrack, 0),
		audioTracks: make([]MediaStreamTrack, 0),
	}
}

// ID returns the stream's unique identifier.
func (s *MediaStream) ID() string {
	return s.id
}

// GetVideoTracks returns all video tracks.
func (s *MediaStream) GetVideoTracks() []MediaStreamTrack {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tracks := make([]MediaStreamTrack, len(s.videoTracks))
	copy(tracks, s.videoTracks)
	return tracks
}

// GetAudioTracks returns all audio tracks.
func (s *MediaStream) GetAudioTracks() []MediaStreamTrack {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tracks := make([]MediaStreamTrack, len(s.audioTracks))
	copy(tracks, s.audioTracks)
	return tracks
}

// GetTracks returns all tracks (video + audio).
func (s *MediaStream) GetTracks() []MediaStreamTrack {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tracks := make([]MediaStreamTrack, 0, len(s.videoTracks)+len(s.audioTracks))
	tracks = append(tracks, s.videoTracks...)
	tracks = append(tracks, s.audioTracks...)
	return tracks
}

// GetTrackByID returns a track by ID.
func (s *MediaStream) GetTrackByID(id string) MediaStreamTrack {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.videoTracks {
		if t.ID() == id {
			return t
		}
	}
	for _, t := range s.audioTracks {
		if t.ID() == id {
			return t
		}
	}
	return nil
}

// AddTrack adds a track to the stream.
func (s *MediaStream) AddTrack(t MediaStreamTrack) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.Kind() == "video" {
		s.videoTracks = append(s.videoTracks, t)
	} else {
		s.audioTracks = append(s.audioTracks, t)
	}
}

// RemoveTrack removes a track from the stream.
func (s *MediaStream) RemoveTrack(t MediaStreamTrack) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.Kind() == "video" {
		for i, track := range s.videoTracks {
			if track.ID() == t.ID() {
				s.videoTracks = append(s.videoTracks[:i], s.videoTracks[i+1:]...)
				return
			}
		}
	} else {
		for i, track := range s.audioTracks {
			if track.ID() == t.ID() {
				s.audioTracks = append(s.audioTracks[:i], s.audioTracks[i+1:]...)
				return
			}
		}
	}
}

// Clone creates a clone of this stream with cloned tracks.
func (s *MediaStream) Clone() *MediaStream {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clone := NewMediaStream()
	for _, t := range s.videoTracks {
		clone.videoTracks = append(clone.videoTracks, t.Clone())
	}
	for _, t := range s.audioTracks {
		clone.audioTracks = append(clone.audioTracks, t.Clone())
	}
	return clone
}

// Active returns true if any track is live.
func (s *MediaStream) Active() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.videoTracks {
		if t.ReadyState() == "live" {
			return true
		}
	}
	for _, t := range s.audioTracks {
		if t.ReadyState() == "live" {
			return true
		}
	}
	return false
}

// VideoTrackSettings represents current video track settings.
type VideoTrackSettings struct {
	Width      int
	Height     int
	FrameRate  float64
	DeviceID   string
	FacingMode string
}

// AudioTrackSettings represents current audio track settings.
type AudioTrackSettings struct {
	SampleRate       int
	ChannelCount     int
	DeviceID         string
	EchoCancellation bool
	NoiseSuppression bool
	AutoGainControl  bool
}

// videoStreamTrack wraps VideoTrack as MediaStreamTrack.
type videoStreamTrack struct {
	track       *track.VideoTrack
	constraints VideoConstraints
	settings    VideoTrackSettings
	enabled     atomic.Bool
	muted       atomic.Bool
	readyState  atomic.Value // "live" or "ended"
	label       string
}

// audioStreamTrack wraps AudioTrack as MediaStreamTrack.
type audioStreamTrack struct {
	track       *track.AudioTrack
	constraints AudioConstraints
	settings    AudioTrackSettings
	enabled     atomic.Bool
	muted       atomic.Bool
	readyState  atomic.Value
	label       string
}

// CreateVideoTrack creates a video track from constraints (like getUserMedia for video).
func CreateVideoTrack(constraints VideoConstraints) (MediaStreamTrack, error) {
	if constraints.Width <= 0 {
		constraints.Width = 1280
	}
	if constraints.Height <= 0 {
		constraints.Height = 720
	}
	if constraints.FrameRate <= 0 {
		constraints.FrameRate = 30
	}
	if constraints.Codec == 0 {
		constraints.Codec = codec.H264
	}
	if constraints.Bitrate == 0 {
		constraints.Bitrate = codec.DefaultH264Config(constraints.Width, constraints.Height).Bitrate
	}

	vt, err := track.NewVideoTrack(track.VideoTrackConfig{
		ID:      generateID(),
		Codec:   constraints.Codec,
		Width:   constraints.Width,
		Height:  constraints.Height,
		Bitrate: constraints.Bitrate,
		FPS:     constraints.FrameRate,
	})
	if err != nil {
		return nil, err
	}

	t := &videoStreamTrack{
		track:       vt,
		constraints: constraints,
		settings: VideoTrackSettings{
			Width:     constraints.Width,
			Height:    constraints.Height,
			FrameRate: constraints.FrameRate,
			DeviceID:  constraints.DeviceID,
		},
		label: "libwebrtc-video",
	}
	t.enabled.Store(true)
	t.muted.Store(false)
	t.readyState.Store("live")

	return t, nil
}

// CreateAudioTrack creates an audio track from constraints (like getUserMedia for audio).
func CreateAudioTrack(constraints AudioConstraints) (MediaStreamTrack, error) {
	if constraints.SampleRate <= 0 {
		constraints.SampleRate = 48000
	}
	if constraints.ChannelCount <= 0 {
		constraints.ChannelCount = 2
	}
	if constraints.Bitrate == 0 {
		constraints.Bitrate = 64000
	}

	at, err := track.NewAudioTrack(track.AudioTrackConfig{
		ID:         generateID(),
		SampleRate: constraints.SampleRate,
		Channels:   constraints.ChannelCount,
		Bitrate:    constraints.Bitrate,
	})
	if err != nil {
		return nil, err
	}

	t := &audioStreamTrack{
		track:       at,
		constraints: constraints,
		settings: AudioTrackSettings{
			SampleRate:       constraints.SampleRate,
			ChannelCount:     constraints.ChannelCount,
			DeviceID:         constraints.DeviceID,
			EchoCancellation: constraints.EchoCancellation,
			NoiseSuppression: constraints.NoiseSuppression,
			AutoGainControl:  constraints.AutoGainControl,
		},
		label: "libwebrtc-audio",
	}
	t.enabled.Store(true)
	t.muted.Store(false)
	t.readyState.Store("live")

	return t, nil
}

// GetUserMedia mirrors browser's navigator.mediaDevices.getUserMedia().
// Returns a MediaStream with requested tracks.
func GetUserMedia(constraints Constraints) (*MediaStream, error) {
	stream := NewMediaStream()

	if constraints.Video != nil {
		vt, err := CreateVideoTrack(*constraints.Video)
		if err != nil {
			return nil, err
		}
		stream.AddTrack(vt)
	}

	if constraints.Audio != nil {
		at, err := CreateAudioTrack(*constraints.Audio)
		if err != nil {
			return nil, err
		}
		stream.AddTrack(at)
	}

	return stream, nil
}

// GetDisplayMedia mirrors browser's navigator.mediaDevices.getDisplayMedia().
// Returns a MediaStream configured for screen sharing.
func GetDisplayMedia(constraints Constraints) (*MediaStream, error) {
	// Default to screen sharing optimized settings
	if constraints.Video != nil {
		if constraints.Video.SVC == nil {
			constraints.Video.SVC = codec.SVCPresetScreenShare()
		}
	}
	return GetUserMedia(constraints)
}

// --- videoStreamTrack implementation ---

func (t *videoStreamTrack) ID() string         { return t.track.ID() }
func (t *videoStreamTrack) Kind() string       { return "video" }
func (t *videoStreamTrack) Label() string      { return t.label }
func (t *videoStreamTrack) Enabled() bool      { return t.enabled.Load() }
func (t *videoStreamTrack) SetEnabled(e bool)  { t.enabled.Store(e) }
func (t *videoStreamTrack) Muted() bool        { return t.muted.Load() }
func (t *videoStreamTrack) ReadyState() string { return t.readyState.Load().(string) }

func (t *videoStreamTrack) Stop() {
	t.readyState.Store("ended")
	t.track.Close()
}

func (t *videoStreamTrack) Clone() MediaStreamTrack {
	clone, _ := CreateVideoTrack(t.constraints)
	return clone
}

func (t *videoStreamTrack) GetConstraints() interface{} { return t.constraints }
func (t *videoStreamTrack) GetSettings() interface{}    { return t.settings }

func (t *videoStreamTrack) ApplyConstraints(c interface{}) error {
	vc, ok := c.(VideoConstraints)
	if !ok {
		return ErrInvalidConstraints
	}
	// Apply bitrate change at runtime
	if vc.Bitrate > 0 && vc.Bitrate != t.constraints.Bitrate {
		t.track.SetBitrate(vc.Bitrate)
		t.constraints.Bitrate = vc.Bitrate
	}
	if vc.FrameRate > 0 && vc.FrameRate != t.constraints.FrameRate {
		t.track.SetFramerate(vc.FrameRate)
		t.constraints.FrameRate = vc.FrameRate
		t.settings.FrameRate = vc.FrameRate
	}
	return nil
}

func (t *videoStreamTrack) PionTrack() webrtc.TrackLocal { return t.track }

// WriteFrame writes a video frame (for feeding raw video data).
func (t *videoStreamTrack) WriteFrame(f *frame.VideoFrame, forceKeyframe bool) error {
	if !t.enabled.Load() || t.readyState.Load().(string) != "live" {
		return nil
	}
	return t.track.WriteFrame(f, forceKeyframe)
}

// RequestKeyFrame requests a keyframe.
func (t *videoStreamTrack) RequestKeyFrame() {
	t.track.RequestKeyFrame()
}

// --- audioStreamTrack implementation ---

func (t *audioStreamTrack) ID() string         { return t.track.ID() }
func (t *audioStreamTrack) Kind() string       { return "audio" }
func (t *audioStreamTrack) Label() string      { return t.label }
func (t *audioStreamTrack) Enabled() bool      { return t.enabled.Load() }
func (t *audioStreamTrack) SetEnabled(e bool)  { t.enabled.Store(e) }
func (t *audioStreamTrack) Muted() bool        { return t.muted.Load() }
func (t *audioStreamTrack) ReadyState() string { return t.readyState.Load().(string) }

func (t *audioStreamTrack) Stop() {
	t.readyState.Store("ended")
	t.track.Close()
}

func (t *audioStreamTrack) Clone() MediaStreamTrack {
	clone, _ := CreateAudioTrack(t.constraints)
	return clone
}

func (t *audioStreamTrack) GetConstraints() interface{} { return t.constraints }
func (t *audioStreamTrack) GetSettings() interface{}    { return t.settings }

func (t *audioStreamTrack) ApplyConstraints(c interface{}) error {
	ac, ok := c.(AudioConstraints)
	if !ok {
		return ErrInvalidConstraints
	}
	if ac.Bitrate > 0 && ac.Bitrate != t.constraints.Bitrate {
		t.track.SetBitrate(ac.Bitrate)
		t.constraints.Bitrate = ac.Bitrate
	}
	return nil
}

func (t *audioStreamTrack) PionTrack() webrtc.TrackLocal { return t.track }

// WriteFrame writes an audio frame.
func (t *audioStreamTrack) WriteFrame(f *frame.AudioFrame) error {
	if !t.enabled.Load() || t.readyState.Load().(string) != "live" {
		return nil
	}
	return t.track.WriteFrame(f)
}

// --- Helper to add tracks to PeerConnection ---

// AddTracksToPC adds all tracks from a MediaStream to a PeerConnection.
// Mirrors browser's pc.addTrack(track, stream) workflow.
func AddTracksToPC(pc *webrtc.PeerConnection, stream *MediaStream) ([]*webrtc.RTPSender, error) {
	var senders []*webrtc.RTPSender

	for _, t := range stream.GetTracks() {
		sender, err := pc.AddTrack(t.PionTrack())
		if err != nil {
			return senders, err
		}
		senders = append(senders, sender)
	}

	return senders, nil
}

// --- Utilities ---

var idCounter atomic.Uint64

func generateID() string {
	return "libwebrtc-" + string(rune('0'+idCounter.Add(1)))
}

// --- Type assertions for accessing underlying tracks ---

// AsVideoTrack returns the underlying VideoTrack if this is a video MediaStreamTrack.
func AsVideoTrack(t MediaStreamTrack) (*track.VideoTrack, bool) {
	if vt, ok := t.(*videoStreamTrack); ok {
		return vt.track, true
	}
	return nil, false
}

// AsAudioTrack returns the underlying AudioTrack if this is an audio MediaStreamTrack.
func AsAudioTrack(t MediaStreamTrack) (*track.AudioTrack, bool) {
	if at, ok := t.(*audioStreamTrack); ok {
		return at.track, true
	}
	return nil, false
}

// AsVideoEncoder returns the underlying encoder if track supports it.
func AsVideoEncoder(t MediaStreamTrack) (encoder.VideoEncoder, bool) {
	// The track doesn't expose encoder directly in this design
	// This would need internal access
	return nil, false
}
