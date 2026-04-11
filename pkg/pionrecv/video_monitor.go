package pionrecv

import (
	"context"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	dd "github.com/thesyncim/libgowebrtc/internal/dependencydescriptor"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

const (
	midRTPHeaderExtensionURI           = "urn:ietf:params:rtp-hdrext:sdes:mid"
	rtpStreamIDHeaderExtensionURI      = "urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id"
	repairedStreamIDHeaderExtensionURI = "urn:ietf:params:rtp-hdrext:sdes:repaired-rtp-stream-id"
	defaultEventHistory                = 32
	defaultFreezeThreshold             = 450 * time.Millisecond
	defaultPacketGapThreshold          = 250 * time.Millisecond
	minAdaptiveFreezeThreshold         = 250 * time.Millisecond
	minAdaptivePacketGapThreshold      = 125 * time.Millisecond
)

// VideoLayer identifies a spatial/temporal layer pair as observed by a
// subscriber on the wire.
type VideoLayer struct {
	Spatial  int
	Temporal int
}

// FreezeEvent describes a packet or frame gap that exceeded the configured
// continuity threshold.
type FreezeEvent struct {
	At   time.Time
	Gap  time.Duration
	Kind string
}

// ResolutionSwitch records a decoded output resolution transition.
type ResolutionSwitch struct {
	At             time.Time
	PreviousWidth  int
	PreviousHeight int
	CurrentWidth   int
	CurrentHeight  int
}

// LayerSwitch records a dependency-descriptor layer transition.
type LayerSwitch struct {
	At                         time.Time
	Previous                   VideoLayer
	Current                    VideoLayer
	ActiveDecodeTargetsBitmask *uint32
	MaxActiveLayer             VideoLayer
	HasMaxActiveLayer          bool
	Width                      int
	Height                     int
}

// RIDSwitch records an RTP Stream ID transition observed on the wire.
type RIDSwitch struct {
	At       time.Time
	Previous string
	Current  string
}

// CodecSwitchObservation records a decoded runtime codec switch.
type CodecSwitchObservation struct {
	At     time.Time
	Change CodecChange
}

// VideoSubscriberMonitorConfig configures subscriber-side monitoring.
type VideoSubscriberMonitorConfig struct {
	FreezeThreshold    time.Duration
	PacketGapThreshold time.Duration
	EventHistory       int
	HeaderExtensions   []webrtc.RTPHeaderExtensionParameter
}

// VideoSubscriberSnapshot captures the current subscriber-side observations.
type VideoSubscriberSnapshot struct {
	TrackID    string
	StreamID   string
	TrackRID   string
	CurrentMID string
	CurrentRID string
	RepairRID  string

	HeaderExtensions       []webrtc.RTPHeaderExtensionParameter
	CurrentCodec           codec.Type
	CurrentCodecParameters webrtc.RTPCodecParameters

	StartedAt    time.Time
	LastPacketAt time.Time
	LastFrameAt  time.Time

	PacketCount              uint64
	FrameCount               uint64
	KeyframeCount            uint64
	ByteCount                uint64
	SequenceGapCount         uint64
	MissingPackets           uint64
	OutOfOrderPackets        uint64
	FreezeCount              uint64
	PacketGapCount           uint64
	FrameGapCount            uint64
	MaxInterPacketGap        time.Duration
	MaxInterFrameGap         time.Duration
	CurrentWidth             int
	CurrentHeight            int
	ResolutionChanges        uint64
	HasCurrentLayer          bool
	CurrentLayer             VideoLayer
	HasMaxActiveLayer        bool
	MaxActiveLayer           VideoLayer
	DependencyDescriptorSeen bool
	DependencyStructureSeen  bool

	CodecSwitches      []CodecSwitchObservation
	ResolutionSwitches []ResolutionSwitch
	LayerSwitches      []LayerSwitch
	RIDSwitches        []RIDSwitch
	FreezeEvents       []FreezeEvent
}

// HasFreeze reports whether the subscriber experienced any freeze-sized gaps.
func (s VideoSubscriberSnapshot) HasFreeze() bool {
	return s.FreezeCount > 0
}

// Continuous reports whether decoded frames have flowed without freeze-sized gaps.
func (s VideoSubscriberSnapshot) Continuous() bool {
	return s.FrameCount > 0 && s.FreezeCount == 0
}

// VideoSubscriberMonitor observes subscriber-visible continuity and switching.
type VideoSubscriberMonitor struct {
	cfg VideoSubscriberMonitorConfig

	mu sync.Mutex

	trackID    string
	streamID   string
	trackRID   string
	headerExts []webrtc.RTPHeaderExtensionParameter

	currentCodec     codec.Type
	currentParams    webrtc.RTPCodecParameters
	currentMID       string
	currentRID       string
	currentRepairRID string

	startedAt     time.Time
	lastPacketAt  time.Time
	lastFrameAt   time.Time
	packetCount   uint64
	frameCount    uint64
	keyframeCount uint64
	byteCount     uint64

	lastSequence      uint16
	hasLastSequence   bool
	sequenceGapCount  uint64
	missingPackets    uint64
	outOfOrderPackets uint64
	freezeCount       uint64
	packetGapCount    uint64
	frameGapCount     uint64
	maxInterPacketGap time.Duration
	maxInterFrameGap  time.Duration
	currentWidth      int
	currentHeight     int
	resolutionChanges uint64
	hasCurrentLayer   bool
	currentLayer      VideoLayer
	hasMaxActiveLayer bool
	maxActiveLayer    VideoLayer
	ddSeen            bool
	ddStructureSeen   bool

	lastFramePTS       uint32
	hasLastFramePTS    bool
	estimatedFrameStep time.Duration
	lastFreezeStart    time.Time
	lastFreezeEnd      time.Time
	hasFreezeWindow    bool

	codecSwitches      []CodecSwitchObservation
	resolutionSwitches []ResolutionSwitch
	layerSwitches      []LayerSwitch
	ridSwitches        []RIDSwitch
	freezeEvents       []FreezeEvent

	ddExtID        uint8
	midExtID       uint8
	ridExtID       uint8
	repairRIDExtID uint8
	ddParser       *dd.Parser
	ddTargets      []dd.DecodeTarget

	changed chan struct{}
}

// NewVideoSubscriberMonitor constructs a monitor suitable for black-box SFU
// subscriber validation.
func NewVideoSubscriberMonitor(cfg VideoSubscriberMonitorConfig) *VideoSubscriberMonitor {
	if cfg.EventHistory <= 0 {
		cfg.EventHistory = defaultEventHistory
	}
	if len(cfg.HeaderExtensions) > 0 {
		cfg.HeaderExtensions = append([]webrtc.RTPHeaderExtensionParameter(nil), cfg.HeaderExtensions...)
	}
	return &VideoSubscriberMonitor{
		cfg:      cfg,
		ddParser: dd.NewParser(),
		changed:  make(chan struct{}),
	}
}

// Snapshot returns a copy of the current monitor state.
func (m *VideoSubscriberMonitor) Snapshot() VideoSubscriberSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked()
}

// Reset clears observed runtime state while preserving configured bindings.
func (m *VideoSubscriberMonitor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.startedAt = time.Time{}
	m.lastPacketAt = time.Time{}
	m.lastFrameAt = time.Time{}
	m.packetCount = 0
	m.frameCount = 0
	m.keyframeCount = 0
	m.byteCount = 0
	m.lastSequence = 0
	m.hasLastSequence = false
	m.sequenceGapCount = 0
	m.missingPackets = 0
	m.outOfOrderPackets = 0
	m.freezeCount = 0
	m.packetGapCount = 0
	m.frameGapCount = 0
	m.maxInterPacketGap = 0
	m.maxInterFrameGap = 0
	m.currentWidth = 0
	m.currentHeight = 0
	m.resolutionChanges = 0
	m.hasCurrentLayer = false
	m.currentLayer = VideoLayer{}
	m.hasMaxActiveLayer = false
	m.maxActiveLayer = VideoLayer{}
	m.ddSeen = false
	m.ddStructureSeen = false
	m.lastFramePTS = 0
	m.hasLastFramePTS = false
	m.estimatedFrameStep = 0
	m.lastFreezeStart = time.Time{}
	m.lastFreezeEnd = time.Time{}
	m.hasFreezeWindow = false
	m.codecSwitches = nil
	m.resolutionSwitches = nil
	m.layerSwitches = nil
	m.ridSwitches = nil
	m.freezeEvents = nil
	m.ddParser = dd.NewParser()
	m.ddTargets = nil
	m.signalLocked()
}

// WaitForFrames waits until at least the target number of decoded frames have
// been observed.
func (m *VideoSubscriberMonitor) WaitForFrames(ctx context.Context, targetFrames uint64) error {
	return m.waitFor(ctx, func(snapshot VideoSubscriberSnapshot) bool {
		return snapshot.FrameCount >= targetFrames
	})
}

// WaitForResolution waits until the decoded output matches the target size.
func (m *VideoSubscriberMonitor) WaitForResolution(ctx context.Context, width, height int) error {
	return m.waitFor(ctx, func(snapshot VideoSubscriberSnapshot) bool {
		return snapshot.CurrentWidth == width && snapshot.CurrentHeight == height
	})
}

// WaitForLayer waits until the dependency descriptor reports the target layer.
func (m *VideoSubscriberMonitor) WaitForLayer(ctx context.Context, spatial, temporal int) error {
	return m.waitFor(ctx, func(snapshot VideoSubscriberSnapshot) bool {
		return snapshot.HasCurrentLayer &&
			snapshot.CurrentLayer.Spatial == spatial &&
			snapshot.CurrentLayer.Temporal == temporal
	})
}

// WaitForRID waits until the observed RTP stream ID matches the target.
func (m *VideoSubscriberMonitor) WaitForRID(ctx context.Context, rid string) error {
	return m.waitFor(ctx, func(snapshot VideoSubscriberSnapshot) bool {
		return snapshot.CurrentRID == rid
	})
}

// WaitForCodec waits until the decoded codec matches the target.
func (m *VideoSubscriberMonitor) WaitForCodec(ctx context.Context, target codec.Type) error {
	return m.waitFor(ctx, func(snapshot VideoSubscriberSnapshot) bool {
		return snapshot.FrameCount > 0 && snapshot.CurrentCodec == target
	})
}

func (m *VideoSubscriberMonitor) bind(track trackReader, receiver *webrtc.RTPReceiver, codecType codec.Type, params webrtc.RTPCodecParameters) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.trackID = track.ID()
	m.streamID = track.StreamID()
	m.trackRID = track.RID()
	m.currentCodec = codecType
	m.currentParams = params

	if receiver != nil {
		m.headerExts = cloneHeaderExtensions(receiver.GetParameters().HeaderExtensions)
	} else {
		m.headerExts = cloneHeaderExtensions(m.cfg.HeaderExtensions)
	}

	m.midExtID = headerExtensionID(m.headerExts, midRTPHeaderExtensionURI)
	m.ridExtID = headerExtensionID(m.headerExts, rtpStreamIDHeaderExtensionURI)
	m.repairRIDExtID = headerExtensionID(m.headerExts, repairedStreamIDHeaderExtensionURI)
	m.ddExtID = headerExtensionID(m.headerExts, dd.ExtensionURI)
}

func (m *VideoSubscriberMonitor) observePacket(pkt *rtp.Packet) {
	if pkt == nil {
		return
	}

	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.startedAt.IsZero() {
		m.startedAt = now
	}
	if !m.lastPacketAt.IsZero() {
		gap := now.Sub(m.lastPacketAt)
		if gap > m.maxInterPacketGap {
			m.maxInterPacketGap = gap
		}
		if gap > m.packetGapThresholdLocked() {
			m.recordFreezeLocked(now, gap, "packet")
		}
	}
	m.lastPacketAt = now
	m.packetCount++
	m.byteCount += uint64(len(pkt.Payload))

	if m.hasLastSequence {
		diff := pkt.SequenceNumber - m.lastSequence
		switch {
		case diff == 0:
		case diff < 0x8000:
			if diff > 1 {
				m.sequenceGapCount++
				m.missingPackets += uint64(diff - 1)
			}
			m.lastSequence = pkt.SequenceNumber
		default:
			m.outOfOrderPackets++
		}
	} else {
		m.lastSequence = pkt.SequenceNumber
		m.hasLastSequence = true
	}

	m.observeStringExtensionLocked(now, pkt, m.midExtID, &m.currentMID, nil)
	m.observeStringExtensionLocked(now, pkt, m.repairRIDExtID, &m.currentRepairRID, nil)
	m.observeStringExtensionLocked(now, pkt, m.ridExtID, &m.currentRID, &m.ridSwitches)

	if m.ddExtID == 0 {
		m.signalLocked()
		return
	}

	payload := pkt.GetExtension(m.ddExtID)
	if len(payload) == 0 {
		m.signalLocked()
		return
	}

	m.ddSeen = true
	descriptor, err := m.ddParser.Parse(payload)
	if err != nil || descriptor == nil || descriptor.FrameDependencies == nil {
		m.signalLocked()
		return
	}

	if descriptor.AttachedStructure != nil {
		m.ddStructureSeen = true
		m.ddTargets = dd.DecodeTargets(descriptor.AttachedStructure)
	}

	layer := VideoLayer{
		Spatial:  descriptor.FrameDependencies.SpatialID,
		Temporal: descriptor.FrameDependencies.TemporalID,
	}
	if !m.hasCurrentLayer || layer != m.currentLayer {
		switchEvent := LayerSwitch{
			At:       now,
			Previous: m.currentLayer,
			Current:  layer,
			Width:    m.currentWidth,
			Height:   m.currentHeight,
		}
		if descriptor.Resolution != nil {
			switchEvent.Width = descriptor.Resolution.Width
			switchEvent.Height = descriptor.Resolution.Height
		}
		if descriptor.ActiveDecodeTargetsBitmask != nil {
			switchEvent.ActiveDecodeTargetsBitmask = cloneUint32Ptr(descriptor.ActiveDecodeTargetsBitmask)
			if spatial, temporal, ok := dd.MaxActiveDecodeTargetLayer(descriptor.ActiveDecodeTargetsBitmask, m.ddTargets); ok {
				switchEvent.HasMaxActiveLayer = true
				switchEvent.MaxActiveLayer = VideoLayer{Spatial: spatial, Temporal: temporal}
			}
		}
		if m.hasCurrentLayer {
			m.layerSwitches = appendLimited(m.layerSwitches, switchEvent, m.cfg.EventHistory)
		}
		m.currentLayer = layer
		m.hasCurrentLayer = true
	}

	if spatial, temporal, ok := dd.MaxActiveDecodeTargetLayer(descriptor.ActiveDecodeTargetsBitmask, m.ddTargets); ok {
		m.maxActiveLayer = VideoLayer{Spatial: spatial, Temporal: temporal}
		m.hasMaxActiveLayer = true
	}

	m.signalLocked()
}

func (m *VideoSubscriberMonitor) observeFrame(f *frame.VideoFrame) {
	if f == nil {
		return
	}

	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.startedAt.IsZero() {
		m.startedAt = now
	}
	if !m.lastFrameAt.IsZero() {
		gap := now.Sub(m.lastFrameAt)
		if gap > m.maxInterFrameGap {
			m.maxInterFrameGap = gap
		}
		if gap > m.freezeThresholdLocked() {
			m.recordFreezeLocked(now, gap, "frame")
		}
	}

	if m.hasLastFramePTS {
		clockRate := m.currentParams.ClockRate
		if clockRate == 0 {
			clockRate = 90000
		}
		delta := f.PTS - m.lastFramePTS
		if delta > 0 {
			m.estimatedFrameStep = time.Duration(delta) * time.Second / time.Duration(clockRate)
		}
	}
	m.lastFramePTS = f.PTS
	m.hasLastFramePTS = true
	m.lastFrameAt = now
	m.frameCount++
	if f.IsKeyframe {
		m.keyframeCount++
	}

	if m.currentWidth != 0 && m.currentHeight != 0 &&
		(m.currentWidth != f.Width || m.currentHeight != f.Height) {
		m.resolutionChanges++
		m.resolutionSwitches = appendLimited(m.resolutionSwitches, ResolutionSwitch{
			At:             now,
			PreviousWidth:  m.currentWidth,
			PreviousHeight: m.currentHeight,
			CurrentWidth:   f.Width,
			CurrentHeight:  f.Height,
		}, m.cfg.EventHistory)
	}

	m.currentWidth = f.Width
	m.currentHeight = f.Height
	m.signalLocked()
}

func (m *VideoSubscriberMonitor) observeCodecChange(change CodecChange) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentCodec = change.CurrentType
	m.currentParams = change.CurrentCodec
	m.codecSwitches = appendLimited(m.codecSwitches, CodecSwitchObservation{
		At:     time.Now(),
		Change: change,
	}, m.cfg.EventHistory)
	m.signalLocked()
}

func (m *VideoSubscriberMonitor) waitFor(ctx context.Context, predicate func(VideoSubscriberSnapshot) bool) error {
	for {
		m.mu.Lock()
		snapshot := m.snapshotLocked()
		changed := m.changed
		m.mu.Unlock()

		if predicate(snapshot) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (m *VideoSubscriberMonitor) snapshotLocked() VideoSubscriberSnapshot {
	return VideoSubscriberSnapshot{
		TrackID:                  m.trackID,
		StreamID:                 m.streamID,
		TrackRID:                 m.trackRID,
		CurrentMID:               m.currentMID,
		CurrentRID:               m.currentRID,
		RepairRID:                m.currentRepairRID,
		HeaderExtensions:         cloneHeaderExtensions(m.headerExts),
		CurrentCodec:             m.currentCodec,
		CurrentCodecParameters:   m.currentParams,
		StartedAt:                m.startedAt,
		LastPacketAt:             m.lastPacketAt,
		LastFrameAt:              m.lastFrameAt,
		PacketCount:              m.packetCount,
		FrameCount:               m.frameCount,
		KeyframeCount:            m.keyframeCount,
		ByteCount:                m.byteCount,
		SequenceGapCount:         m.sequenceGapCount,
		MissingPackets:           m.missingPackets,
		OutOfOrderPackets:        m.outOfOrderPackets,
		FreezeCount:              m.freezeCount,
		PacketGapCount:           m.packetGapCount,
		FrameGapCount:            m.frameGapCount,
		MaxInterPacketGap:        m.maxInterPacketGap,
		MaxInterFrameGap:         m.maxInterFrameGap,
		CurrentWidth:             m.currentWidth,
		CurrentHeight:            m.currentHeight,
		ResolutionChanges:        m.resolutionChanges,
		HasCurrentLayer:          m.hasCurrentLayer,
		CurrentLayer:             m.currentLayer,
		HasMaxActiveLayer:        m.hasMaxActiveLayer,
		MaxActiveLayer:           m.maxActiveLayer,
		DependencyDescriptorSeen: m.ddSeen,
		DependencyStructureSeen:  m.ddStructureSeen,
		CodecSwitches:            append([]CodecSwitchObservation(nil), m.codecSwitches...),
		ResolutionSwitches:       append([]ResolutionSwitch(nil), m.resolutionSwitches...),
		LayerSwitches:            cloneLayerSwitches(m.layerSwitches),
		RIDSwitches:              append([]RIDSwitch(nil), m.ridSwitches...),
		FreezeEvents:             append([]FreezeEvent(nil), m.freezeEvents...),
	}
}

func (m *VideoSubscriberMonitor) signalLocked() {
	close(m.changed)
	m.changed = make(chan struct{})
}

func (m *VideoSubscriberMonitor) recordFreezeLocked(now time.Time, gap time.Duration, kind string) {
	switch kind {
	case "packet":
		m.packetGapCount++
	case "frame":
		m.frameGapCount++
	}

	m.freezeEvents = appendLimited(m.freezeEvents, FreezeEvent{
		At:   now,
		Gap:  gap,
		Kind: kind,
	}, m.cfg.EventHistory)

	start := now.Add(-gap)
	if !m.hasFreezeWindow || start.After(m.lastFreezeEnd) {
		m.freezeCount++
		m.lastFreezeStart = start
		m.lastFreezeEnd = now
		m.hasFreezeWindow = true
		return
	}
	if start.Before(m.lastFreezeStart) {
		m.lastFreezeStart = start
	}
	if now.After(m.lastFreezeEnd) {
		m.lastFreezeEnd = now
	}
}

func (m *VideoSubscriberMonitor) freezeThresholdLocked() time.Duration {
	if m.cfg.FreezeThreshold > 0 {
		return m.cfg.FreezeThreshold
	}
	if m.estimatedFrameStep <= 0 {
		return defaultFreezeThreshold
	}
	threshold := 3 * m.estimatedFrameStep
	if threshold < minAdaptiveFreezeThreshold {
		threshold = minAdaptiveFreezeThreshold
	}
	return threshold
}

func (m *VideoSubscriberMonitor) packetGapThresholdLocked() time.Duration {
	if m.cfg.PacketGapThreshold > 0 {
		return m.cfg.PacketGapThreshold
	}
	if m.estimatedFrameStep <= 0 {
		return defaultPacketGapThreshold
	}
	threshold := 2 * m.estimatedFrameStep
	if threshold < minAdaptivePacketGapThreshold {
		threshold = minAdaptivePacketGapThreshold
	}
	return threshold
}

func (m *VideoSubscriberMonitor) observeStringExtensionLocked(
	now time.Time,
	pkt *rtp.Packet,
	extID uint8,
	current *string,
	switches *[]RIDSwitch,
) {
	if pkt == nil || extID == 0 {
		return
	}

	payload := pkt.GetExtension(extID)
	if len(payload) == 0 {
		return
	}

	value := string(payload)
	if *current == value {
		return
	}
	if switches != nil && *current != "" {
		*switches = appendLimited(*switches, RIDSwitch{
			At:       now,
			Previous: *current,
			Current:  value,
		}, m.cfg.EventHistory)
	}
	*current = value
}

func headerExtensionID(exts []webrtc.RTPHeaderExtensionParameter, uri string) uint8 {
	for _, ext := range exts {
		if ext.URI == uri && ext.ID > 0 && ext.ID < 256 {
			return uint8(ext.ID)
		}
	}
	return 0
}

func cloneHeaderExtensions(in []webrtc.RTPHeaderExtensionParameter) []webrtc.RTPHeaderExtensionParameter {
	if len(in) == 0 {
		return nil
	}
	out := make([]webrtc.RTPHeaderExtensionParameter, len(in))
	copy(out, in)
	return out
}

func cloneUint32Ptr(in *uint32) *uint32 {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneLayerSwitches(in []LayerSwitch) []LayerSwitch {
	if len(in) == 0 {
		return nil
	}
	out := make([]LayerSwitch, len(in))
	copy(out, in)
	for i := range out {
		out[i].ActiveDecodeTargetsBitmask = cloneUint32Ptr(in[i].ActiveDecodeTargetsBitmask)
	}
	return out
}

func appendLimited[T any](dst []T, value T, limit int) []T {
	if limit <= 0 {
		return append(dst[:0], value)
	}
	if len(dst) < limit {
		return append(dst, value)
	}
	copy(dst, dst[1:])
	dst[len(dst)-1] = value
	return dst
}
