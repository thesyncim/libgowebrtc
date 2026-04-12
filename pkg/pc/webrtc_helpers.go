package pc

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
)

var (
	ErrUnsupportedConfiguration     = errors.New("unsupported peer connection configuration")
	ErrUnsupportedRTPParameters     = errors.New("unsupported RTP parameters")
	ErrUnsupportedRTPEncodingFields = errors.New("unsupported RTP encoding fields")
	ErrNilICECandidateInit          = errors.New("ice candidate init is nil")
)

func validateConfigurationSupport(config webrtc.Configuration) error {
	if config.BundlePolicy == webrtc.BundlePolicyUnknown {
		// Zero-value configuration is allowed and falls through to libwebrtc defaults.
	}
	if config.PeerIdentity != "" {
		return fmt.Errorf("%w: peer identity", ErrUnsupportedConfiguration)
	}
	if len(config.Certificates) > 0 {
		return fmt.Errorf("%w: certificates", ErrUnsupportedConfiguration)
	}
	if config.AlwaysNegotiateDataChannels {
		return fmt.Errorf("%w: always negotiate data channels", ErrUnsupportedConfiguration)
	}
	for _, server := range config.ICEServers {
		if err := validateICEServer(server); err != nil {
			return err
		}
	}
	return nil
}

func validateICEServer(server webrtc.ICEServer) error {
	if server.Credential == nil {
		return nil
	}
	if server.CredentialType != webrtc.ICECredentialTypePassword {
		return fmt.Errorf("%w: ICE credential type %q", ErrUnsupportedConfiguration, server.CredentialType.String())
	}
	if _, ok := server.Credential.(string); !ok {
		return fmt.Errorf("%w: ICE credentials must be strings", ErrUnsupportedConfiguration)
	}
	return nil
}

func credentialString(server webrtc.ICEServer) string {
	if server.Credential == nil {
		return ""
	}
	credential, _ := server.Credential.(string)
	return credential
}

func bundlePolicyString(policy webrtc.BundlePolicy) (string, error) {
	switch policy {
	case webrtc.BundlePolicyUnknown:
		return "", nil
	case webrtc.BundlePolicyMaxBundle:
		return "max-bundle", nil
	case webrtc.BundlePolicyMaxCompat:
		return "max-compat", nil
	case webrtc.BundlePolicyBalanced:
		return "balanced", nil
	default:
		return "", fmt.Errorf("%w: bundle policy %q", ErrUnsupportedConfiguration, policy.String())
	}
}

func rtcpMuxPolicyString(policy webrtc.RTCPMuxPolicy) (string, error) {
	switch policy {
	case webrtc.RTCPMuxPolicyUnknown:
		return "", nil
	case webrtc.RTCPMuxPolicyRequire:
		return "require", nil
	case webrtc.RTCPMuxPolicyNegotiate:
		return "negotiate", nil
	default:
		return "", fmt.Errorf("%w: rtcp mux policy %q", ErrUnsupportedConfiguration, policy.String())
	}
}

func sdpSemanticsString(semantics webrtc.SDPSemantics) (string, error) {
	switch semantics {
	case webrtc.SDPSemanticsUnifiedPlan:
		return "unified-plan", nil
	case webrtc.SDPSemanticsPlanB:
		return "plan-b", nil
	default:
		return "", fmt.Errorf("%w: SDP semantics %q", ErrUnsupportedConfiguration, semantics.String())
	}
}

func iceTransportPolicyString(policy webrtc.ICETransportPolicy) (string, error) {
	switch policy {
	case webrtc.ICETransportPolicyAll:
		return "all", nil
	case webrtc.ICETransportPolicyRelay:
		return "relay", nil
	default:
		return "", fmt.Errorf("%w: ICE transport policy %q", ErrUnsupportedConfiguration, policy.String())
	}
}

func sdpTypeToFFI(t webrtc.SDPType) (int, error) {
	switch t {
	case webrtc.SDPTypeOffer:
		return 0, nil
	case webrtc.SDPTypePranswer:
		return 1, nil
	case webrtc.SDPTypeAnswer:
		return 2, nil
	case webrtc.SDPTypeRollback:
		return 3, nil
	default:
		return 0, fmt.Errorf("%w: unknown SDP type %q", ErrInvalidState, t.String())
	}
}

func signalingStateFromFFI(state int) webrtc.SignalingState {
	switch state {
	case 0:
		return webrtc.SignalingStateStable
	case 1:
		return webrtc.SignalingStateHaveLocalOffer
	case 2:
		return webrtc.SignalingStateHaveRemoteOffer
	case 3:
		return webrtc.SignalingStateHaveLocalPranswer
	case 4:
		return webrtc.SignalingStateHaveRemotePranswer
	case 5:
		return webrtc.SignalingStateClosed
	default:
		return webrtc.SignalingStateUnknown
	}
}

func iceConnectionStateFromFFI(state int) webrtc.ICEConnectionState {
	switch state {
	case 0:
		return webrtc.ICEConnectionStateNew
	case 1:
		return webrtc.ICEConnectionStateChecking
	case 2:
		return webrtc.ICEConnectionStateConnected
	case 3:
		return webrtc.ICEConnectionStateCompleted
	case 4:
		return webrtc.ICEConnectionStateDisconnected
	case 5:
		return webrtc.ICEConnectionStateFailed
	case 6:
		return webrtc.ICEConnectionStateClosed
	default:
		return webrtc.ICEConnectionStateUnknown
	}
}

func iceGathererStateFromFFI(state int) webrtc.ICEGathererState {
	switch state {
	case 0:
		return webrtc.ICEGathererStateNew
	case 1:
		return webrtc.ICEGathererStateGathering
	case 2:
		return webrtc.ICEGathererStateComplete
	case 3:
		return webrtc.ICEGathererStateClosed
	default:
		return webrtc.ICEGathererStateUnknown
	}
}

func peerConnectionStateFromFFI(state int) webrtc.PeerConnectionState {
	switch state {
	case 0:
		return webrtc.PeerConnectionStateNew
	case 1:
		return webrtc.PeerConnectionStateConnecting
	case 2:
		return webrtc.PeerConnectionStateConnected
	case 3:
		return webrtc.PeerConnectionStateDisconnected
	case 4:
		return webrtc.PeerConnectionStateFailed
	case 5:
		return webrtc.PeerConnectionStateClosed
	default:
		return webrtc.PeerConnectionStateUnknown
	}
}

func dataChannelStateFromFFI(state int) webrtc.DataChannelState {
	switch state {
	case 0:
		return webrtc.DataChannelStateConnecting
	case 1:
		return webrtc.DataChannelStateOpen
	case 2:
		return webrtc.DataChannelStateClosing
	case 3:
		return webrtc.DataChannelStateClosed
	default:
		return webrtc.DataChannelStateUnknown
	}
}

func transceiverDirectionFromFFI(direction ffi.TransceiverDirection) webrtc.RTPTransceiverDirection {
	switch direction {
	case ffi.TransceiverDirectionSendRecv:
		return webrtc.RTPTransceiverDirectionSendrecv
	case ffi.TransceiverDirectionSendOnly:
		return webrtc.RTPTransceiverDirectionSendonly
	case ffi.TransceiverDirectionRecvOnly:
		return webrtc.RTPTransceiverDirectionRecvonly
	case ffi.TransceiverDirectionInactive:
		return webrtc.RTPTransceiverDirectionInactive
	default:
		return webrtc.RTPTransceiverDirectionUnknown
	}
}

func transceiverDirectionToFFI(direction webrtc.RTPTransceiverDirection) (ffi.TransceiverDirection, error) {
	switch direction {
	case webrtc.RTPTransceiverDirectionSendrecv:
		return ffi.TransceiverDirectionSendRecv, nil
	case webrtc.RTPTransceiverDirectionSendonly:
		return ffi.TransceiverDirectionSendOnly, nil
	case webrtc.RTPTransceiverDirectionRecvonly:
		return ffi.TransceiverDirectionRecvOnly, nil
	case webrtc.RTPTransceiverDirectionInactive:
		return ffi.TransceiverDirectionInactive, nil
	default:
		return 0, fmt.Errorf("%w: transceiver direction %q", ErrNotSupported, direction.String())
	}
}

func codecTypeFromString(kind string) (webrtc.RTPCodecType, error) {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case "audio":
		return webrtc.RTPCodecTypeAudio, nil
	case "video":
		return webrtc.RTPCodecTypeVideo, nil
	default:
		return webrtc.RTPCodecType(0), fmt.Errorf("unknown media kind %q", kind)
	}
}

func mediaKindFromCodecType(kind webrtc.RTPCodecType) (ffi.MediaKind, string, error) {
	switch kind {
	case webrtc.RTPCodecTypeAudio:
		return ffi.MediaKindAudio, "audio", nil
	case webrtc.RTPCodecTypeVideo:
		return ffi.MediaKindVideo, "video", nil
	default:
		return 0, "", fmt.Errorf("unsupported RTP codec type %v", kind)
	}
}

func validateEncodingParameters(enc webrtc.RTPEncodingParameters) error {
	if enc.SSRC != 0 || enc.PayloadType != 0 || enc.RTX.SSRC != 0 || enc.FEC.SSRC != 0 {
		return fmt.Errorf("%w: only RID is supported via Pion RTPEncodingParameters in this API surface", ErrUnsupportedRTPEncodingFields)
	}
	return nil
}

func validateSendParameters(params webrtc.RTPSendParameters) error {
	if len(params.Codecs) > 0 {
		return fmt.Errorf("%w: codec mutation via SetParameters is not supported", ErrUnsupportedRTPParameters)
	}
	if len(params.HeaderExtensions) > 0 {
		return fmt.Errorf("%w: header extension mutation via SetParameters is not supported", ErrUnsupportedRTPParameters)
	}
	for _, enc := range params.Encodings {
		if err := validateEncodingParameters(enc); err != nil {
			return err
		}
	}
	return nil
}

func ffiSendParametersFromWebRTC(params webrtc.RTPSendParameters) *ffi.RTPSendParameters {
	ffiEncodings := make([]ffi.RTPEncodingParameters, len(params.Encodings))
	for i, enc := range params.Encodings {
		copy(ffiEncodings[i].RID[:], enc.RID)
	}

	ffiParams := &ffi.RTPSendParameters{
		EncodingCount: int32(len(ffiEncodings)),
	}
	if len(ffiEncodings) > 0 {
		ffiParams.Encodings = ffi.UintptrFromSlice(ffiEncodings)
	}
	return ffiParams
}

func webRTCSendParametersFromFFI(
	encodings []ffi.RTPEncodingParameters,
	count int,
	headerExtensions []ffi.RTPHeaderExtensionParameter,
	headerExtensionCount int,
	codecs []webrtc.RTPCodecParameters,
) webrtc.RTPSendParameters {
	params := webrtc.RTPSendParameters{
		RTPParameters: webrtc.RTPParameters{
			Codecs:           codecs,
			HeaderExtensions: make([]webrtc.RTPHeaderExtensionParameter, headerExtensionCount),
		},
		Encodings: make([]webrtc.RTPEncodingParameters, count),
	}
	for i := 0; i < count; i++ {
		params.Encodings[i] = webrtc.RTPEncodingParameters{
			RTPCodingParameters: webrtc.RTPCodingParameters{
				RID: ffi.ByteArrayToString(encodings[i].RID[:]),
			},
		}
	}
	for i := 0; i < headerExtensionCount; i++ {
		params.HeaderExtensions[i] = webrtc.RTPHeaderExtensionParameter{
			URI: ffi.ByteArrayToString(headerExtensions[i].URI[:]),
			ID:  int(headerExtensions[i].ID),
		}
	}
	return params
}

func iceCandidateInitFromParts(candidate, sdpMid string, sdpMLineIndex int) *webrtc.ICECandidateInit {
	if candidate == "" && sdpMid == "" && sdpMLineIndex < 0 {
		return nil
	}
	out := &webrtc.ICECandidateInit{
		Candidate: candidate,
	}
	if sdpMid != "" {
		out.SDPMid = &sdpMid
	}
	if sdpMLineIndex >= 0 {
		idx := uint16(sdpMLineIndex)
		out.SDPMLineIndex = &idx
	}
	return out
}

func candidateParts(init webrtc.ICECandidateInit) (candidate, sdpMid string, sdpMLineIndex int) {
	sdpMLineIndex = -1
	candidate = init.Candidate
	if init.SDPMid != nil {
		sdpMid = *init.SDPMid
	}
	if init.SDPMLineIndex != nil {
		sdpMLineIndex = int(*init.SDPMLineIndex)
	}
	return candidate, sdpMid, sdpMLineIndex
}

func statsReportFromJSON(data []byte) (webrtc.StatsReport, error) {
	report := make(webrtc.StatsReport)
	if len(data) == 0 {
		return report, nil
	}

	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err == nil {
		for _, raw := range raws {
			stats, id, err := unmarshalStatsEntry(raw)
			if err != nil {
				return nil, err
			}
			if id == "" {
				continue
			}
			report[id] = stats
		}
		return report, nil
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return nil, fmt.Errorf("unmarshal stats report: %w", err)
	}
	for key, raw := range rawMap {
		stats, id, err := unmarshalStatsEntry(raw)
		if err != nil {
			return nil, err
		}
		if id == "" {
			id = key
		}
		report[id] = stats
	}
	return report, nil
}

func unmarshalStatsEntry(raw json.RawMessage) (webrtc.Stats, string, error) {
	stats, err := webrtc.UnmarshalStatsJSON(raw)
	if err != nil {
		return nil, "", fmt.Errorf("unmarshal stats entry: %w", err)
	}
	var idHolder struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &idHolder); err != nil {
		return nil, "", fmt.Errorf("unmarshal stats id: %w", err)
	}
	return stats, idHolder.ID, nil
}
