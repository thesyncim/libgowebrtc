// Package media provides native capture helpers and track wrappers on top of
// libwebrtc-backed capture sources.
package media

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/track"
)

const defaultTrackMTU uint16 = 1200

// Errors
var (
	ErrInvalidCaptureConfig = errors.New("invalid capture config")
	ErrTrackNotFound        = errors.New("track not found")
	ErrStreamClosed         = errors.New("stream closed")
	ErrDeviceNotFound       = errors.New("device not found")
	ErrCaptureNotSupported  = errors.New("capture not supported without shim library")
)

// DeviceKind represents the type of capture device.
type DeviceKind string

const (
	// DeviceKindVideoInput represents a camera.
	DeviceKindVideoInput DeviceKind = "videoinput"
	// DeviceKindAudioInput represents a microphone.
	DeviceKindAudioInput DeviceKind = "audioinput"
	// DeviceKindAudioOutput represents a speaker.
	DeviceKindAudioOutput DeviceKind = "audiooutput"
)

// DeviceInfo describes an enumerated native capture device.
type DeviceInfo struct {
	// DeviceID is a unique identifier for the device.
	DeviceID string
	// Kind is the type of device (videoinput, audioinput, audiooutput).
	Kind DeviceKind
	// Label is a human-readable name for the device.
	Label string
}

// ListDevices returns available native media input and output devices.
func ListDevices() ([]DeviceInfo, error) {
	ffiDevices, err := ffi.EnumerateDevices()
	if err != nil {
		if errors.Is(err, ffi.ErrLibraryNotLoaded) {
			return nil, ErrCaptureNotSupported
		}
		return nil, err
	}

	return mapFFIDevices(ffiDevices), nil
}

// DisplayInfo describes an available monitor or window capture target.
type DisplayInfo struct {
	// ID is a unique identifier for the screen/window.
	ID int64
	// Title is the window title or screen name.
	Title string
	// IsWindow is true for windows, false for screens.
	IsWindow bool
}

// ListDisplays returns available monitor and window capture targets.
func ListDisplays() ([]DisplayInfo, error) {
	ffiScreens, err := ffi.EnumerateScreens()
	if err != nil {
		if errors.Is(err, ffi.ErrLibraryNotLoaded) {
			return nil, ErrCaptureNotSupported
		}
		return nil, err
	}

	return mapFFIScreens(ffiScreens), nil
}

// DisplayCaptureConfig opens a monitor or window capture source.
type DisplayCaptureConfig struct {
	Video *DisplayVideoConfig // Video configures the required display-capture video source.
	Audio *AudioCaptureConfig // Audio optionally requests additional audio capture alongside video.
}

// DisplayVideoConfig configures monitor or window capture.
type DisplayVideoConfig struct {
	Kind DisplayKind // Kind narrows capture to monitors or windows.
	// ScreenID selects a specific monitor from ListDisplays.
	ScreenID int64
	// WindowID selects a specific window from ListDisplays and takes precedence over ScreenID.
	WindowID         int64
	FrameRate        float64
	Width            int
	Height           int
	Codec            codec.Type
	Bitrate          uint32
	SVC              *codec.SVCConfig // SVC configures scalable or simulcast video output when supported.
	CodecPreferences []webrtc.RTPCodecParameters
}

// VideoCaptureConfig configures camera-backed capture.
type VideoCaptureConfig struct {
	Width            int
	Height           int
	FrameRate        float64
	FacingMode       CameraFacing
	DeviceID         string
	Codec            codec.Type
	Bitrate          uint32
	SVC              *codec.SVCConfig // SVC configures scalable or simulcast video output when supported.
	CodecPreferences []webrtc.RTPCodecParameters
}

// AudioCaptureConfig configures microphone-backed capture.
type AudioCaptureConfig struct {
	SampleRate       int
	ChannelCount     int
	EchoCancellation bool
	NoiseSuppression bool
	AutoGainControl  bool
	DeviceID         string
	Bitrate          uint32
	CodecPreferences []webrtc.RTPCodecParameters
}

// CaptureConfig opens camera and microphone capture sources.
type CaptureConfig struct {
	Video *VideoCaptureConfig
	Audio *AudioCaptureConfig
}

// MediaStreamTrack is the common track surface exposed by pkg/media streams.
type MediaStreamTrack interface {
	ID() string              // ID returns the stable track identifier.
	Kind() string            // Kind returns the media kind, such as "audio" or "video".
	Label() string           // Label returns the human-readable source label when available.
	Enabled() bool           // Enabled reports whether the track is enabled for delivery.
	SetEnabled(enabled bool) // SetEnabled enables or disables the track without stopping it.
	Muted() bool             // Muted reports whether the source is currently muted.
	ReadyState() string      // ReadyState returns the current lifecycle state.
	Stop()                   // Stop permanently ends the track.
	Clone() MediaStreamTrack // Clone returns an independent track view for the same source.
}

// VideoStreamTrack provides type-safe access to capture-backed video track
// config and settings.
type VideoStreamTrack interface {
	MediaStreamTrack
	Config() VideoCaptureConfig                  // Config returns the currently applied video config.
	Reconfigure(config VideoCaptureConfig) error // Reconfigure updates the active video config.
	Settings() VideoTrackSettings                // Settings returns the current concrete video settings.
}

// AudioStreamTrack provides type-safe access to capture-backed audio track
// config and settings.
type AudioStreamTrack interface {
	MediaStreamTrack
	Config() AudioCaptureConfig                  // Config returns the currently applied audio config.
	Reconfigure(config AudioCaptureConfig) error // Reconfigure updates the active audio config.
	Settings() AudioTrackSettings                // Settings returns the current concrete audio settings.
}

// MediaStream mirrors browser's MediaStream interface.
type MediaStream struct {
	id          string
	videoTracks []MediaStreamTrack
	audioTracks []MediaStreamTrack
	mu          sync.RWMutex
}

// NewMediaStream creates a new empty MediaStream.
func NewMediaStream() *MediaStream {
	return newMediaStreamWithID(generateID())
}

func newMediaStreamWithID(id string) *MediaStream {
	if id == "" {
		id = generateID()
	}
	return &MediaStream{
		id:          id,
		videoTracks: make([]MediaStreamTrack, 0),
		audioTracks: make([]MediaStreamTrack, 0),
	}
}

// ID returns the stream's unique identifier.
func (s *MediaStream) ID() string {
	return s.id
}

// GetVideoTracks returns all video tracks with video-specific accessors.
func (s *MediaStream) GetVideoTracks() []VideoStreamTrack {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tracks := make([]VideoStreamTrack, 0, len(s.videoTracks))
	for _, track := range s.videoTracks {
		if video, ok := track.(VideoStreamTrack); ok {
			tracks = append(tracks, video)
		}
	}
	return tracks
}

// GetAudioTracks returns all audio tracks with audio-specific accessors.
func (s *MediaStream) GetAudioTracks() []AudioStreamTrack {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tracks := make([]AudioStreamTrack, 0, len(s.audioTracks))
	for _, track := range s.audioTracks {
		if audio, ok := track.(AudioStreamTrack); ok {
			tracks = append(tracks, audio)
		}
	}
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
// Nil tracks and duplicate track IDs are ignored to match browser-like behavior.
func (s *MediaStream) AddTrack(t MediaStreamTrack) {
	if t == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getTrackByIDLocked(t.ID()) != nil {
		return
	}

	switch t.Kind() {
	case "video":
		s.videoTracks = append(s.videoTracks, t)
	case "audio":
		s.audioTracks = append(s.audioTracks, t)
	}
}

// RemoveTrack removes a track from the stream.
// Nil tracks are ignored.
func (s *MediaStream) RemoveTrack(t MediaStreamTrack) {
	if t == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if removeTrackByID(&s.videoTracks, t.ID()) {
		return
	}
	_ = removeTrackByID(&s.audioTracks, t.ID())
}

func removeTrackByID(tracks *[]MediaStreamTrack, id string) bool {
	for i, track := range *tracks {
		if track.ID() == id {
			*tracks = append((*tracks)[:i], (*tracks)[i+1:]...)
			return true
		}
	}
	return false
}

func (s *MediaStream) getTrackByIDLocked(id string) MediaStreamTrack {
	for _, t := range s.videoTracks {
		if t.ID() == id {
			return t
		}
	}
	for i, track := range s.audioTracks {
		if track.ID() == id {
			return s.audioTracks[i]
		}
	}
	return nil
}

// Clone creates a clone of this stream with cloned tracks.
// Track enabled and ended state are preserved on the cloned tracks.
func (s *MediaStream) Clone() *MediaStream {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clone := NewMediaStream()
	for _, t := range s.videoTracks {
		if cloned := t.Clone(); cloned != nil {
			clone.videoTracks = append(clone.videoTracks, cloned)
		}
	}
	for _, t := range s.audioTracks {
		if cloned := t.Clone(); cloned != nil {
			clone.audioTracks = append(clone.audioTracks, cloned)
		}
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
	Width      int          // Width is the current frame width in pixels.
	Height     int          // Height is the current frame height in pixels.
	FrameRate  float64      // FrameRate is the current capture frame rate.
	DeviceID   string       // DeviceID is the selected capture device identifier, when known.
	FacingMode CameraFacing // FacingMode is the selected camera direction, when applicable.
}

// AudioTrackSettings represents current audio track settings.
type AudioTrackSettings struct {
	SampleRate       int    // SampleRate is the current capture sample rate in Hz.
	ChannelCount     int    // ChannelCount is the current number of captured channels.
	DeviceID         string // DeviceID is the selected audio device identifier, when known.
	EchoCancellation bool   // EchoCancellation reports whether echo cancellation is active.
	NoiseSuppression bool   // NoiseSuppression reports whether noise suppression is active.
	AutoGainControl  bool   // AutoGainControl reports whether automatic gain control is active.
}

type mediaSourceKind int

const (
	sourceNone mediaSourceKind = iota
	sourceDevice
	sourceDisplay
)

type videoCaptureHandle interface {
	Start(callback ffi.VideoCaptureCallback) error
	Close()
}

type audioCaptureHandle interface {
	Start(callback ffi.AudioCaptureCallback) error
	Close()
}

type screenCaptureHandle interface {
	Start(callback ffi.VideoCaptureCallback) error
	Close()
}

var (
	loadCaptureLibrary = ffi.LoadLibrary
	enumerateDevices   = ffi.EnumerateDevices
	enumerateScreens   = ffi.EnumerateScreens
	newVideoCapture    = func(deviceID string, width, height, fps int) (videoCaptureHandle, error) {
		return ffi.NewVideoCapture(deviceID, width, height, fps)
	}
	newAudioCapture = func(deviceID string, sampleRate, channels int) (audioCaptureHandle, error) {
		return ffi.NewAudioCapture(deviceID, sampleRate, channels)
	}
	newScreenCapture = func(id int64, isWindow bool, fps int) (screenCaptureHandle, error) {
		return ffi.NewScreenCapture(id, isWindow, fps)
	}
)

// videoStreamTrack wraps track.VideoTrack as MediaStreamTrack.
type videoStreamTrack struct {
	track         *track.VideoTrack
	config        VideoCaptureConfig
	settings      VideoTrackSettings
	maxFrameRate  float64
	displayConfig *DisplayVideoConfig
	enabled       atomic.Bool
	muted         atomic.Bool
	readyState    atomic.Value
	label         string
	source        mediaSourceKind

	videoCapture  videoCaptureHandle
	screenCapture screenCaptureHandle
	mu            sync.Mutex
}

// audioStreamTrack wraps track.AudioTrack as MediaStreamTrack.
type audioStreamTrack struct {
	track      *track.AudioTrack
	config     AudioCaptureConfig
	settings   AudioTrackSettings
	enabled    atomic.Bool
	muted      atomic.Bool
	readyState atomic.Value
	label      string
	source     mediaSourceKind

	audioCapture audioCaptureHandle
	mu           sync.Mutex
}

// OpenCapture opens camera and microphone capture tracks.
func OpenCapture(config CaptureConfig) (*MediaStream, error) {
	if config.Video == nil && config.Audio == nil {
		return nil, ErrInvalidCaptureConfig
	}
	if config.Video != nil {
		if err := validateVideoCaptureConfig(*config.Video); err != nil {
			return nil, err
		}
	}
	if config.Audio != nil {
		if err := validateAudioCaptureConfig(*config.Audio); err != nil {
			return nil, err
		}
	}
	if err := ensureCaptureBackend(); err != nil {
		return nil, err
	}

	devices, err := listCaptureDevices()
	if err != nil {
		return nil, err
	}

	stream := NewMediaStream()
	cleanup := func() {
		for _, t := range stream.GetTracks() {
			t.Stop()
		}
	}

	if config.Video != nil {
		vt, err := createUserVideoTrack(*config.Video, devices)
		if err != nil {
			cleanup()
			return nil, err
		}
		stream.AddTrack(vt)
	}

	if config.Audio != nil {
		at, err := createUserAudioTrack(*config.Audio, devices)
		if err != nil {
			cleanup()
			return nil, err
		}
		stream.AddTrack(at)
	}

	return stream, nil
}

// OpenDisplay opens monitor or window capture.
func OpenDisplay(c DisplayCaptureConfig) (*MediaStream, error) {
	if c.Video == nil {
		return nil, ErrInvalidCaptureConfig
	}
	if err := validateDisplayVideoConfig(*c.Video); err != nil {
		return nil, err
	}
	if c.Audio != nil {
		if err := validateAudioCaptureConfig(*c.Audio); err != nil {
			return nil, err
		}
	}
	if err := ensureCaptureBackend(); err != nil {
		return nil, err
	}

	screens, err := listCaptureScreens()
	if err != nil {
		return nil, err
	}

	stream := NewMediaStream()
	cleanup := func() {
		for _, t := range stream.GetTracks() {
			t.Stop()
		}
	}

	vt, err := createDisplayVideoTrack(*c.Video, screens)
	if err != nil {
		return nil, err
	}
	stream.AddTrack(vt)

	if c.Audio != nil {
		devices, err := listCaptureDevices()
		if err != nil {
			cleanup()
			return nil, err
		}
		at, err := createUserAudioTrack(*c.Audio, devices)
		if err != nil {
			cleanup()
			return nil, err
		}
		stream.AddTrack(at)
	}

	return stream, nil
}

func ensureCaptureBackend() error {
	if err := loadCaptureLibrary(); err != nil {
		return fmt.Errorf("%w: %v", ErrCaptureNotSupported, err)
	}
	return nil
}

func listCaptureDevices() ([]DeviceInfo, error) {
	ffiDevices, err := enumerateDevices()
	if err != nil {
		if errors.Is(err, ffi.ErrLibraryNotLoaded) {
			return nil, ErrCaptureNotSupported
		}
		return nil, err
	}
	return mapFFIDevices(ffiDevices), nil
}

func listCaptureScreens() ([]DisplayInfo, error) {
	ffiScreens, err := enumerateScreens()
	if err != nil {
		if errors.Is(err, ffi.ErrLibraryNotLoaded) {
			return nil, ErrCaptureNotSupported
		}
		return nil, err
	}
	return mapFFIScreens(ffiScreens), nil
}

func mapFFIDevices(ffiDevices []ffi.DeviceInfo) []DeviceInfo {
	devices := make([]DeviceInfo, len(ffiDevices))
	for i, d := range ffiDevices {
		var kind DeviceKind
		switch d.Kind {
		case ffi.DeviceKindVideoInput:
			kind = DeviceKindVideoInput
		case ffi.DeviceKindAudioInput:
			kind = DeviceKindAudioInput
		case ffi.DeviceKindAudioOutput:
			kind = DeviceKindAudioOutput
		}
		devices[i] = DeviceInfo{
			DeviceID: d.DeviceID,
			Kind:     kind,
			Label:    d.Label,
		}
	}
	return devices
}

func mapFFIScreens(ffiScreens []ffi.ScreenInfo) []DisplayInfo {
	screens := make([]DisplayInfo, len(ffiScreens))
	for i, s := range ffiScreens {
		screens[i] = DisplayInfo{
			ID:       s.ID,
			Title:    s.Title,
			IsWindow: s.IsWindow,
		}
	}
	return screens
}

func createUserVideoTrack(config VideoCaptureConfig, devices []DeviceInfo) (*videoStreamTrack, error) {
	settings, resolved, label, err := resolveVideoCaptureConfig(config, devices)
	if err != nil {
		return nil, err
	}
	cfg, resolved := buildVideoTrackConfig(resolved, settings)
	t, err := newVideoStreamTrack(cfg, resolved, settings, label)
	if err != nil {
		return nil, err
	}
	t.source = sourceDevice
	if err := t.startVideoCapture(); err != nil {
		t.Stop()
		return nil, err
	}
	return t, nil
}

func createDisplayVideoTrack(config DisplayVideoConfig, screens []DisplayInfo) (*videoStreamTrack, error) {
	settings, resolvedVideo, resolvedDisplay, label, err := resolveDisplayCaptureConfig(config, screens)
	if err != nil {
		return nil, err
	}
	cfg, resolvedVideo := buildVideoTrackConfig(resolvedVideo, settings)
	t, err := newVideoStreamTrack(cfg, resolvedVideo, settings, label)
	if err != nil {
		return nil, err
	}
	t.source = sourceDisplay
	t.displayConfig = &resolvedDisplay
	if err := t.startScreenCapture(); err != nil {
		t.Stop()
		return nil, err
	}
	return t, nil
}

func createUserAudioTrack(config AudioCaptureConfig, devices []DeviceInfo) (*audioStreamTrack, error) {
	settings, resolved, label, err := resolveAudioCaptureConfig(config, devices)
	if err != nil {
		return nil, err
	}
	cfg, resolved := buildAudioTrackConfig(resolved, settings)
	t, err := newAudioStreamTrack(cfg, resolved, settings, label)
	if err != nil {
		return nil, err
	}
	t.source = sourceDevice
	if err := t.startAudioCapture(); err != nil {
		t.Stop()
		return nil, err
	}
	return t, nil
}

func newVideoStreamTrack(cfg track.VideoTrackConfig, config VideoCaptureConfig, settings VideoTrackSettings, label string) (*videoStreamTrack, error) {
	if cfg.Bitrate == 0 || cfg.Width <= 0 || cfg.Height <= 0 || cfg.FPS <= 0 {
		return nil, fmt.Errorf("%w: video track config must be fully resolved (bitrate=%d width=%d height=%d fps=%v)", ErrInvalidCaptureConfig, cfg.Bitrate, cfg.Width, cfg.Height, cfg.FPS)
	}
	if settings.Width <= 0 || settings.Height <= 0 || settings.FrameRate <= 0 {
		return nil, fmt.Errorf("%w: video track settings must be fully resolved (width=%d height=%d frameRate=%v)", ErrInvalidCaptureConfig, settings.Width, settings.Height, settings.FrameRate)
	}

	vt, err := track.NewVideoTrack(cfg)
	if err != nil {
		return nil, err
	}

	t := &videoStreamTrack{
		track:        vt,
		config:       config,
		settings:     settings,
		maxFrameRate: settings.FrameRate,
		label:        label,
	}
	t.enabled.Store(true)
	t.muted.Store(false)
	t.readyState.Store("live")
	return t, nil
}

func newAudioStreamTrack(cfg track.AudioTrackConfig, config AudioCaptureConfig, settings AudioTrackSettings, label string) (*audioStreamTrack, error) {
	if cfg.Bitrate == 0 || cfg.SampleRate == 0 || cfg.Channels == 0 {
		return nil, fmt.Errorf("%w: audio track config must be fully resolved (bitrate=%d sampleRate=%d channels=%d)", ErrInvalidCaptureConfig, cfg.Bitrate, cfg.SampleRate, cfg.Channels)
	}
	if settings.SampleRate <= 0 || settings.ChannelCount <= 0 {
		return nil, fmt.Errorf("%w: audio track settings must be fully resolved (sampleRate=%d channelCount=%d)", ErrInvalidCaptureConfig, settings.SampleRate, settings.ChannelCount)
	}

	at, err := track.NewAudioTrack(cfg)
	if err != nil {
		return nil, err
	}

	t := &audioStreamTrack{
		track:    at,
		config:   config,
		settings: settings,
		label:    label,
	}
	t.enabled.Store(true)
	t.muted.Store(false)
	t.readyState.Store("live")
	return t, nil
}

func buildVideoTrackConfig(config VideoCaptureConfig, settings VideoTrackSettings) (track.VideoTrackConfig, VideoCaptureConfig) {
	resolved := config
	resolved.Width = settings.Width
	resolved.Height = settings.Height
	resolved.FrameRate = settings.FrameRate
	trackID := generateID()
	cfg := track.VideoTrackConfig{
		ID:             trackID,
		StreamID:       trackID,
		Codec:          resolved.Codec,
		Width:          settings.Width,
		Height:         settings.Height,
		Bitrate:        resolved.Bitrate,
		FPS:            settings.FrameRate,
		MTU:            defaultTrackMTU,
		AutoKeyframe:   true,
		AutoBitrate:    true,
		AutoFramerate:  true,
		AutoResolution: true,
		SVC:            resolved.SVC,
	}

	if len(resolved.CodecPreferences) > 0 {
		cfg.CodecPreferences = append([]webrtc.RTPCodecParameters(nil), resolved.CodecPreferences...)
		for _, preferred := range cfg.CodecPreferences {
			if selectedCodec, ok := codec.ParseMimeType(preferred.MimeType); ok {
				cfg.Codec = selectedCodec
				resolved.Codec = selectedCodec
				break
			}
		}
	}
	return cfg, resolved
}

func buildAudioTrackConfig(config AudioCaptureConfig, settings AudioTrackSettings) (track.AudioTrackConfig, AudioCaptureConfig) {
	resolved := config
	resolved.SampleRate = settings.SampleRate
	resolved.ChannelCount = settings.ChannelCount
	trackID := generateID()
	cfg := track.AudioTrackConfig{
		ID:         trackID,
		StreamID:   trackID,
		SampleRate: settings.SampleRate,
		Channels:   settings.ChannelCount,
		Bitrate:    resolved.Bitrate,
		MTU:        defaultTrackMTU,
	}
	if len(resolved.CodecPreferences) > 0 {
		cfg.CodecPreferences = append([]webrtc.RTPCodecParameters(nil), resolved.CodecPreferences...)
	}
	return cfg, resolved
}

func resolveVideoCaptureConfig(config VideoCaptureConfig, devices []DeviceInfo) (VideoTrackSettings, VideoCaptureConfig, string, error) {
	if err := validateVideoCaptureConfig(config); err != nil {
		return VideoTrackSettings{}, VideoCaptureConfig{}, "", err
	}
	device, err := selectDevice(devices, DeviceKindVideoInput, config.DeviceID)
	if err != nil {
		return VideoTrackSettings{}, VideoCaptureConfig{}, "", err
	}

	resolved := config
	resolved.DeviceID = device.DeviceID
	settings := VideoTrackSettings{
		Width:      resolved.Width,
		Height:     resolved.Height,
		FrameRate:  resolved.FrameRate,
		DeviceID:   resolved.DeviceID,
		FacingMode: resolved.FacingMode,
	}
	return settings, resolved, device.Label, nil
}

func resolveAudioCaptureConfig(config AudioCaptureConfig, devices []DeviceInfo) (AudioTrackSettings, AudioCaptureConfig, string, error) {
	if err := validateAudioCaptureConfig(config); err != nil {
		return AudioTrackSettings{}, AudioCaptureConfig{}, "", err
	}
	device, err := selectDevice(devices, DeviceKindAudioInput, config.DeviceID)
	if err != nil {
		return AudioTrackSettings{}, AudioCaptureConfig{}, "", err
	}

	resolved := config
	resolved.DeviceID = device.DeviceID
	settings := AudioTrackSettings{
		SampleRate:       resolved.SampleRate,
		ChannelCount:     resolved.ChannelCount,
		DeviceID:         resolved.DeviceID,
		EchoCancellation: resolved.EchoCancellation,
		NoiseSuppression: resolved.NoiseSuppression,
		AutoGainControl:  resolved.AutoGainControl,
	}
	return settings, resolved, device.Label, nil
}

func resolveDisplayCaptureConfig(config DisplayVideoConfig, displays []DisplayInfo) (VideoTrackSettings, VideoCaptureConfig, DisplayVideoConfig, string, error) {
	if err := validateDisplayVideoConfig(config); err != nil {
		return VideoTrackSettings{}, VideoCaptureConfig{}, DisplayVideoConfig{}, "", err
	}
	target, kind, err := selectDisplayTarget(displays, config)
	if err != nil {
		return VideoTrackSettings{}, VideoCaptureConfig{}, DisplayVideoConfig{}, "", err
	}

	videoConfig := VideoCaptureConfig{
		Width:            config.Width,
		Height:           config.Height,
		FrameRate:        config.FrameRate,
		Codec:            config.Codec,
		Bitrate:          config.Bitrate,
		SVC:              config.SVC,
		CodecPreferences: append([]webrtc.RTPCodecParameters(nil), config.CodecPreferences...),
	}

	displayConfig := config
	displayConfig.Kind = kind
	if target.IsWindow {
		displayConfig.WindowID = target.ID
		displayConfig.ScreenID = 0
	} else {
		displayConfig.ScreenID = target.ID
		displayConfig.WindowID = 0
	}

	settings := VideoTrackSettings{
		Width:     config.Width,
		Height:    config.Height,
		FrameRate: config.FrameRate,
	}
	return settings, videoConfig, displayConfig, target.Title, nil
}

func validateVideoCaptureConfig(config VideoCaptureConfig) error {
	if !config.FacingMode.IsValid() {
		return &ConfigError{Field: "facingMode", Message: "must be empty or a supported camera facing value"}
	}
	if !hasExplicitVideoCodecSelection(config.Codec, config.CodecPreferences) {
		return &ConfigError{Field: "codec", Message: "must provide Codec or CodecPreferences"}
	}
	if config.Bitrate == 0 {
		return &ConfigError{Field: "bitrate", Message: "must be greater than zero"}
	}
	if config.Width <= 0 {
		return &ConfigError{Field: "width", Message: "must be greater than zero"}
	}
	if config.Height <= 0 {
		return &ConfigError{Field: "height", Message: "must be greater than zero"}
	}
	if config.FrameRate <= 0 {
		return &ConfigError{Field: "frameRate", Message: "must be greater than zero"}
	}
	if strings.TrimSpace(config.DeviceID) == "" && config.DeviceID != "" {
		return &ConfigError{Field: "deviceID", Message: "must not be blank"}
	}
	return nil
}

func validateAudioCaptureConfig(config AudioCaptureConfig) error {
	if config.SampleRate <= 0 {
		return &ConfigError{Field: "sampleRate", Message: "must be greater than zero"}
	}
	if config.ChannelCount <= 0 {
		return &ConfigError{Field: "channelCount", Message: "must be greater than zero"}
	}
	if strings.TrimSpace(config.DeviceID) == "" && config.DeviceID != "" {
		return &ConfigError{Field: "deviceID", Message: "must not be blank"}
	}
	return nil
}

func validateDisplayVideoConfig(config DisplayVideoConfig) error {
	if !config.Kind.IsValid() {
		return &ConfigError{Field: "kind", Message: "must be empty or a supported display kind"}
	}
	if config.Kind == "" {
		return &ConfigError{Field: "kind", Message: "must specify a display kind"}
	}
	if config.ScreenID != 0 && config.WindowID != 0 {
		return &ConfigError{Field: "screenID", Message: "screenID and windowID are mutually exclusive"}
	}
	if !hasExplicitVideoCodecSelection(config.Codec, config.CodecPreferences) {
		return &ConfigError{Field: "codec", Message: "must provide Codec or CodecPreferences"}
	}
	if config.Bitrate == 0 {
		return &ConfigError{Field: "bitrate", Message: "must be greater than zero"}
	}
	if config.Width <= 0 {
		return &ConfigError{Field: "width", Message: "must be greater than zero"}
	}
	if config.Height <= 0 {
		return &ConfigError{Field: "height", Message: "must be greater than zero"}
	}
	if config.FrameRate <= 0 {
		return &ConfigError{Field: "frameRate", Message: "must be greater than zero"}
	}
	return nil
}

func hasExplicitVideoCodecSelection(codecType codec.Type, preferences []webrtc.RTPCodecParameters) bool {
	if codecType != 0 {
		return true
	}
	for _, preferred := range preferences {
		if _, ok := codec.ParseMimeType(preferred.MimeType); ok {
			return true
		}
	}
	return false
}

func selectDevice(devices []DeviceInfo, kind DeviceKind, requested string) (DeviceInfo, error) {
	candidates := make([]DeviceInfo, 0, len(devices))
	for _, device := range devices {
		if device.Kind == kind {
			candidates = append(candidates, device)
		}
	}
	if len(candidates) == 0 {
		return DeviceInfo{}, ErrDeviceNotFound
	}
	if requested != "" {
		for _, device := range candidates {
			if device.DeviceID == requested {
				return device, nil
			}
		}
		return DeviceInfo{}, &ConfigError{
			Field:   "deviceID",
			Message: fmt.Sprintf("device %q is not available", requested),
		}
	}
	return candidates[0], nil
}

func selectDisplayTarget(displays []DisplayInfo, config DisplayVideoConfig) (DisplayInfo, DisplayKind, error) {
	if len(displays) == 0 {
		return DisplayInfo{}, "", ErrDeviceNotFound
	}
	if config.WindowID != 0 {
		for _, display := range displays {
			if display.IsWindow && display.ID == config.WindowID {
				return display, DisplayKindWindow, nil
			}
		}
		return DisplayInfo{}, "", ErrDeviceNotFound
	}
	if config.ScreenID != 0 {
		for _, display := range displays {
			if !display.IsWindow && display.ID == config.ScreenID {
				return display, DisplayKindMonitor, nil
			}
		}
		return DisplayInfo{}, "", ErrDeviceNotFound
	}
	switch config.Kind {
	case DisplayKindWindow:
		for _, display := range displays {
			if display.IsWindow {
				return display, DisplayKindWindow, nil
			}
		}
	case DisplayKindMonitor:
		for _, display := range displays {
			if !display.IsWindow {
				return display, DisplayKindMonitor, nil
			}
		}
	}
	return DisplayInfo{}, "", ErrDeviceNotFound
}

func validateVideoConfigAgainstSettings(config VideoCaptureConfig, settings VideoTrackSettings) error {
	if config.Width != settings.Width {
		return &ConfigError{Field: "width", Message: fmt.Sprintf("capture is fixed at %d", settings.Width)}
	}
	if config.Height != settings.Height {
		return &ConfigError{Field: "height", Message: fmt.Sprintf("capture is fixed at %d", settings.Height)}
	}
	if config.DeviceID != "" && config.DeviceID != settings.DeviceID {
		return &ConfigError{Field: "deviceID", Message: fmt.Sprintf("capture is fixed to %q", settings.DeviceID)}
	}
	if config.FacingMode != "" && settings.FacingMode != "" && settings.FacingMode != config.FacingMode {
		return &ConfigError{Field: "facingMode", Message: fmt.Sprintf("capture is fixed to %q", settings.FacingMode)}
	}
	return nil
}

func validateAudioConfigAgainstSettings(config AudioCaptureConfig, settings AudioTrackSettings) error {
	if config.SampleRate != settings.SampleRate {
		return &ConfigError{Field: "sampleRate", Message: fmt.Sprintf("capture is fixed at %d", settings.SampleRate)}
	}
	if config.ChannelCount != settings.ChannelCount {
		return &ConfigError{Field: "channelCount", Message: fmt.Sprintf("capture is fixed at %d", settings.ChannelCount)}
	}
	if config.DeviceID != "" && config.DeviceID != settings.DeviceID {
		return &ConfigError{Field: "deviceID", Message: fmt.Sprintf("capture is fixed to %q", settings.DeviceID)}
	}
	return nil
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

	t.mu.Lock()
	if t.videoCapture != nil {
		t.videoCapture.Close()
		t.videoCapture = nil
	}
	if t.screenCapture != nil {
		t.screenCapture.Close()
		t.screenCapture = nil
	}
	t.mu.Unlock()

	_ = t.track.Close()
}

func (t *videoStreamTrack) Clone() MediaStreamTrack {
	var clone *videoStreamTrack
	switch t.source {
	case sourceDevice:
		devices, err := listCaptureDevices()
		if err != nil {
			return nil
		}
		clone, err = createUserVideoTrack(t.config, devices)
		if err != nil {
			return nil
		}
	case sourceDisplay:
		screens, err := listCaptureScreens()
		if err != nil || t.displayConfig == nil {
			return nil
		}
		clone, err = createDisplayVideoTrack(*t.displayConfig, screens)
		if err != nil {
			return nil
		}
	default:
		var err error
		cfg, resolved := buildVideoTrackConfig(t.config, t.settings)
		clone, err = newVideoStreamTrack(cfg, resolved, t.settings, t.label)
		if err != nil {
			return nil
		}
	}

	clone.enabled.Store(t.enabled.Load())
	clone.muted.Store(t.muted.Load())
	if t.ReadyState() == "ended" {
		clone.Stop()
	}
	return clone
}

func (t *videoStreamTrack) Config() VideoCaptureConfig   { return t.config }
func (t *videoStreamTrack) Settings() VideoTrackSettings { return t.settings }

func (t *videoStreamTrack) Reconfigure(config VideoCaptureConfig) error {
	if err := validateVideoCaptureConfig(config); err != nil {
		return err
	}
	if err := validateVideoConfigAgainstSettings(config, t.settings); err != nil {
		return err
	}

	if config.Bitrate != t.config.Bitrate {
		if err := t.track.SetBitrate(config.Bitrate); err != nil {
			return err
		}
	}

	if config.FrameRate > t.maxFrameRate {
		return &ConfigError{
			Field:   "frameRate",
			Message: fmt.Sprintf("capture supports at most %.2f fps", t.maxFrameRate),
		}
	}
	if config.FrameRate != t.settings.FrameRate {
		if err := t.track.SetFramerate(config.FrameRate); err != nil {
			return err
		}
		t.settings.FrameRate = config.FrameRate
	}

	t.config = config
	if config.FacingMode != "" {
		t.settings.FacingMode = config.FacingMode
	}
	return nil
}

var _ VideoStreamTrack = (*videoStreamTrack)(nil)

func (t *videoStreamTrack) pionTrack() webrtc.TrackLocal { return t.track }

func (t *videoStreamTrack) startVideoCapture() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.videoCapture != nil {
		return nil
	}

	capture, err := newVideoCapture(
		t.settings.DeviceID,
		t.settings.Width,
		t.settings.Height,
		int(t.settings.FrameRate),
	)
	if err != nil {
		return err
	}

	err = capture.Start(func(captured *ffi.CapturedVideoFrame) {
		if !t.enabled.Load() || t.muted.Load() || t.readyState.Load().(string) != "live" {
			return
		}
		videoFrame := &frame.VideoFrame{
			Width:  int(captured.Width),
			Height: int(captured.Height),
			PTS:    ptsFromTimestampUs(captured.TimestampUs, 90000),
			Format: frame.PixelFormatI420,
			Data:   [][]byte{captured.YPlane, captured.UPlane, captured.VPlane},
			Stride: []int{int(captured.YStride), int(captured.UStride), int(captured.VStride)},
		}
		_ = t.track.WriteFrame(videoFrame, false)
	})
	if err != nil {
		capture.Close()
		return err
	}

	t.videoCapture = capture
	return nil
}

func (t *videoStreamTrack) startScreenCapture() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.screenCapture != nil {
		return nil
	}
	if t.displayConfig == nil {
		return ErrInvalidCaptureConfig
	}

	screenID := t.displayConfig.ScreenID
	isWindow := false
	if t.displayConfig.WindowID != 0 {
		screenID = t.displayConfig.WindowID
		isWindow = true
	}

	capture, err := newScreenCapture(screenID, isWindow, int(t.settings.FrameRate))
	if err != nil {
		return err
	}

	err = capture.Start(func(captured *ffi.CapturedVideoFrame) {
		if !t.enabled.Load() || t.muted.Load() || t.readyState.Load().(string) != "live" {
			return
		}
		videoFrame := &frame.VideoFrame{
			Width:  int(captured.Width),
			Height: int(captured.Height),
			PTS:    ptsFromTimestampUs(captured.TimestampUs, 90000),
			Format: frame.PixelFormatI420,
			Data:   [][]byte{captured.YPlane, captured.UPlane, captured.VPlane},
			Stride: []int{int(captured.YStride), int(captured.UStride), int(captured.VStride)},
		}
		_ = t.track.WriteFrame(videoFrame, false)
	})
	if err != nil {
		capture.Close()
		return err
	}

	t.screenCapture = capture
	return nil
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

	t.mu.Lock()
	if t.audioCapture != nil {
		t.audioCapture.Close()
		t.audioCapture = nil
	}
	t.mu.Unlock()

	_ = t.track.Close()
}

func (t *audioStreamTrack) Clone() MediaStreamTrack {
	var clone *audioStreamTrack
	switch t.source {
	case sourceDevice:
		devices, err := listCaptureDevices()
		if err != nil {
			return nil
		}
		clone, err = createUserAudioTrack(t.config, devices)
		if err != nil {
			return nil
		}
	default:
		var err error
		cfg, resolved := buildAudioTrackConfig(t.config, t.settings)
		clone, err = newAudioStreamTrack(cfg, resolved, t.settings, t.label)
		if err != nil {
			return nil
		}
	}

	clone.enabled.Store(t.enabled.Load())
	clone.muted.Store(t.muted.Load())
	if t.ReadyState() == "ended" {
		clone.Stop()
	}
	return clone
}

func (t *audioStreamTrack) Config() AudioCaptureConfig   { return t.config }
func (t *audioStreamTrack) Settings() AudioTrackSettings { return t.settings }

func (t *audioStreamTrack) Reconfigure(config AudioCaptureConfig) error {
	if err := validateAudioCaptureConfig(config); err != nil {
		return err
	}
	if err := validateAudioConfigAgainstSettings(config, t.settings); err != nil {
		return err
	}

	if config.Bitrate != t.config.Bitrate {
		if err := t.track.SetBitrate(config.Bitrate); err != nil {
			return err
		}
	}

	t.settings.EchoCancellation = config.EchoCancellation
	t.settings.NoiseSuppression = config.NoiseSuppression
	t.settings.AutoGainControl = config.AutoGainControl
	t.config = config
	return nil
}

var _ AudioStreamTrack = (*audioStreamTrack)(nil)

func (t *audioStreamTrack) pionTrack() webrtc.TrackLocal { return t.track }

func (t *audioStreamTrack) startAudioCapture() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.audioCapture != nil {
		return nil
	}

	capture, err := newAudioCapture(
		t.settings.DeviceID,
		t.settings.SampleRate,
		t.settings.ChannelCount,
	)
	if err != nil {
		return err
	}

	err = capture.Start(func(captured *ffi.CapturedAudioFrame) {
		if !t.enabled.Load() || t.muted.Load() || t.readyState.Load().(string) != "live" {
			return
		}
		audioFrame := frame.NewAudioFrameFromS16(
			captured.Samples,
			int(captured.SampleRate),
			int(captured.NumChannels),
		)
		audioFrame.PTS = ptsFromTimestampUs(captured.TimestampUs, int64(captured.SampleRate))
		_ = t.track.WriteFrame(audioFrame)
	})
	if err != nil {
		capture.Close()
		return err
	}

	t.audioCapture = capture
	return nil
}

// --- Utilities ---

var idCounter atomic.Uint64

func generateID() string {
	return "libwebrtc-" + strconv.FormatUint(idCounter.Add(1), 10)
}

func ptsFromTimestampUs(timestampUs, clockRate int64) uint32 {
	if timestampUs <= 0 || clockRate <= 0 {
		return 0
	}
	return uint32((timestampUs * clockRate) / 1_000_000)
}
