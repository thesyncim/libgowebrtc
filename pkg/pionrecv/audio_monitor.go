package pionrecv

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

const (
	defaultAudioFreezeThreshold        = 500 * time.Millisecond
	defaultAudioPacketGapThreshold     = 250 * time.Millisecond
	minAdaptiveAudioFreezeThreshold    = 120 * time.Millisecond
	minAdaptiveAudioPacketGapThreshold = 80 * time.Millisecond
	defaultAudioSilenceThreshold       = 0.002
	defaultAudioSampleRate             = 48000
	audioClipThreshold                 = 32760
)

// AudioConfigSwitch records an observable decoded-audio configuration change.
type AudioConfigSwitch struct {
	At                 time.Time
	PreviousSampleRate int
	PreviousChannels   int
	PreviousNumSamples int
	PreviousPTime      time.Duration
	CurrentSampleRate  int
	CurrentChannels    int
	CurrentNumSamples  int
	CurrentPTime       time.Duration
}

// AudioSubscriberMonitorConfig configures subscriber-side audio monitoring.
type AudioSubscriberMonitorConfig struct {
	FreezeThreshold    time.Duration
	PacketGapThreshold time.Duration
	SilenceThreshold   float64
	EventHistory       int
}

// AudioSubscriberSnapshot captures the current subscriber-side audio state.
type AudioSubscriberSnapshot struct {
	TrackID                string
	StreamID               string
	TrackRID               string
	CurrentCodec           codec.Type
	CurrentCodecParameters webrtc.RTPCodecParameters

	StartedAt    time.Time
	LastPacketAt time.Time
	LastFrameAt  time.Time

	PacketCount       uint64
	FrameCount        uint64
	ByteCount         uint64
	SequenceGapCount  uint64
	MissingPackets    uint64
	OutOfOrderPackets uint64
	FreezeCount       uint64
	PacketGapCount    uint64
	FrameGapCount     uint64
	MaxInterPacketGap time.Duration
	MaxInterFrameGap  time.Duration

	CurrentSampleRate int
	CurrentChannels   int
	CurrentNumSamples int
	CurrentPTime      time.Duration

	PeakLevel       float64
	RMSLevel        float64
	LastFrameSilent bool
	ActiveFrames    uint64
	SilentFrames    uint64
	ClippedFrames   uint64

	CodecSwitches  []CodecSwitchObservation
	ConfigSwitches []AudioConfigSwitch
	FreezeEvents   []FreezeEvent
}

// HasFreeze reports whether the subscriber experienced any freeze-sized gaps.
func (s AudioSubscriberSnapshot) HasFreeze() bool {
	return s.FreezeCount > 0
}

// Continuous reports whether decoded audio has flowed without freeze-sized gaps.
func (s AudioSubscriberSnapshot) Continuous() bool {
	return s.FrameCount > 0 && s.FreezeCount == 0
}

// AudioSubscriberMonitor observes subscriber-visible continuity and decoded
// audio configuration changes.
type AudioSubscriberMonitor struct {
	cfg AudioSubscriberMonitorConfig

	mu sync.Mutex

	trackID       string
	streamID      string
	trackRID      string
	currentCodec  codec.Type
	currentParams webrtc.RTPCodecParameters

	startedAt         time.Time
	lastPacketAt      time.Time
	lastFrameAt       time.Time
	packetCount       uint64
	frameCount        uint64
	byteCount         uint64
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

	currentSampleRate int
	currentChannels   int
	currentNumSamples int
	currentPTime      time.Duration
	peakLevel         float64
	rmsLevel          float64
	lastFrameSilent   bool
	activeFrames      uint64
	silentFrames      uint64
	clippedFrames     uint64

	lastFramePTS       uint32
	hasLastFramePTS    bool
	estimatedFrameStep time.Duration

	codecSwitches  []CodecSwitchObservation
	configSwitches []AudioConfigSwitch
	freezeEvents   []FreezeEvent

	changed chan struct{}
}

// NewAudioSubscriberMonitor constructs an audio monitor suitable for black-box
// SFU subscriber validation.
func NewAudioSubscriberMonitor(cfg AudioSubscriberMonitorConfig) *AudioSubscriberMonitor {
	if cfg.EventHistory <= 0 {
		cfg.EventHistory = defaultEventHistory
	}
	return &AudioSubscriberMonitor{
		cfg:     cfg,
		changed: make(chan struct{}),
	}
}

// Snapshot returns a copy of the current monitor state.
func (m *AudioSubscriberMonitor) Snapshot() AudioSubscriberSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked()
}

// Reset clears observed runtime state while preserving configured bindings.
func (m *AudioSubscriberMonitor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.startedAt = time.Time{}
	m.lastPacketAt = time.Time{}
	m.lastFrameAt = time.Time{}
	m.packetCount = 0
	m.frameCount = 0
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
	m.currentSampleRate = 0
	m.currentChannels = 0
	m.currentNumSamples = 0
	m.currentPTime = 0
	m.peakLevel = 0
	m.rmsLevel = 0
	m.lastFrameSilent = false
	m.activeFrames = 0
	m.silentFrames = 0
	m.clippedFrames = 0
	m.lastFramePTS = 0
	m.hasLastFramePTS = false
	m.estimatedFrameStep = 0
	m.codecSwitches = nil
	m.configSwitches = nil
	m.freezeEvents = nil
	m.signalLocked()
}

// WaitForFrames waits until at least the target number of decoded frames have
// been observed.
func (m *AudioSubscriberMonitor) WaitForFrames(ctx context.Context, targetFrames uint64) error {
	return m.waitFor(ctx, func(snapshot AudioSubscriberSnapshot) bool {
		return snapshot.FrameCount >= targetFrames
	})
}

// WaitForConfig waits until the decoded audio matches the target sample rate
// and channel count.
func (m *AudioSubscriberMonitor) WaitForConfig(ctx context.Context, sampleRate, channels int) error {
	return m.waitFor(ctx, func(snapshot AudioSubscriberSnapshot) bool {
		return snapshot.CurrentSampleRate == sampleRate && snapshot.CurrentChannels == channels
	})
}

// WaitForPTime waits until the decoded audio frame duration matches the target.
func (m *AudioSubscriberMonitor) WaitForPTime(ctx context.Context, ptime time.Duration) error {
	return m.waitFor(ctx, func(snapshot AudioSubscriberSnapshot) bool {
		return snapshot.CurrentPTime == ptime
	})
}

// WaitForCodec waits until the decoded codec matches the target.
func (m *AudioSubscriberMonitor) WaitForCodec(ctx context.Context, target codec.Type) error {
	return m.waitFor(ctx, func(snapshot AudioSubscriberSnapshot) bool {
		return snapshot.CurrentCodec == target
	})
}

// WaitForActivity waits until at least one non-silent decoded audio frame is observed.
func (m *AudioSubscriberMonitor) WaitForActivity(ctx context.Context) error {
	return m.waitFor(ctx, func(snapshot AudioSubscriberSnapshot) bool {
		return snapshot.ActiveFrames > 0
	})
}

// WaitForSilence waits until at least one silent decoded audio frame is observed.
func (m *AudioSubscriberMonitor) WaitForSilence(ctx context.Context) error {
	return m.waitFor(ctx, func(snapshot AudioSubscriberSnapshot) bool {
		return snapshot.SilentFrames > 0
	})
}

func (m *AudioSubscriberMonitor) bind(track trackReader, _ *webrtc.RTPReceiver, codecType codec.Type, params webrtc.RTPCodecParameters) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.trackID = track.ID()
	m.streamID = track.StreamID()
	m.trackRID = track.RID()
	m.currentCodec = codecType
	m.currentParams = params
	m.currentSampleRate = int(params.ClockRate)
	m.currentChannels = int(params.Channels)
	if m.currentChannels <= 0 && codecType == codec.Opus {
		m.currentChannels = defaultDefaultOpusChannels
	}
}

func (m *AudioSubscriberMonitor) observePacket(pkt *rtp.Packet) {
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
			m.freezeCount++
			m.packetGapCount++
			m.freezeEvents = appendLimited(m.freezeEvents, FreezeEvent{
				At:   now,
				Gap:  gap,
				Kind: "packet",
			}, m.cfg.EventHistory)
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

	m.signalLocked()
}

func (m *AudioSubscriberMonitor) observeFrame(f *frame.AudioFrame) {
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
			m.freezeCount++
			m.frameGapCount++
			m.freezeEvents = appendLimited(m.freezeEvents, FreezeEvent{
				At:   now,
				Gap:  gap,
				Kind: "frame",
			}, m.cfg.EventHistory)
		}
	}

	clockRate := m.currentParams.ClockRate
	if clockRate == 0 && f.SampleRate > 0 {
		clockRate = uint32(f.SampleRate)
	}
	if clockRate == 0 {
		clockRate = defaultAudioSampleRate
	}
	if m.hasLastFramePTS {
		delta := f.PTS - m.lastFramePTS
		if delta > 0 {
			m.estimatedFrameStep = time.Duration(delta) * time.Second / time.Duration(clockRate)
		}
	}
	m.lastFramePTS = f.PTS
	m.hasLastFramePTS = true
	m.lastFrameAt = now
	m.frameCount++

	ptime := f.Duration()
	peakLevel, rmsLevel, clipped := audioLevels(f)
	silent := rmsLevel <= m.silenceThresholdLocked()

	if clipped {
		m.clippedFrames++
	}
	if silent {
		m.silentFrames++
	} else {
		m.activeFrames++
	}

	if m.currentSampleRate != 0 && m.currentChannels != 0 &&
		(m.currentSampleRate != f.SampleRate ||
			m.currentChannels != f.Channels ||
			m.currentNumSamples != f.NumSamples ||
			m.currentPTime != ptime) {
		m.configSwitches = appendLimited(m.configSwitches, AudioConfigSwitch{
			At:                 now,
			PreviousSampleRate: m.currentSampleRate,
			PreviousChannels:   m.currentChannels,
			PreviousNumSamples: m.currentNumSamples,
			PreviousPTime:      m.currentPTime,
			CurrentSampleRate:  f.SampleRate,
			CurrentChannels:    f.Channels,
			CurrentNumSamples:  f.NumSamples,
			CurrentPTime:       ptime,
		}, m.cfg.EventHistory)
	}

	m.currentSampleRate = f.SampleRate
	m.currentChannels = f.Channels
	m.currentNumSamples = f.NumSamples
	m.currentPTime = ptime
	m.peakLevel = peakLevel
	m.rmsLevel = rmsLevel
	m.lastFrameSilent = silent
	m.signalLocked()
}

func (m *AudioSubscriberMonitor) observeCodecChange(change CodecChange) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentCodec = change.CurrentType
	m.currentParams = change.CurrentCodec
	if change.CurrentCodec.ClockRate > 0 {
		m.currentSampleRate = int(change.CurrentCodec.ClockRate)
	}
	if change.CurrentCodec.Channels > 0 {
		m.currentChannels = int(change.CurrentCodec.Channels)
	}
	m.codecSwitches = appendLimited(m.codecSwitches, CodecSwitchObservation{
		At:     time.Now(),
		Change: change,
	}, m.cfg.EventHistory)
	m.signalLocked()
}

func (m *AudioSubscriberMonitor) waitFor(ctx context.Context, predicate func(AudioSubscriberSnapshot) bool) error {
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

func (m *AudioSubscriberMonitor) snapshotLocked() AudioSubscriberSnapshot {
	return AudioSubscriberSnapshot{
		TrackID:                m.trackID,
		StreamID:               m.streamID,
		TrackRID:               m.trackRID,
		CurrentCodec:           m.currentCodec,
		CurrentCodecParameters: m.currentParams,
		StartedAt:              m.startedAt,
		LastPacketAt:           m.lastPacketAt,
		LastFrameAt:            m.lastFrameAt,
		PacketCount:            m.packetCount,
		FrameCount:             m.frameCount,
		ByteCount:              m.byteCount,
		SequenceGapCount:       m.sequenceGapCount,
		MissingPackets:         m.missingPackets,
		OutOfOrderPackets:      m.outOfOrderPackets,
		FreezeCount:            m.freezeCount,
		PacketGapCount:         m.packetGapCount,
		FrameGapCount:          m.frameGapCount,
		MaxInterPacketGap:      m.maxInterPacketGap,
		MaxInterFrameGap:       m.maxInterFrameGap,
		CurrentSampleRate:      m.currentSampleRate,
		CurrentChannels:        m.currentChannels,
		CurrentNumSamples:      m.currentNumSamples,
		CurrentPTime:           m.currentPTime,
		PeakLevel:              m.peakLevel,
		RMSLevel:               m.rmsLevel,
		LastFrameSilent:        m.lastFrameSilent,
		ActiveFrames:           m.activeFrames,
		SilentFrames:           m.silentFrames,
		ClippedFrames:          m.clippedFrames,
		CodecSwitches:          append([]CodecSwitchObservation(nil), m.codecSwitches...),
		ConfigSwitches:         append([]AudioConfigSwitch(nil), m.configSwitches...),
		FreezeEvents:           append([]FreezeEvent(nil), m.freezeEvents...),
	}
}

func (m *AudioSubscriberMonitor) signalLocked() {
	close(m.changed)
	m.changed = make(chan struct{})
}

func (m *AudioSubscriberMonitor) freezeThresholdLocked() time.Duration {
	if m.cfg.FreezeThreshold > 0 {
		return m.cfg.FreezeThreshold
	}
	if m.estimatedFrameStep <= 0 {
		if m.currentPTime > 0 {
			threshold := 3 * m.currentPTime
			if threshold < minAdaptiveAudioFreezeThreshold {
				return minAdaptiveAudioFreezeThreshold
			}
			return threshold
		}
		return defaultAudioFreezeThreshold
	}
	threshold := 3 * m.estimatedFrameStep
	if threshold < minAdaptiveAudioFreezeThreshold {
		threshold = minAdaptiveAudioFreezeThreshold
	}
	return threshold
}

func (m *AudioSubscriberMonitor) packetGapThresholdLocked() time.Duration {
	if m.cfg.PacketGapThreshold > 0 {
		return m.cfg.PacketGapThreshold
	}
	if m.estimatedFrameStep <= 0 {
		if m.currentPTime > 0 {
			threshold := 2 * m.currentPTime
			if threshold < minAdaptiveAudioPacketGapThreshold {
				return minAdaptiveAudioPacketGapThreshold
			}
			return threshold
		}
		return defaultAudioPacketGapThreshold
	}
	threshold := 2 * m.estimatedFrameStep
	if threshold < minAdaptiveAudioPacketGapThreshold {
		threshold = minAdaptiveAudioPacketGapThreshold
	}
	return threshold
}

func (m *AudioSubscriberMonitor) silenceThresholdLocked() float64 {
	if m.cfg.SilenceThreshold > 0 {
		return m.cfg.SilenceThreshold
	}
	return defaultAudioSilenceThreshold
}

func audioLevels(f *frame.AudioFrame) (peakLevel, rmsLevel float64, clipped bool) {
	if f == nil {
		return 0, 0, false
	}

	samples := f.SamplesS16()
	if len(samples) == 0 {
		return 0, 0, false
	}

	var (
		peak float64
		sum  float64
	)

	for _, sample := range samples {
		value := math.Abs(float64(sample))
		if value > peak {
			peak = value
		}
		if value >= audioClipThreshold {
			clipped = true
		}
		normalized := value / math.MaxInt16
		sum += normalized * normalized
	}

	peakLevel = peak / math.MaxInt16
	rmsLevel = math.Sqrt(sum / float64(len(samples)))
	return peakLevel, rmsLevel, clipped
}
