package ffi

import (
	"fmt"
	"unsafe"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

// MaxErrorMsgLen matches SHIM_MAX_ERROR_MSG_LEN in shim.h
const MaxErrorMsgLen = 512

// ShimErrorBuffer matches the C struct for error message passing.
// This struct is memory-compatible with the C ShimErrorBuffer type.
type ShimErrorBuffer struct {
	Message [MaxErrorMsgLen]byte
}

// Ptr returns a uintptr to this error buffer for FFI calls.
func (e *ShimErrorBuffer) Ptr() uintptr {
	return uintptr(unsafe.Pointer(e))
}

// String returns the error message as a Go string.
func (e *ShimErrorBuffer) String() string {
	for i, b := range e.Message {
		if b == 0 {
			return string(e.Message[:i])
		}
	}
	return string(e.Message[:])
}

// Clear resets the error buffer.
func (e *ShimErrorBuffer) Clear() {
	e.Message[0] = 0
}

// ShimErrorWithMessage wraps a shim error code with a detailed message.
// It implements error and supports errors.Is()/errors.As().
type ShimErrorWithMessage struct {
	Code    int32
	Message string
}

// Error returns the error string including the message.
func (e *ShimErrorWithMessage) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", shimErrorName(e.Code), e.Message)
	}
	return shimErrorName(e.Code)
}

// Is implements errors.Is for sentinel error comparison.
func (e *ShimErrorWithMessage) Is(target error) bool {
	return ShimError(e.Code) == target
}

// Unwrap returns the underlying sentinel error.
func (e *ShimErrorWithMessage) Unwrap() error {
	return ShimError(e.Code)
}

// shimErrorName returns the error name for a code.
func shimErrorName(code int32) string {
	switch code {
	case ShimOK:
		return "ok"
	case ShimErrInvalidParam:
		return "invalid parameter"
	case ShimErrInitFailed:
		return "initialization failed"
	case ShimErrEncodeFailed:
		return "encode failed"
	case ShimErrDecodeFailed:
		return "decode failed"
	case ShimErrOutOfMemory:
		return "out of memory"
	case ShimErrNotSupported:
		return "not supported"
	case ShimErrNeedMoreData:
		return "need more data"
	case ShimErrBufferTooSmall:
		return "buffer too small"
	case ShimErrNotFound:
		return "not found"
	case ShimErrRenegotiationNeeded:
		return "renegotiation needed"
	default:
		return fmt.Sprintf("unknown error %d", code)
	}
}

// ToError converts the error buffer and code to a Go error.
// Returns nil if code is ShimOK.
func (e *ShimErrorBuffer) ToError(code int32) error {
	if code == ShimOK {
		return nil
	}
	msg := e.String()
	if msg == "" {
		return ShimError(code)
	}
	return &ShimErrorWithMessage{Code: code, Message: msg}
}

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
// C layout: codec(4) + ssrc(4) + pt(1) + pad(1) + mtu(2) + clockrate(4) = 16 bytes
type PacketizerConfig struct {
	Codec       int32
	SSRC        uint32
	PayloadType uint8
	_           byte   // 1 byte padding to align MTU
	MTU         uint16
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

// UintptrSlicePtr returns a uintptr to the first element of a uintptr slice.
func UintptrSlicePtr(s []uintptr) uintptr {
	if len(s) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&s[0]))
}

// GoBytes copies C memory to a Go byte slice and frees the C memory.
func GoBytes(ptr uintptr, size int) []byte {
	// Bounds validation
	const maxSize = 256 * 1024 * 1024 // 256MB max
	if ptr == 0 || size <= 0 || size > maxSize {
		return nil
	}

	// Copy the data using helper
	data := CopyBytesFromC(ptr, size)

	// Free the C memory
	shimFreeBuffer(ptr)

	return data
}

// GoInt16Slice copies C int16 array to a Go slice and frees the C memory.
func GoInt16Slice(ptr uintptr, numSamples int) []int16 {
	// Bounds validation
	const maxSamples = 48000 * 8 * 10 // 10 seconds of 8-channel 48kHz audio
	if ptr == 0 || numSamples <= 0 || numSamples > maxSamples {
		return nil
	}

	// Copy the data using helper
	samples := CopyInt16FromC(ptr, numSamples)

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

// GoString converts a C string pointer to a Go string.
func GoString(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	// Find the null terminator using unsafe.Add for safe pointer arithmetic
	var length int
	for {
		b := *(*byte)(unsafe.Add(ptr, length))
		if b == 0 {
			break
		}
		length++
		// Safety limit
		if length > 4096 {
			break
		}
	}
	if length == 0 {
		return ""
	}
	// Create slice header pointing to the data
	bytes := unsafe.Slice((*byte)(ptr), length)
	return string(bytes)
}

// UintptrFromSlice returns a uintptr to the first element of any slice.
func UintptrFromSlice[T any](s []T) uintptr {
	if len(s) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&s[0]))
}

// ByteArrayToString converts a fixed-size byte array to a Go string,
// stopping at the first null byte.
func ByteArrayToString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// ============================================================================
// C Memory Access Helpers
// ============================================================================
// These functions safely convert C memory pointers to Go types.
// They use //go:nocheckptr because go vet cannot verify that uintptr values
// from FFI callbacks point to valid C memory. This is a known limitation
// documented in https://github.com/golang/go/issues/58625

// CopyBytesFromC copies bytes from C memory to a new Go slice.
// Returns nil if ptr is 0 or size <= 0.
//
//go:nocheckptr
func CopyBytesFromC(ptr uintptr, size int) []byte {
	if ptr == 0 || size <= 0 {
		return nil
	}
	data := make([]byte, size)
	copy(data, unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size))
	return data
}

// CopyInt16FromC copies int16 values from C memory to a new Go slice.
// Returns nil if ptr is 0 or length <= 0.
//
//go:nocheckptr
func CopyInt16FromC(ptr uintptr, length int) []int16 {
	if ptr == 0 || length <= 0 {
		return nil
	}
	data := make([]int16, length)
	copy(data, unsafe.Slice((*int16)(unsafe.Pointer(ptr)), length))
	return data
}

// ReadUintptrFromC reads a uintptr value from C memory at the given address.
//
//go:nocheckptr
func ReadUintptrFromC(ptr uintptr) uintptr {
	if ptr == 0 {
		return 0
	}
	return *(*uintptr)(unsafe.Pointer(ptr))
}

// ReadInt32FromC reads an int32 value from C memory at the given address.
//
//go:nocheckptr
func ReadInt32FromC(ptr uintptr) int32 {
	if ptr == 0 {
		return 0
	}
	return *(*int32)(unsafe.Pointer(ptr))
}

// ReadUint32FromC reads a uint32 value from C memory at the given address.
//
//go:nocheckptr
func ReadUint32FromC(ptr uintptr) uint32 {
	if ptr == 0 {
		return 0
	}
	return *(*uint32)(unsafe.Pointer(ptr))
}

// ReadFloat64FromC reads a float64 value from C memory at the given address.
//
//go:nocheckptr
func ReadFloat64FromC(ptr uintptr) float64 {
	if ptr == 0 {
		return 0
	}
	return *(*float64)(unsafe.Pointer(ptr))
}

// PtrAt returns pointer offset by n bytes from base.
func PtrAt(base uintptr, offset uintptr) uintptr {
	return base + offset
}

// UnsafePointerFromC converts a uintptr from C to unsafe.Pointer.
// This is used for C string pointers returned by FFI functions.
//
//go:nocheckptr
//go:noinline
func UnsafePointerFromC(ptr uintptr) unsafe.Pointer {
	return unsafe.Pointer(ptr)
}

// GoStringFromC converts a C string pointer (as uintptr) to a Go string.
// This is a convenience function that combines UnsafePointerFromC and GoString.
func GoStringFromC(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	return GoString(UnsafePointerFromC(ptr))
}

// CodecTypeToFFI converts a codec.Type to the FFI CodecType.
func CodecTypeToFFI(t codec.Type) CodecType {
	switch t {
	case codec.H264:
		return CodecH264
	case codec.VP8:
		return CodecVP8
	case codec.VP9:
		return CodecVP9
	case codec.AV1:
		return CodecAV1
	default:
		return CodecH264
	}
}
