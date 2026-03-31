package validate

import (
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/pioncodec"
	"github.com/thesyncim/libgowebrtc/pkg/pionrecv"
)

// SessionConfig configures browser-shaped validation behavior.
type SessionConfig struct {
	Browser                     pioncodec.Browser
	StatsPollInterval           time.Duration
	EventHistory                int
	FreezeThreshold             time.Duration
	PacketGapThreshold          time.Duration
	AudioGapThreshold           time.Duration
	SwitchRecoveryThreshold     time.Duration
	EnableDataChannelHeartbeats bool
	HeartbeatInterval           time.Duration
	HeartbeatTimeout            time.Duration
}

// PeerConnectionStateEvent records a connection state transition.
type PeerConnectionStateEvent struct {
	At    time.Time
	State webrtc.PeerConnectionState
}

// ICEConnectionStateEvent records an ICE connection state transition.
type ICEConnectionStateEvent struct {
	At    time.Time
	State webrtc.ICEConnectionState
}

// SignalingStateEvent records a signaling state transition.
type SignalingStateEvent struct {
	At    time.Time
	State webrtc.SignalingState
}

// RTPStatsSample is a normalized per-stream stats snapshot.
type RTPStatsSample struct {
	At                       time.Time
	Kind                     string
	Direction                string
	MID                      string
	RID                      string
	TrackID                  string
	SSRC                     uint32
	CodecID                  string
	CodecMimeType            string
	CodecPayloadType         webrtc.PayloadType
	TransportID              string
	Packets                  uint64
	PacketsLost              int64
	PacketsDiscarded         uint64
	Bytes                    uint64
	Jitter                   float64
	RoundTripTime            time.Duration
	RemoteRoundTripTime      time.Duration
	FractionLost             float64
	NACKCount                uint32
	PLICount                 uint32
	FIRCount                 uint32
	Frames                   uint64
	KeyFrames                uint64
	FrameWidth               int
	FrameHeight              int
	FramesPerSecond          float64
	AudioLevel               float64
	TotalAudioEnergy         float64
	ConcealedSamples         uint64
	ConcealmentEvents        uint64
	JitterBufferDelay        time.Duration
	JitterBufferTargetDelay  time.Duration
	JitterBufferMinimumDelay time.Duration
	QualityLimitationReason  string
	ScalabilityMode          string
	Active                   bool
}

// DataChannelStatsSample is a normalized data-channel stats snapshot.
type DataChannelStatsSample struct {
	At               time.Time
	Label            string
	ID               int
	State            webrtc.DataChannelState
	MessagesSent     uint64
	MessagesReceived uint64
	BytesSent        uint64
	BytesReceived    uint64
	TransportID      string
}

// TransportSample captures a single transport-level stats sample.
type TransportSample struct {
	At                       time.Time
	PacketsSent              uint64
	PacketsReceived          uint64
	BytesSent                uint64
	BytesReceived            uint64
	SelectedCandidatePairID  string
	ICEState                 string
	DTLSState                string
	CurrentRoundTripTime     time.Duration
	AvailableOutgoingBitrate float64
	AvailableIncomingBitrate float64
}

// TransportSnapshot captures the session's recent transport/stats view.
type TransportSnapshot struct {
	Samples          []TransportSample
	UnmatchedStreams []RTPStatsSample
	DataChannels     []DataChannelStatsSample
}

// VideoTrackSnapshot captures browser-visible and wire-visible video state.
type VideoTrackSnapshot struct {
	TrackID           string
	StreamID          string
	RID               string
	Source            string
	SSRC              uint32
	CurrentCodec      codec.Type
	CurrentMimeType   string
	StartedAt         time.Time
	LastFrameAt       time.Time
	FrameCount        uint64
	KeyframeCount     uint64
	FreezeCount       uint64
	CurrentWidth      int
	CurrentHeight     int
	ResolutionChanges uint64
	CurrentMID        string
	CurrentRID        string
	HasCurrentLayer   bool
	CurrentLayer      pionrecv.VideoLayer
	HasMaxActiveLayer bool
	MaxActiveLayer    pionrecv.VideoLayer
	Continuous        bool
	CodecSwitches     []pionrecv.CodecSwitchObservation
	FreezeEvents      []pionrecv.FreezeEvent
	Stats             []RTPStatsSample
	Wire              *pionrecv.VideoSubscriberSnapshot
}

// AudioTrackSnapshot captures browser-visible and wire-visible audio state.
type AudioTrackSnapshot struct {
	TrackID           string
	StreamID          string
	RID               string
	Source            string
	SSRC              uint32
	CurrentCodec      codec.Type
	CurrentMimeType   string
	StartedAt         time.Time
	LastFrameAt       time.Time
	FrameCount        uint64
	FreezeCount       uint64
	CurrentSampleRate int
	CurrentChannels   int
	CurrentNumSamples int
	CurrentPTime      time.Duration
	PeakLevel         float64
	RMSLevel          float64
	ActiveFrames      uint64
	SilentFrames      uint64
	ClippedFrames     uint64
	Continuous        bool
	CodecSwitches     []pionrecv.CodecSwitchObservation
	ConfigSwitches    []pionrecv.AudioConfigSwitch
	FreezeEvents      []pionrecv.FreezeEvent
	Stats             []RTPStatsSample
	Wire              *pionrecv.AudioSubscriberSnapshot
}

// DataChannelStateEvent records a data-channel state transition.
type DataChannelStateEvent struct {
	At    time.Time
	State string
}

// DataChannelSnapshot captures browser-visible data-channel continuity state.
type DataChannelSnapshot struct {
	Label                string
	ID                   int
	State                string
	OpenedAt             time.Time
	ClosedAt             time.Time
	OpenTransitions      uint64
	CloseTransitions     uint64
	UserMessagesSent     uint64
	UserMessagesReceived uint64
	UserBytesSent        uint64
	UserBytesReceived    uint64
	HeartbeatSent        uint64
	HeartbeatReceived    uint64
	HeartbeatAcked       uint64
	HeartbeatMissed      uint64
	LastHeartbeatRTT     time.Duration
	LastHeartbeatAt      time.Time
	LastHeartbeatAckAt   time.Time
	LastError            string
	StateHistory         []DataChannelStateEvent
	Stats                []DataChannelStatsSample
}

// SessionSnapshot is the aggregate browser-style validation view.
type SessionSnapshot struct {
	Browser             pioncodec.Browser
	Policy              BrowserPolicy
	ConnectionStates    []PeerConnectionStateEvent
	ICEConnectionStates []ICEConnectionStateEvent
	SignalingStates     []SignalingStateEvent
	VideoTracks         map[string]VideoTrackSnapshot
	AudioTracks         map[string]AudioTrackSnapshot
	DataChannels        map[string]DataChannelSnapshot
	Transport           TransportSnapshot
	Failures            []string
	Warnings            []string
	SkippedExpectations []string
}
