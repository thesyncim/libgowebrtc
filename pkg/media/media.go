// Package media provides a browser-like API for media capture and track management.
// This mirrors the Web APIs like getUserMedia, MediaStreamTrack, and addTrack.
package media

import (
	"errors"
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
		// If library not loaded, return empty list (browser-like behavior)
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
			GroupID:  "", // Not provided by shim yet
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
	Video *DisplayVideoConstraints // nil = no video
	Audio *AudioConstraints        // nil = no audio (usually nil for screen share)
}

// DisplayVideoConstraints for screen/window capture.
type DisplayVideoConstraints struct {
	// ScreenID specifies which screen to capture (from EnumerateScreens).
	// If 0 and WindowID is 0, captures the primary screen.
	ScreenID int64
	// WindowID specifies which window to capture (from EnumerateScreens).
	// Takes precedence over ScreenID if non-zero.
	WindowID int64
	// FrameRate is the desired capture framerate (0 = default).
	FrameRate float64
	// Width is the desired width (0 = native resolution).
	Width int
	// Height is the desired height (0 = native resolution).
	Height int
	// Codec is the preferred video codec.
	Codec codec.Type
	// Bitrate is the target bitrate (0 = auto).
	Bitrate uint32
	// SVC configuration for screen sharing.
	SVC *codec.SVCConfig
}

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
// Use VideoStreamTrack or AudioStreamTrack for type-safe constraint access.
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
}

// VideoStreamTrack provides type-safe access to video track constraints and settings.
type VideoStreamTrack interface {
	MediaStreamTrack

	// GetConstraints returns the current video constraints.
	GetConstraints() VideoConstraints

	// ApplyConstraints applies new video constraints.
	ApplyConstraints(constraints VideoConstraints) error

	// GetSettings returns current video settings.
	GetSettings() VideoTrackSettings

	// WriteFrame writes a video frame to the track.
	WriteFrame(f *frame.VideoFrame, forceKeyframe bool) error

	// RequestKeyFrame requests a keyframe from the encoder.
	RequestKeyFrame()
}

// AudioStreamTrack provides type-safe access to audio track constraints and settings.
type AudioStreamTrack interface {
	MediaStreamTrack

	// GetConstraints returns the current audio constraints.
	GetConstraints() AudioConstraints

	// ApplyConstraints applies new audio constraints.
	ApplyConstraints(constraints AudioConstraints) error

	// GetSettings returns current audio settings.
	GetSettings() AudioTrackSettings

	// WriteFrame writes an audio frame to the track.
	WriteFrame(f *frame.AudioFrame) error
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

	// Device capture (optional - started when DeviceID is specified)
	videoCapture  *ffi.VideoCapture
	screenCapture *ffi.ScreenCapture
	mu            sync.Mutex
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

	// Device capture (optional - started when DeviceID is specified)
	audioCapture *ffi.AudioCapture
	mu           sync.Mutex
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

	// Start device capture if DeviceID is specified and shim is loaded
	// Ignore error - capture may fail but track can still be used for manual frame input
	if constraints.DeviceID != "" && ffi.IsLoaded() {
		_ = t.startVideoCapture()
	}

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

	// Start device capture if DeviceID is specified and shim is loaded
	// Ignore error - capture may fail but track can still be used for manual frame input
	if constraints.DeviceID != "" && ffi.IsLoaded() {
		_ = t.startAudioCapture()
	}

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
func GetDisplayMedia(c DisplayConstraints) (*MediaStream, error) {
	stream := NewMediaStream()

	if c.Video != nil {
		// Apply defaults
		width := c.Video.Width
		if width <= 0 {
			width = 1920 // Default to 1080p for screen share
		}
		height := c.Video.Height
		if height <= 0 {
			height = 1080
		}
		frameRate := c.Video.FrameRate
		if frameRate <= 0 {
			frameRate = 30
		}
		codecType := c.Video.Codec
		if codecType == 0 {
			codecType = codec.VP9 // VP9 is better for screen content
		}
		bitrate := c.Video.Bitrate
		if bitrate == 0 {
			bitrate = 3_000_000 // 3 Mbps default for screen share
		}
		svc := c.Video.SVC
		if svc == nil {
			svc = codec.SVCPresetScreenShare()
		}

		// Create video track with screen share optimizations
		vt, err := CreateVideoTrack(VideoConstraints{
			Width:     width,
			Height:    height,
			FrameRate: frameRate,
			Codec:     codecType,
			Bitrate:   bitrate,
			SVC:       svc,
		})
		if err != nil {
			return nil, err
		}

		// Set label to indicate screen capture
		if vst, ok := vt.(*videoStreamTrack); ok {
			if c.Video.WindowID != 0 {
				vst.label = "window-capture"
			} else {
				vst.label = "screen-capture"
			}

			// Start screen capture if shim is loaded
			if ffi.IsLoaded() {
				isWindow := c.Video.WindowID != 0
				screenID := c.Video.ScreenID
				if isWindow {
					screenID = c.Video.WindowID
				}
				// Ignore error - capture may fail but track can still be used for manual frame input
				_ = vst.startScreenCapture(screenID, isWindow)
			}
		}

		stream.AddTrack(vt)
	}

	if c.Audio != nil {
		at, err := CreateAudioTrack(*c.Audio)
		if err != nil {
			return nil, err
		}
		stream.AddTrack(at)
	}

	return stream, nil
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

	// Stop any active capture
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

	t.track.Close()
}

// startVideoCapture starts capturing from the device specified in constraints.
func (t *videoStreamTrack) startVideoCapture() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.videoCapture != nil {
		return nil // Already capturing
	}

	capture, err := ffi.NewVideoCapture(
		t.constraints.DeviceID,
		t.constraints.Width,
		t.constraints.Height,
		int(t.constraints.FrameRate),
	)
	if err != nil {
		return err
	}

	// Start capture with callback that writes to track
	err = capture.Start(func(captured *ffi.CapturedVideoFrame) {
		if !t.enabled.Load() || t.muted.Load() {
			return // Track disabled or muted, skip frame
		}

		// Convert ffi.CapturedVideoFrame to frame.VideoFrame
		videoFrame := &frame.VideoFrame{
			Width:  captured.Width,
			Height: captured.Height,
			Format: frame.PixelFormatI420,
			Data:   [][]byte{captured.YPlane, captured.UPlane, captured.VPlane},
			Stride: []int{captured.YStride, captured.UStride, captured.VStride},
		}

		// Write to track - ignore ErrNotBound (track not yet added to PeerConnection)
		_ = t.track.WriteFrame(videoFrame, false)
	})

	if err != nil {
		capture.Close()
		return err
	}

	t.videoCapture = capture
	return nil
}

// startScreenCapture starts screen capture.
func (t *videoStreamTrack) startScreenCapture(screenID int64, isWindow bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.screenCapture != nil {
		return nil // Already capturing
	}

	capture, err := ffi.NewScreenCapture(screenID, isWindow, int(t.constraints.FrameRate))
	if err != nil {
		return err
	}

	err = capture.Start(func(captured *ffi.CapturedVideoFrame) {
		if !t.enabled.Load() || t.muted.Load() {
			return
		}

		videoFrame := &frame.VideoFrame{
			Width:  captured.Width,
			Height: captured.Height,
			Format: frame.PixelFormatI420,
			Data:   [][]byte{captured.YPlane, captured.UPlane, captured.VPlane},
			Stride: []int{captured.YStride, captured.UStride, captured.VStride},
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

func (t *videoStreamTrack) Clone() MediaStreamTrack {
	clone, _ := CreateVideoTrack(t.constraints)
	return clone
}

func (t *videoStreamTrack) GetConstraints() VideoConstraints { return t.constraints }
func (t *videoStreamTrack) GetSettings() VideoTrackSettings  { return t.settings }

func (t *videoStreamTrack) ApplyConstraints(vc VideoConstraints) error {
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

// Compile-time interface check
var _ VideoStreamTrack = (*videoStreamTrack)(nil)

func (t *videoStreamTrack) pionTrack() webrtc.TrackLocal { return t.track }

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

	// Stop any active capture
	t.mu.Lock()
	if t.audioCapture != nil {
		t.audioCapture.Close()
		t.audioCapture = nil
	}
	t.mu.Unlock()

	t.track.Close()
}

// startAudioCapture starts capturing from the audio device specified in constraints.
func (t *audioStreamTrack) startAudioCapture() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.audioCapture != nil {
		return nil // Already capturing
	}

	capture, err := ffi.NewAudioCapture(
		t.constraints.DeviceID,
		t.constraints.SampleRate,
		t.constraints.ChannelCount,
	)
	if err != nil {
		return err
	}

	// Start capture with callback that writes to track
	err = capture.Start(func(captured *ffi.CapturedAudioFrame) {
		if !t.enabled.Load() || t.muted.Load() {
			return // Track disabled or muted, skip frame
		}

		// Convert ffi.CapturedAudioFrame to frame.AudioFrame
		audioFrame := frame.NewAudioFrameFromS16(
			captured.Samples,
			captured.SampleRate,
			captured.NumChannels,
		)

		// Write to track - ignore ErrNotBound (track not yet added to PeerConnection)
		_ = t.track.WriteFrame(audioFrame)
	})

	if err != nil {
		capture.Close()
		return err
	}

	t.audioCapture = capture
	return nil
}

func (t *audioStreamTrack) Clone() MediaStreamTrack {
	clone, _ := CreateAudioTrack(t.constraints)
	return clone
}

func (t *audioStreamTrack) GetConstraints() AudioConstraints { return t.constraints }
func (t *audioStreamTrack) GetSettings() AudioTrackSettings  { return t.settings }

func (t *audioStreamTrack) ApplyConstraints(ac AudioConstraints) error {
	if ac.Bitrate > 0 && ac.Bitrate != t.constraints.Bitrate {
		t.track.SetBitrate(ac.Bitrate)
		t.constraints.Bitrate = ac.Bitrate
	}
	return nil
}

// Compile-time interface check
var _ AudioStreamTrack = (*audioStreamTrack)(nil)

func (t *audioStreamTrack) pionTrack() webrtc.TrackLocal { return t.track }

// WriteFrame writes an audio frame.
func (t *audioStreamTrack) WriteFrame(f *frame.AudioFrame) error {
	if !t.enabled.Load() || t.readyState.Load().(string) != "live" {
		return nil
	}
	return t.track.WriteFrame(f)
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
