package pc

import (
	"testing"
)

// TestSignalingState tests that SignalingState.String() returns correct values.
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

// TestICEConnectionState tests that ICEConnectionState.String() returns correct values.
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

// TestICEGatheringState tests that ICEGatheringState.String() returns correct values.
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

// TestPeerConnectionState tests that PeerConnectionState.String() returns correct values.
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

// TestSDPType tests that SDPType.String() returns correct values.
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
