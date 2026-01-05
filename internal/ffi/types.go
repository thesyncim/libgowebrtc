package ffi

import (
	"unsafe"
)

// VideoEncoderConfig matches ShimVideoEncoderConfig in shim.h
type VideoEncoderConfig struct {
	Width            int32
	Height           int32
	BitrateBps       uint32
	Framerate        float32
	KeyframeInterval int32
	H264Profile      *byte // C string pointer
	VP9Profile       int32
	PreferHW         int32 // bool as int
}

// AudioEncoderConfig matches ShimAudioEncoderConfig in shim.h
type AudioEncoderConfig struct {
	SampleRate int32
	Channels   int32
	BitrateBps uint32
}

// PacketizerConfig matches ShimPacketizerConfig in shim.h
type PacketizerConfig struct {
	Codec       int32
	SSRC        uint32
	PayloadType uint8
	_           [3]byte // padding
	MTU         uint16
	_           [2]byte // padding
	ClockRate   uint32
}

// Ptr returns a pointer to the config as uintptr for FFI calls.
func (c *VideoEncoderConfig) Ptr() uintptr {
	return uintptr(unsafe.Pointer(c))
}

// Ptr returns a pointer to the config as uintptr for FFI calls.
func (c *AudioEncoderConfig) Ptr() uintptr {
	return uintptr(unsafe.Pointer(c))
}

// Ptr returns a pointer to the config as uintptr for FFI calls.
func (c *PacketizerConfig) Ptr() uintptr {
	return uintptr(unsafe.Pointer(c))
}

// ByteSlicePtr returns a uintptr to the first element of a byte slice.
// Returns 0 if the slice is empty.
func ByteSlicePtr(b []byte) uintptr {
	if len(b) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&b[0]))
}

// Int16SlicePtr returns a uintptr to the first element of an int16 slice.
func Int16SlicePtr(s []int16) uintptr {
	if len(s) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&s[0]))
}

// Int32SlicePtr returns a uintptr to the first element of an int32 slice.
func Int32SlicePtr(s []int32) uintptr {
	if len(s) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&s[0]))
}

// UintptrPtr returns a uintptr to a uintptr variable.
func UintptrPtr(p *uintptr) uintptr {
	return uintptr(unsafe.Pointer(p))
}

// IntPtr returns a uintptr to an int variable.
func IntPtr(p *int) uintptr {
	return uintptr(unsafe.Pointer(p))
}

// Int32Ptr returns a uintptr to an int32 variable.
func Int32Ptr(p *int32) uintptr {
	return uintptr(unsafe.Pointer(p))
}

// Uint32Ptr returns a uintptr to a uint32 variable.
func Uint32Ptr(p *uint32) uintptr {
	return uintptr(unsafe.Pointer(p))
}

// BoolPtr returns a uintptr to an int32 used as bool.
func BoolPtr(p *int32) uintptr {
	return uintptr(unsafe.Pointer(p))
}

// GoBytes copies C memory to a Go byte slice and frees the C memory.
func GoBytes(ptr uintptr, size int) []byte {
	if ptr == 0 || size <= 0 {
		return nil
	}

	// Copy the data
	data := make([]byte, size)
	src := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size)
	copy(data, src)

	// Free the C memory
	shimFreeBuffer(ptr)

	return data
}

// GoInt16Slice copies C int16 array to a Go slice and frees the C memory.
func GoInt16Slice(ptr uintptr, numSamples int) []int16 {
	if ptr == 0 || numSamples <= 0 {
		return nil
	}

	// Copy the data
	samples := make([]int16, numSamples)
	src := unsafe.Slice((*int16)(unsafe.Pointer(ptr)), numSamples)
	copy(samples, src)

	// Free the C memory
	shimFreeBuffer(ptr)

	return samples
}

// CString allocates a null-terminated C string from a Go string.
// The caller is responsible for keeping the returned byte slice alive
// for as long as the C code needs it.
func CString(s string) []byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	b[len(s)] = 0
	return b
}

// CStringPtr returns a pointer to a null-terminated C string.
func CStringPtr(s string) *byte {
	b := CString(s)
	return &b[0]
}
