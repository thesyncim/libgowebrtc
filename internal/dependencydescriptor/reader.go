package dependencydescriptor

import "errors"

var (
	ErrNoStructure            = errors.New("dependency descriptor: no structure available")
	ErrTooManyTemplates       = errors.New("dependency descriptor: too many templates")
	ErrTooManyTemporalLayers  = errors.New("dependency descriptor: too many temporal layers")
	ErrTooManySpatialLayers   = errors.New("dependency descriptor: too many spatial layers")
	ErrInvalidTemplateIndex   = errors.New("dependency descriptor: invalid template index")
	ErrInvalidSpatialLayer    = errors.New("dependency descriptor: invalid spatial layer")
	ErrDecodeTargetMismatch   = errors.New("dependency descriptor: decode target indication count mismatch")
	ErrChainDiffMismatch      = errors.New("dependency descriptor: chain diff count mismatch")
	ErrStructureWithoutAttach = errors.New("dependency descriptor: structure-present flag without attached structure")
)

// Parser incrementally parses dependency descriptor payloads while preserving
// the last announced frame dependency structure across packets.
type Parser struct {
	structure *FrameDependencyStructure
}

// NewParser creates a new dependency descriptor parser.
func NewParser() *Parser {
	return &Parser{}
}

// Structure returns the currently active dependency structure.
func (p *Parser) Structure() *FrameDependencyStructure {
	return p.structure
}

// Parse decodes a dependency descriptor RTP header extension payload.
func (p *Parser) Parse(payload []byte) (*DependencyDescriptor, error) {
	reader := &descriptorReader{
		bits:       newBitReader(payload),
		descriptor: &DependencyDescriptor{},
		structure:  p.structure,
	}
	if err := reader.readMandatoryFields(); err != nil {
		return nil, err
	}
	if len(payload) > 3 {
		if err := reader.readExtendedFields(); err != nil {
			return nil, err
		}
	}

	if reader.descriptor.AttachedStructure != nil {
		reader.structure = reader.descriptor.AttachedStructure
	}
	if reader.structure == nil {
		return nil, ErrNoStructure
	}

	if reader.activeDecodeTargetsPresent {
		mask, err := reader.bits.ReadBits(reader.structure.NumDecodeTargets)
		if err != nil {
			return nil, err
		}
		mask32 := uint32(mask)
		reader.descriptor.ActiveDecodeTargetsBitmask = &mask32
	}

	if err := reader.readFrameDependencyDefinition(); err != nil {
		return nil, err
	}

	if reader.descriptor.AttachedStructure != nil {
		p.structure = reader.descriptor.AttachedStructure
	}
	return reader.descriptor, nil
}

type descriptorReader struct {
	bits *bitReader

	descriptor *DependencyDescriptor

	templateID                 int
	activeDecodeTargetsPresent bool
	customDTIs                 bool
	customFrameDiffs           bool
	customChains               bool
	structure                  *FrameDependencyStructure
}

func (r *descriptorReader) readMandatoryFields() error {
	var err error
	if r.descriptor.FirstPacketInFrame, err = r.bits.ReadBool(); err != nil {
		return err
	}
	if r.descriptor.LastPacketInFrame, err = r.bits.ReadBool(); err != nil {
		return err
	}

	templateID, err := r.bits.ReadBits(6)
	if err != nil {
		return err
	}
	r.templateID = int(templateID)

	frameNumber, err := r.bits.ReadBits(16)
	if err != nil {
		return err
	}
	r.descriptor.FrameNumber = uint16(frameNumber)
	return nil
}

func (r *descriptorReader) readExtendedFields() error {
	structurePresent, err := r.bits.ReadBool()
	if err != nil {
		return err
	}
	if r.activeDecodeTargetsPresent, err = r.bits.ReadBool(); err != nil {
		return err
	}
	if r.customDTIs, err = r.bits.ReadBool(); err != nil {
		return err
	}
	if r.customFrameDiffs, err = r.bits.ReadBool(); err != nil {
		return err
	}
	if r.customChains, err = r.bits.ReadBool(); err != nil {
		return err
	}

	if !structurePresent {
		return nil
	}

	if err := r.readAttachedStructure(); err != nil {
		return err
	}
	if r.descriptor.AttachedStructure == nil {
		return ErrStructureWithoutAttach
	}

	mask := uint32((uint64(1) << r.descriptor.AttachedStructure.NumDecodeTargets) - 1)
	r.descriptor.ActiveDecodeTargetsBitmask = &mask
	return nil
}

func (r *descriptorReader) readAttachedStructure() error {
	structure := &FrameDependencyStructure{}
	structureID, err := r.bits.ReadBits(6)
	if err != nil {
		return err
	}
	structure.StructureID = int(structureID)

	numDecodeTargets, err := r.bits.ReadBits(5)
	if err != nil {
		return err
	}
	structure.NumDecodeTargets = int(numDecodeTargets) + 1

	r.descriptor.AttachedStructure = structure

	err = r.readTemplateLayers(structure)
	if err != nil {
		return err
	}
	err = r.readTemplateDTIs(structure)
	if err != nil {
		return err
	}
	err = r.readTemplateFrameDiffs(structure)
	if err != nil {
		return err
	}
	err = r.readTemplateChains(structure)
	if err != nil {
		return err
	}

	hasResolutions, err := r.bits.ReadBool()
	if err != nil {
		return err
	}
	if !hasResolutions {
		return nil
	}
	return r.readResolutions(structure)
}

func (r *descriptorReader) readTemplateLayers(structure *FrameDependencyStructure) error {
	spatialID := 0
	temporalID := 0

	for {
		if len(structure.Templates) >= maxTemplates {
			return ErrTooManyTemplates
		}

		template := &FrameDependencyTemplate{
			SpatialID:  spatialID,
			TemporalID: temporalID,
		}
		structure.Templates = append(structure.Templates, template)

		nextLayerIDC, err := r.bits.ReadBits(2)
		if err != nil {
			return err
		}

		switch nextLayerIDC {
		case 1:
			temporalID++
			if temporalID >= maxTemporalIDs {
				return ErrTooManyTemporalLayers
			}
		case 2:
			spatialID++
			temporalID = 0
			if spatialID >= maxSpatialIDs {
				return ErrTooManySpatialLayers
			}
		}

		if nextLayerIDC == 3 {
			return nil
		}
	}
}

func (r *descriptorReader) readTemplateDTIs(structure *FrameDependencyStructure) error {
	for _, template := range structure.Templates {
		template.DecodeTargetIndications = make([]DecodeTargetIndication, structure.NumDecodeTargets)
		for i := range template.DecodeTargetIndications {
			value, err := r.bits.ReadBits(2)
			if err != nil {
				return err
			}
			template.DecodeTargetIndications[i] = DecodeTargetIndication(value)
		}
	}
	return nil
}

func (r *descriptorReader) readTemplateFrameDiffs(structure *FrameDependencyStructure) error {
	for _, template := range structure.Templates {
		for {
			follow, err := r.bits.ReadBool()
			if err != nil {
				return err
			}
			if !follow {
				break
			}
			value, err := r.bits.ReadBits(4)
			if err != nil {
				return err
			}
			template.FrameDiffs = append(template.FrameDiffs, int(value)+1)
		}
	}
	return nil
}

func (r *descriptorReader) readTemplateChains(structure *FrameDependencyStructure) error {
	numChains, err := r.bits.ReadNonSymmetric(uint32(structure.NumDecodeTargets) + 1)
	if err != nil {
		return err
	}
	structure.NumChains = int(numChains)
	if structure.NumChains == 0 {
		return nil
	}

	structure.DecodeTargetProtectedByChain = make([]int, 0, structure.NumDecodeTargets)
	for i := 0; i < structure.NumDecodeTargets; i++ {
		chain, err := r.bits.ReadNonSymmetric(uint32(structure.NumChains))
		if err != nil {
			return err
		}
		structure.DecodeTargetProtectedByChain = append(structure.DecodeTargetProtectedByChain, int(chain))
	}

	for _, template := range structure.Templates {
		template.ChainDiffs = make([]int, 0, structure.NumChains)
		for i := 0; i < structure.NumChains; i++ {
			value, err := r.bits.ReadBits(4)
			if err != nil {
				return err
			}
			template.ChainDiffs = append(template.ChainDiffs, int(value))
		}
	}

	return nil
}

func (r *descriptorReader) readResolutions(structure *FrameDependencyStructure) error {
	if len(structure.Templates) == 0 {
		return nil
	}

	spatialLayers := structure.Templates[len(structure.Templates)-1].SpatialID + 1
	structure.Resolutions = make([]RenderResolution, 0, spatialLayers)
	for i := 0; i < spatialLayers; i++ {
		widthMinusOne, err := r.bits.ReadBits(16)
		if err != nil {
			return err
		}
		heightMinusOne, err := r.bits.ReadBits(16)
		if err != nil {
			return err
		}
		structure.Resolutions = append(structure.Resolutions, RenderResolution{
			Width:  int(widthMinusOne) + 1,
			Height: int(heightMinusOne) + 1,
		})
	}
	return nil
}

func (r *descriptorReader) readFrameDependencyDefinition() error {
	templateIndex := (r.templateID + maxTemplates - r.structure.StructureID) % maxTemplates
	if templateIndex >= len(r.structure.Templates) {
		return ErrInvalidTemplateIndex
	}

	r.descriptor.FrameDependencies = r.structure.Templates[templateIndex].Clone()
	if r.customDTIs {
		if err := r.readFrameDTIs(); err != nil {
			return err
		}
	}
	if r.customFrameDiffs {
		if err := r.readFrameDiffs(); err != nil {
			return err
		}
	}
	if r.customChains {
		if err := r.readFrameChains(); err != nil {
			return err
		}
	}

	if len(r.structure.Resolutions) == 0 {
		return nil
	}
	if r.descriptor.FrameDependencies.SpatialID >= len(r.structure.Resolutions) {
		return ErrInvalidSpatialLayer
	}

	resolution := r.structure.Resolutions[r.descriptor.FrameDependencies.SpatialID]
	r.descriptor.Resolution = &resolution
	return nil
}

func (r *descriptorReader) readFrameDTIs() error {
	if len(r.descriptor.FrameDependencies.DecodeTargetIndications) != r.structure.NumDecodeTargets {
		return ErrDecodeTargetMismatch
	}

	for i := range r.descriptor.FrameDependencies.DecodeTargetIndications {
		value, err := r.bits.ReadBits(2)
		if err != nil {
			return err
		}
		r.descriptor.FrameDependencies.DecodeTargetIndications[i] = DecodeTargetIndication(value)
	}
	return nil
}

func (r *descriptorReader) readFrameDiffs() error {
	r.descriptor.FrameDependencies.FrameDiffs = r.descriptor.FrameDependencies.FrameDiffs[:0]
	for {
		sizeCode, err := r.bits.ReadBits(2)
		if err != nil {
			return err
		}
		if sizeCode == 0 {
			return nil
		}

		value, err := r.bits.ReadBits(int(sizeCode) * 4)
		if err != nil {
			return err
		}
		r.descriptor.FrameDependencies.FrameDiffs = append(r.descriptor.FrameDependencies.FrameDiffs, int(value)+1)
	}
}

func (r *descriptorReader) readFrameChains() error {
	if len(r.descriptor.FrameDependencies.ChainDiffs) != r.structure.NumChains {
		return ErrChainDiffMismatch
	}

	for i := range r.descriptor.FrameDependencies.ChainDiffs {
		value, err := r.bits.ReadBits(8)
		if err != nil {
			return err
		}
		r.descriptor.FrameDependencies.ChainDiffs[i] = int(value)
	}
	return nil
}
