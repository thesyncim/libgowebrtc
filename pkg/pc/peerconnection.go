// Package pc provides a browser-like PeerConnection API backed by libwebrtc.
// This wraps libwebrtc's native PeerConnection, not Pion's.
package pc

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pioncodec"
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
	// ErrNotSupported reports APIs that are intentionally exposed but not yet
	// backed by the current shim/runtime implementation.
	ErrNotSupported = ffi.ErrNotSupported
)

// Constants
const (
	maxSDPSize = 64 * 1024 // 64KB should be sufficient for SDP
)

// SignalingState represents the signaling state.
type SignalingState int

// SignalingState values mirror the WebRTC signaling state machine.
const (
	SignalingStateStable SignalingState = iota
	SignalingStateHaveLocalOffer
	SignalingStateHaveRemoteOffer
	SignalingStateHaveLocalPranswer
	SignalingStateHaveRemotePranswer
	SignalingStateClosed
)

// String returns the browser-shaped token for the signaling state.
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

// ICEConnectionState values mirror the WebRTC ICE connection state machine.
const (
	ICEConnectionStateNew ICEConnectionState = iota
	ICEConnectionStateChecking
	ICEConnectionStateConnected
	ICEConnectionStateCompleted
	ICEConnectionStateDisconnected
	ICEConnectionStateFailed
	ICEConnectionStateClosed
)

// String returns the browser-shaped token for the ICE connection state.
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

// ICEGatheringState values mirror the WebRTC ICE gathering state machine.
const (
	ICEGatheringStateNew ICEGatheringState = iota
	ICEGatheringStateGathering
	ICEGatheringStateComplete
)

// String returns the browser-shaped token for the ICE gathering state.
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

// PeerConnectionState values mirror the overall WebRTC connection state machine.
const (
	PeerConnectionStateNew PeerConnectionState = iota
	PeerConnectionStateConnecting
	PeerConnectionStateConnected
	PeerConnectionStateDisconnected
	PeerConnectionStateFailed
	PeerConnectionStateClosed
)

// String returns the browser-shaped token for the PeerConnection state.
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

// SDPType values describe the type of SDP carried in a session description.
const (
	SDPTypeOffer SDPType = iota
	SDPTypePranswer
	SDPTypeAnswer
	SDPTypeRollback
)

// String returns the SDP token for the description type.
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
	Type SDPType // Type identifies whether SDP is an offer, answer, pranswer, or rollback.
	SDP  string  // SDP contains the raw session description.
}

// ICECandidate represents an ICE candidate.
type ICECandidate struct {
	Candidate        string // Candidate contains the raw ICE candidate attribute.
	SDPMid           string // SDPMid identifies the media section associated with the candidate.
	SDPMLineIndex    uint16 // SDPMLineIndex identifies the media section index associated with the candidate.
	UsernameFragment string // UsernameFragment carries the ICE username fragment when supplied.
}

// ICEServer represents an ICE server configuration.
type ICEServer struct {
	URLs       []string // URLs lists STUN or TURN server URLs.
	Username   string   // Username carries the TURN username when required.
	Credential string   // Credential carries the TURN password or token when required.
}

// Configuration for PeerConnection.
type Configuration struct {
	ICEServers           []ICEServer // ICEServers lists the STUN/TURN servers available to the agent.
	ICETransportPolicy   string      // "all" or "relay"
	BundlePolicy         string      // "balanced", "max-compat", "max-bundle"
	RTCPMuxPolicy        string      // "require" or "negotiate"
	PeerIdentity         string      // PeerIdentity carries an application-defined peer identity hint.
	SDPSemantics         string      // "unified-plan" or "plan-b"
	ICECandidatePoolSize int         // ICECandidatePoolSize pre-allocates local ICE candidates when supported.
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
	ICERestart             bool // ICERestart requests fresh ICE credentials in the generated offer.
	VoiceActivityDetection bool // VoiceActivityDetection enables or disables VAD in the offer.
}

// AnswerOptions for createAnswer.
type AnswerOptions struct {
	VoiceActivityDetection bool // VoiceActivityDetection enables or disables VAD in the answer.
}

// CodecCapability represents a supported codec.
type CodecCapability struct {
	MimeType    string // MimeType is the RTP codec MIME type, such as video/VP8.
	ClockRate   int    // ClockRate is the RTP clock rate in Hz.
	Channels    int    // Channels is the audio channel count for audio codecs.
	SDPFmtpLine string // SDPFmtpLine contains codec-specific fmtp parameters.
	PayloadType int    // PayloadType is the preferred RTP payload type when known.
}

// GetSupportedVideoCodecs returns a list of supported video codecs.
func GetSupportedVideoCodecs() ([]CodecCapability, error) {
	ffiCodecs, err := ffi.GetSupportedVideoCodecs()
	if err != nil {
		return nil, err
	}
	return convertCodecCapabilities(ffiCodecs), nil
}

// GetSupportedAudioCodecs returns a list of supported audio codecs.
func GetSupportedAudioCodecs() ([]CodecCapability, error) {
	ffiCodecs, err := ffi.GetSupportedAudioCodecs()
	if err != nil {
		return nil, err
	}
	return convertCodecCapabilities(ffiCodecs), nil
}

// IsCodecSupported checks if a specific codec is supported.
func IsCodecSupported(mimeType string) bool {
	return ffi.IsCodecSupported(mimeType)
}

func convertCodecCapabilities(ffiCodecs []ffi.CodecCapability) []CodecCapability {
	codecs := make([]CodecCapability, len(ffiCodecs))
	for i, c := range ffiCodecs {
		codecs[i] = CodecCapability{
			MimeType:    ffi.CStringToGo(c.MimeType[:]),
			ClockRate:   int(c.ClockRate),
			Channels:    int(c.Channels),
			SDPFmtpLine: ffi.CStringToGo(c.SDPFmtpLine[:]),
			PayloadType: int(c.PayloadType),
		}
	}
	return codecs
}

// RTPSender represents an RTP sender.
type RTPSender struct {
	handle  uintptr
	track   *Track
	pc      *PeerConnection
	id      string
	streams []string
	mu      sync.RWMutex
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

// StreamIDs returns the browser-style MediaStream IDs associated with the sender.
func (s *RTPSender) StreamIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.streams...)
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

// SetParameters sets encoding parameters.
func (s *RTPSender) SetParameters(params RTPSendParameters) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.handle == 0 {
		return errors.New("sender not initialized")
	}

	// Convert to FFI format
	ffiEncodings := make([]ffi.RTPEncodingParameters, len(params.Encodings))
	for i, enc := range params.Encodings {
		copy(ffiEncodings[i].RID[:], enc.RID)
		ffiEncodings[i].MaxBitrateBps = enc.MaxBitrate
		ffiEncodings[i].MaxFramerate = enc.MaxFramerate
		ffiEncodings[i].ScaleResolutionDownBy = enc.ScaleResolutionDownBy
		if enc.Active {
			ffiEncodings[i].Active = 1
		}
		copy(ffiEncodings[i].ScalabilityMode[:], enc.ScalabilityMode)
	}

	ffiParams := &ffi.RTPSendParameters{
		EncodingCount: int32(len(ffiEncodings)),
	}
	if len(ffiEncodings) > 0 {
		ffiParams.Encodings = ffi.UintptrFromSlice(ffiEncodings)
	}

	return ffi.RTPSenderSetParameters(s.handle, ffiParams)
}

// GetParameters gets current parameters.
func (s *RTPSender) GetParameters() RTPSendParameters {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.handle == 0 {
		return RTPSendParameters{}
	}

	const maxEncodings = 8
	ffiEncodings := make([]ffi.RTPEncodingParameters, maxEncodings)
	ffiParams, count, err := ffi.RTPSenderGetParameters(s.handle, ffiEncodings)
	if err != nil || ffiParams == nil {
		return RTPSendParameters{}
	}

	params := RTPSendParameters{
		Encodings: make([]RTPEncodingParameters, count),
	}

	for i := 0; i < count; i++ {
		params.Encodings[i] = RTPEncodingParameters{
			RID:                   ffi.ByteArrayToString(ffiEncodings[i].RID[:]),
			Active:                ffiEncodings[i].Active != 0,
			MaxBitrate:            ffiEncodings[i].MaxBitrateBps,
			MaxFramerate:          ffiEncodings[i].MaxFramerate,
			ScaleResolutionDownBy: ffiEncodings[i].ScaleResolutionDownBy,
			ScalabilityMode:       ffi.ByteArrayToString(ffiEncodings[i].ScalabilityMode[:]),
		}
	}

	return params
}

// GetStats gets statistics for this sender.
// The current shim does not implement sender stats and returns ErrNotSupported.
func (s *RTPSender) GetStats() (*RTCStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.handle == 0 {
		return nil, errors.New("sender not initialized")
	}

	ffiStats, err := ffi.RTPSenderGetStats(s.handle)
	if err != nil {
		return nil, err
	}

	return convertFFIStats(ffiStats), nil
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

// SetOnRTCPFeedback sets a callback for RTCP feedback events.
// The current shim does not implement this surface and returns ErrNotSupported.
func (s *RTPSender) SetOnRTCPFeedback(cb func(feedbackType RTCPFeedbackType, ssrc uint32)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.handle == 0 {
		return errors.New("sender not initialized")
	}

	return ffi.RTPSenderSetOnRTCPFeedback(s.handle, func(feedbackType int, ssrc uint32) {
		cb(RTCPFeedbackType(feedbackType), ssrc)
	})
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
func (s *RTPSender) GetNegotiatedCodecs() ([]CodecCapability, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.handle == 0 {
		return nil, errors.New("sender not initialized")
	}

	ffiCodecs, err := ffi.RTPSenderGetNegotiatedCodecs(s.handle)
	if err != nil {
		return nil, err
	}

	return convertCodecCapabilities(ffiCodecs), nil
}

// SetPreferredCodec sets the preferred codec for this sender.
// The codec must have been negotiated in the initial SDP (check GetNegotiatedCodecs).
// Pass payloadType=0 to auto-select based on mimeType.
//
// Note: Due to WebRTC limitations, this may require renegotiation to take effect.
// If renegotiation is needed, returns ErrRenegotiationNeeded.
// After setting, call CreateOffer/SetLocalDescription to apply the change.
func (s *RTPSender) SetPreferredCodec(mimeType string, payloadType int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.handle == 0 {
		return errors.New("sender not initialized")
	}

	return ffi.RTPSenderSetPreferredCodec(s.handle, mimeType, payloadType)
}

// ErrRenegotiationNeeded is returned when codec switching requires SDP renegotiation.
var ErrRenegotiationNeeded = ffi.ErrRenegotiationNeeded

// ErrNotFound is returned when the requested codec was not negotiated.
var ErrNotFound = ffi.ErrNotFound

// RTCPFeedbackType represents the type of RTCP feedback.
type RTCPFeedbackType int

// RTCPFeedbackType values identify the RTCP feedback message received for a sender.
const (
	RTCPFeedbackTypePLI  RTCPFeedbackType = 0
	RTCPFeedbackTypeFIR  RTCPFeedbackType = 1
	RTCPFeedbackTypeNACK RTCPFeedbackType = 2
)

// String returns the human-readable RTCP feedback type.
func (t RTCPFeedbackType) String() string {
	switch t {
	case RTCPFeedbackTypePLI:
		return "PLI"
	case RTCPFeedbackTypeFIR:
		return "FIR"
	case RTCPFeedbackTypeNACK:
		return "NACK"
	default:
		return "unknown"
	}
}

// RTCStats represents connection statistics.
type RTCStats struct {
	TimestampUs              int64   // TimestampUs is when the stats snapshot was taken, in microseconds.
	BytesSent                int64   // BytesSent is the total number of RTP bytes sent.
	BytesReceived            int64   // BytesReceived is the total number of RTP bytes received.
	PacketsSent              int64   // PacketsSent is the total number of RTP packets sent.
	PacketsReceived          int64   // PacketsReceived is the total number of RTP packets received.
	PacketsLost              int64   // PacketsLost is the cumulative number of lost RTP packets.
	RoundTripTimeMs          float64 // RoundTripTimeMs is the latest measured round-trip time in milliseconds.
	JitterMs                 float64 // JitterMs is the estimated interarrival jitter in milliseconds.
	AvailableOutgoingBitrate float64 // AvailableOutgoingBitrate is the estimated available send bitrate in bps.
	AvailableIncomingBitrate float64 // AvailableIncomingBitrate is the estimated available receive bitrate in bps.
	CurrentRTTMs             int64   // CurrentRTTMs is the most recent RTT estimate in milliseconds.
	TotalRTTMs               int64   // TotalRTTMs is the cumulative RTT over all responses in milliseconds.
	ResponsesReceived        int64   // ResponsesReceived counts transport responses used for RTT calculation.
	FramesEncoded            int     // FramesEncoded is the total number of encoded video frames.
	FramesDecoded            int     // FramesDecoded is the total number of decoded video frames.
	FramesDropped            int     // FramesDropped is the total number of dropped video frames.
	KeyFramesEncoded         int     // KeyFramesEncoded is the total number of encoded keyframes.
	KeyFramesDecoded         int     // KeyFramesDecoded is the total number of decoded keyframes.
	NACKCount                int     // NACKCount is the number of received NACK feedback messages.
	PLICount                 int     // PLICount is the number of received PLI feedback messages.
	FIRCount                 int     // FIRCount is the number of received FIR feedback messages.
	QPSum                    int     // QPSum is the cumulative quantizer sum for encoded video.
	AudioLevel               float64 // AudioLevel is the current audio level estimate.
	TotalAudioEnergy         float64 // TotalAudioEnergy is the cumulative captured audio energy.
	ConcealmentEvents        int     // ConcealmentEvents counts audio concealment events.

	// SCTP/DataChannel stats
	DataChannelsOpened       int64 // DataChannelsOpened is the number of data channels opened.
	DataChannelsClosed       int64 // DataChannelsClosed is the number of data channels closed.
	MessagesSent             int64 // MessagesSent is the number of data-channel messages sent.
	MessagesReceived         int64 // MessagesReceived is the number of data-channel messages received.
	BytesSentDataChannel     int64 // BytesSentDataChannel is the number of SCTP data bytes sent.
	BytesReceivedDataChannel int64 // BytesReceivedDataChannel is the number of SCTP data bytes received.

	// Quality limitation
	QualityLimitationReason     QualityLimitationReason // QualityLimitationReason explains why quality was reduced.
	QualityLimitationDurationMs int                     // QualityLimitationDurationMs is the cumulative limited time in milliseconds.

	// Remote inbound/outbound RTP stats
	RemotePacketsLost     int64   // RemotePacketsLost is the far-end view of packets lost.
	RemoteJitterMs        float64 // RemoteJitterMs is the far-end reported jitter in milliseconds.
	RemoteRoundTripTimeMs float64 // RemoteRoundTripTimeMs is the far-end reported round-trip time in milliseconds.

	// Jitter buffer stats (from RTCInboundRtpStreamStats)
	JitterBufferDelayMs        float64 // JitterBufferDelayMs is the average time samples spend in the jitter buffer.
	JitterBufferTargetDelayMs  float64 // JitterBufferTargetDelayMs is the current adaptive target delay.
	JitterBufferMinimumDelayMs float64 // JitterBufferMinimumDelayMs is the configured minimum jitter-buffer delay.
	JitterBufferEmittedCount   int64   // JitterBufferEmittedCount counts samples or frames emitted from the buffer.
}

// QualityLimitationReason indicates why quality is limited.
type QualityLimitationReason int

// QualityLimitationReason values explain why encoder quality was reduced.
const (
	QualityLimitationNone      QualityLimitationReason = 0
	QualityLimitationCPU       QualityLimitationReason = 1
	QualityLimitationBandwidth QualityLimitationReason = 2
	QualityLimitationOther     QualityLimitationReason = 3
)

// String returns the browser-shaped token for the quality limitation reason.
func (r QualityLimitationReason) String() string {
	switch r {
	case QualityLimitationNone:
		return "none"
	case QualityLimitationCPU:
		return "cpu"
	case QualityLimitationBandwidth:
		return "bandwidth"
	case QualityLimitationOther:
		return "other"
	default:
		return "unknown"
	}
}

func convertFFIStats(s *ffi.RTCStats) *RTCStats {
	if s == nil {
		return nil
	}
	return &RTCStats{
		TimestampUs:                 s.TimestampUs,
		BytesSent:                   s.BytesSent,
		BytesReceived:               s.BytesReceived,
		PacketsSent:                 s.PacketsSent,
		PacketsReceived:             s.PacketsReceived,
		PacketsLost:                 s.PacketsLost,
		RoundTripTimeMs:             s.RoundTripTimeMs,
		JitterMs:                    s.JitterMs,
		AvailableOutgoingBitrate:    s.AvailableOutgoingBitrate,
		AvailableIncomingBitrate:    s.AvailableIncomingBitrate,
		CurrentRTTMs:                s.CurrentRTTMs,
		TotalRTTMs:                  s.TotalRTTMs,
		ResponsesReceived:           s.ResponsesReceived,
		FramesEncoded:               int(s.FramesEncoded),
		FramesDecoded:               int(s.FramesDecoded),
		FramesDropped:               int(s.FramesDropped),
		KeyFramesEncoded:            int(s.KeyFramesEncoded),
		KeyFramesDecoded:            int(s.KeyFramesDecoded),
		NACKCount:                   int(s.NACKCount),
		PLICount:                    int(s.PLICount),
		FIRCount:                    int(s.FIRCount),
		QPSum:                       int(s.QPSum),
		AudioLevel:                  s.AudioLevel,
		TotalAudioEnergy:            s.TotalAudioEnergy,
		ConcealmentEvents:           int(s.ConcealmentEvents),
		DataChannelsOpened:          s.DataChannelsOpened,
		DataChannelsClosed:          s.DataChannelsClosed,
		MessagesSent:                s.MessagesSent,
		MessagesReceived:            s.MessagesReceived,
		BytesSentDataChannel:        s.BytesSentDataChannel,
		BytesReceivedDataChannel:    s.BytesReceivedDataChannel,
		QualityLimitationReason:     QualityLimitationReason(s.QualityLimitationReason),
		QualityLimitationDurationMs: int(s.QualityLimitationDurationMs),
		RemotePacketsLost:           s.RemotePacketsLost,
		RemoteJitterMs:              s.RemoteJitterMs,
		RemoteRoundTripTimeMs:       s.RemoteRoundTripTimeMs,
		// Jitter buffer stats
		JitterBufferDelayMs:        s.JitterBufferDelayMs,
		JitterBufferTargetDelayMs:  s.JitterBufferTargetDelayMs,
		JitterBufferMinimumDelayMs: s.JitterBufferMinimumDelayMs,
		JitterBufferEmittedCount:   s.JitterBufferEmittedCount,
	}
}

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

// GetStats gets statistics for this receiver.
func (r *RTPReceiver) GetStats() (*RTCStats, error) {
	if r.handle == 0 {
		return nil, errors.New("receiver not initialized")
	}

	ffiStats, err := ffi.RTPReceiverGetStats(r.handle)
	if err != nil {
		return nil, err
	}

	return convertFFIStats(ffiStats), nil
}

// SetJitterBufferMinDelay sets the minimum jitter buffer delay in milliseconds.
// This sets a floor for libwebrtc's adaptive jitter buffer. The actual delay
// may be higher based on network conditions, but won't go below this value.
// Pass 0 to let libwebrtc's adaptive algorithm decide without a minimum floor.
//
// For jitter buffer statistics, use GetStats() which returns RTCStats with:
// - JitterBufferDelayMs: Total time spent in buffer / emitted count
// - JitterBufferTargetDelayMs: Target delay for adaptive buffer
// - JitterBufferMinimumDelayMs: User-configured minimum delay
// - JitterBufferEmittedCount: Number of samples/frames emitted from buffer
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

// TransceiverDirection represents the transceiver direction.
type TransceiverDirection int

// TransceiverDirection values mirror the SDP transceiver direction attributes.
const (
	TransceiverDirectionSendRecv TransceiverDirection = iota
	TransceiverDirectionSendOnly
	TransceiverDirectionRecvOnly
	TransceiverDirectionInactive
)

// String returns the SDP token for the transceiver direction.
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
func (t *RTPTransceiver) Direction() TransceiverDirection {
	if t.handle != 0 {
		dir := ffi.TransceiverGetDirection(t.handle)
		return TransceiverDirection(dir)
	}
	return t.direction
}

// SetDirection sets the direction.
func (t *RTPTransceiver) SetDirection(d TransceiverDirection) error {
	if t.handle != 0 {
		if err := ffi.TransceiverSetDirection(t.handle, ffi.TransceiverDirection(d)); err != nil {
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
		return TransceiverDirection(dir)
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
func (t *RTPTransceiver) SetCodecPreferences(codecs []CodecCapability) error {
	if t.handle == 0 {
		return errors.New("transceiver not initialized")
	}

	// Convert to FFI format
	ffiCodecs := make([]ffi.CodecCapability, len(codecs))
	for i, c := range codecs {
		copy(ffiCodecs[i].MimeType[:], c.MimeType)
		ffiCodecs[i].ClockRate = int32(c.ClockRate)
		ffiCodecs[i].Channels = int32(c.Channels)
		copy(ffiCodecs[i].SDPFmtpLine[:], c.SDPFmtpLine)
		ffiCodecs[i].PayloadType = int32(c.PayloadType)
	}

	return ffi.TransceiverSetCodecPreferences(t.handle, ffiCodecs)
}

// SetCodecSet applies the supported subset of a Pion-native codec set.
func (t *RTPTransceiver) SetCodecSet(set pioncodec.CodecSet) error {
	kind := t.Kind()
	if kind == "" {
		kind = inferTransceiverKind(t.handle)
		t.kind = kind
	}
	if kind == "" {
		return errors.New("transceiver media kind is unknown")
	}

	supported := set.SupportedOnly()
	var codecs []webrtc.RTPCodecParameters
	switch kind {
	case "audio":
		codecs = supported.AudioCodecs()
	case "video":
		codecs = supported.VideoCodecs()
	default:
		return fmt.Errorf("unsupported transceiver kind %q", kind)
	}

	caps := make([]CodecCapability, 0, len(codecs))
	for _, codec := range codecs {
		caps = append(caps, CodecCapability{
			MimeType:    codec.MimeType,
			ClockRate:   int(codec.ClockRate),
			Channels:    int(codec.Channels),
			SDPFmtpLine: codec.SDPFmtpLine,
			PayloadType: int(codec.PayloadType),
		})
	}
	return t.SetCodecPreferences(caps)
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

// RTPSendParameters for sender configuration.
type RTPSendParameters struct {
	Encodings []RTPEncodingParameters // Encodings contains the per-encoding send parameters.
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
	codec   codec.Type
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

	callbackMu sync.RWMutex

	onICECandidate             func(candidate *ICECandidate)
	onICEConnectionStateChange func(state ICEConnectionState)
	onICEGatheringStateChange  func(state ICEGatheringState)
	onSignalingStateChange     func(state SignalingState)
	onConnectionStateChange    func(state PeerConnectionState)
	onTrack                    func(track *Track, receiver *RTPReceiver, streams []string)
	onNegotiationNeeded        func()
	onDataChannel              func(dc *DataChannel)

	mu     sync.RWMutex
	closed atomic.Bool
}

// IsValid returns true if the PeerConnection has a valid native handle.
func (pc *PeerConnection) IsValid() bool {
	return pc.handle != 0
}

// SetOnICECandidate installs the callback invoked for newly gathered local ICE candidates.
func (pc *PeerConnection) SetOnICECandidate(cb func(candidate *ICECandidate)) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onICECandidate = cb
}

// SetOnICEConnectionStateChange installs the callback invoked when the ICE connection state changes.
func (pc *PeerConnection) SetOnICEConnectionStateChange(cb func(state ICEConnectionState)) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onICEConnectionStateChange = cb
}

// SetOnICEGatheringStateChange installs the callback invoked when ICE gathering state changes.
func (pc *PeerConnection) SetOnICEGatheringStateChange(cb func(state ICEGatheringState)) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onICEGatheringStateChange = cb
}

// SetOnSignalingStateChange installs the callback invoked when signaling state changes.
func (pc *PeerConnection) SetOnSignalingStateChange(cb func(state SignalingState)) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onSignalingStateChange = cb
}

// SetOnConnectionStateChange installs the callback invoked when the overall connection state changes.
func (pc *PeerConnection) SetOnConnectionStateChange(cb func(state PeerConnectionState)) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onConnectionStateChange = cb
}

// SetOnTrack installs the callback invoked for newly received remote tracks.
func (pc *PeerConnection) SetOnTrack(cb func(track *Track, receiver *RTPReceiver, streams []string)) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onTrack = cb
}

// SetOnNegotiationNeeded installs the callback invoked when renegotiation is required.
func (pc *PeerConnection) SetOnNegotiationNeeded(cb func()) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onNegotiationNeeded = cb
}

// SetOnDataChannel installs the callback invoked for newly created remote data channels.
func (pc *PeerConnection) SetOnDataChannel(cb func(dc *DataChannel)) {
	pc.callbackMu.Lock()
	defer pc.callbackMu.Unlock()
	pc.onDataChannel = cb
}

// DataChannelState represents the state of a data channel.
type DataChannelState int

// DataChannelState values mirror the WebRTC data-channel lifecycle.
const (
	DataChannelStateConnecting DataChannelState = iota
	DataChannelStateOpen
	DataChannelStateClosing
	DataChannelStateClosed
)

// String returns the browser-shaped token for the data-channel state.
func (s DataChannelState) String() string {
	switch s {
	case DataChannelStateConnecting:
		return "connecting"
	case DataChannelStateOpen:
		return "open"
	case DataChannelStateClosing:
		return "closing"
	case DataChannelStateClosed:
		return "closed"
	default:
		return "unknown"
	}
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
	return DataChannelState(ffi.DataChannelReadyState(dc.handle))
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

// SetOnError stores a callback for DataChannel errors.
// The current shim does not emit asynchronous DataChannel error events yet, so
// this is retained for forward compatibility.
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
func buildFFIConfig(config *Configuration) *ffiConfigData {
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
			if server.Credential != "" {
				credStr := ffi.CString(server.Credential)
				data.strings = append(data.strings, credStr)
				data.iceServers[i].Credential = &credStr[0]
			}
		}
		data.config.ICEServers = data.iceServers[0].Ptr()
		data.config.ICEServerCount = int32(len(data.iceServers))
	}

	// Set policies
	if config.BundlePolicy != "" {
		bundleStr := ffi.CString(config.BundlePolicy)
		data.strings = append(data.strings, bundleStr)
		data.config.BundlePolicy = &bundleStr[0]
	}
	if config.RTCPMuxPolicy != "" {
		rtcpStr := ffi.CString(config.RTCPMuxPolicy)
		data.strings = append(data.strings, rtcpStr)
		data.config.RTCPMuxPolicy = &rtcpStr[0]
	}
	if config.SDPSemantics != "" {
		sdpStr := ffi.CString(config.SDPSemantics)
		data.strings = append(data.strings, sdpStr)
		data.config.SDPSemantics = &sdpStr[0]
	}

	return data
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

	// Build FFI config - keep data alive during FFI call
	configData := buildFFIConfig(&config)
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
		newState := PeerConnectionState(state)
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
			cb(&ICECandidate{
				Candidate:     candidate,
				SDPMid:        sdpMid,
				SDPMLineIndex: uint16(sdpMLineIndex),
			})
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

			streamIDs := splitStreamIDs(streams)
			if len(streamIDs) == 0 {
				streamIDs = pc.remoteTrackStreamIDs(trackID)
			}
			cb(track, receiver, streamIDs)
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
		newState := SignalingState(state)
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
		newState := ICEConnectionState(state)
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
		newState := ICEGatheringState(state)
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
func (pc *PeerConnection) CreateOffer(options *OfferOptions) (*SessionDescription, error) {
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}

	// Note: Don't hold lock during FFI call - it can trigger callbacks that need the lock.
	// Allocate buffer for SDP output
	sdpBuf := make([]byte, maxSDPSize)
	sdpLen, err := ffi.PeerConnectionCreateOffer(pc.handle, sdpBuf)
	if err != nil {
		return nil, ErrCreateOfferFailed
	}

	return &SessionDescription{
		Type: SDPTypeOffer,
		SDP:  pc.expandLocalTrackStreamIDs(string(sdpBuf[:sdpLen])),
	}, nil
}

// CreateAnswer creates an SDP answer.
func (pc *PeerConnection) CreateAnswer(options *AnswerOptions) (*SessionDescription, error) {
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}

	// Note: Don't hold lock during FFI call - it can trigger callbacks that need the lock.
	// Allocate buffer for SDP output
	sdpBuf := make([]byte, maxSDPSize)
	sdpLen, err := ffi.PeerConnectionCreateAnswer(pc.handle, sdpBuf)
	if err != nil {
		return nil, ErrCreateAnswerFailed
	}

	return &SessionDescription{
		Type: SDPTypeAnswer,
		SDP:  pc.expandLocalTrackStreamIDs(string(sdpBuf[:sdpLen])),
	}, nil
}

// SetLocalDescription sets the local description.
func (pc *PeerConnection) SetLocalDescription(desc *SessionDescription) error {
	if pc.closed.Load() {
		return ErrPeerConnectionClosed
	}

	// Note: Don't hold lock during FFI call - it can trigger callbacks that need the lock
	if err := ffi.PeerConnectionSetLocalDescription(pc.handle, int(desc.Type), desc.SDP); err != nil {
		return ErrSetDescriptionFailed
	}

	pc.mu.Lock()
	pc.localDescription = desc
	pc.mu.Unlock()
	return nil
}

// SetRemoteDescription sets the remote description.
func (pc *PeerConnection) SetRemoteDescription(desc *SessionDescription) error {
	if pc.closed.Load() {
		return ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	prevRemoteDescription := pc.remoteDescription
	pc.remoteDescription = desc
	pc.mu.Unlock()

	// Note: Don't hold lock during FFI call - it can trigger callbacks that need the lock.
	// We publish the description first so callbacks fired during SetRemoteDescription
	// can still recover track/stream relationships from SDP. Roll back on failure.
	if err := ffi.PeerConnectionSetRemoteDescription(pc.handle, int(desc.Type), desc.SDP); err != nil {
		pc.mu.Lock()
		if pc.remoteDescription == desc {
			pc.remoteDescription = prevRemoteDescription
		}
		pc.mu.Unlock()
		return ErrSetDescriptionFailed
	}

	return nil
}

// AddICECandidate adds an ICE candidate.
func (pc *PeerConnection) AddICECandidate(candidate *ICECandidate) error {
	if candidate == nil {
		return errors.New("candidate is nil")
	}
	if pc.closed.Load() {
		return ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.handle == 0 {
		return ErrPeerConnectionClosed
	}

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

	streamIDs := normalizeTrackStreamIDs(track.id, streams)
	streamID := streamIDs[0]

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
		handle:  senderHandle,
		track:   track,
		pc:      pc,
		id:      track.id,
		streams: streamIDs,
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
	if pc.closed.Load() {
		return ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	var trackToRemove *Track
	if sender != nil {
		trackToRemove = sender.track
	}

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
func (pc *PeerConnection) AddTransceiver(kind string, init *TransceiverInit) (*RTPTransceiver, error) {
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Determine media kind
	var mediaKind ffi.MediaKind
	if kind == "video" {
		mediaKind = ffi.MediaKindVideo
	} else if kind == "audio" {
		mediaKind = ffi.MediaKindAudio
	} else {
		return nil, errors.New("unknown media kind")
	}

	// Determine direction
	direction := TransceiverDirectionSendRecv
	if init != nil {
		direction = init.Direction
	}

	// Call FFI to add transceiver
	handle := ffi.PeerConnectionAddTransceiver(pc.handle, mediaKind, ffi.TransceiverDirection(direction))
	if handle == 0 {
		return nil, errors.New("failed to add transceiver")
	}

	transceiver := &RTPTransceiver{
		handle:    handle,
		pc:        pc,
		direction: direction,
		kind:      kind,
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

// TransceiverInit for AddTransceiver.
type TransceiverInit struct {
	Direction TransceiverDirection // Direction is the initial direction of the new transceiver.
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
func (pc *PeerConnection) CreateDataChannel(label string, options *DataChannelInit) (*DataChannel, error) {
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
		protocol = options.Protocol
		negotiated = options.Negotiated
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

// DataChannelInit for CreateDataChannel.
type DataChannelInit struct {
	Ordered           *bool   // Ordered overrides ordered delivery when non-nil.
	MaxPacketLifeTime *uint16 // MaxPacketLifeTime caps retransmission time in milliseconds.
	MaxRetransmits    *uint16 // MaxRetransmits caps the number of retransmission attempts.
	Protocol          string  // Protocol names the subprotocol used by the data channel.
	Negotiated        bool    // Negotiated reports whether out-of-band negotiation is used.
	ID                *uint16 // ID overrides the SCTP stream identifier when negotiated.
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

func (pc *PeerConnection) remoteTrackStreamIDs(trackID string) []string {
	if trackID == "" {
		return nil
	}

	pc.mu.RLock()
	desc := pc.remoteDescription
	pc.mu.RUnlock()
	if desc == nil {
		return nil
	}

	return streamIDsForTrackID(desc.SDP, trackID)
}

func (pc *PeerConnection) expandLocalTrackStreamIDs(sdp string) string {
	if sdp == "" {
		return sdp
	}

	trackStreams, allStreams := pc.localTrackStreamIDs()
	if len(trackStreams) == 0 || len(allStreams) == 0 {
		return sdp
	}

	lineEnding := "\n"
	if strings.Contains(sdp, "\r\n") {
		lineEnding = "\r\n"
	}

	lines := strings.Split(sdp, lineEnding)
	var out []string
	semanticUpdated := false
	insertedSemantic := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "a=msid-semantic: WMS") {
			out = append(out, "a=msid-semantic: WMS "+strings.Join(allStreams, " "))
			semanticUpdated = true
			continue
		}

		if !semanticUpdated && !insertedSemantic && strings.HasPrefix(trimmed, "m=") {
			out = append(out, "a=msid-semantic: WMS "+strings.Join(allStreams, " "))
			insertedSemantic = true
		}

		out = append(out, line)

		streamID, trackID, prefix, ok := parseMSIDLine(trimmed)
		if !ok {
			if i == len(lines)-1 && !semanticUpdated && !insertedSemantic {
				out = append(out, "a=msid-semantic: WMS "+strings.Join(allStreams, " "))
			}
			continue
		}

		streams := trackStreams[trackID]
		if len(streams) <= 1 {
			continue
		}
		for _, extraStreamID := range streams[1:] {
			if extraStreamID == "" || extraStreamID == streamID {
				continue
			}
			if prefix == "a=msid:" {
				out = append(out, prefix+extraStreamID+" "+trackID)
				continue
			}
			out = append(out, prefix+extraStreamID+" "+trackID)
		}
	}

	return strings.Join(out, lineEnding)
}

func (pc *PeerConnection) localTrackStreamIDs() (map[string][]string, []string) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	trackStreams := make(map[string][]string)
	seenStreams := make(map[string]struct{})
	allStreams := make([]string, 0, len(pc.senders))

	for _, sender := range pc.senders {
		if sender == nil {
			continue
		}
		sender.mu.RLock()
		track := sender.track
		streams := append([]string(nil), sender.streams...)
		sender.mu.RUnlock()
		if track == nil || len(streams) == 0 {
			continue
		}
		trackStreams[track.id] = streams
		for _, streamID := range streams {
			if _, ok := seenStreams[streamID]; ok {
				continue
			}
			seenStreams[streamID] = struct{}{}
			allStreams = append(allStreams, streamID)
		}
	}

	return trackStreams, allStreams
}

func streamIDsForTrackID(sdp, trackID string) []string {
	if sdp == "" || trackID == "" {
		return nil
	}

	seen := make(map[string]struct{})
	streamIDs := make([]string, 0, 2)
	lines := strings.Split(sdp, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var value string
		switch {
		case strings.HasPrefix(line, "a=msid:"):
			value = strings.TrimPrefix(line, "a=msid:")
		case strings.HasPrefix(line, "a=ssrc:"):
			idx := strings.Index(line, " msid:")
			if idx < 0 {
				continue
			}
			value = line[idx+len(" msid:"):]
		default:
			continue
		}

		parts := strings.Fields(value)
		if len(parts) < 2 || parts[1] != trackID {
			continue
		}
		streamID := strings.TrimSpace(parts[0])
		if streamID == "" {
			continue
		}
		if _, ok := seen[streamID]; ok {
			continue
		}
		seen[streamID] = struct{}{}
		streamIDs = append(streamIDs, streamID)
	}

	if len(streamIDs) == 0 {
		return nil
	}
	return streamIDs
}

func normalizeTrackStreamIDs(trackID string, streams []string) []string {
	if trackID == "" {
		return nil
	}

	seen := make(map[string]struct{}, len(streams)+1)
	out := make([]string, 0, len(streams)+1)
	for _, streamID := range streams {
		streamID = strings.TrimSpace(streamID)
		if streamID == "" {
			continue
		}
		if _, ok := seen[streamID]; ok {
			continue
		}
		seen[streamID] = struct{}{}
		out = append(out, streamID)
	}
	if len(out) == 0 {
		return []string{trackID}
	}
	return out
}

func parseMSIDLine(line string) (streamID, trackID, prefix string, ok bool) {
	if line == "" {
		return "", "", "", false
	}

	value := ""
	switch {
	case strings.HasPrefix(line, "a=msid:"):
		prefix = "a=msid:"
		value = strings.TrimPrefix(line, prefix)
	case strings.HasPrefix(line, "a=ssrc:"):
		idx := strings.Index(line, " msid:")
		if idx < 0 {
			return "", "", "", false
		}
		prefix = line[:idx+len(" msid:")]
		value = line[idx+len(" msid:"):]
	default:
		return "", "", "", false
	}

	parts := strings.Fields(value)
	if len(parts) < 2 {
		return "", "", "", false
	}
	return parts[0], parts[1], prefix, true
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
		ffi.UnregisterBandwidthEstimateCallback(pc.handle)

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
func (pc *PeerConnection) GetStats() (*RTCStats, error) {
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}

	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if pc.handle == 0 {
		return nil, errors.New("peer connection not initialized")
	}

	ffiStats, err := ffi.PeerConnectionGetStats(pc.handle)
	if err != nil {
		return nil, err
	}

	return convertFFIStats(ffiStats), nil
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

// BandwidthEstimate contains bandwidth estimation data from libwebrtc's BWE engine.
type BandwidthEstimate struct {
	TimestampUs      int64   // TimestampUs is when the estimate was produced, in microseconds.
	TargetBitrateBps int64   // TargetBitrateBps is libwebrtc's target send bitrate in bps.
	AvailableSendBps int64   // AvailableSendBps is the estimated available uplink bitrate in bps.
	AvailableRecvBps int64   // AvailableRecvBps is the estimated available downlink bitrate in bps.
	PacingRateBps    int64   // PacingRateBps is the suggested packet pacing rate in bps.
	CongestionWindow int32   // CongestionWindow is the estimated congestion window in bytes.
	LossRate         float64 // LossRate is the recent packet loss fraction in the range [0,1].
}

// SetOnBandwidthEstimate sets a callback for bandwidth estimation updates from libwebrtc.
// The current shim does not implement this surface and returns ErrNotSupported.
func (pc *PeerConnection) SetOnBandwidthEstimate(cb func(*BandwidthEstimate)) error {
	if pc.closed.Load() {
		return ErrPeerConnectionClosed
	}
	if pc.handle == 0 {
		return errors.New("peer connection not initialized")
	}

	return ffi.PeerConnectionSetOnBandwidthEstimate(pc.handle, func(bwe *ffi.BandwidthEstimate) {
		if cb != nil && bwe != nil {
			cb(&BandwidthEstimate{
				TimestampUs:      bwe.TimestampUs,
				TargetBitrateBps: bwe.TargetBitrateBps,
				AvailableSendBps: bwe.AvailableSendBps,
				AvailableRecvBps: bwe.AvailableRecvBps,
				PacingRateBps:    bwe.PacingRateBps,
				CongestionWindow: bwe.CongestionWindow,
				LossRate:         bwe.LossRate,
			})
		}
	})
}

// GetCurrentBandwidthEstimate returns the current bandwidth estimate from libwebrtc.
// The current shim does not implement this surface and returns ErrNotSupported.
func (pc *PeerConnection) GetCurrentBandwidthEstimate() (*BandwidthEstimate, error) {
	if pc.closed.Load() {
		return nil, ErrPeerConnectionClosed
	}
	if pc.handle == 0 {
		return nil, errors.New("peer connection not initialized")
	}

	bwe, err := ffi.PeerConnectionGetBandwidthEstimate(pc.handle)
	if err != nil {
		return nil, err
	}
	if bwe == nil {
		return nil, ErrNotSupported
	}

	return &BandwidthEstimate{
		TimestampUs:      bwe.TimestampUs,
		TargetBitrateBps: bwe.TargetBitrateBps,
		AvailableSendBps: bwe.AvailableSendBps,
		AvailableRecvBps: bwe.AvailableRecvBps,
		PacingRateBps:    bwe.PacingRateBps,
		CongestionWindow: bwe.CongestionWindow,
		LossRate:         bwe.LossRate,
	}, nil
}
