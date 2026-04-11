package media

// IntCapabilityRange mirrors the browser pattern of reporting supported
// numeric ranges for a track capability.
type IntCapabilityRange struct {
	Min int // Min is the lowest supported value.
	Max int // Max is the highest supported value.
}

// FloatCapabilityRange mirrors the browser pattern of reporting supported
// floating-point ranges for a track capability.
type FloatCapabilityRange struct {
	Min float64 // Min is the lowest supported value.
	Max float64 // Max is the highest supported value.
}

// VideoTrackCapabilities reports the browser-shaped video capability subset
// implemented by pkg/media for a concrete track.
type VideoTrackCapabilities struct {
	Width          IntCapabilityRange   // Width reports the supported capture width range.
	Height         IntCapabilityRange   // Height reports the supported capture height range.
	FrameRate      FloatCapabilityRange // FrameRate reports the supported capture frame-rate range.
	DeviceID       []string             // DeviceID lists addressable capture devices, when known.
	FacingMode     []FacingMode         // FacingMode lists supported facing-mode values.
	DisplaySurface []DisplaySurface     // DisplaySurface lists supported display-capture surface kinds.
}

// AudioTrackCapabilities reports the browser-shaped audio capability subset
// implemented by pkg/media for a concrete track.
type AudioTrackCapabilities struct {
	SampleRate       IntCapabilityRange // SampleRate reports the supported audio sample-rate range.
	ChannelCount     IntCapabilityRange // ChannelCount reports the supported audio channel-count range.
	DeviceID         []string           // DeviceID lists addressable audio devices, when known.
	EchoCancellation []bool             // EchoCancellation lists supported echo-cancellation states.
	NoiseSuppression []bool             // NoiseSuppression lists supported noise-suppression states.
	AutoGainControl  []bool             // AutoGainControl lists supported automatic gain control states.
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
