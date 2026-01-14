// Package testutil provides shared test utilities for libgowebrtc tests.
package testutil

import (
	"math"
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

// SkipIfNoShim fails the test if the shim library is not available.
// Note: Despite the name, this function now fails instead of skipping.
// The shim is always required for tests to be meaningful.
func SkipIfNoShim(t *testing.T) {
	t.Helper()
	if err := ffi.LoadLibrary(); err != nil {
		t.Fatalf("shim library required but not available: %v", err)
	}
}

// RequireShim fails the test if the shim library is not available.
func RequireShim(tb testing.TB) {
	tb.Helper()
	if err := ffi.LoadLibrary(); err != nil {
		tb.Fatalf("shim library required: %v", err)
	}
}

// CreateTestVideoFrame creates an I420 video frame with a gradient pattern.
// The pattern allows visual verification and is recognizable when decoded.
func CreateTestVideoFrame(width, height int) *frame.VideoFrame {
	f := frame.NewI420Frame(width, height)

	// Fill Y plane with gradient
	ySize := width * height
	for i := 0; i < ySize; i++ {
		y := i / width
		x := i % width
		// Create a diagonal gradient pattern
		f.Data[0][i] = byte((x + y) % 256)
	}

	// Fill U and V planes with mid-gray (128)
	uvWidth := (width + 1) / 2
	uvHeight := (height + 1) / 2
	uvSize := uvWidth * uvHeight
	for i := 0; i < uvSize; i++ {
		f.Data[1][i] = 128
		f.Data[2][i] = 128
	}

	return f
}

// CreateGrayVideoFrame creates a uniform gray I420 video frame.
// Gray frames compress very efficiently (useful for benchmarks).
func CreateGrayVideoFrame(width, height int) *frame.VideoFrame {
	f := frame.NewI420Frame(width, height)

	// Fill Y plane with mid-gray
	for i := range f.Data[0] {
		f.Data[0][i] = 128
	}

	// Fill U and V planes with mid-gray (no color)
	for i := range f.Data[1] {
		f.Data[1][i] = 128
	}
	for i := range f.Data[2] {
		f.Data[2][i] = 128
	}

	return f
}

// CreateTestAudioFrame creates an audio frame with a sine wave pattern.
// Uses S16 format (16-bit signed little-endian).
func CreateTestAudioFrame(sampleRate, channels, samplesPerChannel int) *frame.AudioFrame {
	f := frame.NewAudioFrameS16(sampleRate, channels, samplesPerChannel)

	// Generate sine wave at 440Hz
	frequency := 440.0
	amplitude := 10000.0

	totalSamples := samplesPerChannel * channels
	for i := 0; i < totalSamples; i++ {
		sampleIndex := i / channels
		t := float64(sampleIndex) / float64(sampleRate)
		value := int16(amplitude * math.Sin(2*math.Pi*frequency*t))

		// Write as little-endian
		f.Samples[i*2] = byte(value)
		f.Samples[i*2+1] = byte(value >> 8)
	}

	f.NumSamples = samplesPerChannel
	return f
}

// CreateSilentAudioFrame creates a silent audio frame.
// Silent frames compress very efficiently (useful for tests focusing on behavior, not compression).
func CreateSilentAudioFrame(sampleRate, channels, samplesPerChannel int) *frame.AudioFrame {
	f := frame.NewAudioFrameS16(sampleRate, channels, samplesPerChannel)
	// Samples are already zeroed by make()
	f.NumSamples = samplesPerChannel
	return f
}
