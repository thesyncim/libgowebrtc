package media

// SupportedConstraints mirrors browser MediaTrackSupportedConstraints for the
// subset implemented by pkg/media.
type SupportedConstraints struct {
	Width            bool
	Height           bool
	FrameRate        bool
	FacingMode       bool
	DeviceID         bool
	SampleRate       bool
	ChannelCount     bool
	EchoCancellation bool
	NoiseSuppression bool
	AutoGainControl  bool
	DisplaySurface   bool
}

// GetSupportedConstraints reports which browser-shaped constraints this
// package understands and applies.
func GetSupportedConstraints() SupportedConstraints {
	return SupportedConstraints{
		Width:            true,
		Height:           true,
		FrameRate:        true,
		FacingMode:       true,
		DeviceID:         true,
		SampleRate:       true,
		ChannelCount:     true,
		EchoCancellation: true,
		NoiseSuppression: true,
		AutoGainControl:  true,
		DisplaySurface:   true,
	}
}

// IntCapabilityRange mirrors the browser pattern of reporting supported
// numeric ranges for a track capability.
type IntCapabilityRange struct {
	Min int
	Max int
}

// FloatCapabilityRange mirrors the browser pattern of reporting supported
// floating-point ranges for a track capability.
type FloatCapabilityRange struct {
	Min float64
	Max float64
}

// VideoTrackCapabilities reports the browser-shaped video capability subset
// implemented by pkg/media for a concrete track.
type VideoTrackCapabilities struct {
	Width          IntCapabilityRange
	Height         IntCapabilityRange
	FrameRate      FloatCapabilityRange
	DeviceID       []string
	FacingMode     []FacingMode
	DisplaySurface []DisplaySurface
}

// AudioTrackCapabilities reports the browser-shaped audio capability subset
// implemented by pkg/media for a concrete track.
type AudioTrackCapabilities struct {
	SampleRate       IntCapabilityRange
	ChannelCount     IntCapabilityRange
	DeviceID         []string
	EchoCancellation []bool
	NoiseSuppression []bool
	AutoGainControl  []bool
}

func exactIntCapability(v int) IntCapabilityRange {
	return IntCapabilityRange{Min: v, Max: v}
}

func frameRateCapability(max float64) FloatCapabilityRange {
	if max <= 0 {
		return FloatCapabilityRange{}
	}
	return FloatCapabilityRange{Min: 1, Max: max}
}

func copyVideoTrackCapabilities(c VideoTrackCapabilities) VideoTrackCapabilities {
	cloned := c
	if len(c.DeviceID) > 0 {
		cloned.DeviceID = append([]string(nil), c.DeviceID...)
	}
	if len(c.FacingMode) > 0 {
		cloned.FacingMode = append([]FacingMode(nil), c.FacingMode...)
	}
	if len(c.DisplaySurface) > 0 {
		cloned.DisplaySurface = append([]DisplaySurface(nil), c.DisplaySurface...)
	}
	return cloned
}

func copyAudioTrackCapabilities(c AudioTrackCapabilities) AudioTrackCapabilities {
	cloned := c
	if len(c.DeviceID) > 0 {
		cloned.DeviceID = append([]string(nil), c.DeviceID...)
	}
	if len(c.EchoCancellation) > 0 {
		cloned.EchoCancellation = append([]bool(nil), c.EchoCancellation...)
	}
	if len(c.NoiseSuppression) > 0 {
		cloned.NoiseSuppression = append([]bool(nil), c.NoiseSuppression...)
	}
	if len(c.AutoGainControl) > 0 {
		cloned.AutoGainControl = append([]bool(nil), c.AutoGainControl...)
	}
	return cloned
}
