package pc

import (
	"errors"
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestBuildFFIConfigAllowsZeroConfiguration(t *testing.T) {
	data, err := buildFFIConfig(&webrtc.Configuration{})
	if err != nil {
		t.Fatalf("buildFFIConfig(zero config) = %v, want nil", err)
	}
	if data.config.BundlePolicy != nil {
		t.Fatal("BundlePolicy should be omitted for zero config")
	}
	if data.config.RTCPMuxPolicy != nil {
		t.Fatal("RTCPMuxPolicy should be omitted for zero config")
	}
	if data.config.ICETransportPolicy != nil {
		if got := cStringFromPtr(data.config.ICETransportPolicy); got != "all" {
			t.Fatalf("ICETransportPolicy = %q, want %q", got, "all")
		}
	}
	if data.config.SDPSemantics != nil {
		if got := cStringFromPtr(data.config.SDPSemantics); got != "unified-plan" {
			t.Fatalf("SDPSemantics = %q, want %q", got, "unified-plan")
		}
	}
}

func TestBuildFFIConfigRejectsUnsupportedConfigurationFields(t *testing.T) {
	tests := []struct {
		name   string
		config webrtc.Configuration
	}{
		{
			name: "peer identity",
			config: webrtc.Configuration{
				PeerIdentity: "peer-1",
			},
		},
		{
			name: "certificates",
			config: webrtc.Configuration{
				Certificates: []webrtc.Certificate{{}},
			},
		},
		{
			name: "always negotiate data channels",
			config: webrtc.Configuration{
				AlwaysNegotiateDataChannels: true,
			},
		},
		{
			name: "oauth credential type",
			config: webrtc.Configuration{
				ICEServers: []webrtc.ICEServer{{
					URLs:           []string{"turn:turn.example.com:3478"},
					Credential:     "token",
					CredentialType: webrtc.ICECredentialTypeOauth,
				}},
			},
		},
		{
			name: "non string credential",
			config: webrtc.Configuration{
				ICEServers: []webrtc.ICEServer{{
					URLs:           []string{"turn:turn.example.com:3478"},
					Credential:     []byte("secret"),
					CredentialType: webrtc.ICECredentialTypePassword,
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildFFIConfig(&tt.config)
			if err == nil {
				t.Fatal("buildFFIConfig() error = nil, want unsupported configuration")
			}
			if !errors.Is(err, ErrUnsupportedConfiguration) {
				t.Fatalf("buildFFIConfig() error = %v, want %v", err, ErrUnsupportedConfiguration)
			}
		})
	}
}
