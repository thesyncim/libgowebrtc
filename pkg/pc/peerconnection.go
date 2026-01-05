// Package pc provides a browser-like PeerConnection API backed by libwebrtc.
// This wraps libwebrtc's native PeerConnection, not Pion's.
package pc

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
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
)

// Constants
const (
	maxSDPSize = 64 * 1024 // 64KB should be sufficient for SDP
)

// SignalingState represents the signaling state.
type SignalingState int

const (
	SignalingStateStable SignalingState = iota
	SignalingStateHaveLocalOffer
	SignalingStateHaveRemoteOffer
	SignalingStateHaveLocalPranswer
	SignalingStateHaveRemotePranswer
	SignalingStateClosed
)

func (s SignalingState) String() string {
	switch s {
	case SignalingStateStable:
		return "stable"
	case SignalingStateHaveLocalOffer:
		return "have-local-offer"
	case SignalingStateHaveRemoteOffer:
		return "have-remote-offer"
	case SignalingStateHaveLocalPranswer:
		return "have-local-pranswer"
	case SignalingStateHaveRemotePranswer:
		return "have-remote-pranswer"
	case SignalingStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// ICEConnectionState represents the ICE connection state.
type ICEConnectionState int

const (
	ICEConnectionStateNew ICEConnectionState = iota
	ICEConnectionStateChecking
	ICEConnectionStateConnected
	ICEConnectionStateCompleted
	ICEConnectionStateDisconnected
	ICEConnectionStateFailed
	ICEConnectionStateClosed
)

func (s ICEConnectionState) String() string {
	switch s {
	case ICEConnectionStateNew:
		return "new"
	case ICEConnectionStateChecking:
		return "checking"
	case ICEConnectionStateConnected:
		return "connected"
	case ICEConnectionStateCompleted:
		return "completed"
	case ICEConnectionStateDisconnected:
		return "disconnected"
	case ICEConnectionStateFailed:
		return "failed"
	case ICEConnectionStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// ICEGatheringState represents the ICE gathering state.
type ICEGatheringState int

const (
	ICEGatheringStateNew ICEGatheringState = iota
	ICEGatheringStateGathering
	ICEGatheringStateComplete
)

func (s ICEGatheringState) String() string {
	switch s {
	case ICEGatheringStateNew:
		return "new"
	case ICEGatheringStateGathering:
		return "gathering"
	case ICEGatheringStateComplete:
		return "complete"
	default:
		return "unknown"
	}
}

// PeerConnectionState represents the overall connection state.
type PeerConnectionState int

const (
	PeerConnectionStateNew PeerConnectionState = iota
	PeerConnectionStateConnecting
	PeerConnectionStateConnected
	PeerConnectionStateDisconnected
	PeerConnectionStateFailed
	PeerConnectionStateClosed
)

func (s PeerConnectionState) String() string {
	switch s {
	case PeerConnectionStateNew:
		return "new"
	case PeerConnectionStateConnecting:
		return "connecting"
	case PeerConnectionStateConnected:
		return "connected"
	case PeerConnectionStateDisconnected:
		return "disconnected"
	case PeerConnectionStateFailed:
		return "failed"
	case PeerConnectionStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// SDPType represents the type of session description.
type SDPType int

const (
	SDPTypeOffer SDPType = iota
	SDPTypePranswer
	SDPTypeAnswer
	SDPTypeRollback
)

func (t SDPType) String() string {
	switch t {
	case SDPTypeOffer:
		return "offer"
	case SDPTypePranswer:
		return "pranswer"
	case SDPTypeAnswer:
		return "answer"
	case SDPTypeRollback:
		return "rollback"
	default:
		return "unknown"
	}
}

// SessionDescription represents an SDP session description.
type SessionDescription struct {
	Type SDPType
	SDP  string
}

// ICECandidate represents an ICE candidate.
type ICECandidate struct {
	Candidate        string
	SDPMid           string
	SDPMLineIndex    uint16
	UsernameFragment string
}

// ICEServer represents an ICE server configuration.
type ICEServer struct {
	URLs       []string
	Username   string
	Credential string
}

// Configuration for PeerConnection.
type Configuration struct {
	ICEServers           []ICEServer
	ICETransportPolicy   string // "all" or "relay"
	BundlePolicy         string // "balanced", "max-compat", "max-bundle"
	RTCPMuxPolicy        string // "require" or "negotiate"
	PeerIdentity         string
	SDPSemantics         string // "unified-plan" or "plan-b"
	ICECandidatePoolSize int
}

// DefaultConfiguration returns a default configuration.
func DefaultConfiguration() Configuration {
	return Configuration{
		ICEServers: []ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
		BundlePolicy:  "max-bundle",
		RTCPMuxPolicy: "require",
		SDPSemantics:  "unified-plan",
	}
}

// OfferOptions for createOffer.
type OfferOptions struct {
	ICERestart             bool
	VoiceActivityDetection bool
}

// AnswerOptions for createAnswer.
type AnswerOptions struct {
	VoiceActivityDetection bool
}

// RTPSender represents an RTP sender.
type RTPSender struct {
	handle uintptr
	track  *Track
	pc     *PeerConnection
	id     string
	mu     sync.RWMutex
}

// Track returns the sender's track.
func (s *RTPSender) Track() *Track {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.track
}

// ReplaceTrack replaces the sender's track.
func (s *RTPSender) ReplaceTrack(t *Track) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// TODO: Call FFI to replace track
	s.track = t
	return nil
}

// SetParameters sets encoding parameters.
func (s *RTPSender) SetParameters(params RTPSendParameters) error {
	// TODO: Call FFI
	return nil
}

// GetParameters gets current parameters.
func (s *RTPSender) GetParameters() RTPSendParameters {
	// TODO: Call FFI
	return RTPSendParameters{}
}

// RTPReceiver represents an RTP receiver.
type RTPReceiver struct {
	handle uintptr
	track  *Track
	pc     *PeerConnection
	id     string
}

// Track returns the receiver's track.
func (r *RTPReceiver) Track() *Track {
	return r.track
}

// RTPTransceiver represents an RTP transceiver.
type RTPTransceiver struct {
	handle    uintptr
	sender    *RTPSender
	receiver  *RTPReceiver
	direction TransceiverDirection
	mid       string
	pc        *PeerConnection
}

// TransceiverDirection represents the transceiver direction.
type TransceiverDirection int

const (
	TransceiverDirectionSendRecv TransceiverDirection = iota
	TransceiverDirectionSendOnly
	TransceiverDirectionRecvOnly
	TransceiverDirectionInactive
)

func (d TransceiverDirection) String() string {
	switch d {
	case TransceiverDirectionSendRecv:
		return "sendrecv"
	case TransceiverDirectionSendOnly:
		return "sendonly"
	case TransceiverDirectionRecvOnly:
		return "recvonly"
	case TransceiverDirectionInactive:
		return "inactive"
	default:
		return "unknown"
	}
}

// Sender returns the transceiver's sender.
func (t *RTPTransceiver) Sender() *RTPSender { return t.sender }

// Receiver returns the transceiver's receiver.
func (t *RTPTransceiver) Receiver() *RTPReceiver { return t.receiver }

// Direction returns current direction.
func (t *RTPTransceiver) Direction() TransceiverDirection { return t.direction }

// SetDirection sets the direction.
func (t *RTPTransceiver) SetDirection(d TransceiverDirection) error {
	// TODO: Call FFI
	t.direction = d
	return nil
}

// Mid returns the transceiver's mid.
func (t *RTPTransceiver) Mid() string { return t.mid }

// Stop stops the transceiver.
func (t *RTPTransceiver) Stop() error {
	// TODO: Call FFI
	return nil
}

// RTPSendParameters for sender configuration.
type RTPSendParameters struct {
	Encodings []RTPEncodingParameters
}

// RTPEncodingParameters for per-encoding configuration.
type RTPEncodingParameters struct {
	RID                   string  // RTP stream ID (for simulcast)
	Active                bool    // Whether this encoding is active
	MaxBitrate            uint32  // Max bitrate in bps
	MaxFramerate          float64 // Max framerate
	ScaleResolutionDownBy float64 // Scale factor for resolution
	ScalabilityMode       string  // SVC mode string (e.g., "L3T3_KEY")
}

// Track represents a media track (can be local or remote).
type Track struct {
	handle  uintptr
	id      string
	kind    string // "video" or "audio"
	label   string
	codec   codec.Type
	enabled atomic.Bool
	muted   atomic.Bool
	pc      *PeerConnection

	// Video/audio track source for frame injection
	sourceHandle uintptr
	width        int
	height       int
	sampleRate   int
	channels     int

	// For writing frames
	mu sync.Mutex
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

// WriteVideoFrame writes a video frame to the track.
func (t *Track) WriteVideoFrame(f *frame.VideoFrame) error {
	if t.kind != "video" {
		return errors.New("not a video track")
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
	return ffi.VideoTrackSourcePushFrame(
		t.sourceHandle,
		f.Data[0], // Y plane
		f.Data[1], // U plane
		f.Data[2], // V plane
		f.Stride[0],
		f.Stride[1],
		f.Stride[2],
		int64(f.PTS)*1000, // Convert to microseconds
	)
}

// WriteAudioFrame writes an audio frame to the track.
func (t *Track) WriteAudioFrame(f *frame.AudioFrame) error {
	if t.kind != "audio" {
		return errors.New("not an audio track")
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
	return ffi.AudioTrackSourcePushFrame(
		t.sourceHandle,
		samples,
		int64(f.PTS)*1000, // Convert to microseconds
	)
}

// PeerConnection wraps libwebrtc's native PeerConnection.
type PeerConnection struct {
	handle uintptr
	config Configuration

	signalingState     atomic.Value
	iceConnectionState atomic.Value
	iceGatheringState  atomic.Value
	connectionState    atomic.Value

	localDescription  *SessionDescription
	remoteDescription *SessionDescription

	senders      []*RTPSender
	receivers    []*RTPReceiver
	transceivers []*RTPTransceiver
	localTracks  []*Track

	// Event handlers (browser-like callbacks)
	OnICECandidate             func(candidate *ICECandidate)
	OnICEConnectionStateChange func(state ICEConnectionState)
	OnICEGatheringStateChange  func(state ICEGatheringState)
	OnSignalingStateChange     func(state SignalingState)
	OnConnectionStateChange    func(state PeerConnectionState)
	OnTrack                    func(track *Track, receiver *RTPReceiver, streams []string)
	OnNegotiationNeeded        func()
	OnDataChannel              func(dc *DataChannel)

	mu     sync.RWMutex
	closed atomic.Bool
}

// DataChannel represents a data channel.
type DataChannel struct {
	handle uintptr
	label  string
	id     uint16
	pc     *PeerConnection

	OnOpen    func()
	OnClose   func()
	OnMessage func(data []byte)
	OnError   func(err error)
}

// Label returns the data channel label.
func (dc *DataChannel) Label() string { return dc.label }

// ID returns the data channel ID.
func (dc *DataChannel) ID() uint16 { return dc.id }

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
	ffi.DataChannelClose(dc.handle)
	return nil
}

// buildFFIConfig converts a Configuration to FFI-compatible format.
// Note: This allocates memory that must be kept alive during the FFI call.
func buildFFIConfig(config *Configuration) *ffi.PeerConnectionConfig {
	ffiConfig := &ffi.PeerConnectionConfig{
		ICECandidatePoolSize: int32(config.ICECandidatePoolSize),
	}

	// Convert ICE servers
	if len(config.ICEServers) > 0 {
		iceServers := make([]ffi.ICEServerConfig, len(config.ICEServers))
		for i, server := range config.ICEServers {
			if len(server.URLs) > 0 {
				urlPtrs := make([]uintptr, len(server.URLs))
				for j, url := range server.URLs {
					urlPtrs[j] = ffi.ByteSlicePtr(ffi.CString(url))
				}
				iceServers[i].URLs = ffi.ByteSlicePtr((*[8]byte)(nil)[:]) // Will be set properly
				iceServers[i].URLCount = int32(len(server.URLs))
			}
			if server.Username != "" {
				iceServers[i].Username = ffi.CStringPtr(server.Username)
			}
			if server.Credential != "" {
				iceServers[i].Credential = ffi.CStringPtr(server.Credential)
			}
		}
		ffiConfig.ICEServers = iceServers[0].Ptr()
		ffiConfig.ICEServerCount = int32(len(iceServers))
	}

	// Set policies
	if config.BundlePolicy != "" {
		ffiConfig.BundlePolicy = ffi.CStringPtr(config.BundlePolicy)
	}
	if config.RTCPMuxPolicy != "" {
		ffiConfig.RTCPMuxPolicy = ffi.CStringPtr(config.RTCPMuxPolicy)
	}
	if config.SDPSemantics != "" {
		ffiConfig.SDPSemantics = ffi.CStringPtr(config.SDPSemantics)
	}

	return ffiConfig
}

// NewPeerConnection creates a new libwebrtc-backed PeerConnection.
func NewPeerConnection(config Configuration) (*PeerConnection, error) {
	if err := ffi.LoadLibrary(); err != nil {
		return nil, err
	}

	pc := &PeerConnection{
		config:       config,
		senders:      make([]*RTPSender, 0),
		receivers:    make([]*RTPReceiver, 0),
		transceivers: make([]*RTPTransceiver, 0),
		localTracks:  make([]*Track, 0),
	}

	pc.signalingState.Store(SignalingStateStable)
	pc.iceConnectionState.Store(ICEConnectionStateNew)
	pc.iceGatheringState.Store(ICEGatheringStateNew)
	pc.connectionState.Store(PeerConnectionStateNew)

	// Build FFI config
	ffiConfig := buildFFIConfig(&config)
	handle := ffi.CreatePeerConnection(ffiConfig)
	if handle == 0 {
		return nil, errors.New("failed to create peer connection")
	}
	pc.handle = handle

	return pc, nil
}

// CreateOffer creates an SDP offer.
func (pc *PeerConnection) CreateOffer(options *OfferOptions) (*SessionDescription, error) {
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Allocate buffer for SDP output
	sdpBuf := make([]byte, maxSDPSize)
	sdpLen, err := ffi.PeerConnectionCreateOffer(pc.handle, sdpBuf)
	if err != nil {
		return nil, ErrCreateOfferFailed
	}

	return &SessionDescription{
		Type: SDPTypeOffer,
		SDP:  string(sdpBuf[:sdpLen]),
	}, nil
}

// CreateAnswer creates an SDP answer.
func (pc *PeerConnection) CreateAnswer(options *AnswerOptions) (*SessionDescription, error) {
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Allocate buffer for SDP output
	sdpBuf := make([]byte, maxSDPSize)
	sdpLen, err := ffi.PeerConnectionCreateAnswer(pc.handle, sdpBuf)
	if err != nil {
		return nil, ErrCreateAnswerFailed
	}

	return &SessionDescription{
		Type: SDPTypeAnswer,
		SDP:  string(sdpBuf[:sdpLen]),
	}, nil
}

// SetLocalDescription sets the local description.
func (pc *PeerConnection) SetLocalDescription(desc *SessionDescription) error {
	if pc.closed.Load() {
		return ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	if err := ffi.PeerConnectionSetLocalDescription(pc.handle, int(desc.Type), desc.SDP); err != nil {
		return ErrSetDescriptionFailed
	}

	pc.localDescription = desc
	return nil
}

// SetRemoteDescription sets the remote description.
func (pc *PeerConnection) SetRemoteDescription(desc *SessionDescription) error {
	if pc.closed.Load() {
		return ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	if err := ffi.PeerConnectionSetRemoteDescription(pc.handle, int(desc.Type), desc.SDP); err != nil {
		return ErrSetDescriptionFailed
	}

	pc.remoteDescription = desc
	return nil
}

// AddICECandidate adds an ICE candidate.
func (pc *PeerConnection) AddICECandidate(candidate *ICECandidate) error {
	if pc.closed.Load() {
		return ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	if err := ffi.PeerConnectionAddICECandidate(pc.handle, candidate.Candidate, candidate.SDPMid, int(candidate.SDPMLineIndex)); err != nil {
		return ErrAddICECandidateFailed
	}

	return nil
}

// LocalDescription returns the local description.
func (pc *PeerConnection) LocalDescription() *SessionDescription {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.localDescription
}

// RemoteDescription returns the remote description.
func (pc *PeerConnection) RemoteDescription() *SessionDescription {
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

// AddTrack adds a track to the connection (mirrors browser's addTrack).
func (pc *PeerConnection) AddTrack(track *Track, streams ...string) (*RTPSender, error) {
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Get stream ID (use first stream or track ID)
	streamID := track.id
	if len(streams) > 0 {
		streamID = streams[0]
	}

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
		handle: senderHandle,
		track:  track,
		pc:     pc,
		id:     track.id,
	}

	pc.senders = append(pc.senders, sender)
	pc.localTracks = append(pc.localTracks, track)

	// Trigger negotiation needed
	if pc.OnNegotiationNeeded != nil {
		go pc.OnNegotiationNeeded()
	}

	return sender, nil
}

// RemoveTrack removes a track from the connection.
func (pc *PeerConnection) RemoveTrack(sender *RTPSender) error {
	if pc.closed.Load() {
		return ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

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

	return nil
}

// AddTransceiver adds a transceiver.
func (pc *PeerConnection) AddTransceiver(kind string, init *TransceiverInit) (*RTPTransceiver, error) {
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// TODO: Call FFI to add transceiver

	transceiver := &RTPTransceiver{
		pc:        pc,
		direction: TransceiverDirectionSendRecv,
	}

	if init != nil {
		transceiver.direction = init.Direction
	}

	pc.transceivers = append(pc.transceivers, transceiver)

	return transceiver, nil
}

// TransceiverInit for AddTransceiver.
type TransceiverInit struct {
	Direction     TransceiverDirection
	SendEncodings []RTPEncodingParameters
	Streams       []string
}

// GetTransceivers returns all transceivers.
func (pc *PeerConnection) GetTransceivers() []*RTPTransceiver {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	result := make([]*RTPTransceiver, len(pc.transceivers))
	copy(result, pc.transceivers)
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
func (pc *PeerConnection) CreateDataChannel(label string, options *DataChannelInit) (*DataChannel, error) {
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Set defaults
	ordered := true
	maxRetransmits := -1 // -1 means unset/unlimited
	protocol := ""

	if options != nil {
		if options.Ordered != nil {
			ordered = *options.Ordered
		}
		if options.MaxRetransmits != nil {
			maxRetransmits = int(*options.MaxRetransmits)
		}
		protocol = options.Protocol
	}

	// Call FFI to create data channel
	handle := ffi.PeerConnectionCreateDataChannel(pc.handle, label, ordered, maxRetransmits, protocol)
	if handle == 0 {
		return nil, errors.New("failed to create data channel")
	}

	dc := &DataChannel{
		handle: handle,
		label:  label,
		pc:     pc,
	}

	return dc, nil
}

// DataChannelInit for CreateDataChannel.
type DataChannelInit struct {
	Ordered           *bool
	MaxPacketLifeTime *uint16
	MaxRetransmits    *uint16
	Protocol          string
	Negotiated        bool
	ID                *uint16
}

// Close closes the peer connection.
func (pc *PeerConnection) Close() error {
	if !pc.closed.CompareAndSwap(false, true) {
		return nil
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.handle != 0 {
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
func (pc *PeerConnection) CreateVideoTrack(id string, codecType codec.Type, width, height int) (*Track, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("invalid video dimensions")
	}

	track := &Track{
		id:     id,
		kind:   "video",
		label:  "libwebrtc-video-" + id,
		codec:  codecType,
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
		codec:      codec.Opus,
		pc:         pc,
		sampleRate: sampleRate,
		channels:   channels,
	}
	track.enabled.Store(true)
	track.muted.Store(false)

	return track, nil
}

// --- Stats API ---

// GetStats returns connection statistics.
func (pc *PeerConnection) GetStats() (map[string]interface{}, error) {
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}

	// TODO: Call FFI to get stats

	return make(map[string]interface{}), nil
}
