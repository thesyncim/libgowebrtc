package dependencydescriptor

// ExtensionURI is the RTP header extension URI used for AV1/VP9 dependency
// descriptors.
const ExtensionURI = "https://aomediacodec.github.io/av1-rtp-spec/#dependency-descriptor-rtp-header-extension"

const (
	maxSpatialIDs    = 4
	maxTemporalIDs   = 8
	maxDecodeTargets = 32
	maxTemplates     = 64
)

// DecodeTargetIndication describes a frame's relation to a decode target.
type DecodeTargetIndication uint8

const (
	DecodeTargetNotPresent DecodeTargetIndication = iota
	DecodeTargetDiscardable
	DecodeTargetSwitch
	DecodeTargetRequired
)

// RenderResolution describes the render resolution for a spatial layer.
type RenderResolution struct {
	Width  int
	Height int
}

// FrameDependencyTemplate describes a template or frame dependency definition.
type FrameDependencyTemplate struct {
	SpatialID               int
	TemporalID              int
	DecodeTargetIndications []DecodeTargetIndication
	FrameDiffs              []int
	ChainDiffs              []int
}

// Clone returns a deep copy of the template.
func (t *FrameDependencyTemplate) Clone() *FrameDependencyTemplate {
	if t == nil {
		return nil
	}

	out := &FrameDependencyTemplate{
		SpatialID:  t.SpatialID,
		TemporalID: t.TemporalID,
	}
	if len(t.DecodeTargetIndications) > 0 {
		out.DecodeTargetIndications = append([]DecodeTargetIndication(nil), t.DecodeTargetIndications...)
	}
	if len(t.FrameDiffs) > 0 {
		out.FrameDiffs = append([]int(nil), t.FrameDiffs...)
	}
	if len(t.ChainDiffs) > 0 {
		out.ChainDiffs = append([]int(nil), t.ChainDiffs...)
	}
	return out
}

// FrameDependencyStructure describes the template dependency structure.
type FrameDependencyStructure struct {
	StructureID                  int
	NumDecodeTargets             int
	NumChains                    int
	DecodeTargetProtectedByChain []int
	Resolutions                  []RenderResolution
	Templates                    []*FrameDependencyTemplate
}

// DependencyDescriptor describes a decoded dependency descriptor payload.
type DependencyDescriptor struct {
	FirstPacketInFrame         bool
	LastPacketInFrame          bool
	FrameNumber                uint16
	FrameDependencies          *FrameDependencyTemplate
	Resolution                 *RenderResolution
	ActiveDecodeTargetsBitmask *uint32
	AttachedStructure          *FrameDependencyStructure
}

// DecodeTarget maps a decode target to its highest reachable layer.
type DecodeTarget struct {
	Target   int
	Spatial  int
	Temporal int
}

// DecodeTargets derives decode-target-to-layer mappings from a structure.
func DecodeTargets(structure *FrameDependencyStructure) []DecodeTarget {
	if structure == nil || structure.NumDecodeTargets <= 0 || structure.NumDecodeTargets > maxDecodeTargets {
		return nil
	}

	targets := make([]DecodeTarget, 0, structure.NumDecodeTargets)
	for target := 0; target < structure.NumDecodeTargets; target++ {
		derived := DecodeTarget{Target: target}
		for _, tmpl := range structure.Templates {
			if tmpl == nil || target >= len(tmpl.DecodeTargetIndications) {
				continue
			}
			if tmpl.DecodeTargetIndications[target] == DecodeTargetNotPresent {
				continue
			}
			if tmpl.SpatialID > derived.Spatial {
				derived.Spatial = tmpl.SpatialID
			}
			if tmpl.TemporalID > derived.Temporal {
				derived.Temporal = tmpl.TemporalID
			}
		}
		targets = append(targets, derived)
	}
	return targets
}

// MaxActiveDecodeTargetLayer returns the highest layer allowed by the active
// decode target bitmask.
func MaxActiveDecodeTargetLayer(mask *uint32, targets []DecodeTarget) (spatial, temporal int, ok bool) {
	if mask == nil {
		return 0, 0, false
	}

	for _, target := range targets {
		if (*mask & (uint32(1) << target.Target)) == 0 {
			continue
		}
		if !ok || target.Spatial > spatial {
			spatial = target.Spatial
		}
		if !ok || target.Temporal > temporal {
			temporal = target.Temporal
		}
		ok = true
	}
	return spatial, temporal, ok
}
