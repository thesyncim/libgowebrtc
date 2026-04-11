package pc

import (
	"errors"
	"strings"
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestNewPeerConnectionRejectsImplicitConfiguration(t *testing.T) {
	_, err := NewPeerConnection(webrtc.Configuration{})
	if err == nil {
		t.Fatal("NewPeerConnection() with zero config error = nil, want unsupported configuration")
	}
	if !errors.Is(err, ErrUnsupportedConfiguration) {
		t.Fatalf("NewPeerConnection() error = %v, want %v", err, ErrUnsupportedConfiguration)
	}
	if !strings.Contains(err.Error(), "bundle policy") {
		t.Fatalf("NewPeerConnection() error = %q, want bundle policy hint", err.Error())
	}
}

func TestValidateConfigurationAllowsDefaultConfiguration(t *testing.T) {
	if err := validateConfiguration(DefaultConfiguration()); err != nil {
		t.Fatalf("validateConfiguration(DefaultConfiguration()) = %v, want nil", err)
	}
}
