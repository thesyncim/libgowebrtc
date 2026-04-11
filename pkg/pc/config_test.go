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
	cfg := DefaultConfiguration()
	if len(cfg.ICEServers) != 0 {
		t.Fatalf("DefaultConfiguration().ICEServers len = %d, want 0", len(cfg.ICEServers))
	}
	if err := validateConfiguration(cfg); err != nil {
		t.Fatalf("validateConfiguration(DefaultConfiguration()) = %v, want nil", err)
	}
}
