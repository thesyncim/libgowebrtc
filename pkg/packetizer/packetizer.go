// Package packetizer provides RTP packetization for encoded media.
package packetizer

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/pion/rtp"
	pioncodecs "github.com/pion/rtp/codecs"

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
	Codec       codec.Type // Codec selects the RTP payloader and timestamp behavior.
	SSRC        uint32     // SSRC is the synchronization source written into output packets.
	PayloadType uint8      // PayloadType is the negotiated RTP payload type to emit.
	MTU         uint16     // Maximum transmission unit (typically 1200)
	ClockRate   uint32     // RTP clock rate (90000 for video, 48000 for Opus)
}

// PacketInfo describes a single RTP packet in the output buffer.
type PacketInfo struct {
	Offset int // Offset into the buffer where this packet starts
	Size   int // Size of this packet
}

// Packetizer converts encoded frames into RTP packets.
// All operations are allocation-free from the caller's perspective - caller
// provides the output buffer and packet metadata slices.
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

	// SequenceNumber returns the next sequence number that will be used.
	SequenceNumber() uint16

	// Close releases resources.
	Close() error
}

type packetizer struct {
	config    Config
	payloader rtp.Payloader
	sequence  uint32
	closed    atomic.Bool
	mu        sync.Mutex
}

// New creates a new RTP packetizer.
func New(cfg Config) (Packetizer, error) {
	if cfg.MTU == 0 {
		cfg.MTU = 1200
	}
	if cfg.MTU <= 12 {
		return nil, ErrBufferTooSmall
	}
	if cfg.ClockRate == 0 {
		return nil, fmt.Errorf("%w: clock rate is required", ErrInvalidData)
	}

	payloader, err := newPayloader(cfg.Codec)
	if err != nil {
		return nil, err
	}

	return &packetizer{
		config:    cfg,
		payloader: payloader,
	}, nil
}

func (p *packetizer) PacketizeInto(data []byte, timestamp uint32, _ bool, dst []byte, packets []PacketInfo) (int, error) {
	if p.closed.Load() {
		return 0, ErrPacketizerClosed
	}
	if len(data) == 0 {
		return 0, ErrInvalidData
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	payloads := p.payloader.Payload(p.config.MTU-12, data)
	if len(payloads) == 0 {
		return 0, ErrInvalidData
	}
	if len(payloads) > len(packets) {
		return 0, ErrBufferTooSmall
	}

	offset := 0
	sequence := uint16(p.sequence)
	for i, payload := range payloads {
		packet := rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         i == len(payloads)-1,
				PayloadType:    p.config.PayloadType,
				SequenceNumber: sequence,
				Timestamp:      timestamp,
				SSRC:           p.config.SSRC,
			},
			Payload: payload,
		}

		size := packet.MarshalSize()
		if len(dst[offset:]) < size {
			return 0, ErrBufferTooSmall
		}

		n, err := packet.MarshalTo(dst[offset : offset+size])
		if err != nil {
			return 0, err
		}
		packets[i] = PacketInfo{
			Offset: offset,
			Size:   n,
		}

		offset += n
		sequence++
	}

	p.sequence = uint32(sequence)
	return len(payloads), nil
}

func (p *packetizer) MaxPackets(frameSize int) int {
	// Worst case: each packet carries about MTU - RTP header - codec payload header.
	// Use a conservative header estimate so callers over-allocate.
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
	return uint16(p.sequence)
}

func (p *packetizer) Close() error {
	p.closed.Store(true)
	return nil
}

func newPayloader(codecType codec.Type) (rtp.Payloader, error) {
	switch codecType {
	case codec.H264:
		return &pioncodecs.H264Payloader{}, nil
	case codec.VP8:
		return &pioncodecs.VP8Payloader{}, nil
	case codec.VP9:
		return &pioncodecs.VP9Payloader{}, nil
	case codec.AV1:
		return &pioncodecs.AV1Payloader{}, nil
	case codec.Opus:
		return &pioncodecs.OpusPayloader{}, nil
	default:
		return nil, fmt.Errorf("unsupported packetizer codec %s", codecType)
	}
}
