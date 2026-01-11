package ffi_test

import (
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
)

// Benchmark FFI call overhead for purego vs cgo comparison.
// Run with: go test -bench=. -benchmem ./internal/ffi/
// Run with CGO: go test -tags cgo -bench=. -benchmem ./internal/ffi/
//
// Example output comparison:
//   go test -bench=. -benchmem ./internal/ffi/ > purego.txt
//   go test -tags cgo -bench=. -benchmem ./internal/ffi/ > cgo.txt
//   benchstat purego.txt cgo.txt

func BenchmarkShimVersion(b *testing.B) {
	if err := ffi.LoadLibrary(); err != nil {
		b.Skip("library not available:", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ffi.ShimVersion()
	}
}

func BenchmarkLibWebRTCVersion(b *testing.B) {
	if err := ffi.LoadLibrary(); err != nil {
		b.Skip("library not available:", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ffi.LibWebRTCVersion()
	}
}

func BenchmarkVideoEncoderCreate(b *testing.B) {
	if err := ffi.LoadLibrary(); err != nil {
		b.Skip("library not available:", err)
	}

	cfg := &ffi.VideoEncoderConfig{
		Width:      640,
		Height:     480,
		BitrateBps: 1_000_000,
		Framerate:  30,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc, err := ffi.CreateVideoEncoder(ffi.CodecVP8, cfg)
		if err != nil {
			b.Fatal(err)
		}
		ffi.VideoEncoderDestroy(enc)
	}
}

func BenchmarkVideoEncoderEncode(b *testing.B) {
	if err := ffi.LoadLibrary(); err != nil {
		b.Skip("library not available:", err)
	}

	cfg := &ffi.VideoEncoderConfig{
		Width:      640,
		Height:     480,
		BitrateBps: 1_000_000,
		Framerate:  30,
	}

	enc, err := ffi.CreateVideoEncoder(ffi.CodecVP8, cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer ffi.VideoEncoderDestroy(enc)

	// I420 frame
	ySize := 640 * 480
	uvSize := 320 * 240
	y := make([]byte, ySize)
	u := make([]byte, uvSize)
	v := make([]byte, uvSize)
	dst := make([]byte, ySize) // Output buffer

	b.SetBytes(int64(ySize + uvSize*2))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, _ = ffi.VideoEncoderEncodeInto(enc, y, u, v, 640, 320, 320, uint32(i), i%30 == 0, dst)
	}
}

func BenchmarkVideoDecoderCreate(b *testing.B) {
	if err := ffi.LoadLibrary(); err != nil {
		b.Skip("library not available:", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dec, err := ffi.CreateVideoDecoder(ffi.CodecVP8)
		if err != nil {
			b.Fatal(err)
		}
		ffi.VideoDecoderDestroy(dec)
	}
}
