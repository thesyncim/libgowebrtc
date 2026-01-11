package ffi

// shimRTPSenderSetBitrateParams matches ShimRTPSenderSetBitrateParams in shim.h.
type shimRTPSenderSetBitrateParams struct {
	Sender   uintptr
	Bitrate  uint32
	ErrorOut uintptr
}

// shimRTPSenderReplaceTrackParams matches ShimRTPSenderReplaceTrackParams in shim.h.
type shimRTPSenderReplaceTrackParams struct {
	Sender uintptr
	Track  uintptr
}

// shimRTPSenderGetParametersParams matches ShimRTPSenderGetParametersParams in shim.h.
type shimRTPSenderGetParametersParams struct {
	Sender      uintptr
	Encodings   uintptr
	MaxEncodings int32
	OutParams   RTPSendParameters
}

// shimRTPSenderSetParametersParams matches ShimRTPSenderSetParametersParams in shim.h.
type shimRTPSenderSetParametersParams struct {
	Sender   uintptr
	Params   uintptr
	ErrorOut uintptr
}

// shimRTPSenderSetOnRTCPFeedbackParams matches ShimRTPSenderSetOnRTCPFeedbackParams in shim.h.
type shimRTPSenderSetOnRTCPFeedbackParams struct {
	Sender   uintptr
	Callback uintptr
	Ctx      uintptr
}

// shimRTPSenderSetLayerActiveParams matches ShimRTPSenderSetLayerActiveParams in shim.h.
type shimRTPSenderSetLayerActiveParams struct {
	Sender   uintptr
	RID      uintptr
	Active   int32
	ErrorOut uintptr
}

// shimRTPSenderSetLayerBitrateParams matches ShimRTPSenderSetLayerBitrateParams in shim.h.
type shimRTPSenderSetLayerBitrateParams struct {
	Sender       uintptr
	RID          uintptr
	MaxBitrateBps uint32
	ErrorOut     uintptr
}

// shimRTPSenderGetActiveLayersParams matches ShimRTPSenderGetActiveLayersParams in shim.h.
type shimRTPSenderGetActiveLayersParams struct {
	Sender     uintptr
	OutSpatial int32
	OutTemporal int32
}

// shimRTPSenderSetScalabilityModeParams matches ShimRTPSenderSetScalabilityModeParams in shim.h.
type shimRTPSenderSetScalabilityModeParams struct {
	Sender           uintptr
	ScalabilityMode  uintptr
	ErrorOut         uintptr
}

// shimRTPSenderGetScalabilityModeParams matches ShimRTPSenderGetScalabilityModeParams in shim.h.
type shimRTPSenderGetScalabilityModeParams struct {
	Sender      uintptr
	ModeOut     uintptr
	ModeOutSize int32
}

// shimRTPSenderSetPreferredCodecParams matches ShimRTPSenderSetPreferredCodecParams in shim.h.
type shimRTPSenderSetPreferredCodecParams struct {
	Sender     uintptr
	MimeType   uintptr
	PayloadType int32
	ErrorOut   uintptr
}

// shimRTPReceiverSetJitterBufferMinDelayParams matches ShimRTPReceiverSetJitterBufferMinDelayParams in shim.h.
type shimRTPReceiverSetJitterBufferMinDelayParams struct {
	Receiver  uintptr
	MinDelayMs int32
}

// shimTransceiverSetDirectionParams matches ShimTransceiverSetDirectionParams in shim.h.
type shimTransceiverSetDirectionParams struct {
	Transceiver uintptr
	Direction   int32
	ErrorOut    uintptr
}

// shimTransceiverStopParams matches ShimTransceiverStopParams in shim.h.
type shimTransceiverStopParams struct {
	Transceiver uintptr
	ErrorOut    uintptr
}

// shimTransceiverSetCodecPreferencesParams matches ShimTransceiverSetCodecPreferencesParams in shim.h.
type shimTransceiverSetCodecPreferencesParams struct {
	Transceiver uintptr
	Codecs      uintptr
	Count       int32
	ErrorOut    uintptr
}

// shimTransceiverGetCodecPreferencesParams matches ShimTransceiverGetCodecPreferencesParams in shim.h.
type shimTransceiverGetCodecPreferencesParams struct {
	Transceiver uintptr
	Codecs      uintptr
	MaxCodecs   int32
	OutCount    int32
}
