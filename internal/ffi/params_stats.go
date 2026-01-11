package ffi

// shimGetSupportedVideoCodecsParams matches ShimGetSupportedVideoCodecsParams in shim.h.
type shimGetSupportedVideoCodecsParams struct {
	Codecs    uintptr
	MaxCodecs int32
	OutCount  int32
}

// shimGetSupportedAudioCodecsParams matches ShimGetSupportedAudioCodecsParams in shim.h.
type shimGetSupportedAudioCodecsParams struct {
	Codecs    uintptr
	MaxCodecs int32
	OutCount  int32
}

// shimRTPSenderGetNegotiatedCodecsParams matches ShimRTPSenderGetNegotiatedCodecsParams in shim.h.
type shimRTPSenderGetNegotiatedCodecsParams struct {
	Sender    uintptr
	Codecs    uintptr
	MaxCodecs int32
	OutCount  int32
}

// shimPeerConnectionGetStatsParams matches ShimPeerConnectionGetStatsParams in shim.h.
type shimPeerConnectionGetStatsParams struct {
	PC       uintptr
	OutStats RTCStats
}

// shimRTPSenderGetStatsParams matches ShimRTPSenderGetStatsParams in shim.h.
type shimRTPSenderGetStatsParams struct {
	Sender   uintptr
	OutStats RTCStats
}

// shimRTPReceiverGetStatsParams matches ShimRTPReceiverGetStatsParams in shim.h.
type shimRTPReceiverGetStatsParams struct {
	Receiver uintptr
	OutStats RTCStats
}

// shimPeerConnectionGetBandwidthEstimateParams matches ShimPeerConnectionGetBandwidthEstimateParams in shim.h.
type shimPeerConnectionGetBandwidthEstimateParams struct {
	PC          uintptr
	OutEstimate BandwidthEstimate
}
