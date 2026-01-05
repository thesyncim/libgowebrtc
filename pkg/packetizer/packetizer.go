// Package packetizer provides RTP packetization using libwebrtc.
package packetizer

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

// Errors
var (
	ErrPacketizerClosed = errors.New("packetizer is closed")
	ErrBufferTooSmall   = errors.New("buffer too small")
	ErrInvalidData      = errors.New("invalid data")
)

// Config configures an RTP packetizer.
type Config struct {
	Codec       codec.Type
	SSRC        uint32
	PayloadType uint8
	MTU         uint16 // Maximum transmission unit (typically 1200)
	ClockRate   uint32 // RTP clock rate (90000 for video, 48000 for Opus)
}

// PacketInfo describes a single RTP packet in the output buffer.
type PacketInfo struct {
	Offset int // Offset into the buffer where this packet starts
	Size   int // Size of this packet
}

// Packetizer converts encoded frames into RTP packets.
// All operations are allocation-free - caller provides buffers.
type Packetizer interface {
	// PacketizeInto packetizes encoded data into RTP packets.
	// dst is a pre-allocated buffer to hold all packets contiguously.
	// packets is a pre-allocated slice to receive packet info (offset/size).
	// Returns the number of packets written.
	PacketizeInto(data []byte, timestamp uint32, isKeyframe bool, dst []byte, packets []PacketInfo) (int, error)

	// MaxPackets returns the maximum number of packets that could be generated
	// for a frame of the given size.
	MaxPackets(frameSize int) int

	// MaxPacketSize returns the maximum size of a single RTP packet.
	MaxPacketSize() int

	// SequenceNumber returns the current sequence number.
	SequenceNumber() uint16

	// Close releases resources.
	Close() error
}

type packetizer struct {
	handle uintptr
	config Config
	closed atomic.Bool
	mu     sync.Mutex
}

// New creates a new RTP packetizer.
func New(cfg Config) (Packetizer, error) {
	if err := ffi.LoadLibrary(); err != nil {
		return nil, err
	}

	if cfg.MTU == 0 {
		cfg.MTU = 1200
	}
	if cfg.ClockRate == 0 {
		cfg.ClockRate = cfg.Codec.ClockRate()
	}

	p := &packetizer{config: cfg}
	if err := p.init(); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *packetizer) init() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	ffiConfig := &ffi.PacketizerConfig{
		Codec:       int32(p.config.Codec),
		SSRC:        p.config.SSRC,
		PayloadType: p.config.PayloadType,
		MTU:         p.config.MTU,
		ClockRate:   p.config.ClockRate,
	}

	handle := ffi.CreatePacketizer(ffiConfig)
	if handle == 0 {
		return errors.New("failed to create packetizer")
	}

	p.handle = handle
	return nil
}

func (p *packetizer) PacketizeInto(data []byte, timestamp uint32, isKeyframe bool, dst []byte, packets []PacketInfo) (int, error) {
	if p.closed.Load() {
		return 0, ErrPacketizerClosed
	}
	if len(data) == 0 {
		return 0, ErrInvalidData
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.handle == 0 {
		return 0, ErrPacketizerClosed
	}

	// Prepare output arrays for FFI
	maxPackets := len(packets)
	offsets := make([]int32, maxPackets)
	sizes := make([]int32, maxPackets)

	count, err := ffi.PacketizerPacketizeInto(
		p.handle, data, timestamp, isKeyframe,
		dst, offsets, sizes, maxPackets,
	)
	if err != nil {
		return 0, err
	}

	// Copy results to PacketInfo slice
	for i := 0; i < count; i++ {
		packets[i] = PacketInfo{
			Offset: int(offsets[i]),
			Size:   int(sizes[i]),
		}
	}

	return count, nil
}

func (p *packetizer) MaxPackets(frameSize int) int {
	// Worst case: each packet has MTU - RTP header (12 bytes) - payload header
	// For safety, assume ~100 bytes overhead per packet
	payloadPerPacket := int(p.config.MTU) - 100
	if payloadPerPacket <= 0 {
		payloadPerPacket = 1000
	}
	return (frameSize + payloadPerPacket - 1) / payloadPerPacket
}

func (p *packetizer) MaxPacketSize() int {
	return int(p.config.MTU)
}

func (p *packetizer) SequenceNumber() uint16 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 {
		return 0
	}
	return ffi.PacketizerSequenceNumber(p.handle)
}

func (p *packetizer) Close() error {
	if !p.closed.CompareAndSwap(false, true) {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle != 0 {
		ffi.PacketizerDestroy(p.handle)
		p.handle = 0
	}
	return nil
}
