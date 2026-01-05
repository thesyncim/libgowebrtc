package frame

import (
	"math"
	"sync"
	"time"
)

// AudioFormat represents the audio sample format.
type AudioFormat int

const (
	// AudioFormatS16 is signed 16-bit little-endian samples.
	AudioFormatS16 AudioFormat = iota

	// AudioFormatF32 is 32-bit float samples.
	AudioFormatF32
)

// String returns the string representation of the audio format.
func (f AudioFormat) String() string {
	switch f {
	case AudioFormatS16:
		return "S16"
	case AudioFormatF32:
		return "F32"
	default:
		return "Unknown"
	}
}

// AudioFrame represents decoded audio samples.
type AudioFrame struct {
	// SampleRate is the audio sample rate in Hz.
	SampleRate int

	// Channels is the number of audio channels.
	Channels int

	// Format is the sample format.
	Format AudioFormat

	// Samples contains the audio samples.
	// For S16 format: interleaved int16 samples as []byte
	// For F32 format: interleaved float32 samples as []byte
	Samples []byte

	// NumSamples is the number of samples per channel.
	NumSamples int

	// Timestamp is the presentation timestamp.
	Timestamp time.Duration

	// PTS is the RTP timestamp.
	PTS uint32

	// pool is the pool this frame belongs to (for recycling).
	pool *AudioFramePool
}

// Release returns the frame to its pool for reuse.
func (f *AudioFrame) Release() {
	if f.pool != nil {
		f.pool.Put(f)
	}
}

// Clone creates a deep copy of the frame.
func (f *AudioFrame) Clone() *AudioFrame {
	clone := &AudioFrame{
		SampleRate: f.SampleRate,
		Channels:   f.Channels,
		Format:     f.Format,
		NumSamples: f.NumSamples,
		Timestamp:  f.Timestamp,
		PTS:        f.PTS,
		Samples:    make([]byte, len(f.Samples)),
	}
	copy(clone.Samples, f.Samples)
	return clone
}

// SamplesS16 returns the samples as int16 slice.
// Only valid for AudioFormatS16 frames.
func (f *AudioFrame) SamplesS16() []int16 {
	if f.Format != AudioFormatS16 || len(f.Samples) == 0 {
		return nil
	}

	numSamples := len(f.Samples) / 2
	result := make([]int16, numSamples)
	for i := 0; i < numSamples; i++ {
		result[i] = int16(f.Samples[i*2]) | int16(f.Samples[i*2+1])<<8
	}
	return result
}

// SamplesF32 returns the samples as float32 slice.
// Only valid for AudioFormatF32 frames.
func (f *AudioFrame) SamplesF32() []float32 {
	if f.Format != AudioFormatF32 || len(f.Samples) == 0 {
		return nil
	}

	numSamples := len(f.Samples) / 4
	result := make([]float32, numSamples)
	for i := 0; i < numSamples; i++ {
		bits := uint32(f.Samples[i*4]) |
			uint32(f.Samples[i*4+1])<<8 |
			uint32(f.Samples[i*4+2])<<16 |
			uint32(f.Samples[i*4+3])<<24
		result[i] = float32frombits(bits)
	}
	return result
}

func float32frombits(b uint32) float32 {
	return math.Float32frombits(b)
}

// Duration returns the duration of the audio in this frame.
func (f *AudioFrame) Duration() time.Duration {
	if f.SampleRate == 0 {
		return 0
	}
	return time.Duration(f.NumSamples) * time.Second / time.Duration(f.SampleRate)
}

// NewAudioFrameS16 creates a new audio frame with S16 format.
func NewAudioFrameS16(sampleRate, channels, numSamples int) *AudioFrame {
	return &AudioFrame{
		SampleRate: sampleRate,
		Channels:   channels,
		Format:     AudioFormatS16,
		NumSamples: numSamples,
		Samples:    make([]byte, numSamples*channels*2), // 2 bytes per sample
	}
}

// NewAudioFrameF32 creates a new audio frame with F32 format.
func NewAudioFrameF32(sampleRate, channels, numSamples int) *AudioFrame {
	return &AudioFrame{
		SampleRate: sampleRate,
		Channels:   channels,
		Format:     AudioFormatF32,
		NumSamples: numSamples,
		Samples:    make([]byte, numSamples*channels*4), // 4 bytes per sample
	}
}

// AudioFramePool manages reusable audio frames to reduce allocations.
type AudioFramePool struct {
	mu         sync.Mutex
	frames     []*AudioFrame
	maxSize    int
	sampleRate int
	channels   int
	numSamples int
	format     AudioFormat
}

// NewAudioFramePool creates a pool for audio frames.
func NewAudioFramePool(sampleRate, channels, numSamples int, format AudioFormat, poolSize int) *AudioFramePool {
	pool := &AudioFramePool{
		maxSize:    poolSize,
		sampleRate: sampleRate,
		channels:   channels,
		numSamples: numSamples,
		format:     format,
		frames:     make([]*AudioFrame, 0, poolSize),
	}

	// Pre-allocate frames
	for i := 0; i < poolSize; i++ {
		frame := pool.allocFrame()
		frame.pool = pool
		pool.frames = append(pool.frames, frame)
	}

	return pool
}

// Get returns a frame from the pool or allocates a new one.
func (p *AudioFramePool) Get() *AudioFrame {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.frames) > 0 {
		f := p.frames[len(p.frames)-1]
		p.frames = p.frames[:len(p.frames)-1]
		// Reset metadata
		f.Timestamp = 0
		f.PTS = 0
		return f
	}

	// Pool exhausted, allocate new
	f := p.allocFrame()
	f.pool = p
	return f
}

// Put returns a frame to the pool.
func (p *AudioFramePool) Put(f *AudioFrame) {
	if f == nil || f.pool != p {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.frames) < p.maxSize {
		p.frames = append(p.frames, f)
	}
}

func (p *AudioFramePool) allocFrame() *AudioFrame {
	switch p.format {
	case AudioFormatF32:
		return NewAudioFrameF32(p.sampleRate, p.channels, p.numSamples)
	default:
		return NewAudioFrameS16(p.sampleRate, p.channels, p.numSamples)
	}
}
