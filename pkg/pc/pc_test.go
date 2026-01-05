package pc

import (
	"testing"
)

func TestPCErrors(t *testing.T) {
	errors := []error{
		ErrPeerConnectionClosed,
		ErrInvalidState,
		ErrCreateOfferFailed,
		ErrCreateAnswerFailed,
		ErrSetDescriptionFailed,
		ErrAddICECandidateFailed,
		ErrTrackNotFound,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("Error should not be nil")
		}
		if err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	}
}

func TestSignalingState(t *testing.T) {
	tests := []struct {
		state SignalingState
		str   string
	}{
		{SignalingStateStable, "stable"},
		{SignalingStateHaveLocalOffer, "have-local-offer"},
		{SignalingStateHaveRemoteOffer, "have-remote-offer"},
		{SignalingStateHaveLocalPranswer, "have-local-pranswer"},
		{SignalingStateHaveRemotePranswer, "have-remote-pranswer"},
		{SignalingStateClosed, "closed"},
		{SignalingState(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			if got := tt.state.String(); got != tt.str {
				t.Errorf("SignalingState.String() = %v, want %v", got, tt.str)
			}
		})
	}
}

func TestICEConnectionState(t *testing.T) {
	tests := []struct {
		state ICEConnectionState
		str   string
	}{
		{ICEConnectionStateNew, "new"},
		{ICEConnectionStateChecking, "checking"},
		{ICEConnectionStateConnected, "connected"},
		{ICEConnectionStateCompleted, "completed"},
		{ICEConnectionStateDisconnected, "disconnected"},
		{ICEConnectionStateFailed, "failed"},
		{ICEConnectionStateClosed, "closed"},
		{ICEConnectionState(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			if got := tt.state.String(); got != tt.str {
				t.Errorf("ICEConnectionState.String() = %v, want %v", got, tt.str)
			}
		})
	}
}

func TestICEGatheringState(t *testing.T) {
	tests := []struct {
		state ICEGatheringState
		str   string
	}{
		{ICEGatheringStateNew, "new"},
		{ICEGatheringStateGathering, "gathering"},
		{ICEGatheringStateComplete, "complete"},
		{ICEGatheringState(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			if got := tt.state.String(); got != tt.str {
				t.Errorf("ICEGatheringState.String() = %v, want %v", got, tt.str)
			}
		})
	}
}

func TestPeerConnectionState(t *testing.T) {
	tests := []struct {
		state PeerConnectionState
		str   string
	}{
		{PeerConnectionStateNew, "new"},
		{PeerConnectionStateConnecting, "connecting"},
		{PeerConnectionStateConnected, "connected"},
		{PeerConnectionStateDisconnected, "disconnected"},
		{PeerConnectionStateFailed, "failed"},
		{PeerConnectionStateClosed, "closed"},
		{PeerConnectionState(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			if got := tt.state.String(); got != tt.str {
				t.Errorf("PeerConnectionState.String() = %v, want %v", got, tt.str)
			}
		})
	}
}

func TestSDPType(t *testing.T) {
	tests := []struct {
		sdpType SDPType
		str     string
	}{
		{SDPTypeOffer, "offer"},
		{SDPTypePranswer, "pranswer"},
		{SDPTypeAnswer, "answer"},
		{SDPTypeRollback, "rollback"},
		{SDPType(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			if got := tt.sdpType.String(); got != tt.str {
				t.Errorf("SDPType.String() = %v, want %v", got, tt.str)
			}
		})
	}
}

func TestSessionDescription(t *testing.T) {
	sd := SessionDescription{
		Type: SDPTypeOffer,
		SDP:  "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n",
	}

	if sd.Type != SDPTypeOffer {
		t.Errorf("Type = %v, want offer", sd.Type)
	}
	if sd.SDP == "" {
		t.Error("SDP should not be empty")
	}
}

func TestICECandidate(t *testing.T) {
	candidate := ICECandidate{
		Candidate:        "candidate:1 1 UDP 2130706431 192.168.1.1 12345 typ host",
		SDPMid:           "0",
		SDPMLineIndex:    0,
		UsernameFragment: "abcd",
	}

	if candidate.Candidate == "" {
		t.Error("Candidate should not be empty")
	}
	if candidate.SDPMid != "0" {
		t.Errorf("SDPMid = %v, want 0", candidate.SDPMid)
	}
}

func TestICEServer(t *testing.T) {
	server := ICEServer{
		URLs:       []string{"stun:stun.l.google.com:19302"},
		Username:   "user",
		Credential: "pass",
	}

	if len(server.URLs) != 1 {
		t.Errorf("URLs len = %v, want 1", len(server.URLs))
	}
	if server.URLs[0] != "stun:stun.l.google.com:19302" {
		t.Errorf("URLs[0] = %v, want stun:stun.l.google.com:19302", server.URLs[0])
	}
}

func TestConfiguration(t *testing.T) {
	cfg := Configuration{
		ICEServers: []ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
			{
				URLs:       []string{"turn:turn.example.com:3478"},
				Username:   "user",
				Credential: "pass",
			},
		},
		ICETransportPolicy:   "all",
		BundlePolicy:         "max-bundle",
		RTCPMuxPolicy:        "require",
		SDPSemantics:         "unified-plan",
		ICECandidatePoolSize: 10,
	}

	if len(cfg.ICEServers) != 2 {
		t.Errorf("ICEServers len = %v, want 2", len(cfg.ICEServers))
	}
	if cfg.ICETransportPolicy != "all" {
		t.Errorf("ICETransportPolicy = %v, want all", cfg.ICETransportPolicy)
	}
	if cfg.BundlePolicy != "max-bundle" {
		t.Errorf("BundlePolicy = %v, want max-bundle", cfg.BundlePolicy)
	}
	if cfg.SDPSemantics != "unified-plan" {
		t.Errorf("SDPSemantics = %v, want unified-plan", cfg.SDPSemantics)
	}
	if cfg.ICECandidatePoolSize != 10 {
		t.Errorf("ICECandidatePoolSize = %v, want 10", cfg.ICECandidatePoolSize)
	}
}

func TestConfigurationDefaults(t *testing.T) {
	// Empty config should work
	cfg := Configuration{}

	if len(cfg.ICEServers) != 0 {
		t.Errorf("ICEServers default len = %v, want 0", len(cfg.ICEServers))
	}
	if cfg.ICETransportPolicy != "" {
		t.Errorf("ICETransportPolicy default = %v, want empty", cfg.ICETransportPolicy)
	}
}

func TestICEServerMultipleURLs(t *testing.T) {
	server := ICEServer{
		URLs: []string{
			"stun:stun1.l.google.com:19302",
			"stun:stun2.l.google.com:19302",
			"stun:stun3.l.google.com:19302",
		},
	}

	if len(server.URLs) != 3 {
		t.Errorf("URLs len = %v, want 3", len(server.URLs))
	}
}

func TestSignalingStateValues(t *testing.T) {
	// Test that enum values are sequential starting from 0
	if SignalingStateStable != 0 {
		t.Error("SignalingStateStable should be 0")
	}
	if SignalingStateClosed != 5 {
		t.Error("SignalingStateClosed should be 5")
	}
}

func TestICEConnectionStateValues(t *testing.T) {
	if ICEConnectionStateNew != 0 {
		t.Error("ICEConnectionStateNew should be 0")
	}
	if ICEConnectionStateClosed != 6 {
		t.Error("ICEConnectionStateClosed should be 6")
	}
}

func TestPeerConnectionStateValues(t *testing.T) {
	if PeerConnectionStateNew != 0 {
		t.Error("PeerConnectionStateNew should be 0")
	}
	if PeerConnectionStateClosed != 5 {
		t.Error("PeerConnectionStateClosed should be 5")
	}
}

func BenchmarkSignalingStateString(b *testing.B) {
	state := SignalingStateHaveLocalOffer
	for i := 0; i < b.N; i++ {
		_ = state.String()
	}
}

func BenchmarkICEConnectionStateString(b *testing.B) {
	state := ICEConnectionStateConnected
	for i := 0; i < b.N; i++ {
		_ = state.String()
	}
}

func BenchmarkPeerConnectionStateString(b *testing.B) {
	state := PeerConnectionStateConnected
	for i := 0; i < b.N; i++ {
		_ = state.String()
	}
}

func BenchmarkConfigurationAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cfg := Configuration{
			ICEServers: []ICEServer{
				{URLs: []string{"stun:stun.l.google.com:19302"}},
				{
					URLs:       []string{"turn:turn.example.com:3478"},
					Username:   "user",
					Credential: "pass",
				},
			},
			ICETransportPolicy:   "all",
			BundlePolicy:         "max-bundle",
			RTCPMuxPolicy:        "require",
			SDPSemantics:         "unified-plan",
			ICECandidatePoolSize: 10,
		}
		_ = cfg
	}
}

func BenchmarkSessionDescriptionAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sd := SessionDescription{
			Type: SDPTypeOffer,
			SDP:  "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n",
		}
		_ = sd
	}
}
