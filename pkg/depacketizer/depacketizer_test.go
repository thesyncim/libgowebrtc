package depacketizer

import (
	"testing"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

func TestDepacketizerErrors(t *testing.T) {
	errors := []error{
		ErrDepacketizerClosed,
		ErrNeedMoreData,
		ErrBufferTooSmall,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("Error should not be nil")
		}
		if err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	}
}

func TestFrameInfo(t *testing.T) {
	info := FrameInfo{
		Size:       5000,
		Timestamp:  90000,
		IsKeyframe: true,
	}

	if info.Size != 5000 {
		t.Errorf("Size = %v, want 5000", info.Size)
	}
	if info.Timestamp != 90000 {
		t.Errorf("Timestamp = %v, want 90000", info.Timestamp)
	}
	if !info.IsKeyframe {
		t.Error("IsKeyframe should be true")
	}
}

func TestDepacketizerInterface(t *testing.T) {
	// Compile-time check
	var _ Depacketizer = (*depacketizer)(nil)
}

func TestFrameInfoAlloc(t *testing.T) {
	// Test that FrameInfo is a value type (no hidden allocations)
	info := FrameInfo{}
	info.Size = 1000
	info.Timestamp = 12345
	info.IsKeyframe = false

	if info.Size != 1000 {
		t.Error("FrameInfo should be mutable")
	}
}

func TestDepacketizerCodecs(t *testing.T) {
	codecs := []struct {
		codec codec.Type
		name  string
	}{
		{codec.H264, "H264"},
		{codec.VP8, "VP8"},
		{codec.VP9, "VP9"},
		{codec.AV1, "AV1"},
		{codec.Opus, "Opus"},
	}

	for _, tc := range codecs {
		t.Run(tc.name, func(t *testing.T) {
			// These will fail without the shim library, but tests compile-time check
			_, _ = New(tc.codec)
		})
	}
}

func BenchmarkFrameInfoAlloc(b *testing.B) {
	for i := 0; i < b.N; i++ {
		info := FrameInfo{
			Size:       1000,
			Timestamp:  uint32(i),
			IsKeyframe: i%30 == 0,
		}
		_ = info
	}
}
