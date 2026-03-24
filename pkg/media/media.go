// Package media provides a browser-like API for media capture and track management.
// This mirrors the Web APIs like getUserMedia, getDisplayMedia, MediaStream, and MediaStreamTrack.
package media

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/track"
)

// Errors
var (
	ErrInvalidConstraints  = errors.New("invalid constraints")
	ErrTrackNotFound       = errors.New("track not found")
	ErrStreamClosed        = errors.New("stream closed")
	ErrDeviceNotFound      = errors.New("device not found")
	ErrCaptureNotSupported = errors.New("capture not supported without shim library")
)

// MediaDeviceKind represents the type of media device.
type MediaDeviceKind string

const (
	// MediaDeviceKindVideoInput represents a camera.
	MediaDeviceKindVideoInput MediaDeviceKind = "videoinput"
	// MediaDeviceKindAudioInput represents a microphone.
	MediaDeviceKindAudioInput MediaDeviceKind = "audioinput"
	// MediaDeviceKindAudioOutput represents a speaker.
	MediaDeviceKindAudioOutput MediaDeviceKind = "audiooutput"
)

// MediaDeviceInfo mirrors browser's MediaDeviceInfo interface.
// Returned by EnumerateDevices().
type MediaDeviceInfo struct {
	// DeviceID is a unique identifier for the device.
	DeviceID string
	// Kind is the type of device (videoinput, audioinput, audiooutput).
	Kind MediaDeviceKind
	// Label is a human-readable name for the device.
	// May be empty if permission not granted.
	Label string
	// GroupID identifies devices that belong together (e.g., camera + mic on same device).
	GroupID string
}

// EnumerateDevices mirrors browser's navigator.mediaDevices.enumerateDevices().
// Returns a list of available media input and output devices.
func EnumerateDevices() ([]MediaDeviceInfo, error) {
	ffiDevices, err := ffi.EnumerateDevices()
	if err != nil {
		// If library is not available, mirror browser behavior with an empty list.
		if errors.Is(err, ffi.ErrLibraryNotLoaded) {
			return []MediaDeviceInfo{}, nil
		}
		return nil, err
	}

	devices := make([]MediaDeviceInfo, len(ffiDevices))
	for i, d := range ffiDevices {
		var kind MediaDeviceKind
		switch d.Kind {
		case ffi.DeviceKindVideoInput:
			kind = MediaDeviceKindVideoInput
		case ffi.DeviceKindAudioInput:
			kind = MediaDeviceKindAudioInput
		case ffi.DeviceKindAudioOutput:
			kind = MediaDeviceKindAudioOutput
		}
		devices[i] = MediaDeviceInfo{
			DeviceID: d.DeviceID,
			Kind:     kind,
			Label:    d.Label,
			GroupID:  "",
		}
	}

	return devices, nil
}

// ScreenInfo represents a screen or window available for capture.
type ScreenInfo struct {
	// ID is a unique identifier for the screen/window.
	ID int64
	// Title is the window title or screen name.
	Title string
	// IsWindow is true for windows, false for screens.
	IsWindow bool
}

// EnumerateScreens returns a list of available screens and windows for capture.
// This is an extension to the browser API (browsers use getDisplayMedia picker).
func EnumerateScreens() ([]ScreenInfo, error) {
	ffiScreens, err := ffi.EnumerateScreens()
	if err != nil {
		if errors.Is(err, ffi.ErrLibraryNotLoaded) {
			return []ScreenInfo{}, nil
		}
		return nil, err
	}

	screens := make([]ScreenInfo, len(ffiScreens))
	for i, s := range ffiScreens {
		screens[i] = ScreenInfo{
			ID:       s.ID,
			Title:    s.Title,
			IsWindow: s.IsWindow,
		}
	}

	return screens, nil
}

// DisplayConstraints is used with GetDisplayMedia for screen/window capture.
type DisplayConstraints struct {
	Video *DisplayVideoConstraints // nil = invalid for getDisplayMedia
	Audio *AudioConstraints        // optional additional audio capture
}

// DisplayVideoConstraints for screen/window capture.
type DisplayVideoConstraints struct {
	DisplaySurface DisplaySurface
	// ScreenID specifies which screen to capture (from EnumerateScreens).
	// If 0 and WindowID is 0, captures the first matching screen for the requested DisplaySurface.
	ScreenID int64
	// WindowID specifies which window to capture (from EnumerateScreens).
	// Takes precedence over ScreenID if non-zero.
	WindowID  int64
	FrameRate FloatConstraint
	Width     IntConstraint
	Height    IntConstraint
	Codec     codec.Type
	Bitrate   uint32
	SVC       *codec.SVCConfig
	// CodecPreferences is the advanced codec selection path and takes precedence over Codec.
	CodecPreferences []webrtc.RTPCodecParameters
}

// VideoConstraints mirrors the supported subset of browser MediaTrackConstraints for video capture.
type VideoConstraints struct {
	Width      IntConstraint
	Height     IntConstraint
	FrameRate  FloatConstraint
	FacingMode FacingMode
	DeviceID   StringConstraint
	Codec      codec.Type
	Bitrate    uint32
	SVC        *codec.SVCConfig
	// CodecPreferences is the advanced codec selection path and takes precedence over Codec.
	CodecPreferences []webrtc.RTPCodecParameters
}

// AudioConstraints mirrors the supported subset of browser MediaTrackConstraints for audio capture.
type AudioConstraints struct {
	SampleRate       IntConstraint
	ChannelCount     IntConstraint
	EchoCancellation BoolConstraint
	NoiseSuppression BoolConstraint
	AutoGainControl  BoolConstraint
	DeviceID         StringConstraint
	Bitrate          uint32
	// CodecPreferences is the advanced codec selection path.
	CodecPreferences []webrtc.RTPCodecParameters
}

// Constraints mirrors browser's MediaStreamConstraints.
type Constraints struct {
	Video *VideoConstraints
	Audio *AudioConstraints
}

// MediaStreamTrack mirrors browser's MediaStreamTrack interface.
// Use VideoStreamTrack or AudioStreamTrack for type-safe constraint access.
type MediaStreamTrack interface {
	ID() string
	Kind() string
	Label() string
	Enabled() bool
	SetEnabled(enabled bool)
	Muted() bool
	ReadyState() string
	Stop()
	Clone() MediaStreamTrack
}

// VideoStreamTrack provides type-safe access to video track constraints and settings.
type VideoStreamTrack interface {
	MediaStreamTrack
	GetConstraints() VideoConstraints
	GetCapabilities() VideoTrackCapabilities
	ApplyConstraints(constraints VideoConstraints) error
	GetSettings() VideoTrackSettings
}

// AudioStreamTrack provides type-safe access to audio track constraints and settings.
type AudioStreamTrack interface {
	MediaStreamTrack
	GetConstraints() AudioConstraints
	GetCapabilities() AudioTrackCapabilities
	ApplyConstraints(constraints AudioConstraints) error
	GetSettings() AudioTrackSettings
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
func (s *MediaStream) AddTrack(t MediaStreamTrack) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.Kind() == "video" {
		s.videoTracks = append(s.videoTracks, t)
		return
	}
	s.audioTracks = append(s.audioTracks, t)
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
		return
	}
	for i, track := range s.audioTracks {
		if track.ID() == t.ID() {
			s.audioTracks = append(s.audioTracks[:i], s.audioTracks[i+1:]...)
			return
		}
	}
}

// Clone creates a clone of this stream with cloned tracks.
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
	Width      int
	Height     int
	FrameRate  float64
	DeviceID   string
	FacingMode FacingMode
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
	track              *track.VideoTrack
	constraints        VideoConstraints
	capabilities       VideoTrackCapabilities
	settings           VideoTrackSettings
	displayConstraints *DisplayVideoConstraints
	enabled            atomic.Bool
	muted              atomic.Bool
	readyState         atomic.Value
	label              string
	source             mediaSourceKind

	videoCapture  videoCaptureHandle
	screenCapture screenCaptureHandle
	mu            sync.Mutex
}

// audioStreamTrack wraps track.AudioTrack as MediaStreamTrack.
type audioStreamTrack struct {
	track        *track.AudioTrack
	constraints  AudioConstraints
	capabilities AudioTrackCapabilities
	settings     AudioTrackSettings
	enabled      atomic.Bool
	muted        atomic.Bool
	readyState   atomic.Value
	label        string
	source       mediaSourceKind

	audioCapture audioCaptureHandle
	mu           sync.Mutex
}

// GetUserMedia mirrors browser's navigator.mediaDevices.getUserMedia().
// Returns a MediaStream with real capture-backed tracks.
func GetUserMedia(constraints Constraints) (*MediaStream, error) {
	if constraints.Video == nil && constraints.Audio == nil {
		return nil, ErrInvalidConstraints
	}
	if constraints.Video != nil {
		if err := validateVideoConstraints(*constraints.Video); err != nil {
			return nil, err
		}
	}
	if constraints.Audio != nil {
		if err := validateAudioConstraints(*constraints.Audio); err != nil {
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

	if constraints.Video != nil {
		vt, err := createUserVideoTrack(*constraints.Video, devices)
		if err != nil {
			cleanup()
			return nil, err
		}
		stream.AddTrack(vt)
	}

	if constraints.Audio != nil {
		at, err := createUserAudioTrack(*constraints.Audio, devices)
		if err != nil {
			cleanup()
			return nil, err
		}
		stream.AddTrack(at)
	}

	return stream, nil
}

// GetDisplayMedia mirrors browser's navigator.mediaDevices.getDisplayMedia().
// Returns a MediaStream configured for screen sharing.
func GetDisplayMedia(c DisplayConstraints) (*MediaStream, error) {
	if c.Video == nil {
		return nil, ErrInvalidConstraints
	}
	if err := validateDisplayVideoConstraints(*c.Video); err != nil {
		return nil, err
	}
	if c.Audio != nil {
		if err := validateAudioConstraints(*c.Audio); err != nil {
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

func listCaptureDevices() ([]MediaDeviceInfo, error) {
	ffiDevices, err := enumerateDevices()
	if err != nil {
		if errors.Is(err, ffi.ErrLibraryNotLoaded) {
			return nil, ErrCaptureNotSupported
		}
		return nil, err
	}
	devices := make([]MediaDeviceInfo, len(ffiDevices))
	for i, d := range ffiDevices {
		var kind MediaDeviceKind
		switch d.Kind {
		case ffi.DeviceKindVideoInput:
			kind = MediaDeviceKindVideoInput
		case ffi.DeviceKindAudioInput:
			kind = MediaDeviceKindAudioInput
		case ffi.DeviceKindAudioOutput:
			kind = MediaDeviceKindAudioOutput
		}
		devices[i] = MediaDeviceInfo{
			DeviceID: d.DeviceID,
			Kind:     kind,
			Label:    d.Label,
			GroupID:  "",
		}
	}
	return devices, nil
}

func listCaptureScreens() ([]ScreenInfo, error) {
	ffiScreens, err := enumerateScreens()
	if err != nil {
		if errors.Is(err, ffi.ErrLibraryNotLoaded) {
			return nil, ErrCaptureNotSupported
		}
		return nil, err
	}
	screens := make([]ScreenInfo, len(ffiScreens))
	for i, s := range ffiScreens {
		screens[i] = ScreenInfo{
			ID:       s.ID,
			Title:    s.Title,
			IsWindow: s.IsWindow,
		}
	}
	return screens, nil
}

func createUserVideoTrack(request VideoConstraints, devices []MediaDeviceInfo) (*videoStreamTrack, error) {
	settings, resolved, label, err := resolveVideoCaptureRequest(request, devices)
	if err != nil {
		return nil, err
	}
	t, err := newVideoStreamTrack(resolved, settings, label)
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

func createDisplayVideoTrack(request DisplayVideoConstraints, screens []ScreenInfo) (*videoStreamTrack, error) {
	settings, resolvedVideo, resolvedDisplay, label, err := resolveDisplayCaptureRequest(request, screens)
	if err != nil {
		return nil, err
	}
	t, err := newVideoStreamTrack(resolvedVideo, settings, label)
	if err != nil {
		return nil, err
	}
	t.source = sourceDisplay
	t.displayConstraints = &resolvedDisplay
	t.capabilities.DisplaySurface = []DisplaySurface{resolvedDisplay.DisplaySurface}
	if err := t.startScreenCapture(); err != nil {
		t.Stop()
		return nil, err
	}
	return t, nil
}

func createUserAudioTrack(request AudioConstraints, devices []MediaDeviceInfo) (*audioStreamTrack, error) {
	settings, resolved, label, err := resolveAudioCaptureRequest(request, devices)
	if err != nil {
		return nil, err
	}
	t, err := newAudioStreamTrack(resolved, settings, label)
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

func newVideoStreamTrack(constraints VideoConstraints, settings VideoTrackSettings, label string) (*videoStreamTrack, error) {
	codecType := constraints.Codec
	if codecType == 0 {
		codecType = codec.H264
		constraints.Codec = codecType
	}
	if constraints.Bitrate == 0 {
		constraints.Bitrate = defaultVideoBitrate(codecType, settings.Width, settings.Height)
	}

	trackCfg := track.VideoTrackConfig{
		ID:      generateID(),
		Codec:   codecType,
		Width:   settings.Width,
		Height:  settings.Height,
		Bitrate: constraints.Bitrate,
		FPS:     settings.FrameRate,
	}
	if len(constraints.CodecPreferences) > 0 {
		trackCfg.CodecPreferences = append([]webrtc.RTPCodecParameters(nil), constraints.CodecPreferences...)
		for _, preferred := range trackCfg.CodecPreferences {
			if selectedCodec, ok := codec.ParseMimeType(preferred.MimeType); ok {
				trackCfg.Codec = selectedCodec
				constraints.Codec = selectedCodec
				break
			}
		}
	}

	vt, err := track.NewVideoTrack(trackCfg)
	if err != nil {
		return nil, err
	}

	t := &videoStreamTrack{
		track:        vt,
		constraints:  constraints,
		capabilities: newVideoTrackCapabilities(settings),
		settings:     settings,
		label:        label,
	}
	t.enabled.Store(true)
	t.muted.Store(false)
	t.readyState.Store("live")
	return t, nil
}

func newAudioStreamTrack(constraints AudioConstraints, settings AudioTrackSettings, label string) (*audioStreamTrack, error) {
	if constraints.Bitrate == 0 {
		constraints.Bitrate = codec.DefaultOpusConfig().Bitrate
	}
	at, err := track.NewAudioTrack(track.AudioTrackConfig{
		ID:               generateID(),
		SampleRate:       settings.SampleRate,
		Channels:         settings.ChannelCount,
		Bitrate:          constraints.Bitrate,
		CodecPreferences: append([]webrtc.RTPCodecParameters(nil), constraints.CodecPreferences...),
	})
	if err != nil {
		return nil, err
	}

	t := &audioStreamTrack{
		track:        at,
		constraints:  constraints,
		capabilities: newAudioTrackCapabilities(settings),
		settings:     settings,
		label:        label,
	}
	t.enabled.Store(true)
	t.muted.Store(false)
	t.readyState.Store("live")
	return t, nil
}

func resolveVideoCaptureRequest(request VideoConstraints, devices []MediaDeviceInfo) (VideoTrackSettings, VideoConstraints, string, error) {
	if err := validateVideoConstraints(request); err != nil {
		return VideoTrackSettings{}, VideoConstraints{}, "", err
	}
	device, err := selectDevice(devices, MediaDeviceKindVideoInput, request.DeviceID)
	if err != nil {
		return VideoTrackSettings{}, VideoConstraints{}, "", err
	}

	width := resolveIntConstraint(request.Width, 1280)
	height := resolveIntConstraint(request.Height, 720)
	frameRate := resolveFloatConstraint(request.FrameRate, 30)
	if width <= 0 || height <= 0 || frameRate <= 0 {
		return VideoTrackSettings{}, VideoConstraints{}, "", ErrInvalidConstraints
	}

	settings := VideoTrackSettings{
		Width:      width,
		Height:     height,
		FrameRate:  frameRate,
		DeviceID:   device.DeviceID,
		FacingMode: request.FacingMode,
	}

	resolved := request
	resolved.Width = ExactInt(width)
	resolved.Height = ExactInt(height)
	resolved.FrameRate = ExactFloat(frameRate)
	resolved.DeviceID = ExactString(device.DeviceID)
	if resolved.Codec == 0 {
		resolved.Codec = codec.H264
	}
	if resolved.Bitrate == 0 {
		resolved.Bitrate = defaultVideoBitrate(resolved.Codec, width, height)
	}

	label := device.Label
	if label == "" {
		label = "camera"
	}

	return settings, resolved, label, nil
}

func resolveAudioCaptureRequest(request AudioConstraints, devices []MediaDeviceInfo) (AudioTrackSettings, AudioConstraints, string, error) {
	if err := validateAudioConstraints(request); err != nil {
		return AudioTrackSettings{}, AudioConstraints{}, "", err
	}
	device, err := selectDevice(devices, MediaDeviceKindAudioInput, request.DeviceID)
	if err != nil {
		return AudioTrackSettings{}, AudioConstraints{}, "", err
	}

	sampleRate := resolveIntConstraint(request.SampleRate, 48000)
	channelCount := resolveIntConstraint(request.ChannelCount, 2)
	if sampleRate <= 0 || channelCount <= 0 {
		return AudioTrackSettings{}, AudioConstraints{}, "", ErrInvalidConstraints
	}

	settings := AudioTrackSettings{
		SampleRate:       sampleRate,
		ChannelCount:     channelCount,
		DeviceID:         device.DeviceID,
		EchoCancellation: resolveBoolConstraint(request.EchoCancellation, false),
		NoiseSuppression: resolveBoolConstraint(request.NoiseSuppression, false),
		AutoGainControl:  resolveBoolConstraint(request.AutoGainControl, false),
	}

	resolved := request
	resolved.SampleRate = ExactInt(sampleRate)
	resolved.ChannelCount = ExactInt(channelCount)
	resolved.DeviceID = ExactString(device.DeviceID)
	if resolved.Bitrate == 0 {
		resolved.Bitrate = codec.DefaultOpusConfig().Bitrate
	}
	resolved.EchoCancellation = ExactBool(settings.EchoCancellation)
	resolved.NoiseSuppression = ExactBool(settings.NoiseSuppression)
	resolved.AutoGainControl = ExactBool(settings.AutoGainControl)

	label := device.Label
	if label == "" {
		label = "microphone"
	}

	return settings, resolved, label, nil
}

func resolveDisplayCaptureRequest(request DisplayVideoConstraints, screens []ScreenInfo) (VideoTrackSettings, VideoConstraints, DisplayVideoConstraints, string, error) {
	if err := validateDisplayVideoConstraints(request); err != nil {
		return VideoTrackSettings{}, VideoConstraints{}, DisplayVideoConstraints{}, "", err
	}
	target, surface, err := selectDisplayTarget(screens, request)
	if err != nil {
		return VideoTrackSettings{}, VideoConstraints{}, DisplayVideoConstraints{}, "", err
	}

	width := resolveIntConstraint(request.Width, 1920)
	height := resolveIntConstraint(request.Height, 1080)
	frameRate := resolveFloatConstraint(request.FrameRate, 30)
	if width <= 0 || height <= 0 || frameRate <= 0 {
		return VideoTrackSettings{}, VideoConstraints{}, DisplayVideoConstraints{}, "", ErrInvalidConstraints
	}

	videoConstraints := VideoConstraints{
		Width:            ExactInt(width),
		Height:           ExactInt(height),
		FrameRate:        ExactFloat(frameRate),
		Codec:            request.Codec,
		Bitrate:          request.Bitrate,
		SVC:              request.SVC,
		CodecPreferences: append([]webrtc.RTPCodecParameters(nil), request.CodecPreferences...),
	}
	if videoConstraints.Codec == 0 {
		videoConstraints.Codec = codec.H264
	}
	if videoConstraints.Bitrate == 0 {
		videoConstraints.Bitrate = 3_000_000
	}
	if videoConstraints.SVC == nil {
		videoConstraints.SVC = codec.SVCPresetScreenShare()
	}

	displayConstraints := request
	displayConstraints.DisplaySurface = surface
	displayConstraints.Width = ExactInt(width)
	displayConstraints.Height = ExactInt(height)
	displayConstraints.FrameRate = ExactFloat(frameRate)
	if target.IsWindow {
		displayConstraints.WindowID = target.ID
		displayConstraints.ScreenID = 0
	} else {
		displayConstraints.ScreenID = target.ID
		displayConstraints.WindowID = 0
	}

	settings := VideoTrackSettings{
		Width:     width,
		Height:    height,
		FrameRate: frameRate,
	}

	label := "screen-capture"
	if target.IsWindow {
		label = "window-capture"
	}

	return settings, videoConstraints, displayConstraints, label, nil
}

func defaultVideoBitrate(codecType codec.Type, width, height int) uint32 {
	switch codecType {
	case codec.VP8:
		return codec.DefaultVP8Config(width, height).Bitrate
	case codec.VP9:
		return codec.DefaultVP9Config(width, height).Bitrate
	case codec.AV1:
		return codec.DefaultAV1Config(width, height).Bitrate
	default:
		return codec.DefaultH264Config(width, height).Bitrate
	}
}

func newVideoTrackCapabilities(settings VideoTrackSettings) VideoTrackCapabilities {
	capabilities := VideoTrackCapabilities{
		Width:     exactIntCapability(settings.Width),
		Height:    exactIntCapability(settings.Height),
		FrameRate: frameRateCapability(settings.FrameRate),
	}
	if settings.DeviceID != "" {
		capabilities.DeviceID = []string{settings.DeviceID}
	}
	if settings.FacingMode != "" {
		capabilities.FacingMode = []FacingMode{settings.FacingMode}
	}
	return capabilities
}

func newAudioTrackCapabilities(settings AudioTrackSettings) AudioTrackCapabilities {
	capabilities := AudioTrackCapabilities{
		SampleRate:       exactIntCapability(settings.SampleRate),
		ChannelCount:     exactIntCapability(settings.ChannelCount),
		EchoCancellation: []bool{false, true},
		NoiseSuppression: []bool{false, true},
		AutoGainControl:  []bool{false, true},
	}
	if settings.DeviceID != "" {
		capabilities.DeviceID = []string{settings.DeviceID}
	}
	return capabilities
}

func validateVideoConstraints(c VideoConstraints) error {
	if !c.FacingMode.IsValid() {
		return ErrInvalidConstraints
	}
	if err := validateStringConstraintShape(c.DeviceID); err != nil {
		return err
	}
	if err := validateIntConstraintShape(c.Width); err != nil {
		return err
	}
	if err := validateIntConstraintShape(c.Height); err != nil {
		return err
	}
	if err := validateFloatConstraintShape(c.FrameRate); err != nil {
		return err
	}
	return nil
}

func validateAudioConstraints(c AudioConstraints) error {
	if err := validateStringConstraintShape(c.DeviceID); err != nil {
		return err
	}
	if err := validateIntConstraintShape(c.SampleRate); err != nil {
		return err
	}
	if err := validateIntConstraintShape(c.ChannelCount); err != nil {
		return err
	}
	return nil
}

func validateDisplayVideoConstraints(c DisplayVideoConstraints) error {
	if !c.DisplaySurface.IsValid() {
		return ErrInvalidConstraints
	}
	if c.ScreenID != 0 && c.WindowID != 0 {
		return ErrInvalidConstraints
	}
	if c.DisplaySurface == DisplaySurfaceBrowser {
		return &OverconstrainedError{
			Constraint: "displaySurface",
			Message:    "browser surface capture is not supported",
		}
	}
	if err := validateIntConstraintShape(c.Width); err != nil {
		return err
	}
	if err := validateIntConstraintShape(c.Height); err != nil {
		return err
	}
	if err := validateFloatConstraintShape(c.FrameRate); err != nil {
		return err
	}
	return nil
}

func validateIntConstraintShape(c IntConstraint) error {
	if c.Exact != nil && *c.Exact <= 0 {
		return ErrInvalidConstraints
	}
	if c.Ideal != nil && *c.Ideal <= 0 {
		return ErrInvalidConstraints
	}
	if c.Min != nil && *c.Min <= 0 {
		return ErrInvalidConstraints
	}
	if c.Max != nil && *c.Max <= 0 {
		return ErrInvalidConstraints
	}
	if c.Min != nil && c.Max != nil && *c.Min > *c.Max {
		return ErrInvalidConstraints
	}
	return nil
}

func validateFloatConstraintShape(c FloatConstraint) error {
	if c.Exact != nil && *c.Exact <= 0 {
		return ErrInvalidConstraints
	}
	if c.Ideal != nil && *c.Ideal <= 0 {
		return ErrInvalidConstraints
	}
	if c.Min != nil && *c.Min <= 0 {
		return ErrInvalidConstraints
	}
	if c.Max != nil && *c.Max <= 0 {
		return ErrInvalidConstraints
	}
	if c.Min != nil && c.Max != nil && *c.Min > *c.Max {
		return ErrInvalidConstraints
	}
	return nil
}

func validateStringConstraintShape(c StringConstraint) error {
	if c.Exact != nil && *c.Exact == "" {
		return ErrInvalidConstraints
	}
	if c.Ideal != nil && *c.Ideal == "" {
		return ErrInvalidConstraints
	}
	return nil
}

func selectDevice(devices []MediaDeviceInfo, kind MediaDeviceKind, requested StringConstraint) (MediaDeviceInfo, error) {
	candidates := make([]MediaDeviceInfo, 0, len(devices))
	for _, device := range devices {
		if device.Kind == kind {
			candidates = append(candidates, device)
		}
	}
	if len(candidates) == 0 {
		return MediaDeviceInfo{}, ErrDeviceNotFound
	}
	if requested.Exact != nil {
		for _, device := range candidates {
			if device.DeviceID == *requested.Exact {
				return device, nil
			}
		}
		return MediaDeviceInfo{}, &OverconstrainedError{
			Constraint: "deviceId",
			Message:    fmt.Sprintf("requires exact %q", *requested.Exact),
		}
	}
	if requested.Ideal != nil {
		for _, device := range candidates {
			if device.DeviceID == *requested.Ideal {
				return device, nil
			}
		}
	}
	return candidates[0], nil
}

func selectDisplayTarget(screens []ScreenInfo, request DisplayVideoConstraints) (ScreenInfo, DisplaySurface, error) {
	if len(screens) == 0 {
		return ScreenInfo{}, "", ErrDeviceNotFound
	}
	if request.WindowID != 0 {
		for _, screen := range screens {
			if screen.IsWindow && screen.ID == request.WindowID {
				return screen, DisplaySurfaceWindow, nil
			}
		}
		return ScreenInfo{}, "", ErrDeviceNotFound
	}
	if request.ScreenID != 0 {
		for _, screen := range screens {
			if !screen.IsWindow && screen.ID == request.ScreenID {
				return screen, DisplaySurfaceMonitor, nil
			}
		}
		return ScreenInfo{}, "", ErrDeviceNotFound
	}

	surface := request.DisplaySurface
	if surface == "" {
		surface = DisplaySurfaceMonitor
	}
	switch surface {
	case DisplaySurfaceWindow:
		for _, screen := range screens {
			if screen.IsWindow {
				return screen, surface, nil
			}
		}
	case DisplaySurfaceMonitor:
		for _, screen := range screens {
			if !screen.IsWindow {
				return screen, surface, nil
			}
		}
	}
	return ScreenInfo{}, "", ErrDeviceNotFound
}

func resolveIntConstraint(c IntConstraint, def int) int {
	if v, ok := c.Value(); ok {
		return v
	}
	return def
}

func resolveFloatConstraint(c FloatConstraint, def float64) float64 {
	if v, ok := c.Value(); ok {
		return v
	}
	return def
}

func resolveBoolConstraint(c BoolConstraint, def bool) bool {
	if v, ok := c.Value(); ok {
		return v
	}
	return def
}

func resolveVideoFrameRateConstraint(c FloatConstraint, current float64, capabilities FloatCapabilityRange) (float64, error) {
	if !c.IsSet() {
		return current, nil
	}

	minSupported := capabilities.Min
	maxSupported := capabilities.Max
	if minSupported <= 0 {
		minSupported = 1
	}
	if maxSupported <= 0 {
		maxSupported = current
	}

	if c.Min != nil && *c.Min > maxSupported {
		return 0, &OverconstrainedError{
			Constraint: "frameRate",
			Message:    fmt.Sprintf("minimum is %.2f, max supported is %.2f", *c.Min, maxSupported),
		}
	}
	if c.Max != nil && *c.Max < minSupported {
		return 0, &OverconstrainedError{
			Constraint: "frameRate",
			Message:    fmt.Sprintf("maximum is %.2f, min supported is %.2f", *c.Max, minSupported),
		}
	}
	if c.Exact != nil {
		if *c.Exact < minSupported || *c.Exact > maxSupported {
			return 0, &OverconstrainedError{
				Constraint: "frameRate",
				Message:    fmt.Sprintf("requires exact %.2f, supported range is %.2f-%.2f", *c.Exact, minSupported, maxSupported),
			}
		}
		return *c.Exact, nil
	}

	target := current
	if c.Ideal != nil {
		target = *c.Ideal
	}
	if target < minSupported {
		target = minSupported
	}
	if target > maxSupported {
		target = maxSupported
	}
	if c.Max != nil && target > *c.Max {
		target = *c.Max
	}
	if c.Min != nil && target < *c.Min {
		target = *c.Min
	}
	if target < minSupported || target > maxSupported {
		return 0, &OverconstrainedError{
			Constraint: "frameRate",
			Message:    fmt.Sprintf("supported range is %.2f-%.2f", minSupported, maxSupported),
		}
	}
	return target, nil
}

func mergeVideoConstraints(base, update VideoConstraints) VideoConstraints {
	merged := base
	if update.Width.IsSet() {
		merged.Width = update.Width
	}
	if update.Height.IsSet() {
		merged.Height = update.Height
	}
	if update.FrameRate.IsSet() {
		merged.FrameRate = update.FrameRate
	}
	if update.FacingMode != "" {
		merged.FacingMode = update.FacingMode
	}
	if update.DeviceID.IsSet() {
		merged.DeviceID = update.DeviceID
	}
	if update.Codec != 0 {
		merged.Codec = update.Codec
	}
	if update.Bitrate != 0 {
		merged.Bitrate = update.Bitrate
	}
	if update.SVC != nil {
		merged.SVC = update.SVC
	}
	if len(update.CodecPreferences) > 0 {
		merged.CodecPreferences = append([]webrtc.RTPCodecParameters(nil), update.CodecPreferences...)
	}
	return merged
}

func mergeAudioConstraints(base, update AudioConstraints) AudioConstraints {
	merged := base
	if update.SampleRate.IsSet() {
		merged.SampleRate = update.SampleRate
	}
	if update.ChannelCount.IsSet() {
		merged.ChannelCount = update.ChannelCount
	}
	if update.EchoCancellation.IsSet() {
		merged.EchoCancellation = update.EchoCancellation
	}
	if update.NoiseSuppression.IsSet() {
		merged.NoiseSuppression = update.NoiseSuppression
	}
	if update.AutoGainControl.IsSet() {
		merged.AutoGainControl = update.AutoGainControl
	}
	if update.DeviceID.IsSet() {
		merged.DeviceID = update.DeviceID
	}
	if update.Bitrate != 0 {
		merged.Bitrate = update.Bitrate
	}
	if len(update.CodecPreferences) > 0 {
		merged.CodecPreferences = append([]webrtc.RTPCodecParameters(nil), update.CodecPreferences...)
	}
	return merged
}

func validateVideoConstraintsAgainstSettings(c VideoConstraints, settings VideoTrackSettings) error {
	if err := c.Width.Validate(settings.Width); err != nil {
		return withConstraintName(err, "width")
	}
	if err := c.Height.Validate(settings.Height); err != nil {
		return withConstraintName(err, "height")
	}
	if c.DeviceID.Exact != nil && settings.DeviceID != *c.DeviceID.Exact {
		return &OverconstrainedError{
			Constraint: "deviceId",
			Message:    fmt.Sprintf("requires exact %q", *c.DeviceID.Exact),
		}
	}
	if c.FacingMode != "" && settings.FacingMode != "" && settings.FacingMode != c.FacingMode {
		return &OverconstrainedError{
			Constraint: "facingMode",
			Message:    fmt.Sprintf("requires %q", c.FacingMode),
		}
	}
	return nil
}

func validateAudioConstraintsAgainstSettings(c AudioConstraints, settings AudioTrackSettings) error {
	if err := c.SampleRate.Validate(settings.SampleRate); err != nil {
		return withConstraintName(err, "sampleRate")
	}
	if err := c.ChannelCount.Validate(settings.ChannelCount); err != nil {
		return withConstraintName(err, "channelCount")
	}
	if c.DeviceID.Exact != nil && settings.DeviceID != *c.DeviceID.Exact {
		return &OverconstrainedError{
			Constraint: "deviceId",
			Message:    fmt.Sprintf("requires exact %q", *c.DeviceID.Exact),
		}
	}
	return nil
}

func withConstraintName(err error, constraint string) error {
	var overconstrained *OverconstrainedError
	if errors.As(err, &overconstrained) {
		overconstrained.Constraint = constraint
		return overconstrained
	}
	return err
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
	switch t.source {
	case sourceDevice:
		devices, err := listCaptureDevices()
		if err != nil {
			return nil
		}
		clone, err := createUserVideoTrack(t.constraints, devices)
		if err != nil {
			return nil
		}
		return clone
	case sourceDisplay:
		screens, err := listCaptureScreens()
		if err != nil || t.displayConstraints == nil {
			return nil
		}
		clone, err := createDisplayVideoTrack(*t.displayConstraints, screens)
		if err != nil {
			return nil
		}
		return clone
	default:
		clone, err := newVideoStreamTrack(t.constraints, t.settings, t.label)
		if err != nil {
			return nil
		}
		return clone
	}
}

func (t *videoStreamTrack) GetConstraints() VideoConstraints { return t.constraints }
func (t *videoStreamTrack) GetCapabilities() VideoTrackCapabilities {
	return copyVideoTrackCapabilities(t.capabilities)
}
func (t *videoStreamTrack) GetSettings() VideoTrackSettings { return t.settings }

func (t *videoStreamTrack) ApplyConstraints(vc VideoConstraints) error {
	if err := validateVideoConstraints(vc); err != nil {
		return err
	}

	merged := mergeVideoConstraints(t.constraints, vc)
	if err := validateVideoConstraintsAgainstSettings(merged, t.settings); err != nil {
		return err
	}

	if merged.Bitrate > 0 && merged.Bitrate != t.constraints.Bitrate {
		if err := t.track.SetBitrate(merged.Bitrate); err != nil {
			return err
		}
	}

	nextFrameRate, err := resolveVideoFrameRateConstraint(
		merged.FrameRate,
		t.settings.FrameRate,
		t.capabilities.FrameRate,
	)
	if err != nil {
		return err
	}
	if nextFrameRate != t.settings.FrameRate {
		if err := t.track.SetFramerate(nextFrameRate); err != nil {
			return err
		}
		t.settings.FrameRate = nextFrameRate
		merged.FrameRate = ExactFloat(nextFrameRate)
	}

	t.constraints = merged
	if merged.FacingMode != "" {
		t.settings.FacingMode = merged.FacingMode
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
	if t.displayConstraints == nil {
		return ErrInvalidConstraints
	}

	screenID := t.displayConstraints.ScreenID
	isWindow := false
	if t.displayConstraints.WindowID != 0 {
		screenID = t.displayConstraints.WindowID
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
	switch t.source {
	case sourceDevice:
		devices, err := listCaptureDevices()
		if err != nil {
			return nil
		}
		clone, err := createUserAudioTrack(t.constraints, devices)
		if err != nil {
			return nil
		}
		return clone
	default:
		clone, err := newAudioStreamTrack(t.constraints, t.settings, t.label)
		if err != nil {
			return nil
		}
		return clone
	}
}

func (t *audioStreamTrack) GetConstraints() AudioConstraints { return t.constraints }
func (t *audioStreamTrack) GetCapabilities() AudioTrackCapabilities {
	return copyAudioTrackCapabilities(t.capabilities)
}
func (t *audioStreamTrack) GetSettings() AudioTrackSettings { return t.settings }

func (t *audioStreamTrack) ApplyConstraints(ac AudioConstraints) error {
	if err := validateAudioConstraints(ac); err != nil {
		return err
	}

	merged := mergeAudioConstraints(t.constraints, ac)
	if err := validateAudioConstraintsAgainstSettings(merged, t.settings); err != nil {
		return err
	}

	if merged.Bitrate > 0 && merged.Bitrate != t.constraints.Bitrate {
		if err := t.track.SetBitrate(merged.Bitrate); err != nil {
			return err
		}
	}

	if merged.EchoCancellation.IsSet() {
		t.settings.EchoCancellation = resolveBoolConstraint(merged.EchoCancellation, t.settings.EchoCancellation)
		merged.EchoCancellation = ExactBool(t.settings.EchoCancellation)
	}
	if merged.NoiseSuppression.IsSet() {
		t.settings.NoiseSuppression = resolveBoolConstraint(merged.NoiseSuppression, t.settings.NoiseSuppression)
		merged.NoiseSuppression = ExactBool(t.settings.NoiseSuppression)
	}
	if merged.AutoGainControl.IsSet() {
		t.settings.AutoGainControl = resolveBoolConstraint(merged.AutoGainControl, t.settings.AutoGainControl)
		merged.AutoGainControl = ExactBool(t.settings.AutoGainControl)
	}

	t.constraints = merged
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
