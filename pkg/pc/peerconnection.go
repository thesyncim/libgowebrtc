// Package pc provides a thin PeerConnection wrapper backed by libwebrtc.
// It stays close to libwebrtc's native transport model rather than emulating
// browser multi-stream helper semantics.
package pc

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

// Errors
var (
	ErrPeerConnectionClosed  = errors.New("peer connection closed")
	ErrInvalidState          = errors.New("invalid state")
	ErrCreateOfferFailed     = errors.New("create offer failed")
	ErrCreateAnswerFailed    = errors.New("create answer failed")
	ErrSetDescriptionFailed  = errors.New("set description failed")
	ErrAddICECandidateFailed = errors.New("add ice candidate failed")
	ErrTrackNotFound         = errors.New("track not found")
	ErrNilTrack              = errors.New("track is nil")
	ErrNilSender             = errors.New("sender is nil")
	ErrInvalidStreamID       = errors.New("stream id is required")
	ErrNilVideoFrame         = errors.New("video frame is nil")
	ErrNilAudioFrame         = errors.New("audio frame is nil")
	// ErrNotSupported reports APIs that are intentionally exposed but not yet
	// backed by the current shim/runtime implementation.
	ErrNotSupported = ffi.ErrNotSupported
)

// Compatibility notes kept alongside the thin API surface:
// The current shim does not implement sender stats and returns ErrNotSupported.
// The current shim does not implement this surface and returns ErrNotSupported.

// Constants
const (
	maxSDPSize = 64 * 1024 // 64KB should be sufficient for SDP
)

type (
	SignalingState        = webrtc.SignalingState
	ICEConnectionState    = webrtc.ICEConnectionState
	ICEGatheringState     = webrtc.ICEGathererState
	PeerConnectionState   = webrtc.PeerConnectionState
	SDPType               = webrtc.SDPType
	SessionDescription    = webrtc.SessionDescription
	ICECandidate          = webrtc.ICECandidateInit
	Configuration         = webrtc.Configuration
	OfferOptions          = webrtc.OfferOptions
	AnswerOptions         = webrtc.AnswerOptions
	DataChannelState      = webrtc.DataChannelState
	TransceiverDirection  = webrtc.RTPTransceiverDirection
	TransceiverInit       = webrtc.RTPTransceiverInit
	DataChannelInit       = webrtc.DataChannelInit
	RTPSendParameters     = webrtc.RTPSendParameters
	RTPEncodingParameters = webrtc.RTPEncodingParameters
)

const (
	SignalingStateStable             = webrtc.SignalingStateStable
	SignalingStateHaveLocalOffer     = webrtc.SignalingStateHaveLocalOffer
	SignalingStateHaveRemoteOffer    = webrtc.SignalingStateHaveRemoteOffer
	SignalingStateHaveLocalPranswer  = webrtc.SignalingStateHaveLocalPranswer
	SignalingStateHaveRemotePranswer = webrtc.SignalingStateHaveRemotePranswer
	SignalingStateClosed             = webrtc.SignalingStateClosed
	ICEConnectionStateNew            = webrtc.ICEConnectionStateNew
	ICEConnectionStateChecking       = webrtc.ICEConnectionStateChecking
	ICEConnectionStateConnected      = webrtc.ICEConnectionStateConnected
	ICEConnectionStateCompleted      = webrtc.ICEConnectionStateCompleted
	ICEConnectionStateDisconnected   = webrtc.ICEConnectionStateDisconnected
	ICEConnectionStateFailed         = webrtc.ICEConnectionStateFailed
	ICEConnectionStateClosed         = webrtc.ICEConnectionStateClosed
	ICEGatheringStateNew             = webrtc.ICEGathererStateNew
	ICEGatheringStateGathering       = webrtc.ICEGathererStateGathering
	ICEGatheringStateComplete        = webrtc.ICEGathererStateComplete
	ICEGatheringStateClosed          = webrtc.ICEGathererStateClosed
	PeerConnectionStateNew           = webrtc.PeerConnectionStateNew
	PeerConnectionStateConnecting    = webrtc.PeerConnectionStateConnecting
	PeerConnectionStateConnected     = webrtc.PeerConnectionStateConnected
	PeerConnectionStateDisconnected  = webrtc.PeerConnectionStateDisconnected
	PeerConnectionStateFailed        = webrtc.PeerConnectionStateFailed
	PeerConnectionStateClosed        = webrtc.PeerConnectionStateClosed
	SDPTypeOffer                     = webrtc.SDPTypeOffer
	SDPTypePranswer                  = webrtc.SDPTypePranswer
	SDPTypeAnswer                    = webrtc.SDPTypeAnswer
	SDPTypeRollback                  = webrtc.SDPTypeRollback
	BundlePolicyBalanced             = webrtc.BundlePolicyBalanced
	BundlePolicyMaxCompat            = webrtc.BundlePolicyMaxCompat
	BundlePolicyMaxBundle            = webrtc.BundlePolicyMaxBundle
	RTCPMuxPolicyRequire             = webrtc.RTCPMuxPolicyRequire
	RTCPMuxPolicyNegotiate           = webrtc.RTCPMuxPolicyNegotiate
	SDPSemanticsUnifiedPlan          = webrtc.SDPSemanticsUnifiedPlan
	SDPSemanticsPlanB                = webrtc.SDPSemanticsPlanB
	ICETransportPolicyAll            = webrtc.ICETransportPolicyAll
	ICETransportPolicyRelay          = webrtc.ICETransportPolicyRelay
	DataChannelStateConnecting       = webrtc.DataChannelStateConnecting
	DataChannelStateOpen             = webrtc.DataChannelStateOpen
	DataChannelStateClosing          = webrtc.DataChannelStateClosing
	DataChannelStateClosed           = webrtc.DataChannelStateClosed
	TransceiverDirectionSendRecv     = webrtc.RTPTransceiverDirectionSendrecv
	TransceiverDirectionSendOnly     = webrtc.RTPTransceiverDirectionSendonly
	TransceiverDirectionRecvOnly     = webrtc.RTPTransceiverDirectionRecvonly
	TransceiverDirectionInactive     = webrtc.RTPTransceiverDirectionInactive
)

// DefaultConfiguration returns a default configuration.
func DefaultConfiguration() webrtc.Configuration {
	return webrtc.Configuration{
		BundlePolicy:       webrtc.BundlePolicyBalanced,
		RTCPMuxPolicy:      webrtc.RTCPMuxPolicyRequire,
		ICETransportPolicy: webrtc.ICETransportPolicyAll,
		SDPSemantics:       webrtc.SDPSemanticsUnifiedPlan,
	}
}

func codecParametersFromFFICapabilities(ffiCodecs []ffi.CodecCapability) []webrtc.RTPCodecParameters {
	codecs := make([]webrtc.RTPCodecParameters, len(ffiCodecs))
	for i, c := range ffiCodecs {
		codecs[i] = webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    ffi.CStringToGo(c.MimeType[:]),
				ClockRate:   uint32(c.ClockRate),
				Channels:    uint16(c.Channels),
				SDPFmtpLine: ffi.CStringToGo(c.SDPFmtpLine[:]),
			},
			PayloadType: webrtc.PayloadType(c.PayloadType),
		}
	}
	return codecs
}

func ffiCodecCapabilitiesFromParameters(codecs []webrtc.RTPCodecParameters) []ffi.CodecCapability {
	if len(codecs) == 0 {
		return nil
	}

	out := make([]ffi.CodecCapability, len(codecs))
	for i, c := range codecs {
		copy(out[i].MimeType[:], c.MimeType)
		out[i].ClockRate = int32(c.ClockRate)
		out[i].Channels = int32(c.Channels)
		copy(out[i].SDPFmtpLine[:], c.SDPFmtpLine)
		out[i].PayloadType = int32(c.PayloadType)
	}
	return out
}

// RTPSender represents an RTP sender.
type RTPSender struct {
	handle   uintptr
	track    *Track
	pc       *PeerConnection
	id       string
	streamID string
	mu       sync.RWMutex
}

// IsValid returns true if the sender has a valid native handle.
func (s *RTPSender) IsValid() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.handle != 0
}

// Track returns the sender's track.
func (s *RTPSender) Track() *Track {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.track
}

// StreamID returns the explicit MediaStream ID associated with the sender.
func (s *RTPSender) StreamID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.streamID
}

// ReplaceTrack replaces the sender's track.
// Pass nil to remove the track without replacing it.
func (s *RTPSender) ReplaceTrack(t *Track) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.handle == 0 {
		return errors.New("sender not initialized")
	}

	var trackHandle uintptr
	if t != nil {
		trackHandle = t.handle
	}

	if err := ffi.RTPSenderReplaceTrack(s.handle, trackHandle); err != nil {
		return err
	}

	s.track = t
	return nil
}

// SetParameters sets sender parameters.
func (s *RTPSender) SetParameters(params webrtc.RTPSendParameters) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.handle == 0 {
		return errors.New("sender not initialized")
	}
	if err := validateSendParameters(params); err != nil {
		return err
	}

	ffiParams := ffiSendParametersFromWebRTC(params)
	return ffi.RTPSenderSetParameters(s.handle, ffiParams)
}

// GetParameters gets current parameters.
func (s *RTPSender) GetParameters() webrtc.RTPSendParameters {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.handle == 0 {
		return webrtc.RTPSendParameters{}
	}

	const maxEncodings = 8
	const maxHeaderExtensions = 32
	ffiEncodings := make([]ffi.RTPEncodingParameters, maxEncodings)
	ffiHeaderExtensions := make([]ffi.RTPHeaderExtensionParameter, maxHeaderExtensions)
	ffiParams, count, headerExtensionCount, err := ffi.RTPSenderGetParameters(s.handle, ffiEncodings, ffiHeaderExtensions)
	if err != nil || ffiParams == nil {
		return webrtc.RTPSendParameters{}
	}

	ffiCodecs, err := ffi.RTPSenderGetNegotiatedCodecs(s.handle)
	if err != nil {
		return webrtc.RTPSendParameters{}
	}
	return webRTCSendParametersFromFFI(
		ffiEncodings[:count],
		count,
		ffiHeaderExtensions[:headerExtensionCount],
		headerExtensionCount,
		codecParametersFromFFICapabilities(ffiCodecs),
	)
}

// SetLayerActive enables or disables a simulcast layer.
func (s *RTPSender) SetLayerActive(rid string, active bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.handle == 0 {
		return errors.New("sender not initialized")
	}

	return ffi.RTPSenderSetLayerActive(s.handle, rid, active)
}

// SetLayerBitrate sets the maximum bitrate for a layer.
func (s *RTPSender) SetLayerBitrate(rid string, maxBitrate uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.handle == 0 {
		return errors.New("sender not initialized")
	}

	return ffi.RTPSenderSetLayerBitrate(s.handle, rid, maxBitrate)
}

// GetActiveLayers gets the number of active layers.
func (s *RTPSender) GetActiveLayers() (spatial, temporal int, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.handle == 0 {
		return 0, 0, errors.New("sender not initialized")
	}

	return ffi.RTPSenderGetActiveLayers(s.handle)
}

// SetScalabilityMode sets the SVC scalability mode (e.g., "L3T3_KEY", "L1T2").
// This allows runtime configuration of spatial/temporal layers.
func (s *RTPSender) SetScalabilityMode(mode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.handle == 0 {
		return errors.New("sender not initialized")
	}

	return ffi.RTPSenderSetScalabilityMode(s.handle, mode)
}

// GetScalabilityMode gets the current SVC scalability mode.
func (s *RTPSender) GetScalabilityMode() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.handle == 0 {
		return "", errors.New("sender not initialized")
	}

	return ffi.RTPSenderGetScalabilityMode(s.handle)
}

// GetNegotiatedCodecs returns the list of codecs negotiated in SDP for this sender.
// These are the codecs available for use with SetPreferredCodec.
func (s *RTPSender) GetNegotiatedCodecs() ([]webrtc.RTPCodecParameters, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.handle == 0 {
		return nil, errors.New("sender not initialized")
	}

	ffiCodecs, err := ffi.RTPSenderGetNegotiatedCodecs(s.handle)
	if err != nil {
		return nil, err
	}

	return codecParametersFromFFICapabilities(ffiCodecs), nil
}

// SetPreferredCodec sets the preferred codec for this sender.
// The codec must have been negotiated in the initial SDP (check GetNegotiatedCodecs).
//
// Note: Due to WebRTC limitations, this may require renegotiation to take effect.
// If renegotiation is needed, returns ErrRenegotiationNeeded.
// After setting, call CreateOffer/SetLocalDescription to apply the change.
func (s *RTPSender) SetPreferredCodec(codec webrtc.RTPCodecParameters) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.handle == 0 {
		return errors.New("sender not initialized")
	}
	if strings.TrimSpace(codec.MimeType) == "" {
		return errors.New("codec mime type is required")
	}

	return ffi.RTPSenderSetPreferredCodec(s.handle, codec.MimeType, int(codec.PayloadType))
}

// ErrRenegotiationNeeded is returned when codec switching requires SDP renegotiation.
var ErrRenegotiationNeeded = ffi.ErrRenegotiationNeeded

// ErrNotFound is returned when the requested codec was not negotiated.
var ErrNotFound = ffi.ErrNotFound

// RTPReceiver represents an RTP receiver.
type RTPReceiver struct {
	handle uintptr
	track  *Track
	pc     *PeerConnection
}

// IsValid returns true if the receiver has a valid native handle.
func (r *RTPReceiver) IsValid() bool {
	return r.handle != 0
}

// Track returns the receiver's track.
func (r *RTPReceiver) Track() *Track {
	return r.track
}

// SetJitterBufferMinDelay sets the minimum jitter buffer delay in milliseconds.
// This sets a floor for libwebrtc's adaptive jitter buffer. The actual delay
// may be higher based on network conditions, but won't go below this value.
// Pass 0 to let libwebrtc's adaptive algorithm decide without a minimum floor.
//
// For jitter buffer statistics, use PeerConnection.GetStats() and inspect the
// resulting inbound RTP stats entries.
func (r *RTPReceiver) SetJitterBufferMinDelay(minDelayMs int) error {
	if r.handle == 0 {
		return errors.New("receiver not initialized")
	}

	return ffi.RTPReceiverSetJitterBufferMinDelay(r.handle, minDelayMs)
}

// RTPTransceiver represents an RTP transceiver.
type RTPTransceiver struct {
	handle    uintptr
	sender    *RTPSender
	receiver  *RTPReceiver
	direction TransceiverDirection
	mid       string
	kind      string
	pc        *PeerConnection
}

// IsValid returns true if the transceiver has a valid native handle.
func (t *RTPTransceiver) IsValid() bool {
	return t.handle != 0
}

// Sender returns the transceiver's sender.
func (t *RTPTransceiver) Sender() *RTPSender { return t.sender }

// Receiver returns the transceiver's receiver.
func (t *RTPTransceiver) Receiver() *RTPReceiver { return t.receiver }

// Direction returns current direction.
func (t *RTPTransceiver) Direction() TransceiverDirection {
	if t.handle != 0 {
		dir := ffi.TransceiverGetDirection(t.handle)
		return transceiverDirectionFromFFI(dir)
	}
	return t.direction
}

// SetDirection sets the direction.
func (t *RTPTransceiver) SetDirection(d TransceiverDirection) error {
	direction, err := transceiverDirectionToFFI(d)
	if err != nil {
		return err
	}
	if t.handle != 0 {
		if err := ffi.TransceiverSetDirection(t.handle, direction); err != nil {
			return err
		}
	}
	t.direction = d
	return nil
}

// CurrentDirection returns the current direction as negotiated in SDP.
func (t *RTPTransceiver) CurrentDirection() TransceiverDirection {
	if t.handle != 0 {
		dir := ffi.TransceiverGetCurrentDirection(t.handle)
		return transceiverDirectionFromFFI(dir)
	}
	return t.direction
}

// Mid returns the transceiver's mid.
func (t *RTPTransceiver) Mid() string {
	if t.handle != 0 {
		return ffi.TransceiverMid(t.handle)
	}
	return t.mid
}

// Stop stops the transceiver.
func (t *RTPTransceiver) Stop() error {
	if t.handle != 0 {
		return ffi.TransceiverStop(t.handle)
	}
	return nil
}

// Kind returns the transceiver media kind when known.
func (t *RTPTransceiver) Kind() string {
	return t.kind
}

// SetCodecPreferences sets which codecs are negotiated for this transceiver.
// Must be called before creating offer/answer.
// This allows specifying which codecs should be included in SDP negotiation.
func (t *RTPTransceiver) SetCodecPreferences(codecs []webrtc.RTPCodecParameters) error {
	if t.handle == 0 {
		return errors.New("transceiver not initialized")
	}

	return ffi.TransceiverSetCodecPreferences(t.handle, ffiCodecCapabilitiesFromParameters(codecs))
}

func inferTransceiverKind(handle uintptr) string {
	if handle == 0 {
		return ""
	}
	codecs, err := ffi.TransceiverGetCodecPreferences(handle)
	if err != nil || len(codecs) == 0 {
		return ""
	}
	mime := strings.ToLower(strings.TrimSpace(ffi.CStringToGo(codecs[0].MimeType[:])))
	switch {
	case strings.HasPrefix(mime, "audio/"):
		return "audio"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	default:
		return ""
	}
}

// VideoFrameHandler is called when a video frame is received on a remote track.
type VideoFrameHandler func(f *frame.VideoFrame)

// AudioFrameHandler is called when an audio frame is received on a remote track.
type AudioFrameHandler func(f *frame.AudioFrame)

// Track represents a media track (can be local or remote).
type Track struct {
	handle  uintptr
	id      string
	kind    string // "video" or "audio"
	label   string
	enabled atomic.Bool
	muted   atomic.Bool
	pc      *PeerConnection

	// Video/audio track source for frame injection (local tracks)
	sourceHandle uintptr
	width        int
	height       int
	sampleRate   int
	channels     int

	// Frame handlers (remote tracks)
	onVideoFrame VideoFrameHandler
	onAudioFrame AudioFrameHandler

	// For writing frames
	mu sync.Mutex
}

// IsValid returns true if the track has a valid native handle.
// For local tracks, checks sourceHandle; for remote tracks, checks handle.
func (t *Track) IsValid() bool {
	return t.handle != 0 || t.sourceHandle != 0
}

// ID returns the track ID.
func (t *Track) ID() string { return t.id }

// Kind returns "video" or "audio".
func (t *Track) Kind() string { return t.kind }

// Label returns the track label.
func (t *Track) Label() string { return t.label }

// Enabled returns whether the track is enabled.
func (t *Track) Enabled() bool { return t.enabled.Load() }

// SetEnabled enables or disables the track.
func (t *Track) SetEnabled(e bool) { t.enabled.Store(e) }

// Muted returns whether the track is muted.
func (t *Track) Muted() bool { return t.muted.Load() }

// SetOnVideoFrame sets a callback to receive video frames from a remote track.
// This is the Pion/browser-like interface for reading frames from received tracks.
func (t *Track) SetOnVideoFrame(handler VideoFrameHandler) error {
	if t.kind != "video" {
		return errors.New("not a video track")
	}
	if t.handle == 0 {
		return errors.New("track handle not initialized")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Remove existing sink if any
	if t.onVideoFrame != nil {
		ffi.TrackRemoveVideoSink(t.handle)
		ffi.UnregisterVideoCallback(t.handle)
	}

	t.onVideoFrame = handler

	if handler == nil {
		return nil
	}

	// Register the callback in the FFI layer
	ffi.RegisterVideoCallback(t.handle, func(width, height int, yPlane, uPlane, vPlane []byte, yStride, uStride, vStride int, timestampUs int64) {
		// Convert to frame.VideoFrame
		f := &frame.VideoFrame{
			Width:  width,
			Height: height,
			Format: frame.PixelFormatI420,
			Data:   [][]byte{yPlane, uPlane, vPlane},
			Stride: []int{yStride, uStride, vStride},
			PTS:    uint32(timestampUs / 1000), // Convert to milliseconds
		}
		handler(f)
	})

	// Set the native sink - use track handle as context for callback lookup
	return ffi.TrackSetVideoSink(t.handle, ffi.GetVideoSinkCallbackPtr(), t.handle)
}

// SetOnAudioFrame sets a callback to receive audio frames from a remote track.
// This is the Pion/browser-like interface for reading frames from received tracks.
func (t *Track) SetOnAudioFrame(handler AudioFrameHandler) error {
	if t.kind != "audio" {
		return errors.New("not an audio track")
	}
	if t.handle == 0 {
		return errors.New("track handle not initialized")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Remove existing sink if any
	if t.onAudioFrame != nil {
		ffi.TrackRemoveAudioSink(t.handle)
		ffi.UnregisterAudioCallback(t.handle)
	}

	t.onAudioFrame = handler

	if handler == nil {
		return nil
	}

	// Register the callback in the FFI layer
	ffi.RegisterAudioCallback(t.handle, func(samples []int16, sampleRate, channels int, timestampUs int64) {
		// Convert to frame.AudioFrame
		f := frame.NewAudioFrameFromS16(samples, sampleRate, channels)
		f.PTS = uint32(timestampUs / 1000) // Convert to milliseconds
		handler(f)
	})

	// Set the native sink - use track handle as context for callback lookup
	return ffi.TrackSetAudioSink(t.handle, ffi.GetAudioSinkCallbackPtr(), t.handle)
}

// WriteVideoFrame writes a video frame to the track.
func (t *Track) WriteVideoFrame(f *frame.VideoFrame) error {
	if t.kind != "video" {
		return errors.New("not a video track")
	}
	if f == nil {
		return ErrNilVideoFrame
	}
	if !t.enabled.Load() {
		return nil
	}
	if t.sourceHandle == 0 {
		return errors.New("track source not initialized")
	}
	if f.Format != frame.PixelFormatI420 {
		return errors.New("only I420 format supported")
	}
	if len(f.Data) < 3 || len(f.Stride) < 3 {
		return errors.New("invalid I420 frame data")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Push frame to the native track source via FFI
	// PTS is in 90kHz RTP clock units, convert to microseconds for libwebrtc
	// microseconds = pts_90khz * 1_000_000 / 90_000
	timestampUs := int64(f.PTS) * 1000000 / 90000
	return ffi.VideoTrackSourcePushFrame(
		t.sourceHandle,
		f.Data[0], // Y plane
		f.Data[1], // U plane
		f.Data[2], // V plane
		f.Stride[0],
		f.Stride[1],
		f.Stride[2],
		timestampUs,
	)
}

// WriteAudioFrame writes an audio frame to the track.
func (t *Track) WriteAudioFrame(f *frame.AudioFrame) error {
	if t.kind != "audio" {
		return errors.New("not an audio track")
	}
	if f == nil {
		return ErrNilAudioFrame
	}
	if !t.enabled.Load() {
		return nil
	}
	if t.sourceHandle == 0 {
		return errors.New("track source not initialized")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Convert bytes to int16 samples for FFI
	samples := f.SamplesS16()
	if samples == nil {
		return errors.New("failed to get audio samples")
	}

	// Push audio samples to the native track source via FFI
	// PTS is in 90kHz RTP clock units, convert to microseconds for libwebrtc
	timestampUs := int64(f.PTS) * 1000000 / 90000
	return ffi.AudioTrackSourcePushFrame(
		t.sourceHandle,
		samples,
		timestampUs,
	)
}

// PeerConnection wraps libwebrtc's native PeerConnection.
type PeerConnection struct {
	handle uintptr
	config webrtc.Configuration

	signalingState     atomic.Value
	iceConnectionState atomic.Value
	iceGatheringState  atomic.Value
	connectionState    atomic.Value

	localDescription  *webrtc.SessionDescription
	remoteDescription *webrtc.SessionDescription

	senders      []*RTPSender
	receivers    []*RTPReceiver
	transceivers []*RTPTransceiver
	localTracks  []*Track

	callbackMu sync.RWMutex

	onICECandidate             func(candidate *webrtc.ICECandidateInit)
	onICEConnectionStateChange func(state ICEConnectionState)
	onICEGatheringStateChange  func(state ICEGatheringState)
	onSignalingStateChange     func(state SignalingState)
	onConnectionStateChange    func(state PeerConnectionState)
	onTrack                    func(track *Track, receiver *RTPReceiver, streamID string)
	onNegotiationNeeded        func()
	onDataChannel              func(dc *DataChannel)

	mu     sync.RWMutex
	closed atomic.Bool
}

// IsValid returns true if the PeerConnection has a valid native handle.
func (pc *PeerConnection) IsValid() bool {
	return pc.handle != 0
}

func (pc *PeerConnection) SetOnICECandidate(cb func(candidate *webrtc.ICECandidateInit)) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onICECandidate = cb
}

func (pc *PeerConnection) SetOnICEConnectionStateChange(cb func(state ICEConnectionState)) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onICEConnectionStateChange = cb
}

func (pc *PeerConnection) SetOnICEGatheringStateChange(cb func(state ICEGatheringState)) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onICEGatheringStateChange = cb
}

func (pc *PeerConnection) SetOnSignalingStateChange(cb func(state SignalingState)) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onSignalingStateChange = cb
}

func (pc *PeerConnection) SetOnConnectionStateChange(cb func(state PeerConnectionState)) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onConnectionStateChange = cb
}

func (pc *PeerConnection) SetOnTrack(cb func(track *Track, receiver *RTPReceiver, streamID string)) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onTrack = cb
}

func (pc *PeerConnection) SetOnNegotiationNeeded(cb func()) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onNegotiationNeeded = cb
}

func (pc *PeerConnection) SetOnDataChannel(cb func(dc *DataChannel)) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onDataChannel = cb
}

// DataChannel represents a data channel.
type DataChannel struct {
	handle uintptr
	label  string
	id     uint16
	pc     *PeerConnection

	onOpen    func()
	onClose   func()
	onMessage func(data []byte)
	onError   func(err error)
}

// IsValid returns true if the DataChannel has a valid native handle.
func (dc *DataChannel) IsValid() bool {
	return dc.handle != 0
}

// Label returns the data channel label.
func (dc *DataChannel) Label() string { return dc.label }

// ID returns the data channel ID.
func (dc *DataChannel) ID() uint16 { return dc.id }

// ReadyState returns the current state of the data channel.
func (dc *DataChannel) ReadyState() DataChannelState {
	if dc.handle == 0 {
		return DataChannelStateClosed
	}
	return dataChannelStateFromFFI(ffi.DataChannelReadyState(dc.handle))
}

// SetOnOpen sets the callback for when the data channel opens.
func (dc *DataChannel) SetOnOpen(cb func()) {
	dc.onOpen = cb
	if dc.handle != 0 {
		ffi.DataChannelSetOnOpen(dc.handle, func() {
			if dc.onOpen != nil {
				dc.onOpen()
			}
		})
	}
}

// SetOnClose sets the callback for when the data channel closes.
func (dc *DataChannel) SetOnClose(cb func()) {
	dc.onClose = cb
	if dc.handle != 0 {
		ffi.DataChannelSetOnClose(dc.handle, func() {
			if dc.onClose != nil {
				dc.onClose()
			}
		})
	}
}

// SetOnMessage sets the callback for when a message is received.
func (dc *DataChannel) SetOnMessage(cb func(data []byte)) {
	dc.onMessage = cb
	if dc.handle != 0 {
		ffi.DataChannelSetOnMessage(dc.handle, func(data []byte, isBinary bool) {
			if dc.onMessage != nil {
				dc.onMessage(data)
			}
		})
	}
}

// SetOnError sets the callback for errors.
func (dc *DataChannel) SetOnError(cb func(err error)) {
	dc.onError = cb
}

// Send sends data on the channel.
func (dc *DataChannel) Send(data []byte) error {
	if dc.handle == 0 {
		return errors.New("data channel not initialized")
	}
	return ffi.DataChannelSend(dc.handle, data, true) // binary mode by default
}

// SendText sends text data on the channel.
func (dc *DataChannel) SendText(text string) error {
	if dc.handle == 0 {
		return errors.New("data channel not initialized")
	}
	return ffi.DataChannelSend(dc.handle, []byte(text), false)
}

// Close closes the data channel.
func (dc *DataChannel) Close() error {
	if dc.handle == 0 {
		return nil
	}
	ffi.UnregisterDataChannelCallbacks(dc.handle)
	ffi.DataChannelClose(dc.handle)
	return nil
}

// ffiConfigData holds FFI config and keeps allocations alive
type ffiConfigData struct {
	config     *ffi.PeerConnectionConfig
	iceServers []ffi.ICEServerConfig
	urlArrays  [][]uintptr
	urlStrings [][]byte
	strings    [][]byte
}

// buildFFIConfig converts a Configuration to FFI-compatible format.
// Returns config data that must be kept alive during the FFI call.
func buildFFIConfig(config *webrtc.Configuration) (*ffiConfigData, error) {
	data := &ffiConfigData{
		config: &ffi.PeerConnectionConfig{
			ICECandidatePoolSize: int32(config.ICECandidatePoolSize),
		},
	}

	// Convert ICE servers
	if len(config.ICEServers) > 0 {
		data.iceServers = make([]ffi.ICEServerConfig, len(config.ICEServers))
		data.urlArrays = make([][]uintptr, len(config.ICEServers))
		data.urlStrings = make([][]byte, 0)

		for i, server := range config.ICEServers {
			if len(server.URLs) > 0 {
				urlPtrs := make([]uintptr, len(server.URLs))
				for j, url := range server.URLs {
					urlStr := ffi.CString(url)
					data.urlStrings = append(data.urlStrings, urlStr)
					urlPtrs[j] = ffi.ByteSlicePtr(urlStr)
				}
				data.urlArrays[i] = urlPtrs
				data.iceServers[i].URLs = ffi.UintptrSlicePtr(urlPtrs)
				data.iceServers[i].URLCount = int32(len(server.URLs))
			}
			if server.Username != "" {
				usernameStr := ffi.CString(server.Username)
				data.strings = append(data.strings, usernameStr)
				data.iceServers[i].Username = &usernameStr[0]
			}
			if credential := credentialString(server); credential != "" {
				credStr := ffi.CString(credential)
				data.strings = append(data.strings, credStr)
				data.iceServers[i].Credential = &credStr[0]
			}
		}
		data.config.ICEServers = data.iceServers[0].Ptr()
		data.config.ICEServerCount = int32(len(data.iceServers))
	}

	// Set policies
	bundlePolicy, err := bundlePolicyString(config.BundlePolicy)
	if err != nil {
		return nil, err
	}
	bundleStr := ffi.CString(bundlePolicy)
	data.strings = append(data.strings, bundleStr)
	data.config.BundlePolicy = &bundleStr[0]

	rtcpMuxPolicy, err := rtcpMuxPolicyString(config.RTCPMuxPolicy)
	if err != nil {
		return nil, err
	}
	rtcpStr := ffi.CString(rtcpMuxPolicy)
	data.strings = append(data.strings, rtcpStr)
	data.config.RTCPMuxPolicy = &rtcpStr[0]

	iceTransportPolicy, err := iceTransportPolicyString(config.ICETransportPolicy)
	if err != nil {
		return nil, err
	}
	iceStr := ffi.CString(iceTransportPolicy)
	data.strings = append(data.strings, iceStr)
	data.config.ICETransportPolicy = &iceStr[0]

	sdpSemantics, err := sdpSemanticsString(config.SDPSemantics)
	if err != nil {
		return nil, err
	}
	sdpStr := ffi.CString(sdpSemantics)
	data.strings = append(data.strings, sdpStr)
	data.config.SDPSemantics = &sdpStr[0]

	return data, nil
}

// NewPeerConnection creates a new libwebrtc-backed PeerConnection.
func NewPeerConnection(config webrtc.Configuration) (*PeerConnection, error) {
	if err := ffi.LoadLibrary(); err != nil {
		return nil, err
	}
	if err := validateConfiguration(config); err != nil {
		return nil, err
	}

	pc := &PeerConnection{
		config:       config,
		senders:      make([]*RTPSender, 0),
		receivers:    make([]*RTPReceiver, 0),
		transceivers: make([]*RTPTransceiver, 0),
		localTracks:  make([]*Track, 0),
	}

	pc.signalingState.Store(webrtc.SignalingStateStable)
	pc.iceConnectionState.Store(webrtc.ICEConnectionStateNew)
	pc.iceGatheringState.Store(webrtc.ICEGathererStateNew)
	pc.connectionState.Store(webrtc.PeerConnectionStateNew)

	// Build FFI config - keep data alive during FFI call
	configData, err := buildFFIConfig(&config)
	if err != nil {
		return nil, err
	}
	handle, err := ffi.CreatePeerConnection(configData.config)
	// Ensure configData is kept alive until after FFI call completes
	_ = configData
	if err != nil {
		return nil, fmt.Errorf("create peer connection: %w", err)
	}
	pc.handle = handle

	// Set up connection state change callback
	ffi.PeerConnectionSetOnConnectionStateChange(handle, func(state int) {
		if pc.closed.Load() {
			return // Ignore if closed
		}
		newState := peerConnectionStateFromFFI(state)
		pc.connectionState.Store(newState)
		pc.callbackMu.RLock()
		cb := pc.onConnectionStateChange
		pc.callbackMu.RUnlock()
		if cb != nil {
			cb(newState)
		}
	})

	// Set up ICE candidate callback
	ffi.PeerConnectionSetOnICECandidate(handle, func(candidate, sdpMid string, sdpMLineIndex int) {
		if pc.closed.Load() {
			return // Ignore if closed
		}
		pc.callbackMu.RLock()
		cb := pc.onICECandidate
		pc.callbackMu.RUnlock()
		if cb != nil {
			cb(iceCandidateInitFromParts(candidate, sdpMid, sdpMLineIndex))
		}
	})

	// Set up track callback
	ffi.PeerConnectionSetOnTrack(handle, func(trackHandle, receiverHandle uintptr, streams string) {
		if pc.closed.Load() {
			return // Ignore if closed
		}
		pc.callbackMu.RLock()
		cb := pc.onTrack
		pc.callbackMu.RUnlock()
		if cb != nil {
			// Create track wrapper
			kind := ffi.TrackKind(trackHandle)
			trackID := ffi.TrackID(trackHandle)

			track := &Track{
				handle: trackHandle,
				id:     trackID,
				kind:   kind,
				pc:     pc,
			}
			track.enabled.Store(true)

			receiver := &RTPReceiver{
				handle: receiverHandle,
				track:  track,
				pc:     pc,
			}

			pc.mu.Lock()
			pc.receivers = append(pc.receivers, receiver)
			pc.mu.Unlock()

			cb(track, receiver, firstStreamID(splitStreamIDs(streams)))
		}
	})

	// Set up data channel callback
	ffi.PeerConnectionSetOnDataChannel(handle, func(dcHandle uintptr) {
		if pc.closed.Load() {
			return // Ignore if closed
		}
		pc.callbackMu.RLock()
		cb := pc.onDataChannel
		pc.callbackMu.RUnlock()
		if cb != nil {
			label := ffi.DataChannelLabel(dcHandle)
			dc := &DataChannel{
				handle: dcHandle,
				label:  label,
				pc:     pc,
			}
			cb(dc)
		}
	})

	// Set up signaling state change callback
	ffi.PeerConnectionSetOnSignalingStateChange(handle, func(state int) {
		if pc.closed.Load() {
			return // Ignore if closed
		}
		newState := signalingStateFromFFI(state)
		pc.signalingState.Store(newState)
		pc.callbackMu.RLock()
		cb := pc.onSignalingStateChange
		pc.callbackMu.RUnlock()
		if cb != nil {
			cb(newState)
		}
	})

	// Set up ICE connection state change callback
	ffi.PeerConnectionSetOnICEConnectionStateChange(handle, func(state int) {
		if pc.closed.Load() {
			return // Ignore if closed
		}
		newState := iceConnectionStateFromFFI(state)
		pc.iceConnectionState.Store(newState)
		pc.callbackMu.RLock()
		cb := pc.onICEConnectionStateChange
		pc.callbackMu.RUnlock()
		if cb != nil {
			cb(newState)
		}
	})

	// Set up ICE gathering state change callback
	ffi.PeerConnectionSetOnICEGatheringStateChange(handle, func(state int) {
		if pc.closed.Load() {
			return // Ignore if closed
		}
		newState := iceGathererStateFromFFI(state)
		pc.iceGatheringState.Store(newState)
		pc.callbackMu.RLock()
		cb := pc.onICEGatheringStateChange
		pc.callbackMu.RUnlock()
		if cb != nil {
			cb(newState)
		}
	})

	// Set up negotiation needed callback
	ffi.PeerConnectionSetOnNegotiationNeeded(handle, func() {
		if pc.closed.Load() {
			return // Ignore if closed
		}
		pc.callbackMu.RLock()
		cb := pc.onNegotiationNeeded
		pc.callbackMu.RUnlock()
		if cb != nil {
			cb()
		}
	})

	return pc, nil
}

// CreateOffer creates an SDP offer.
func (pc *PeerConnection) CreateOffer(options *webrtc.OfferOptions) (webrtc.SessionDescription, error) {
	if pc.closed.Load() {
		return webrtc.SessionDescription{}, ErrPeerConnectionClosed
	}

	// Note: Don't hold lock during FFI call - it can trigger callbacks that need the lock.
	// Allocate buffer for SDP output
	sdpBuf := make([]byte, maxSDPSize)
	sdpLen, err := ffi.PeerConnectionCreateOffer(pc.handle, sdpBuf, options)
	if err != nil {
		return webrtc.SessionDescription{}, ErrCreateOfferFailed
	}

	return webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  string(sdpBuf[:sdpLen]),
	}, nil
}

// CreateAnswer creates an SDP answer.
func (pc *PeerConnection) CreateAnswer(options *webrtc.AnswerOptions) (webrtc.SessionDescription, error) {
	if pc.closed.Load() {
		return webrtc.SessionDescription{}, ErrPeerConnectionClosed
	}

	// Note: Don't hold lock during FFI call - it can trigger callbacks that need the lock.
	// Allocate buffer for SDP output
	sdpBuf := make([]byte, maxSDPSize)
	sdpLen, err := ffi.PeerConnectionCreateAnswer(pc.handle, sdpBuf, options)
	if err != nil {
		return webrtc.SessionDescription{}, ErrCreateAnswerFailed
	}

	return webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  string(sdpBuf[:sdpLen]),
	}, nil
}

// SetLocalDescription sets the local description.
func (pc *PeerConnection) SetLocalDescription(desc webrtc.SessionDescription) error {
	if pc.closed.Load() {
		return ErrPeerConnectionClosed
	}
	if pc.handle == 0 {
		return ErrPeerConnectionClosed
	}
	sdpType, err := sdpTypeToFFI(desc.Type)
	if err != nil {
		return err
	}

	// Note: Don't hold lock during FFI call - it can trigger callbacks that need the lock
	if err := ffi.PeerConnectionSetLocalDescription(pc.handle, sdpType, desc.SDP); err != nil {
		return ErrSetDescriptionFailed
	}

	pc.mu.Lock()
	pc.localDescription = &webrtc.SessionDescription{
		Type: desc.Type,
		SDP:  desc.SDP,
	}
	pc.mu.Unlock()
	return nil
}

// SetRemoteDescription sets the remote description.
func (pc *PeerConnection) SetRemoteDescription(desc webrtc.SessionDescription) error {
	if pc.closed.Load() {
		return ErrPeerConnectionClosed
	}
	if pc.handle == 0 {
		return ErrPeerConnectionClosed
	}
	sdpType, err := sdpTypeToFFI(desc.Type)
	if err != nil {
		return err
	}

	pc.mu.Lock()
	prevRemoteDescription := pc.remoteDescription
	pc.remoteDescription = &webrtc.SessionDescription{
		Type: desc.Type,
		SDP:  desc.SDP,
	}
	pc.mu.Unlock()

	// Note: Don't hold lock during FFI call - it can trigger callbacks that need the lock.
	// We publish the description first so callbacks fired during SetRemoteDescription
	// can still recover track/stream relationships from SDP. Roll back on failure.
	if err := ffi.PeerConnectionSetRemoteDescription(pc.handle, sdpType, desc.SDP); err != nil {
		pc.mu.Lock()
		if pc.remoteDescription != nil && pc.remoteDescription.Type == desc.Type && pc.remoteDescription.SDP == desc.SDP {
			pc.remoteDescription = prevRemoteDescription
		}
		pc.mu.Unlock()
		return ErrSetDescriptionFailed
	}

	return nil
}

// AddICECandidate adds an ICE candidate.
func (pc *PeerConnection) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	if pc.closed.Load() {
		return ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.handle == 0 {
		return ErrPeerConnectionClosed
	}

	candidateValue, sdpMid, sdpMLineIndex := candidateParts(candidate)
	if err := ffi.PeerConnectionAddICECandidate(pc.handle, candidateValue, sdpMid, sdpMLineIndex); err != nil {
		return ErrAddICECandidateFailed
	}

	return nil
}

// LocalDescription returns the local description.
func (pc *PeerConnection) LocalDescription() *webrtc.SessionDescription {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.localDescription
}

// RemoteDescription returns the remote description.
func (pc *PeerConnection) RemoteDescription() *webrtc.SessionDescription {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.remoteDescription
}

// SignalingState returns the signaling state.
func (pc *PeerConnection) SignalingState() SignalingState {
	return pc.signalingState.Load().(SignalingState)
}

// ICEConnectionState returns the ICE connection state.
func (pc *PeerConnection) ICEConnectionState() ICEConnectionState {
	return pc.iceConnectionState.Load().(ICEConnectionState)
}

// ICEGatheringState returns the ICE gathering state.
func (pc *PeerConnection) ICEGatheringState() ICEGatheringState {
	return pc.iceGatheringState.Load().(ICEGatheringState)
}

// ConnectionState returns the overall connection state.
func (pc *PeerConnection) ConnectionState() PeerConnectionState {
	return pc.connectionState.Load().(PeerConnectionState)
}

// AddTrack adds a track to the connection with one explicit MediaStream ID.
func (pc *PeerConnection) AddTrack(track *Track, streamID string) (*RTPSender, error) {
	if track == nil {
		return nil, ErrNilTrack
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return nil, ErrInvalidStreamID
	}
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}
	if pc.handle == 0 {
		return nil, ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	var senderHandle uintptr

	if track.kind == "video" {
		// Create video track source for frame injection
		sourceHandle := ffi.VideoTrackSourceCreate(pc.handle, track.width, track.height)
		if sourceHandle == 0 {
			return nil, errors.New("failed to create video track source")
		}
		track.sourceHandle = sourceHandle

		// Add video track from source
		senderHandle = ffi.PeerConnectionAddVideoTrackFromSource(pc.handle, sourceHandle, track.id, streamID)
	} else if track.kind == "audio" {
		// Create audio track source for frame injection
		sourceHandle := ffi.AudioTrackSourceCreate(pc.handle, track.sampleRate, track.channels)
		if sourceHandle == 0 {
			return nil, errors.New("failed to create audio track source")
		}
		track.sourceHandle = sourceHandle

		// Add audio track from source
		senderHandle = ffi.PeerConnectionAddAudioTrackFromSource(pc.handle, sourceHandle, track.id, streamID)
	} else {
		return nil, errors.New("unknown track kind")
	}

	if senderHandle == 0 {
		// Cleanup source on failure
		if track.kind == "video" {
			ffi.VideoTrackSourceDestroy(track.sourceHandle)
		} else {
			ffi.AudioTrackSourceDestroy(track.sourceHandle)
		}
		track.sourceHandle = 0
		return nil, errors.New("failed to add track")
	}

	sender := &RTPSender{
		handle:   senderHandle,
		track:    track,
		pc:       pc,
		id:       track.id,
		streamID: streamID,
	}

	pc.senders = append(pc.senders, sender)
	pc.localTracks = append(pc.localTracks, track)

	// Trigger negotiation needed
	pc.callbackMu.RLock()
	cb := pc.onNegotiationNeeded
	pc.callbackMu.RUnlock()
	if cb != nil {
		go cb()
	}

	return sender, nil
}

// RemoveTrack removes a track from the connection.
func (pc *PeerConnection) RemoveTrack(sender *RTPSender) error {
	if sender == nil {
		return ErrNilSender
	}
	if pc.closed.Load() {
		return ErrPeerConnectionClosed
	}
	if pc.handle == 0 {
		return ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	trackToRemove := sender.track

	// Call FFI to remove track
	if sender.handle != 0 {
		if err := ffi.PeerConnectionRemoveTrack(pc.handle, sender.handle); err != nil {
			return err
		}
	}

	for i, s := range pc.senders {
		if s == sender {
			pc.senders = append(pc.senders[:i], pc.senders[i+1:]...)
			break
		}
	}

	for i, t := range pc.localTracks {
		if t == trackToRemove {
			pc.localTracks = append(pc.localTracks[:i], pc.localTracks[i+1:]...)
			break
		}
	}

	if trackToRemove != nil && trackToRemove.sourceHandle != 0 {
		if trackToRemove.kind == "video" {
			ffi.VideoTrackSourceDestroy(trackToRemove.sourceHandle)
		} else if trackToRemove.kind == "audio" {
			ffi.AudioTrackSourceDestroy(trackToRemove.sourceHandle)
		}
		trackToRemove.sourceHandle = 0
	}

	return nil
}

// AddTransceiver adds a transceiver.
func (pc *PeerConnection) AddTransceiver(kind string, init *webrtc.RTPTransceiverInit) (*RTPTransceiver, error) {
	codecType, err := codecTypeFromString(kind)
	if err != nil {
		return nil, err
	}
	return pc.addTransceiver(codecType, kind, init)
}

// AddTransceiverFromKind adds a transceiver using Pion's RTP codec kind enum.
func (pc *PeerConnection) AddTransceiverFromKind(kind webrtc.RTPCodecType, init ...webrtc.RTPTransceiverInit) (*RTPTransceiver, error) {
	var config *webrtc.RTPTransceiverInit
	if len(init) > 1 {
		return nil, errors.New("only one RTPTransceiverInit is supported")
	}
	if len(init) == 1 {
		config = &init[0]
	}
	_, kindString, err := mediaKindFromCodecType(kind)
	if err != nil {
		return nil, err
	}
	return pc.addTransceiver(kind, kindString, config)
}

func (pc *PeerConnection) addTransceiver(kind webrtc.RTPCodecType, kindString string, init *webrtc.RTPTransceiverInit) (*RTPTransceiver, error) {
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Determine media kind
	mediaKind, _, err := mediaKindFromCodecType(kind)
	if err != nil {
		return nil, err
	}

	// Determine direction
	direction := webrtc.RTPTransceiverDirectionSendrecv
	if init != nil {
		direction = init.Direction
		for _, encoding := range init.SendEncodings {
			if validationErr := validateEncodingParameters(encoding); validationErr != nil {
				return nil, validationErr
			}
		}
	}
	ffiDirection, err := transceiverDirectionToFFI(direction)
	if err != nil {
		return nil, err
	}
	sendParams := ffiSendParametersFromWebRTC(webrtc.RTPSendParameters{Encodings: nil})
	if init != nil && len(init.SendEncodings) > 0 {
		sendParams = ffiSendParametersFromWebRTC(webrtc.RTPSendParameters{Encodings: init.SendEncodings})
	}

	// Call FFI to add transceiver
	handle := ffi.PeerConnectionAddTransceiver(pc.handle, mediaKind, ffiDirection, sendParams)
	if handle == 0 {
		return nil, errors.New("failed to add transceiver")
	}

	transceiver := &RTPTransceiver{
		handle:    handle,
		pc:        pc,
		direction: direction,
		kind:      kindString,
	}

	// Get sender and receiver handles
	senderHandle := ffi.TransceiverGetSender(handle)
	receiverHandle := ffi.TransceiverGetReceiver(handle)

	if senderHandle != 0 {
		transceiver.sender = &RTPSender{handle: senderHandle, pc: pc}
	}
	if receiverHandle != 0 {
		transceiver.receiver = &RTPReceiver{handle: receiverHandle, pc: pc}
	}

	pc.transceivers = append(pc.transceivers, transceiver)

	return transceiver, nil
}

// GetTransceivers returns all transceivers.
// This queries libwebrtc for the current list, which includes transceivers
// created implicitly by AddTrack().
func (pc *PeerConnection) GetTransceivers() []*RTPTransceiver {
	if pc.closed.Load() || pc.handle == 0 {
		return nil
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Query libwebrtc for actual transceivers
	const maxTransceivers = 16
	handles, err := ffi.PeerConnectionGetTransceivers(pc.handle, maxTransceivers)
	if err != nil {
		return nil
	}

	result := make([]*RTPTransceiver, 0, len(handles))
	for _, handle := range handles {
		if handle == 0 {
			continue
		}

		// Check if we already have this transceiver cached
		var existing *RTPTransceiver
		for _, t := range pc.transceivers {
			if t.handle == handle {
				existing = t
				break
			}
		}

		if existing != nil {
			result = append(result, existing)
		} else {
			// Create new wrapper for transceiver we haven't seen
			transceiver := &RTPTransceiver{
				handle: handle,
				pc:     pc,
				kind:   inferTransceiverKind(handle),
			}

			// Get sender and receiver handles
			senderHandle := ffi.TransceiverGetSender(handle)
			receiverHandle := ffi.TransceiverGetReceiver(handle)

			if senderHandle != 0 {
				transceiver.sender = &RTPSender{handle: senderHandle, pc: pc}
			}
			if receiverHandle != 0 {
				transceiver.receiver = &RTPReceiver{handle: receiverHandle, pc: pc}
			}

			// Cache for future calls
			pc.transceivers = append(pc.transceivers, transceiver)
			result = append(result, transceiver)
		}
	}

	return result
}

// GetSenders returns all senders.
func (pc *PeerConnection) GetSenders() []*RTPSender {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	result := make([]*RTPSender, len(pc.senders))
	copy(result, pc.senders)
	return result
}

// GetReceivers returns all receivers.
func (pc *PeerConnection) GetReceivers() []*RTPReceiver {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	result := make([]*RTPReceiver, len(pc.receivers))
	copy(result, pc.receivers)
	return result
}

// CreateDataChannel creates a data channel.
func (pc *PeerConnection) CreateDataChannel(label string, options *webrtc.DataChannelInit) (*DataChannel, error) {
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Set defaults
	ordered := true
	maxPacketLifeTime := -1 // -1 means unset/unlimited
	maxRetransmits := -1    // -1 means unset/unlimited
	protocol := ""
	negotiated := false
	id := -1

	if options != nil {
		if options.Ordered != nil {
			ordered = *options.Ordered
		}
		if options.MaxPacketLifeTime != nil {
			maxPacketLifeTime = int(*options.MaxPacketLifeTime)
		}
		if options.MaxRetransmits != nil {
			maxRetransmits = int(*options.MaxRetransmits)
		}
		if options.Protocol != nil {
			protocol = *options.Protocol
		}
		if options.Negotiated != nil {
			negotiated = *options.Negotiated
		}
		if options.ID != nil {
			id = int(*options.ID)
		}
	}

	if maxPacketLifeTime >= 0 && maxRetransmits >= 0 {
		return nil, errors.New("max packet lifetime and max retransmits are mutually exclusive")
	}
	if negotiated && id < 0 {
		return nil, errors.New("negotiated data channel requires explicit ID")
	}

	// Call FFI to create data channel
	handle := ffi.PeerConnectionCreateDataChannel(
		pc.handle,
		label,
		ordered,
		maxPacketLifeTime,
		maxRetransmits,
		protocol,
		negotiated,
		id,
	)
	if handle == 0 {
		return nil, errors.New("failed to create data channel")
	}

	dc := &DataChannel{
		handle: handle,
		label:  label,
		pc:     pc,
	}
	if id >= 0 {
		dc.id = uint16(id)
	}

	return dc, nil
}

func splitStreamIDs(streams string) []string {
	if streams == "" {
		return nil
	}

	rawStreamIDs := strings.Split(streams, ",")
	streamIDs := make([]string, 0, len(rawStreamIDs))
	for _, streamID := range rawStreamIDs {
		streamID = strings.TrimSpace(streamID)
		if streamID != "" {
			streamIDs = append(streamIDs, streamID)
		}
	}
	if len(streamIDs) == 0 {
		return nil
	}
	return streamIDs
}

func firstStreamID(streamIDs []string) string {
	if len(streamIDs) == 0 {
		return ""
	}
	return streamIDs[0]
}

// Close closes the peer connection.
func (pc *PeerConnection) Close() error {
	if !pc.closed.CompareAndSwap(false, true) {
		return nil
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.handle != 0 {
		// Unregister all callbacks BEFORE destroying the handle to prevent
		// callbacks firing on destroyed state (use-after-free prevention)
		ffi.UnregisterConnectionStateCallback(pc.handle)
		ffi.UnregisterOnTrackCallback(pc.handle)
		ffi.UnregisterOnICECandidateCallback(pc.handle)
		ffi.UnregisterOnDataChannelCallback(pc.handle)
		ffi.UnregisterSignalingStateCallback(pc.handle)
		ffi.UnregisterICEConnectionStateCallback(pc.handle)
		ffi.UnregisterICEGatheringStateCallback(pc.handle)
		ffi.UnregisterNegotiationNeededCallback(pc.handle)
		ffi.PeerConnectionClose(pc.handle)
		ffi.PeerConnectionDestroy(pc.handle)
		pc.handle = 0
	}

	pc.signalingState.Store(SignalingStateClosed)
	pc.connectionState.Store(PeerConnectionStateClosed)
	pc.iceConnectionState.Store(ICEConnectionStateClosed)

	return nil
}

// --- Track creation helpers ---

// CreateVideoTrack creates a video track for this peer connection.
// Width and height specify the video dimensions for the track source.
func (pc *PeerConnection) CreateVideoTrack(id string, width, height int) (*Track, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("invalid video dimensions")
	}

	track := &Track{
		id:     id,
		kind:   "video",
		label:  "libwebrtc-video-" + id,
		pc:     pc,
		width:  width,
		height: height,
	}
	track.enabled.Store(true)
	track.muted.Store(false)

	return track, nil
}

// CreateAudioTrack creates an audio track for this peer connection.
// Uses 48kHz stereo by default (can be overridden with CreateAudioTrackWithOptions).
func (pc *PeerConnection) CreateAudioTrack(id string) (*Track, error) {
	return pc.CreateAudioTrackWithOptions(id, 48000, 2)
}

// CreateAudioTrackWithOptions creates an audio track with specific sample rate and channels.
func (pc *PeerConnection) CreateAudioTrackWithOptions(id string, sampleRate, channels int) (*Track, error) {
	if sampleRate <= 0 || channels <= 0 || channels > 2 {
		return nil, errors.New("invalid audio parameters")
	}

	track := &Track{
		id:         id,
		kind:       "audio",
		label:      "libwebrtc-audio-" + id,
		pc:         pc,
		sampleRate: sampleRate,
		channels:   channels,
	}
	track.enabled.Store(true)
	track.muted.Store(false)

	return track, nil
}

// --- Stats API ---

// GetStats returns the current stats report.
func (pc *PeerConnection) GetStats() (webrtc.StatsReport, error) {
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}

	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if pc.handle == 0 {
		return nil, errors.New("peer connection not initialized")
	}

	data, err := ffi.PeerConnectionGetStatsJSON(pc.handle)
	if err != nil {
		return nil, err
	}
	return statsReportFromJSON(data)
}

// RestartICE triggers an ICE restart on the next offer.
func (pc *PeerConnection) RestartICE() error {
	if pc.closed.Load() {
		return ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.handle == 0 {
		return errors.New("peer connection not initialized")
	}

	return ffi.PeerConnectionRestartICE(pc.handle)
}
