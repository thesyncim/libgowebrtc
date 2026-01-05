// Package frame provides video and audio frame types for media processing.
package frame

import (
	"sync"
	"time"
)

// PixelFormat represents the pixel format of a video frame.
type PixelFormat int

const (
	// PixelFormatI420 is the standard YUV 4:2:0 planar format.
	// Y plane followed by U plane followed by V plane.
	// Most commonly used format for video encoding.
	PixelFormatI420 PixelFormat = iota

	// PixelFormatNV12 is YUV 4:2:0 semi-planar format.
	// Y plane followed by interleaved UV plane.
	// Common on hardware encoders (VideoToolbox, VAAPI).
	PixelFormatNV12

	// PixelFormatNV21 is YUV 4:2:0 semi-planar format.
	// Y plane followed by interleaved VU plane.
	// Common on Android cameras.
	PixelFormatNV21

	// PixelFormatRGBA is 32-bit RGBA format.
	PixelFormatRGBA

	// PixelFormatBGRA is 32-bit BGRA format.
	PixelFormatBGRA
)

// String returns the string representation of the pixel format.
func (f PixelFormat) String() string {
	switch f {
	case PixelFormatI420:
		return "I420"
	case PixelFormatNV12:
		return "NV12"
	case PixelFormatNV21:
		return "NV21"
	case PixelFormatRGBA:
		return "RGBA"
	case PixelFormatBGRA:
		return "BGRA"
	default:
		return "Unknown"
	}
}

// VideoFrame represents a decoded video frame.
type VideoFrame struct {
	// Width of the frame in pixels.
	Width int

	// Height of the frame in pixels.
	Height int

	// Format specifies the pixel format.
	Format PixelFormat

	// Data contains the pixel data.
	// For I420: [Y, U, V] planes
	// For NV12/NV21: [Y, UV] planes
	// For RGBA/BGRA: single plane
	Data [][]byte

	// Stride is the number of bytes per row for each plane.
	Stride []int

	// Timestamp is the presentation timestamp.
	Timestamp time.Duration

	// PTS is the RTP timestamp (90kHz clock for video).
	PTS uint32

	// IsKeyframe indicates if this is an I-frame.
	IsKeyframe bool

	// pool is the pool this frame belongs to (for recycling).
	pool *VideoFramePool
}

// Release returns the frame to its pool for reuse.
// After calling Release, the frame must not be used.
func (f *VideoFrame) Release() {
	if f.pool != nil {
		f.pool.Put(f)
	}
}

// Clone creates a deep copy of the frame.
func (f *VideoFrame) Clone() *VideoFrame {
	clone := &VideoFrame{
		Width:      f.Width,
		Height:     f.Height,
		Format:     f.Format,
		Timestamp:  f.Timestamp,
		PTS:        f.PTS,
		IsKeyframe: f.IsKeyframe,
		Data:       make([][]byte, len(f.Data)),
		Stride:     make([]int, len(f.Stride)),
	}

	for i, plane := range f.Data {
		clone.Data[i] = make([]byte, len(plane))
		copy(clone.Data[i], plane)
	}
	copy(clone.Stride, f.Stride)

	return clone
}

// YPlane returns the Y plane data for YUV formats.
// Returns nil for non-YUV formats.
func (f *VideoFrame) YPlane() []byte {
	if len(f.Data) > 0 {
		return f.Data[0]
	}
	return nil
}

// UPlane returns the U plane data for I420 format.
// Returns nil for other formats.
func (f *VideoFrame) UPlane() []byte {
	if f.Format == PixelFormatI420 && len(f.Data) > 1 {
		return f.Data[1]
	}
	return nil
}

// VPlane returns the V plane data for I420 format.
// Returns nil for other formats.
func (f *VideoFrame) VPlane() []byte {
	if f.Format == PixelFormatI420 && len(f.Data) > 2 {
		return f.Data[2]
	}
	return nil
}

// UVPlane returns the interleaved UV plane for NV12/NV21 formats.
// Returns nil for other formats.
func (f *VideoFrame) UVPlane() []byte {
	if (f.Format == PixelFormatNV12 || f.Format == PixelFormatNV21) && len(f.Data) > 1 {
		return f.Data[1]
	}
	return nil
}

// NewI420Frame creates a new I420 video frame with allocated buffers.
func NewI420Frame(width, height int) *VideoFrame {
	// Calculate plane sizes
	ySize := width * height
	uvWidth := (width + 1) / 2
	uvHeight := (height + 1) / 2
	uvSize := uvWidth * uvHeight

	return &VideoFrame{
		Width:  width,
		Height: height,
		Format: PixelFormatI420,
		Data: [][]byte{
			make([]byte, ySize),
			make([]byte, uvSize),
			make([]byte, uvSize),
		},
		Stride: []int{width, uvWidth, uvWidth},
	}
}

// NewNV12Frame creates a new NV12 video frame with allocated buffers.
func NewNV12Frame(width, height int) *VideoFrame {
	ySize := width * height
	uvWidth := (width + 1) / 2
	uvHeight := (height + 1) / 2
	uvSize := uvWidth * uvHeight * 2 // Interleaved UV

	return &VideoFrame{
		Width:  width,
		Height: height,
		Format: PixelFormatNV12,
		Data: [][]byte{
			make([]byte, ySize),
			make([]byte, uvSize),
		},
		Stride: []int{width, width}, // UV stride equals width (interleaved)
	}
}

// VideoFramePool manages reusable video frames to reduce allocations.
type VideoFramePool struct {
	mu      sync.Mutex
	frames  []*VideoFrame
	maxSize int
	width   int
	height  int
	format  PixelFormat
}

// NewVideoFramePool creates a pool for video frames of a specific size and format.
func NewVideoFramePool(width, height int, format PixelFormat, poolSize int) *VideoFramePool {
	pool := &VideoFramePool{
		maxSize: poolSize,
		width:   width,
		height:  height,
		format:  format,
		frames:  make([]*VideoFrame, 0, poolSize),
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
func (p *VideoFramePool) Get() *VideoFrame {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.frames) > 0 {
		f := p.frames[len(p.frames)-1]
		p.frames = p.frames[:len(p.frames)-1]
		// Reset metadata
		f.Timestamp = 0
		f.PTS = 0
		f.IsKeyframe = false
		return f
	}

	// Pool exhausted, allocate new
	f := p.allocFrame()
	f.pool = p
	return f
}

// Put returns a frame to the pool.
func (p *VideoFramePool) Put(f *VideoFrame) {
	if f == nil || f.pool != p {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.frames) < p.maxSize {
		p.frames = append(p.frames, f)
	}
	// Otherwise let GC handle it
}

func (p *VideoFramePool) allocFrame() *VideoFrame {
	switch p.format {
	case PixelFormatNV12:
		return NewNV12Frame(p.width, p.height)
	default:
		return NewI420Frame(p.width, p.height)
	}
}
