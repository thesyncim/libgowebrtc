// Package codec defines codec types and configurations for libgowebrtc.
package codec

// Type represents a video or audio codec type.
type Type int

const (
	// Video codecs
	H264 Type = iota
	VP8
	VP9
	AV1

	// Audio codecs
	Opus
	PCMU
	PCMA
)

// String returns the string representation of the codec type.
func (t Type) String() string {
	switch t {
	case H264:
		return "H264"
	case VP8:
		return "VP8"
	case VP9:
		return "VP9"
	case AV1:
		return "AV1"
	case Opus:
		return "Opus"
	case PCMU:
		return "PCMU"
	case PCMA:
		return "PCMA"
	default:
		return "Unknown"
	}
}

// MimeType returns the MIME type for the codec.
func (t Type) MimeType() string {
	switch t {
	case H264:
		return "video/H264"
	case VP8:
		return "video/VP8"
	case VP9:
		return "video/VP9"
	case AV1:
		return "video/AV1"
	case Opus:
		return "audio/opus"
	case PCMU:
		return "audio/PCMU"
	case PCMA:
		return "audio/PCMA"
	default:
		return ""
	}
}

// IsVideo returns true if this is a video codec.
func (t Type) IsVideo() bool {
	switch t {
	case H264, VP8, VP9, AV1:
		return true
	default:
		return false
	}
}

// IsAudio returns true if this is an audio codec.
func (t Type) IsAudio() bool {
	switch t {
	case Opus, PCMU, PCMA:
		return true
	default:
		return false
	}
}

// ClockRate returns the RTP clock rate for the codec.
func (t Type) ClockRate() uint32 {
	switch t {
	case H264, VP8, VP9, AV1:
		return 90000
	case Opus:
		return 48000
	case PCMU, PCMA:
		return 8000
	default:
		return 0
	}
}

// RateControlMode specifies the encoder rate control strategy.
type RateControlMode int

const (
	RateControlCBR RateControlMode = iota // Constant bitrate
	RateControlVBR                        // Variable bitrate
	RateControlCQ                         // Constant quality (CRF/CQ mode)
)

// SVCMode specifies the scalable video coding mode.
// L = spatial Layers, T = Temporal layers, S = Simulcast, K = Key-frame dependent
type SVCMode int

const (
	SVCModeNone SVCMode = iota // No SVC

	// Standard SVC (inter-layer prediction)
	SVCModeL1T1 // 1 spatial, 1 temporal layer (no SVC)
	SVCModeL1T2 // 1 spatial, 2 temporal layers
	SVCModeL1T3 // 1 spatial, 3 temporal layers
	SVCModeL2T1 // 2 spatial, 1 temporal layer
	SVCModeL2T2 // 2 spatial, 2 temporal layers
	SVCModeL2T3 // 2 spatial, 3 temporal layers
	SVCModeL3T1 // 3 spatial, 1 temporal layer
	SVCModeL3T2 // 3 spatial, 2 temporal layers
	SVCModeL3T3 // 3 spatial, 3 temporal layers (full SVC)

	// K-SVC (Key-frame dependent, no inter-layer prediction - better for SFU)
	SVCModeL1T2_KEY // 1 spatial, 2 temporal (key-frame dependent)
	SVCModeL1T3_KEY // 1 spatial, 3 temporal (key-frame dependent)
	SVCModeL2T1_KEY // 2 spatial, 1 temporal (key-frame dependent)
	SVCModeL2T2_KEY // 2 spatial, 2 temporal (key-frame dependent)
	SVCModeL2T3_KEY // 2 spatial, 3 temporal (key-frame dependent)
	SVCModeL3T1_KEY // 3 spatial, 1 temporal (key-frame dependent)
	SVCModeL3T2_KEY // 3 spatial, 2 temporal (key-frame dependent)
	SVCModeL3T3_KEY // 3 spatial, 3 temporal (key-frame dependent, best for SFU)

	// Simulcast (separate independent encoders)
	SVCModeS2T1 // 2 simulcast streams, 1 temporal
	SVCModeS2T3 // 2 simulcast streams, 3 temporal
	SVCModeS3T1 // 3 simulcast streams, 1 temporal
	SVCModeS3T3 // 3 simulcast streams, 3 temporal (full simulcast)
)

// svcInfo holds metadata for each SVC mode.
type svcInfo struct {
	name      string
	spatial   int
	temporal  int
	simulcast bool
	keyDep    bool
}

// svcModeTable maps SVC modes to their metadata.
var svcModeTable = map[SVCMode]svcInfo{
	SVCModeNone: {"none", 1, 1, false, false},

	// Standard SVC
	SVCModeL1T1: {"L1T1", 1, 1, false, false},
	SVCModeL1T2: {"L1T2", 1, 2, false, false},
	SVCModeL1T3: {"L1T3", 1, 3, false, false},
	SVCModeL2T1: {"L2T1", 2, 1, false, false},
	SVCModeL2T2: {"L2T2", 2, 2, false, false},
	SVCModeL2T3: {"L2T3", 2, 3, false, false},
	SVCModeL3T1: {"L3T1", 3, 1, false, false},
	SVCModeL3T2: {"L3T2", 3, 2, false, false},
	SVCModeL3T3: {"L3T3", 3, 3, false, false},

	// K-SVC (key-frame dependent)
	SVCModeL1T2_KEY: {"L1T2_KEY", 1, 2, false, true},
	SVCModeL1T3_KEY: {"L1T3_KEY", 1, 3, false, true},
	SVCModeL2T1_KEY: {"L2T1_KEY", 2, 1, false, true},
	SVCModeL2T2_KEY: {"L2T2_KEY", 2, 2, false, true},
	SVCModeL2T3_KEY: {"L2T3_KEY", 2, 3, false, true},
	SVCModeL3T1_KEY: {"L3T1_KEY", 3, 1, false, true},
	SVCModeL3T2_KEY: {"L3T2_KEY", 3, 2, false, true},
	SVCModeL3T3_KEY: {"L3T3_KEY", 3, 3, false, true},

	// Simulcast
	SVCModeS2T1: {"S2T1", 2, 1, true, false},
	SVCModeS2T3: {"S2T3", 2, 3, true, false},
	SVCModeS3T1: {"S3T1", 3, 1, true, false},
	SVCModeS3T3: {"S3T3", 3, 3, true, false},
}

// String returns string representation of SVC mode.
func (m SVCMode) String() string {
	if info, ok := svcModeTable[m]; ok {
		return info.name
	}
	return "unknown"
}

// SpatialLayers returns number of spatial layers.
func (m SVCMode) SpatialLayers() int {
	if info, ok := svcModeTable[m]; ok {
		return info.spatial
	}
	return 1
}

// TemporalLayers returns number of temporal layers.
func (m SVCMode) TemporalLayers() int {
	if info, ok := svcModeTable[m]; ok {
		return info.temporal
	}
	return 1
}

// IsSimulcast returns true if this is a simulcast mode (separate encoders).
func (m SVCMode) IsSimulcast() bool {
	if info, ok := svcModeTable[m]; ok {
		return info.simulcast
	}
	return false
}

// IsKeyFrameDependent returns true if this is K-SVC mode (no inter-layer prediction).
func (m SVCMode) IsKeyFrameDependent() bool {
	if info, ok := svcModeTable[m]; ok {
		return info.keyDep
	}
	return false
}

// SVCLayerConfig configures a single SVC/simulcast layer.
type SVCLayerConfig struct {
	Width      int     // Resolution width (0 = derive from base)
	Height     int     // Resolution height (0 = derive from base)
	Bitrate    uint32  // Target bitrate for this layer
	MaxBitrate uint32  // Max bitrate for this layer
	FPS        float64 // Framerate for this layer (0 = same as base)
	Active     bool    // Whether this layer is active
}

// SVCConfig configures scalable video coding.
type SVCConfig struct {
	Mode   SVCMode          // SVC/simulcast mode
	Layers []SVCLayerConfig // Per-layer configuration (optional, auto if nil)
}

// SVC Presets

// SVCPresetNone returns no SVC configuration.
func SVCPresetNone() *SVCConfig {
	return nil
}

// SVCPresetScreenShare returns optimal SVC for screen sharing (temporal only).
// Uses L1T3 for smooth motion at varying framerates.
func SVCPresetScreenShare() *SVCConfig {
	return &SVCConfig{Mode: SVCModeL1T3}
}

// SVCPresetLowLatency returns optimal SVC for low-latency use cases.
// Uses L1T2 temporal layers for bandwidth adaptation without spatial complexity.
func SVCPresetLowLatency() *SVCConfig {
	return &SVCConfig{Mode: SVCModeL1T2}
}

// SVCPresetSFU returns optimal SVC for SFU/media server usage.
// Uses L3T3_KEY (K-SVC) - best for selective forwarding without transcoding.
func SVCPresetSFU() *SVCConfig {
	return &SVCConfig{Mode: SVCModeL3T3_KEY}
}

// SVCPresetSFULite returns lighter SVC for SFU with 2 spatial layers.
// Uses L2T3_KEY - good balance of adaptability and complexity.
func SVCPresetSFULite() *SVCConfig {
	return &SVCConfig{Mode: SVCModeL2T3_KEY}
}

// SVCPresetSimulcast returns classic simulcast (3 independent streams).
// Best compatibility, highest bandwidth usage.
func SVCPresetSimulcast() *SVCConfig {
	return &SVCConfig{Mode: SVCModeS3T3}
}

// SVCPresetSimulcastLite returns 2-stream simulcast.
// Good for mobile or bandwidth-constrained scenarios.
func SVCPresetSimulcastLite() *SVCConfig {
	return &SVCConfig{Mode: SVCModeS2T1}
}

// H264Profile represents H.264 profile levels.
type H264Profile string

const (
	H264ProfileBaseline        H264Profile = "42001f" // Baseline Level 3.1
	H264ProfileConstrainedBase H264Profile = "42e01f" // Constrained Baseline Level 3.1
	H264ProfileMain            H264Profile = "4d001f" // Main Level 3.1
	H264ProfileHigh            H264Profile = "64001f" // High Level 3.1
	H264ProfileHigh10          H264Profile = "6e001f" // High 10 Level 3.1
)

// VP9Profile represents VP9 profiles.
type VP9Profile int

const (
	VP9Profile0 VP9Profile = 0 // 8-bit 4:2:0
	VP9Profile1 VP9Profile = 1 // 8-bit 4:2:2/4:4:4
	VP9Profile2 VP9Profile = 2 // 10/12-bit 4:2:0
	VP9Profile3 VP9Profile = 3 // 10/12-bit 4:2:2/4:4:4
)

// AV1Profile represents AV1 profiles.
type AV1Profile int

const (
	AV1ProfileMain         AV1Profile = 0 // 8/10-bit 4:2:0
	AV1ProfileHigh         AV1Profile = 1 // 8/10-bit 4:4:4
	AV1ProfileProfessional AV1Profile = 2 // 8/10/12-bit, all subsampling
)

// VideoEncoderConfig is the unified video encoder configuration for all codecs.
type VideoEncoderConfig struct {
	// Codec type (required)
	Codec Type

	// Resolution (required)
	Width  int
	Height int

	// Bitrate control
	Bitrate     uint32          // Target bitrate in bps (0 = auto based on resolution)
	MaxBitrate  uint32          // Max bitrate for VBR mode (0 = 1.5x Bitrate)
	MinBitrate  uint32          // Min bitrate for VBR mode (0 = 0.5x Bitrate)
	RateControl RateControlMode // CBR, VBR, or CQ

	// Quality
	FPS         float64 // Target framerate (0 = 30)
	KeyInterval int     // Keyframe interval in frames (0 = 2 seconds worth)
	CQ          int     // Quality level for CQ mode (0-63, lower = better)

	// Performance
	Threads  int  // Encoding threads (0 = auto)
	LowDelay bool // Optimize for low latency
	PreferHW bool // Prefer hardware encoder if available

	// H264-specific
	Profile     H264Profile // H.264 profile (empty = ConstrainedBaseline)
	Level       string      // H.264 level (empty = auto)
	ZeroLatency bool        // Ultra low latency mode (H264: disables B-frames, lookahead)

	// VP8-specific
	ErrorResilient bool // Enable error resilience features (VP8)
	Deadline       int  // Encoding deadline: 0=best, 1=good, 2=realtime (VP8)

	// VP9-specific
	VP9Profile VP9Profile // VP9 profile (0-3)

	// AV1-specific
	AV1Profile    AV1Profile // AV1 profile
	ScreenContent bool       // Optimize for screen content (AV1)

	// VP9/AV1 shared
	Speed         int  // Encoding speed (VP9: 0-9, AV1: 0-10, higher = faster)
	TileColumns   int  // Tile columns (log2)
	TileRows      int  // Tile rows (log2)
	FrameParallel bool // Enable frame parallel decoding

	// SVC/Simulcast (VP9/AV1 native SVC, H264 simulcast only)
	SVC *SVCConfig
}

// FPSOrDefault returns FPS or default value.
func (c VideoEncoderConfig) FPSOrDefault() float64 {
	if c.FPS <= 0 {
		return 30
	}
	return c.FPS
}

// DefaultVideoEncoderConfig returns sensible defaults for any video codec.
func DefaultVideoEncoderConfig(codec Type, width, height int) VideoEncoderConfig {
	cfg := VideoEncoderConfig{
		Codec:       codec,
		Width:       width,
		Height:      height,
		Bitrate:     estimateVideoBitrate(width, height),
		RateControl: RateControlVBR,
		FPS:         30,
		KeyInterval: 60, // 2 seconds at 30fps
		LowDelay:    true,
		PreferHW:    defaultPreferHW(),
	}

	// Codec-specific defaults
	switch codec {
	case H264:
		cfg.Profile = H264ProfileConstrainedBase
		cfg.PreferHW = defaultPreferHWH264()
	case VP8:
		cfg.Deadline = 2 // realtime
		cfg.ErrorResilient = true
	case VP9:
		cfg.VP9Profile = VP9Profile0
		cfg.Speed = 6
	case AV1:
		cfg.AV1Profile = AV1ProfileMain
		cfg.Speed = 8 // AV1 is slow, use faster preset
	}

	return cfg
}


// OpusApplication specifies the Opus encoder application type.
type OpusApplication int

const (
	OpusApplicationVoIP     OpusApplication = 2048 // Voice over IP (speech)
	OpusApplicationAudio    OpusApplication = 2049 // Audio (music, mixed content)
	OpusApplicationLowDelay OpusApplication = 2051 // Low delay audio
)

// OpusBandwidth specifies the audio bandwidth.
type OpusBandwidth int

const (
	OpusBandwidthAuto      OpusBandwidth = -1000 // Auto-detect
	OpusBandwidthNarrow    OpusBandwidth = 1101  // 4kHz (narrowband)
	OpusBandwidthMedium    OpusBandwidth = 1102  // 6kHz (medium band)
	OpusBandwidthWide      OpusBandwidth = 1103  // 8kHz (wideband)
	OpusBandwidthSuperWide OpusBandwidth = 1104  // 12kHz (super wideband)
	OpusBandwidthFull      OpusBandwidth = 1105  // 20kHz (fullband)
)

// OpusConfig contains Opus encoder configuration.
type OpusConfig struct {
	// Audio format
	SampleRate int // 8000, 12000, 16000, 24000, or 48000
	Channels   int // 1 (mono) or 2 (stereo)

	// Bitrate
	Bitrate     uint32 // Target bitrate in bps (6000-510000)
	VBR         bool   // Variable bitrate (default: true)
	Constrained bool   // Constrained VBR (useful for streaming)

	// Quality
	Application OpusApplication // VoIP, Audio, or LowDelay
	Bandwidth   OpusBandwidth   // Audio bandwidth
	Complexity  int             // Encoding complexity (0-10, higher = better quality)
	FrameSize   float64         // Frame size in ms: 2.5, 5, 10, 20, 40, 60

	// Features
	FEC        bool // Forward Error Correction
	DTX        bool // Discontinuous transmission (silence suppression)
	InBandFEC  bool // In-band FEC for packet loss recovery
	PacketLoss int  // Expected packet loss percentage (for FEC tuning)
}

// DefaultOpusConfig returns sensible defaults for Opus.
func DefaultOpusConfig() OpusConfig {
	return OpusConfig{
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     64000,
		VBR:         true,
		Application: OpusApplicationAudio,
		Bandwidth:   OpusBandwidthAuto,
		Complexity:  10,
		FrameSize:   20,
		InBandFEC:   true,
		PacketLoss:  10,
	}
}

// estimateVideoBitrate returns a reasonable bitrate for the resolution.
func estimateVideoBitrate(width, height int) uint32 {
	pixels := width * height
	switch {
	case pixels >= 3840*2160: // 4K
		return 15_000_000
	case pixels >= 2560*1440: // 1440p
		return 8_000_000
	case pixels >= 1920*1080: // 1080p
		return 4_000_000
	case pixels >= 1280*720: // 720p
		return 2_000_000
	case pixels >= 854*480: // 480p
		return 1_000_000
	case pixels >= 640*360: // 360p
		return 500_000
	default:
		return 300_000
	}
}
