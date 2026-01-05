package ffi

import (
	"testing"
)

// --- Error Code Tests ---

func TestShimErrorCodes(t *testing.T) {
	tests := []struct {
		code    int
		wantErr bool
		errMsg  string
	}{
		{ShimOK, false, ""},
		{ShimErrInvalidParam, true, "invalid parameter"},
		{ShimErrInitFailed, true, "initialization failed"},
		{ShimErrEncodeFailed, true, "encode failed"},
		{ShimErrDecodeFailed, true, "decode failed"},
		{ShimErrOutOfMemory, true, "out of memory"},
		{ShimErrNotSupported, true, "not supported"},
		{ShimErrNeedMoreData, true, "need more data"},
		{-999, true, "unknown error: -999"},
	}

	for _, tt := range tests {
		err := ShimError(tt.code)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ShimError(%d) = nil, want error", tt.code)
			} else if err.Error() != tt.errMsg {
				t.Errorf("ShimError(%d) = %q, want %q", tt.code, err.Error(), tt.errMsg)
			}
		} else {
			if err != nil {
				t.Errorf("ShimError(%d) = %v, want nil", tt.code, err)
			}
		}
	}
}

func TestShimErrorConstants(t *testing.T) {
	// Verify error codes match expected values
	if ShimOK != 0 {
		t.Errorf("ShimOK = %d, want 0", ShimOK)
	}
	if ShimErrInvalidParam != -1 {
		t.Errorf("ShimErrInvalidParam = %d, want -1", ShimErrInvalidParam)
	}
	if ShimErrNeedMoreData != -7 {
		t.Errorf("ShimErrNeedMoreData = %d, want -7", ShimErrNeedMoreData)
	}
}

// --- Codec Type Tests ---

func TestCodecTypes(t *testing.T) {
	tests := []struct {
		codec CodecType
		value int
	}{
		{CodecH264, 0},
		{CodecVP8, 1},
		{CodecVP9, 2},
		{CodecAV1, 3},
		{CodecOpus, 10},
	}

	for _, tt := range tests {
		if int(tt.codec) != tt.value {
			t.Errorf("CodecType %v = %d, want %d", tt.codec, int(tt.codec), tt.value)
		}
	}
}

// --- Type Helper Tests ---

func TestByteSlicePtr(t *testing.T) {
	// Empty slice should return 0
	empty := []byte{}
	if ptr := ByteSlicePtr(empty); ptr != 0 {
		t.Errorf("ByteSlicePtr(empty) = %d, want 0", ptr)
	}

	// Non-empty slice should return non-zero
	data := []byte{1, 2, 3, 4}
	if ptr := ByteSlicePtr(data); ptr == 0 {
		t.Error("ByteSlicePtr(data) = 0, want non-zero")
	}
}

func TestInt16SlicePtr(t *testing.T) {
	empty := []int16{}
	if ptr := Int16SlicePtr(empty); ptr != 0 {
		t.Errorf("Int16SlicePtr(empty) = %d, want 0", ptr)
	}

	data := []int16{1, 2, 3}
	if ptr := Int16SlicePtr(data); ptr == 0 {
		t.Error("Int16SlicePtr(data) = 0, want non-zero")
	}
}

func TestInt32SlicePtr(t *testing.T) {
	empty := []int32{}
	if ptr := Int32SlicePtr(empty); ptr != 0 {
		t.Errorf("Int32SlicePtr(empty) = %d, want 0", ptr)
	}

	data := []int32{1, 2, 3}
	if ptr := Int32SlicePtr(data); ptr == 0 {
		t.Error("Int32SlicePtr(data) = 0, want non-zero")
	}
}

func TestIntPtr(t *testing.T) {
	var val int = 42
	ptr := IntPtr(&val)
	if ptr == 0 {
		t.Error("IntPtr should return non-zero for valid pointer")
	}
}

func TestInt32Ptr(t *testing.T) {
	var val int32 = 42
	ptr := Int32Ptr(&val)
	if ptr == 0 {
		t.Error("Int32Ptr should return non-zero for valid pointer")
	}
}

func TestUint32Ptr(t *testing.T) {
	var val uint32 = 42
	ptr := Uint32Ptr(&val)
	if ptr == 0 {
		t.Error("Uint32Ptr should return non-zero for valid pointer")
	}
}

func TestBoolPtr(t *testing.T) {
	var val int32 = 1
	ptr := BoolPtr(&val)
	if ptr == 0 {
		t.Error("BoolPtr should return non-zero for valid pointer")
	}
}

func TestUintptrPtr(t *testing.T) {
	var val uintptr = 12345
	ptr := UintptrPtr(&val)
	if ptr == 0 {
		t.Error("UintptrPtr should return non-zero for valid pointer")
	}
}

// --- CString Tests ---

func TestCString(t *testing.T) {
	s := "hello"
	cstr := CString(s)

	if len(cstr) != len(s)+1 {
		t.Errorf("CString length = %d, want %d", len(cstr), len(s)+1)
	}

	// Check null terminator
	if cstr[len(cstr)-1] != 0 {
		t.Error("CString should be null-terminated")
	}

	// Check content
	for i := 0; i < len(s); i++ {
		if cstr[i] != s[i] {
			t.Errorf("CString[%d] = %d, want %d", i, cstr[i], s[i])
		}
	}
}

func TestCStringPtr(t *testing.T) {
	s := "test"
	ptr := CStringPtr(s)
	if ptr == nil {
		t.Error("CStringPtr should return non-nil")
	}
}

func TestCStringEmpty(t *testing.T) {
	cstr := CString("")
	if len(cstr) != 1 || cstr[0] != 0 {
		t.Error("CString(\"\") should be single null byte")
	}
}

// --- Config Struct Tests ---

func TestVideoEncoderConfigPtr(t *testing.T) {
	cfg := VideoEncoderConfig{
		Width:      1920,
		Height:     1080,
		BitrateBps: 4_000_000,
		Framerate:  30.0,
	}

	ptr := cfg.Ptr()
	if ptr == 0 {
		t.Error("VideoEncoderConfig.Ptr() should return non-zero")
	}
}

func TestAudioEncoderConfigPtr(t *testing.T) {
	cfg := AudioEncoderConfig{
		SampleRate: 48000,
		Channels:   2,
		BitrateBps: 64000,
	}

	ptr := cfg.Ptr()
	if ptr == 0 {
		t.Error("AudioEncoderConfig.Ptr() should return non-zero")
	}
}

func TestPacketizerConfigPtr(t *testing.T) {
	cfg := PacketizerConfig{
		Codec:       int32(CodecH264),
		SSRC:        12345,
		PayloadType: 96,
		MTU:         1200,
		ClockRate:   90000,
	}

	ptr := cfg.Ptr()
	if ptr == 0 {
		t.Error("PacketizerConfig.Ptr() should return non-zero")
	}
}

// --- Library State Tests ---

func TestIsLoadedBeforeLoad(t *testing.T) {
	// Note: This test assumes the shim library is NOT loaded in test environment
	// If the library IS loaded, this test will be skipped
	if IsLoaded() {
		t.Skip("Library already loaded, skipping")
	}
}

func TestErrLibraryNotLoaded(t *testing.T) {
	if ErrLibraryNotLoaded == nil {
		t.Error("ErrLibraryNotLoaded should not be nil")
	}
	if ErrLibraryNotLoaded.Error() == "" {
		t.Error("ErrLibraryNotLoaded should have message")
	}
}

func TestErrLibraryNotFound(t *testing.T) {
	if ErrLibraryNotFound == nil {
		t.Error("ErrLibraryNotFound should not be nil")
	}
	if ErrLibraryNotFound.Error() == "" {
		t.Error("ErrLibraryNotFound should have message")
	}
}

// --- GoBytes Tests (without shim) ---

func TestGoBytesNilPtr(t *testing.T) {
	result := GoBytes(0, 10)
	if result != nil {
		t.Error("GoBytes(0, n) should return nil")
	}
}

func TestGoBytesZeroSize(t *testing.T) {
	result := GoBytes(12345, 0)
	if result != nil {
		t.Error("GoBytes(ptr, 0) should return nil")
	}
}

func TestGoBytesNegativeSize(t *testing.T) {
	result := GoBytes(12345, -1)
	if result != nil {
		t.Error("GoBytes(ptr, -1) should return nil")
	}
}

func TestGoInt16SliceNilPtr(t *testing.T) {
	result := GoInt16Slice(0, 10)
	if result != nil {
		t.Error("GoInt16Slice(0, n) should return nil")
	}
}

func TestGoInt16SliceZeroSamples(t *testing.T) {
	result := GoInt16Slice(12345, 0)
	if result != nil {
		t.Error("GoInt16Slice(ptr, 0) should return nil")
	}
}

// --- Benchmark Tests ---

func BenchmarkByteSlicePtr(b *testing.B) {
	data := make([]byte, 1920*1080*3/2) // I420 frame size
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ByteSlicePtr(data)
	}
}

func BenchmarkCString(b *testing.B) {
	s := "42e014"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CString(s)
	}
}

func BenchmarkVideoEncoderConfigPtr(b *testing.B) {
	cfg := VideoEncoderConfig{
		Width:      1920,
		Height:     1080,
		BitrateBps: 4_000_000,
		Framerate:  30.0,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.Ptr()
	}
}

func BenchmarkShimError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ShimError(ShimOK)
		_ = ShimError(ShimErrEncodeFailed)
	}
}
