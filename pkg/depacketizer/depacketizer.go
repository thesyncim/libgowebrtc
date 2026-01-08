// Package depacketizer provides RTP depacketization using libwebrtc.
package depacketizer

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

// Errors
var (
	ErrDepacketizerClosed = errors.New("depacketizer is closed")
	ErrNeedMoreData       = errors.New("need more data")
	ErrBufferTooSmall     = errors.New("buffer too small")
)

// FrameInfo contains metadata about a reassembled frame.
type FrameInfo struct {
	Size       int
	Timestamp  uint32
	IsKeyframe bool
}

// Depacketizer reassembles RTP packets into complete frames.
// All operations are allocation-free - caller provides buffers.
type Depacketizer interface {
	// Push adds an RTP packet to the reassembly buffer.
	Push(packet []byte) error

	// PopInto attempts to pop a complete frame into the provided buffer.
	// Returns ErrNeedMoreData if no complete frame is available.
	PopInto(dst []byte) (FrameInfo, error)

	// Close releases resources.
	Close() error
}

type depacketizer struct {
	handle    uintptr
	codecType codec.Type
	closed    atomic.Bool
	mu        sync.Mutex
}

// New creates a new RTP depacketizer.
func New(codecType codec.Type) (Depacketizer, error) {
	if err := ffi.LoadLibrary(); err != nil {
		return nil, err
	}

	d := &depacketizer{codecType: codecType}
	if err := d.init(); err != nil {
		return nil, err
	}

	return d, nil
}

func (d *depacketizer) init() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	handle := ffi.CreateDepacketizer(ffi.CodecType(d.codecType))
	if handle == 0 {
		return errors.New("failed to create depacketizer")
	}

	d.handle = handle
	return nil
}

func (d *depacketizer) Push(packet []byte) error {
	if d.closed.Load() {
		return ErrDepacketizerClosed
	}
	if len(packet) == 0 {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.handle == 0 {
		return ErrDepacketizerClosed
	}

	return ffi.DepacketizerPush(d.handle, packet)
}

func (d *depacketizer) PopInto(dst []byte) (FrameInfo, error) {
	if d.closed.Load() {
		return FrameInfo{}, ErrDepacketizerClosed
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.handle == 0 {
		return FrameInfo{}, ErrDepacketizerClosed
	}

	size, timestamp, isKeyframe, err := ffi.DepacketizerPopInto(d.handle, dst)
	if err != nil {
		if errors.Is(err, ffi.ErrNeedMoreData) {
			return FrameInfo{}, ErrNeedMoreData
		}
		return FrameInfo{}, err
	}

	return FrameInfo{
		Size:       size,
		Timestamp:  timestamp,
		IsKeyframe: isKeyframe,
	}, nil
}

func (d *depacketizer) Close() error {
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle != 0 {
		ffi.DepacketizerDestroy(d.handle)
		d.handle = 0
	}
	return nil
}
