package ffi

import (
	"errors"
	"testing"
	"unsafe"
)

func TestCopyBytesFromC_NullPointer(t *testing.T) {
	result := CopyBytesFromC(0, 100)
	if result != nil {
		t.Errorf("expected nil for null pointer, got %v", result)
	}
}

func TestCopyBytesFromC_ZeroSize(t *testing.T) {
	// Use a valid pointer but zero size
	data := []byte{1, 2, 3}
	ptr := uintptr(unsafe.Pointer(&data[0]))

	result := CopyBytesFromC(ptr, 0)
	if result != nil {
		t.Errorf("expected nil for zero size, got %v", result)
	}
}

func TestCopyBytesFromC_NegativeSize(t *testing.T) {
	data := []byte{1, 2, 3}
	ptr := uintptr(unsafe.Pointer(&data[0]))

	result := CopyBytesFromC(ptr, -1)
	if result != nil {
		t.Errorf("expected nil for negative size, got %v", result)
	}
}

func TestCopyBytesFromC_ValidData(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5}
	ptr := uintptr(unsafe.Pointer(&data[0]))

	result := CopyBytesFromC(ptr, 3)
	if len(result) != 3 {
		t.Fatalf("expected length 3, got %d", len(result))
	}
	if result[0] != 1 || result[1] != 2 || result[2] != 3 {
		t.Errorf("data mismatch: %v", result)
	}
}

func TestCopyInt16FromC_NullPointer(t *testing.T) {
	result := CopyInt16FromC(0, 10)
	if result != nil {
		t.Errorf("expected nil for null pointer, got %v", result)
	}
}

func TestCopyInt16FromC_ZeroLength(t *testing.T) {
	data := []int16{1, 2, 3}
	ptr := uintptr(unsafe.Pointer(&data[0]))

	result := CopyInt16FromC(ptr, 0)
	if result != nil {
		t.Errorf("expected nil for zero length, got %v", result)
	}
}

func TestCopyInt16FromC_ValidData(t *testing.T) {
	data := []int16{100, 200, 300, 400}
	ptr := uintptr(unsafe.Pointer(&data[0]))

	result := CopyInt16FromC(ptr, 3)
	if len(result) != 3 {
		t.Fatalf("expected length 3, got %d", len(result))
	}
	if result[0] != 100 || result[1] != 200 || result[2] != 300 {
		t.Errorf("data mismatch: %v", result)
	}
}

func TestShimErrorBuffer_Empty(t *testing.T) {
	var buf ShimErrorBuffer
	if buf.String() != "" {
		t.Errorf("expected empty string, got %q", buf.String())
	}
}

func TestShimErrorBuffer_WithMessage(t *testing.T) {
	var buf ShimErrorBuffer
	msg := "test error message"
	copy(buf.Message[:], msg)

	if buf.String() != msg {
		t.Errorf("expected %q, got %q", msg, buf.String())
	}
}

func TestShimErrorBuffer_ToError_OK(t *testing.T) {
	var buf ShimErrorBuffer
	copy(buf.Message[:], "this should be ignored")

	err := buf.ToError(ShimOK)
	if err != nil {
		t.Errorf("expected nil for ShimOK, got %v", err)
	}
}

func TestShimErrorBuffer_ToError_WithMessage(t *testing.T) {
	var buf ShimErrorBuffer
	msg := "detailed error info"
	copy(buf.Message[:], msg)

	err := buf.ToError(ShimErrInitFailed)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should contain the message
	if errStr := err.Error(); errStr == "" {
		t.Error("error string is empty")
	}

	// Should support errors.Is
	if !errors.Is(err, ErrInitFailed) {
		t.Error("errors.Is should match ErrInitFailed")
	}
}

func TestShimErrorBuffer_ToError_NoMessage(t *testing.T) {
	var buf ShimErrorBuffer // empty message

	err := buf.ToError(ShimErrInvalidParam)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should be the sentinel error itself
	if err != ErrInvalidParam {
		t.Errorf("expected ErrInvalidParam, got %v", err)
	}
}

func TestShimErrorBuffer_Clear(t *testing.T) {
	var buf ShimErrorBuffer
	copy(buf.Message[:], "some message")

	buf.Clear()

	if buf.String() != "" {
		t.Errorf("expected empty after Clear, got %q", buf.String())
	}
}

func TestShimError_AllCodes(t *testing.T) {
	tests := []struct {
		code     int32
		expected error
	}{
		{ShimOK, nil},
		{ShimErrInvalidParam, ErrInvalidParam},
		{ShimErrInitFailed, ErrInitFailed},
		{ShimErrEncodeFailed, ErrEncodeFailed},
		{ShimErrDecodeFailed, ErrDecodeFailed},
		{ShimErrOutOfMemory, ErrOutOfMemory},
		{ShimErrNotSupported, ErrNotSupported},
		{ShimErrNeedMoreData, ErrNeedMoreData},
		{ShimErrBufferTooSmall, ErrBufferTooSmall},
		{ShimErrNotFound, ErrNotFound},
		{ShimErrRenegotiationNeeded, ErrRenegotiationNeeded},
	}

	for _, tc := range tests {
		result := ShimError(tc.code)
		if result != tc.expected {
			t.Errorf("ShimError(%d) = %v, want %v", tc.code, result, tc.expected)
		}
	}
}

func TestShimError_UnknownCode(t *testing.T) {
	err := ShimError(-999)
	if err == nil {
		t.Error("expected error for unknown code")
	}
}

func TestByteSlicePtr_Empty(t *testing.T) {
	var empty []byte
	ptr := ByteSlicePtr(empty)
	if ptr != 0 {
		t.Errorf("expected 0 for empty slice, got %d", ptr)
	}
}

func TestByteSlicePtr_Valid(t *testing.T) {
	data := []byte{1, 2, 3}
	ptr := ByteSlicePtr(data)
	if ptr == 0 {
		t.Error("expected non-zero pointer for valid slice")
	}
}

func TestByteArrayToString_WithNull(t *testing.T) {
	data := []byte{'h', 'e', 'l', 'l', 'o', 0, 'x', 'y', 'z'}
	result := ByteArrayToString(data)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestByteArrayToString_NoNull(t *testing.T) {
	data := []byte{'h', 'e', 'l', 'l', 'o'}
	result := ByteArrayToString(data)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestByteArrayToString_Empty(t *testing.T) {
	var data []byte
	result := ByteArrayToString(data)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestReadUintptrFromC_Null(t *testing.T) {
	result := ReadUintptrFromC(0)
	if result != 0 {
		t.Errorf("expected 0 for null pointer, got %d", result)
	}
}

func TestReadInt32FromC_Null(t *testing.T) {
	result := ReadInt32FromC(0)
	if result != 0 {
		t.Errorf("expected 0 for null pointer, got %d", result)
	}
}

func TestReadUint32FromC_Null(t *testing.T) {
	result := ReadUint32FromC(0)
	if result != 0 {
		t.Errorf("expected 0 for null pointer, got %d", result)
	}
}

func TestReadFloat64FromC_Null(t *testing.T) {
	result := ReadFloat64FromC(0)
	if result != 0 {
		t.Errorf("expected 0 for null pointer, got %f", result)
	}
}

func TestGoStringFromC_Null(t *testing.T) {
	result := GoStringFromC(0)
	if result != "" {
		t.Errorf("expected empty string for null pointer, got %q", result)
	}
}
