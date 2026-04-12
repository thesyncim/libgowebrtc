package validate

import (
	"time"
)

// Profile identifies the validation capability envelope the session should
// assume when deciding what assertions are meaningful.
type Profile string

const (
	ProfileChrome  Profile = "chrome"
	ProfileFirefox Profile = "firefox"
	ProfileSafari  Profile = "safari"
)

// ProfilePolicy captures profile-specific defaults and feature gates used by
// the validation suite.
type ProfilePolicy struct {
	Profile                       Profile
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

// PolicyForProfile returns the policy used by the validator for the requested
// validation profile.
func PolicyForProfile(profile Profile) ProfilePolicy {
	switch profile {
	case ProfileFirefox:
		return ProfilePolicy{
			Profile:                        ProfileFirefox,
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
	case ProfileSafari:
		return ProfilePolicy{
			Profile:                        ProfileSafari,
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
		return ProfilePolicy{
			Profile:                        ProfileChrome,
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

func normalizeSessionConfig(cfg SessionConfig) (SessionConfig, ProfilePolicy) {
	if cfg.Profile == "" {
		cfg.Profile = ProfileChrome
	}
	policy := PolicyForProfile(cfg.Profile)
	if cfg.StatsPollInterval <= 0 {
		cfg.StatsPollInterval = policy.DefaultStatsPollInterval
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
