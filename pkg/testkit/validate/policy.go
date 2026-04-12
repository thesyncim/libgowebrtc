package validate

import (
	"time"
)

const (
	defaultStatsPollInterval = 250 * time.Millisecond
	defaultVideoFreeze       = 450 * time.Millisecond
	defaultPacketGap         = 250 * time.Millisecond
	defaultAudioFreeze       = 150 * time.Millisecond
	defaultRecoveryTimeout   = 1500 * time.Millisecond
	defaultHeartbeatInterval = 2 * time.Second
	defaultHeartbeatTimeout  = 6 * time.Second
)

// AssertionPolicy declares which optional validation assertions and scenario
// controls are meaningful for a specific test session.
type AssertionPolicy struct {
	CodecSwitch  bool
	RID          bool
	VideoLayer   bool
	LayerControl bool
}

// DefaultSessionConfig returns the package's baseline timing defaults with all
// optional assertions disabled until the caller opts into them.
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		StatsPollInterval:    defaultStatsPollInterval,
		VideoFreezeThreshold: defaultVideoFreeze,
		PacketGapThreshold:   defaultPacketGap,
		AudioFreezeThreshold: defaultAudioFreeze,
		RecoveryTimeout:      defaultRecoveryTimeout,
		HeartbeatInterval:    defaultHeartbeatInterval,
		HeartbeatTimeout:     defaultHeartbeatTimeout,
	}
}

func normalizeSessionConfig(cfg SessionConfig) SessionConfig {
	if cfg.StatsPollInterval <= 0 {
		cfg.StatsPollInterval = defaultStatsPollInterval
	}
	if cfg.VideoFreezeThreshold < 0 {
		cfg.VideoFreezeThreshold = 0
	}
	if cfg.PacketGapThreshold < 0 {
		cfg.PacketGapThreshold = 0
	}
	if cfg.AudioFreezeThreshold < 0 {
		cfg.AudioFreezeThreshold = 0
	}
	if cfg.VideoFreezeThreshold == 0 {
		cfg.VideoFreezeThreshold = defaultVideoFreeze
	}
	if cfg.PacketGapThreshold == 0 {
		cfg.PacketGapThreshold = defaultPacketGap
	}
	if cfg.AudioFreezeThreshold == 0 {
		cfg.AudioFreezeThreshold = defaultAudioFreeze
	}
	if cfg.RecoveryTimeout <= 0 {
		cfg.RecoveryTimeout = defaultRecoveryTimeout
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = defaultHeartbeatInterval
	}
	if cfg.HeartbeatTimeout <= 0 {
		cfg.HeartbeatTimeout = defaultHeartbeatTimeout
	}
	return cfg
}
