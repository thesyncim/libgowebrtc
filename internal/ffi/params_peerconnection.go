package ffi

// shimPeerConnectionCreateParams matches ShimPeerConnectionCreateParams in shim.h.
type shimPeerConnectionCreateParams struct {
	Config   uintptr
	ErrorOut uintptr
}

// shimPeerConnectionSetOnICECandidateParams matches ShimPeerConnectionSetOnICECandidateParams in shim.h.
type shimPeerConnectionSetOnICECandidateParams struct {
	PC       uintptr
	Callback uintptr
	Ctx      uintptr
}

// shimPeerConnectionSetOnConnectionStateChangeParams matches ShimPeerConnectionSetOnConnectionStateChangeParams in shim.h.
type shimPeerConnectionSetOnConnectionStateChangeParams struct {
	PC       uintptr
	Callback uintptr
	Ctx      uintptr
}

// shimPeerConnectionSetOnTrackParams matches ShimPeerConnectionSetOnTrackParams in shim.h.
type shimPeerConnectionSetOnTrackParams struct {
	PC       uintptr
	Callback uintptr
	Ctx      uintptr
}

// shimPeerConnectionSetOnDataChannelParams matches ShimPeerConnectionSetOnDataChannelParams in shim.h.
type shimPeerConnectionSetOnDataChannelParams struct {
	PC       uintptr
	Callback uintptr
	Ctx      uintptr
}

// shimPeerConnectionSetOnSignalingStateChangeParams matches ShimPeerConnectionSetOnSignalingStateChangeParams in shim.h.
type shimPeerConnectionSetOnSignalingStateChangeParams struct {
	PC       uintptr
	Callback uintptr
	Ctx      uintptr
}

// shimPeerConnectionSetOnICEConnectionStateChangeParams matches ShimPeerConnectionSetOnICEConnectionStateChangeParams in shim.h.
type shimPeerConnectionSetOnICEConnectionStateChangeParams struct {
	PC       uintptr
	Callback uintptr
	Ctx      uintptr
}

// shimPeerConnectionSetOnICEGatheringStateChangeParams matches ShimPeerConnectionSetOnICEGatheringStateChangeParams in shim.h.
type shimPeerConnectionSetOnICEGatheringStateChangeParams struct {
	PC       uintptr
	Callback uintptr
	Ctx      uintptr
}

// shimPeerConnectionSetOnNegotiationNeededParams matches ShimPeerConnectionSetOnNegotiationNeededParams in shim.h.
type shimPeerConnectionSetOnNegotiationNeededParams struct {
	PC       uintptr
	Callback uintptr
	Ctx      uintptr
}

// shimPeerConnectionCreateOfferParams matches ShimPeerConnectionCreateOfferParams in shim.h.
type shimPeerConnectionCreateOfferParams struct {
	PC         uintptr
	SDPOut     uintptr
	SDPOutSize int32
	OutSDPLen  int32
	ErrorOut   uintptr
}

// shimPeerConnectionCreateAnswerParams matches ShimPeerConnectionCreateAnswerParams in shim.h.
type shimPeerConnectionCreateAnswerParams struct {
	PC         uintptr
	SDPOut     uintptr
	SDPOutSize int32
	OutSDPLen  int32
	ErrorOut   uintptr
}

// shimPeerConnectionSetLocalDescriptionParams matches ShimPeerConnectionSetLocalDescriptionParams in shim.h.
type shimPeerConnectionSetLocalDescriptionParams struct {
	PC       uintptr
	Type     int32
	SDP      uintptr
	ErrorOut uintptr
}

// shimPeerConnectionSetRemoteDescriptionParams matches ShimPeerConnectionSetRemoteDescriptionParams in shim.h.
type shimPeerConnectionSetRemoteDescriptionParams struct {
	PC       uintptr
	Type     int32
	SDP      uintptr
	ErrorOut uintptr
}

// shimPeerConnectionAddICECandidateParams matches ShimPeerConnectionAddICECandidateParams in shim.h.
type shimPeerConnectionAddICECandidateParams struct {
	PC            uintptr
	Candidate     uintptr
	SDPMid        uintptr
	SDPMLineIndex int32
	ErrorOut      uintptr
}

// shimPeerConnectionAddTrackParams matches ShimPeerConnectionAddTrackParams in shim.h.
type shimPeerConnectionAddTrackParams struct {
	PC       uintptr
	Codec    int32
	TrackID  uintptr
	StreamID uintptr
	ErrorOut uintptr
}

// shimPeerConnectionRemoveTrackParams matches ShimPeerConnectionRemoveTrackParams in shim.h.
type shimPeerConnectionRemoveTrackParams struct {
	PC       uintptr
	Sender   uintptr
	ErrorOut uintptr
}

// shimPeerConnectionCreateDataChannelParams matches ShimPeerConnectionCreateDataChannelParams in shim.h.
type shimPeerConnectionCreateDataChannelParams struct {
	PC            uintptr
	Label         uintptr
	Ordered       int32
	MaxRetransmits int32
	Protocol      uintptr
	ErrorOut      uintptr
}

// shimPeerConnectionAddTransceiverParams matches ShimPeerConnectionAddTransceiverParams in shim.h.
type shimPeerConnectionAddTransceiverParams struct {
	PC        uintptr
	Kind      int32
	Direction int32
	ErrorOut  uintptr
}

// shimPeerConnectionGetSendersParams matches ShimPeerConnectionGetSendersParams in shim.h.
type shimPeerConnectionGetSendersParams struct {
	PC        uintptr
	Senders   uintptr
	MaxSenders int32
	OutCount  int32
}

// shimPeerConnectionGetReceiversParams matches ShimPeerConnectionGetReceiversParams in shim.h.
type shimPeerConnectionGetReceiversParams struct {
	PC           uintptr
	Receivers    uintptr
	MaxReceivers int32
	OutCount     int32
}

// shimPeerConnectionGetTransceiversParams matches ShimPeerConnectionGetTransceiversParams in shim.h.
type shimPeerConnectionGetTransceiversParams struct {
	PC             uintptr
	Transceivers   uintptr
	MaxTransceivers int32
	OutCount       int32
}

// shimPeerConnectionSetOnBandwidthEstimateParams matches ShimPeerConnectionSetOnBandwidthEstimateParams in shim.h.
type shimPeerConnectionSetOnBandwidthEstimateParams struct {
	PC       uintptr
	Callback uintptr
	Ctx      uintptr
}
