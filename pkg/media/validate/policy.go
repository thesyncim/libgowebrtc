package validate

import (
	"time"

	"github.com/thesyncim/libgowebrtc/pkg/pioncodec"
)

// BrowserPolicy captures browser-shaped defaults and feature gates used by the
// validation suite.
type BrowserPolicy struct {
	Browser                       pioncodec.Browser
	SupportsSimulcast             bool
	SupportsDependencyDescriptor  bool
	SupportsLayeredVP9            bool
	SupportsLayeredAV1            bool
	SupportsRID                   bool
	SupportsCodecSwitchAssertions bool

	DefaultStatsPollInterval       time.Duration
	DefaultFreezeThreshold         time.Duration
	DefaultPacketGapThreshold      time.Duration
	DefaultAudioGapThreshold       time.Duration
	DefaultSwitchRecoveryThreshold time.Duration
	DefaultHeartbeatInterval       time.Duration
	DefaultHeartbeatTimeout        time.Duration
}

// PolicyForBrowser returns the browser-shaped policy used by the validator.
func PolicyForBrowser(browser pioncodec.Browser) BrowserPolicy {
	switch browser {
	case pioncodec.BrowserFirefox:
		return BrowserPolicy{
			Browser:                        pioncodec.BrowserFirefox,
			SupportsSimulcast:              true,
			SupportsDependencyDescriptor:   false,
			SupportsLayeredVP9:             false,
			SupportsLayeredAV1:             false,
			SupportsRID:                    true,
			SupportsCodecSwitchAssertions:  true,
			DefaultStatsPollInterval:       250 * time.Millisecond,
			DefaultFreezeThreshold:         500 * time.Millisecond,
			DefaultPacketGapThreshold:      300 * time.Millisecond,
			DefaultAudioGapThreshold:       180 * time.Millisecond,
			DefaultSwitchRecoveryThreshold: 1750 * time.Millisecond,
			DefaultHeartbeatInterval:       2 * time.Second,
			DefaultHeartbeatTimeout:        6 * time.Second,
		}
	case pioncodec.BrowserSafari:
		return BrowserPolicy{
			Browser:                        pioncodec.BrowserSafari,
			SupportsSimulcast:              false,
			SupportsDependencyDescriptor:   false,
			SupportsLayeredVP9:             false,
			SupportsLayeredAV1:             false,
			SupportsRID:                    false,
			SupportsCodecSwitchAssertions:  false,
			DefaultStatsPollInterval:       300 * time.Millisecond,
			DefaultFreezeThreshold:         600 * time.Millisecond,
			DefaultPacketGapThreshold:      350 * time.Millisecond,
			DefaultAudioGapThreshold:       200 * time.Millisecond,
			DefaultSwitchRecoveryThreshold: 2 * time.Second,
			DefaultHeartbeatInterval:       2 * time.Second,
			DefaultHeartbeatTimeout:        6 * time.Second,
		}
	default:
		return BrowserPolicy{
			Browser:                        pioncodec.BrowserChrome,
			SupportsSimulcast:              true,
			SupportsDependencyDescriptor:   true,
			SupportsLayeredVP9:             true,
			SupportsLayeredAV1:             true,
			SupportsRID:                    true,
			SupportsCodecSwitchAssertions:  true,
			DefaultStatsPollInterval:       250 * time.Millisecond,
			DefaultFreezeThreshold:         450 * time.Millisecond,
			DefaultPacketGapThreshold:      250 * time.Millisecond,
			DefaultAudioGapThreshold:       150 * time.Millisecond,
			DefaultSwitchRecoveryThreshold: 1500 * time.Millisecond,
			DefaultHeartbeatInterval:       2 * time.Second,
			DefaultHeartbeatTimeout:        6 * time.Second,
		}
	}
}

func normalizeSessionConfig(cfg SessionConfig) (SessionConfig, BrowserPolicy) {
	if cfg.Browser == "" {
		cfg.Browser = pioncodec.BrowserChrome
	}
	policy := PolicyForBrowser(cfg.Browser)
	if cfg.StatsPollInterval <= 0 {
		cfg.StatsPollInterval = policy.DefaultStatsPollInterval
	}
	if cfg.EventHistory <= 0 {
		cfg.EventHistory = 32
	}
	if cfg.FreezeThreshold < 0 {
		cfg.FreezeThreshold = 0
	}
	if cfg.PacketGapThreshold < 0 {
		cfg.PacketGapThreshold = 0
	}
	if cfg.AudioGapThreshold < 0 {
		cfg.AudioGapThreshold = 0
	}
	if cfg.SwitchRecoveryThreshold <= 0 {
		cfg.SwitchRecoveryThreshold = policy.DefaultSwitchRecoveryThreshold
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = policy.DefaultHeartbeatInterval
	}
	if cfg.HeartbeatTimeout <= 0 {
		cfg.HeartbeatTimeout = policy.DefaultHeartbeatTimeout
	}
	return cfg, policy
}
