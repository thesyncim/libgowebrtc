package codec

import (
	"testing"
)

func TestCodecType(t *testing.T) {
	tests := []struct {
		codec     Type
		str       string
		mime      string
		isVideo   bool
		isAudio   bool
		clockRate uint32
	}{
		{H264, "H264", "video/H264", true, false, 90000},
		{VP8, "VP8", "video/VP8", true, false, 90000},
		{VP9, "VP9", "video/VP9", true, false, 90000},
		{AV1, "AV1", "video/AV1", true, false, 90000},
		{Opus, "Opus", "audio/opus", false, true, 48000},
		{PCMU, "PCMU", "audio/PCMU", false, true, 8000},
		{PCMA, "PCMA", "audio/PCMA", false, true, 8000},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			if got := tt.codec.String(); got != tt.str {
				t.Errorf("String() = %v, want %v", got, tt.str)
			}
			if got := tt.codec.MimeType(); got != tt.mime {
				t.Errorf("MimeType() = %v, want %v", got, tt.mime)
			}
			if got := tt.codec.IsVideo(); got != tt.isVideo {
				t.Errorf("IsVideo() = %v, want %v", got, tt.isVideo)
			}
			if got := tt.codec.IsAudio(); got != tt.isAudio {
				t.Errorf("IsAudio() = %v, want %v", got, tt.isAudio)
			}
			if got := tt.codec.ClockRate(); got != tt.clockRate {
				t.Errorf("ClockRate() = %v, want %v", got, tt.clockRate)
			}
		})
	}
}

func TestSVCMode(t *testing.T) {
	tests := []struct {
		mode           SVCMode
		str            string
		spatialLayers  int
		temporalLayers int
		isSimulcast    bool
		isKeyDep       bool
	}{
		{SVCModeNone, "none", 1, 1, false, false},
		{SVCModeL1T1, "L1T1", 1, 1, false, false},
		{SVCModeL1T2, "L1T2", 1, 2, false, false},
		{SVCModeL1T3, "L1T3", 1, 3, false, false},
		{SVCModeL2T1, "L2T1", 2, 1, false, false},
		{SVCModeL2T2, "L2T2", 2, 2, false, false},
		{SVCModeL2T3, "L2T3", 2, 3, false, false},
		{SVCModeL3T1, "L3T1", 3, 1, false, false},
		{SVCModeL3T2, "L3T2", 3, 2, false, false},
		{SVCModeL3T3, "L3T3", 3, 3, false, false},

		// K-SVC modes
		{SVCModeL1T2_KEY, "L1T2_KEY", 1, 2, false, true},
		{SVCModeL1T3_KEY, "L1T3_KEY", 1, 3, false, true},
		{SVCModeL2T1_KEY, "L2T1_KEY", 2, 1, false, true},
		{SVCModeL2T2_KEY, "L2T2_KEY", 2, 2, false, true},
		{SVCModeL2T3_KEY, "L2T3_KEY", 2, 3, false, true},
		{SVCModeL3T1_KEY, "L3T1_KEY", 3, 1, false, true},
		{SVCModeL3T2_KEY, "L3T2_KEY", 3, 2, false, true},
		{SVCModeL3T3_KEY, "L3T3_KEY", 3, 3, false, true},

		// Simulcast modes
		{SVCModeS2T1, "S2T1", 2, 1, true, false},
		{SVCModeS2T3, "S2T3", 2, 3, true, false},
		{SVCModeS3T1, "S3T1", 3, 1, true, false},
		{SVCModeS3T3, "S3T3", 3, 3, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.str {
				t.Errorf("String() = %v, want %v", got, tt.str)
			}
			if got := tt.mode.SpatialLayers(); got != tt.spatialLayers {
				t.Errorf("SpatialLayers() = %v, want %v", got, tt.spatialLayers)
			}
			if got := tt.mode.TemporalLayers(); got != tt.temporalLayers {
				t.Errorf("TemporalLayers() = %v, want %v", got, tt.temporalLayers)
			}
			if got := tt.mode.IsSimulcast(); got != tt.isSimulcast {
				t.Errorf("IsSimulcast() = %v, want %v", got, tt.isSimulcast)
			}
			if got := tt.mode.IsKeyFrameDependent(); got != tt.isKeyDep {
				t.Errorf("IsKeyFrameDependent() = %v, want %v", got, tt.isKeyDep)
			}
		})
	}
}

func TestSVCPresets(t *testing.T) {
	tests := []struct {
		name   string
		preset func() *SVCConfig
		mode   SVCMode
	}{
		{"SVCPresetNone", SVCPresetNone, SVCModeNone},
		{"SVCPresetScreenShare", SVCPresetScreenShare, SVCModeL1T3},
		{"SVCPresetLowLatency", SVCPresetLowLatency, SVCModeL1T2},
		{"SVCPresetSFU", SVCPresetSFU, SVCModeL3T3_KEY},
		{"SVCPresetSFULite", SVCPresetSFULite, SVCModeL2T3_KEY},
		{"SVCPresetSimulcast", SVCPresetSimulcast, SVCModeS3T3},
		{"SVCPresetSimulcastLite", SVCPresetSimulcastLite, SVCModeS2T1},
		{"SVCPresetChrome", SVCPresetChrome, SVCModeL3T3_KEY},
		{"SVCPresetFirefox", SVCPresetFirefox, SVCModeL2T3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.preset()
			if tt.mode == SVCModeNone {
				if config != nil {
					t.Errorf("%s should return nil, got %v", tt.name, config)
				}
				return
			}
			if config == nil {
				t.Errorf("%s returned nil", tt.name)
				return
			}
			if config.Mode != tt.mode {
				t.Errorf("%s.Mode = %v, want %v", tt.name, config.Mode, tt.mode)
			}
		})
	}
}

func TestDefaultConfigs(t *testing.T) {
	t.Run("DefaultH264Config", func(t *testing.T) {
		cfg := DefaultH264Config(1280, 720)
		if cfg.Width != 1280 {
			t.Errorf("Width = %v, want 1280", cfg.Width)
		}
		if cfg.Height != 720 {
			t.Errorf("Height = %v, want 720", cfg.Height)
		}
		if cfg.Bitrate == 0 {
			t.Error("Bitrate should be auto-calculated")
		}
		if cfg.FPS != 30 {
			t.Errorf("FPS = %v, want 30", cfg.FPS)
		}
		if cfg.Profile != H264ProfileConstrainedBase {
			t.Errorf("Profile = %v, want %v", cfg.Profile, H264ProfileConstrainedBase)
		}
		if !cfg.LowDelay {
			t.Error("LowDelay should be true")
		}
	})

	t.Run("DefaultVP9Config", func(t *testing.T) {
		cfg := DefaultVP9Config(1920, 1080)
		if cfg.Width != 1920 {
			t.Errorf("Width = %v, want 1920", cfg.Width)
		}
		if cfg.Height != 1080 {
			t.Errorf("Height = %v, want 1080", cfg.Height)
		}
		if cfg.Bitrate == 0 {
			t.Error("Bitrate should be auto-calculated")
		}
		if cfg.Profile != VP9Profile0 {
			t.Errorf("Profile = %v, want %v", cfg.Profile, VP9Profile0)
		}
	})

	t.Run("DefaultAV1Config", func(t *testing.T) {
		cfg := DefaultAV1Config(1920, 1080)
		if cfg.Width != 1920 || cfg.Height != 1080 {
			t.Errorf("Resolution = %dx%d, want 1920x1080", cfg.Width, cfg.Height)
		}
		if cfg.Speed != 8 {
			t.Errorf("Speed = %v, want 8 (fast preset for AV1)", cfg.Speed)
		}
	})

	t.Run("DefaultOpusConfig", func(t *testing.T) {
		cfg := DefaultOpusConfig()
		if cfg.SampleRate != 48000 {
			t.Errorf("SampleRate = %v, want 48000", cfg.SampleRate)
		}
		if cfg.Channels != 2 {
			t.Errorf("Channels = %v, want 2", cfg.Channels)
		}
		if cfg.Bitrate != 64000 {
			t.Errorf("Bitrate = %v, want 64000", cfg.Bitrate)
		}
		if !cfg.VBR {
			t.Error("VBR should be true")
		}
		if !cfg.InBandFEC {
			t.Error("InBandFEC should be true")
		}
	})
}

func TestEstimateVideoBitrate(t *testing.T) {
	tests := []struct {
		width, height int
		minExpected   uint32
		maxExpected   uint32
	}{
		{640, 360, 400_000, 600_000},         // 360p
		{854, 480, 800_000, 1_200_000},       // 480p
		{1280, 720, 1_500_000, 2_500_000},    // 720p
		{1920, 1080, 3_000_000, 5_000_000},   // 1080p
		{2560, 1440, 6_000_000, 10_000_000},  // 1440p
		{3840, 2160, 12_000_000, 20_000_000}, // 4K
	}

	for _, tt := range tests {
		bitrate := estimateVideoBitrate(tt.width, tt.height)
		if bitrate < tt.minExpected || bitrate > tt.maxExpected {
			t.Errorf("estimateVideoBitrate(%d, %d) = %d, want between %d and %d",
				tt.width, tt.height, bitrate, tt.minExpected, tt.maxExpected)
		}
	}
}

func TestH264Profiles(t *testing.T) {
	// Verify H264 profile strings are valid hex
	profiles := []H264Profile{
		H264ProfileBaseline,
		H264ProfileConstrainedBase,
		H264ProfileMain,
		H264ProfileHigh,
		H264ProfileHigh10,
	}

	for _, p := range profiles {
		if len(p) != 6 {
			t.Errorf("Profile %s should be 6 hex characters", p)
		}
	}
}

func TestVP9Profiles(t *testing.T) {
	if VP9Profile0 != 0 || VP9Profile1 != 1 || VP9Profile2 != 2 || VP9Profile3 != 3 {
		t.Error("VP9 profiles should be 0-3")
	}
}

func TestAV1Profiles(t *testing.T) {
	if AV1ProfileMain != 0 || AV1ProfileHigh != 1 || AV1ProfileProfessional != 2 {
		t.Error("AV1 profiles should be 0-2")
	}
}

func TestOpusConfig(t *testing.T) {
	tests := []struct {
		name        string
		application OpusApplication
		bandwidth   OpusBandwidth
	}{
		{"VoIP", OpusApplicationVoIP, OpusBandwidthWide},
		{"Audio", OpusApplicationAudio, OpusBandwidthFull},
		{"LowDelay", OpusApplicationLowDelay, OpusBandwidthSuperWide},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := OpusConfig{
				SampleRate:  48000,
				Channels:    2,
				Bitrate:     128000,
				Application: tt.application,
				Bandwidth:   tt.bandwidth,
				Complexity:  10,
			}
			if cfg.Complexity > 10 || cfg.Complexity < 0 {
				t.Error("Complexity should be 0-10")
			}
		})
	}
}
