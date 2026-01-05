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

// String returns string representation of SVC mode.
func (m SVCMode) String() string {
	switch m {
	case SVCModeNone:
		return "none"
	case SVCModeL1T1:
		return "L1T1"
	case SVCModeL1T2:
		return "L1T2"
	case SVCModeL1T3:
		return "L1T3"
	case SVCModeL2T1:
		return "L2T1"
	case SVCModeL2T2:
		return "L2T2"
	case SVCModeL2T3:
		return "L2T3"
	case SVCModeL3T1:
		return "L3T1"
	case SVCModeL3T2:
		return "L3T2"
	case SVCModeL3T3:
		return "L3T3"
	case SVCModeL1T2_KEY:
		return "L1T2_KEY"
	case SVCModeL1T3_KEY:
		return "L1T3_KEY"
	case SVCModeL2T1_KEY:
		return "L2T1_KEY"
	case SVCModeL2T2_KEY:
		return "L2T2_KEY"
	case SVCModeL2T3_KEY:
		return "L2T3_KEY"
	case SVCModeL3T1_KEY:
		return "L3T1_KEY"
	case SVCModeL3T2_KEY:
		return "L3T2_KEY"
	case SVCModeL3T3_KEY:
		return "L3T3_KEY"
	case SVCModeS2T1:
		return "S2T1"
	case SVCModeS2T3:
		return "S2T3"
	case SVCModeS3T1:
		return "S3T1"
	case SVCModeS3T3:
		return "S3T3"
	default:
		return "unknown"
	}
}

// SpatialLayers returns number of spatial layers.
func (m SVCMode) SpatialLayers() int {
	switch m {
	case SVCModeL2T1, SVCModeL2T2, SVCModeL2T3,
		SVCModeL2T1_KEY, SVCModeL2T2_KEY, SVCModeL2T3_KEY,
		SVCModeS2T1, SVCModeS2T3:
		return 2
	case SVCModeL3T1, SVCModeL3T2, SVCModeL3T3,
		SVCModeL3T1_KEY, SVCModeL3T2_KEY, SVCModeL3T3_KEY,
		SVCModeS3T1, SVCModeS3T3:
		return 3
	default:
		return 1
	}
}

// TemporalLayers returns number of temporal layers.
func (m SVCMode) TemporalLayers() int {
	switch m {
	case SVCModeL1T2, SVCModeL2T2, SVCModeL3T2,
		SVCModeL1T2_KEY, SVCModeL2T2_KEY, SVCModeL3T2_KEY:
		return 2
	case SVCModeL1T3, SVCModeL2T3, SVCModeL3T3,
		SVCModeL1T3_KEY, SVCModeL2T3_KEY, SVCModeL3T3_KEY,
		SVCModeS2T3, SVCModeS3T3:
		return 3
	default:
		return 1
	}
}

// IsSimulcast returns true if this is a simulcast mode (separate encoders).
func (m SVCMode) IsSimulcast() bool {
	switch m {
	case SVCModeS2T1, SVCModeS2T3, SVCModeS3T1, SVCModeS3T3:
		return true
	default:
		return false
	}
}

// IsKeyFrameDependent returns true if this is K-SVC mode (no inter-layer prediction).
func (m SVCMode) IsKeyFrameDependent() bool {
	switch m {
	case SVCModeL1T2_KEY, SVCModeL1T3_KEY,
		SVCModeL2T1_KEY, SVCModeL2T2_KEY, SVCModeL2T3_KEY,
		SVCModeL3T1_KEY, SVCModeL3T2_KEY, SVCModeL3T3_KEY:
		return true
	default:
		return false
	}
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

// Browser-like SVC Presets (match Chrome/Firefox defaults)

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

// SVCPresetChrome returns Chrome's default SVC for VP9/AV1.
// L3T3_KEY is Chrome's preferred mode for WebRTC.
func SVCPresetChrome() *SVCConfig {
	return &SVCConfig{Mode: SVCModeL3T3_KEY}
}

// SVCPresetFirefox returns Firefox's typical SVC configuration.
// Firefox tends to use L2T3 with inter-layer prediction.
func SVCPresetFirefox() *SVCConfig {
	return &SVCConfig{Mode: SVCModeL2T3}
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

// H264Config contains H.264 encoder configuration.
type H264Config struct {
	// Required
	Width  int
	Height int

	// Bitrate control
	Bitrate     uint32          // Target bitrate in bps (0 = auto based on resolution)
	MaxBitrate  uint32          // Max bitrate for VBR mode (0 = 1.5x Bitrate)
	MinBitrate  uint32          // Min bitrate for VBR mode (0 = 0.5x Bitrate)
	RateControl RateControlMode // CBR, VBR, or CQ

	// Quality
	FPS         float64     // Target framerate (0 = 30)
	KeyInterval int         // Keyframe interval in frames (0 = 2 seconds worth)
	Profile     H264Profile // H.264 profile (empty = ConstrainedBaseline)
	Level       string      // H.264 level (empty = auto)
	CRF         int         // Quality for CQ mode (0-51, lower = better, 0 = lossless)

	// Performance
	Threads     int  // Encoding threads (0 = auto)
	LowDelay    bool // Optimize for low latency
	ZeroLatency bool // Ultra low latency mode (disables B-frames, lookahead)
	PreferHW    bool // Prefer hardware encoder if available

	// Simulcast (H.264 doesn't support true SVC, only simulcast)
	Simulcast *SVCConfig // Simulcast configuration (nil = disabled)
}

// FPSOrDefault returns FPS or default value.
func (c H264Config) FPSOrDefault() float64 {
	if c.FPS <= 0 {
		return 30
	}
	return c.FPS
}

// VP8Config contains VP8 encoder configuration.
type VP8Config struct {
	// Required
	Width  int
	Height int

	// Bitrate control
	Bitrate     uint32 // Target bitrate in bps
	MaxBitrate  uint32 // Max bitrate for VBR
	RateControl RateControlMode

	// Quality
	FPS         float64 // Target framerate
	KeyInterval int     // Keyframe interval in frames
	CQ          int     // Quality level for CQ mode (0-63, lower = better)
	Deadline    int     // Encoding deadline: 0=best, 1=good, 2=realtime

	// Performance
	Threads        int  // Encoding threads
	LowDelay       bool // Low latency mode
	PreferHW       bool // Prefer hardware encoder
	ErrorResilient bool // Enable error resilience features
}

// VP9Profile represents VP9 profiles.
type VP9Profile int

const (
	VP9Profile0 VP9Profile = 0 // 8-bit 4:2:0
	VP9Profile1 VP9Profile = 1 // 8-bit 4:2:2/4:4:4
	VP9Profile2 VP9Profile = 2 // 10/12-bit 4:2:0
	VP9Profile3 VP9Profile = 3 // 10/12-bit 4:2:2/4:4:4
)

// VP9Config contains VP9 encoder configuration.
type VP9Config struct {
	// Required
	Width  int
	Height int

	// Bitrate control
	Bitrate     uint32 // Target bitrate in bps
	MaxBitrate  uint32 // Max bitrate for VBR
	RateControl RateControlMode

	// Quality
	FPS         float64    // Target framerate
	KeyInterval int        // Keyframe interval in frames
	Profile     VP9Profile // VP9 profile (0-3)
	CQ          int        // Quality level for CQ mode (0-63)
	Speed       int        // Encoding speed (0-9, higher = faster)

	// Features
	Threads       int  // Encoding threads
	TileColumns   int  // Tile columns (log2)
	TileRows      int  // Tile rows (log2)
	FrameParallel bool // Enable frame parallel decoding
	LowDelay      bool // Low latency mode
	PreferHW      bool // Prefer hardware encoder

	// SVC/Simulcast (VP9 has native SVC support)
	SVC *SVCConfig // SVC configuration (nil = disabled, use SVCPreset* helpers)
}

// AV1Profile represents AV1 profiles.
type AV1Profile int

const (
	AV1ProfileMain         AV1Profile = 0 // 8/10-bit 4:2:0
	AV1ProfileHigh         AV1Profile = 1 // 8/10-bit 4:4:4
	AV1ProfileProfessional AV1Profile = 2 // 8/10/12-bit, all subsampling
)

// AV1Config contains AV1 encoder configuration.
type AV1Config struct {
	// Required
	Width  int
	Height int

	// Bitrate control
	Bitrate     uint32 // Target bitrate in bps
	MaxBitrate  uint32 // Max bitrate for VBR
	RateControl RateControlMode

	// Quality
	FPS         float64    // Target framerate
	KeyInterval int        // Keyframe interval in frames
	Profile     AV1Profile // AV1 profile
	CQ          int        // Quality level for CQ mode (0-63)
	Speed       int        // Encoding speed (0-10, higher = faster)

	// Features
	Threads       int  // Encoding threads
	TileColumns   int  // Tile columns (log2)
	TileRows      int  // Tile rows (log2)
	FrameParallel bool // Enable frame parallel features
	LowDelay      bool // Low latency mode
	PreferHW      bool // Prefer hardware encoder
	ScreenContent bool // Optimize for screen content

	// SVC/Simulcast (AV1 has excellent native SVC support)
	SVC *SVCConfig // SVC configuration (nil = disabled, use SVCPreset* helpers)
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

// DefaultH264Config returns sensible defaults for H.264.
func DefaultH264Config(width, height int) H264Config {
	return H264Config{
		Width:       width,
		Height:      height,
		Bitrate:     estimateVideoBitrate(width, height),
		RateControl: RateControlVBR,
		FPS:         30,
		KeyInterval: 60, // 2 seconds at 30fps
		Profile:     H264ProfileConstrainedBase,
		LowDelay:    true,
	}
}

// DefaultVP8Config returns sensible defaults for VP8.
func DefaultVP8Config(width, height int) VP8Config {
	return VP8Config{
		Width:          width,
		Height:         height,
		Bitrate:        estimateVideoBitrate(width, height),
		RateControl:    RateControlVBR,
		FPS:            30,
		KeyInterval:    60,
		Deadline:       2, // realtime
		LowDelay:       true,
		ErrorResilient: true,
	}
}

// DefaultVP9Config returns sensible defaults for VP9.
func DefaultVP9Config(width, height int) VP9Config {
	return VP9Config{
		Width:       width,
		Height:      height,
		Bitrate:     estimateVideoBitrate(width, height),
		RateControl: RateControlVBR,
		FPS:         30,
		KeyInterval: 60,
		Profile:     VP9Profile0,
		Speed:       6,
		LowDelay:    true,
	}
}

// DefaultAV1Config returns sensible defaults for AV1.
func DefaultAV1Config(width, height int) AV1Config {
	return AV1Config{
		Width:       width,
		Height:      height,
		Bitrate:     estimateVideoBitrate(width, height),
		RateControl: RateControlVBR,
		FPS:         30,
		KeyInterval: 60,
		Profile:     AV1ProfileMain,
		Speed:       8, // AV1 is slow, use faster preset
		LowDelay:    true,
	}
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
