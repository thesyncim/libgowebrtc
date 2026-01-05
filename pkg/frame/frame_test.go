package frame

import (
	"testing"
	"time"
)

func TestNewI420Frame(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"720p", 1280, 720},
		{"1080p", 1920, 1080},
		{"4K", 3840, 2160},
		{"small", 320, 240},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewI420Frame(tt.width, tt.height)

			if f.Width != tt.width {
				t.Errorf("Width = %v, want %v", f.Width, tt.width)
			}
			if f.Height != tt.height {
				t.Errorf("Height = %v, want %v", f.Height, tt.height)
			}
			if f.Format != PixelFormatI420 {
				t.Errorf("Format = %v, want %v", f.Format, PixelFormatI420)
			}

			// I420: Y plane = w*h, U plane = w*h/4, V plane = w*h/4
			expectedY := tt.width * tt.height
			expectedUV := expectedY / 4

			if len(f.Data[0]) != expectedY {
				t.Errorf("Y plane size = %v, want %v", len(f.Data[0]), expectedY)
			}
			if len(f.Data[1]) != expectedUV {
				t.Errorf("U plane size = %v, want %v", len(f.Data[1]), expectedUV)
			}
			if len(f.Data[2]) != expectedUV {
				t.Errorf("V plane size = %v, want %v", len(f.Data[2]), expectedUV)
			}

			// Strides
			if f.Stride[0] != tt.width {
				t.Errorf("Y stride = %v, want %v", f.Stride[0], tt.width)
			}
			if f.Stride[1] != tt.width/2 {
				t.Errorf("U stride = %v, want %v", f.Stride[1], tt.width/2)
			}
			if f.Stride[2] != tt.width/2 {
				t.Errorf("V stride = %v, want %v", f.Stride[2], tt.width/2)
			}
		})
	}
}

func TestNewNV12Frame(t *testing.T) {
	f := NewNV12Frame(1920, 1080)

	if f.Width != 1920 || f.Height != 1080 {
		t.Errorf("Dimensions = %dx%d, want 1920x1080", f.Width, f.Height)
	}
	if f.Format != PixelFormatNV12 {
		t.Errorf("Format = %v, want %v", f.Format, PixelFormatNV12)
	}

	// NV12: Y plane = w*h, UV plane = w*h/2 (interleaved)
	expectedY := 1920 * 1080
	expectedUV := expectedY / 2

	if len(f.Data[0]) != expectedY {
		t.Errorf("Y plane size = %v, want %v", len(f.Data[0]), expectedY)
	}
	if len(f.Data[1]) != expectedUV {
		t.Errorf("UV plane size = %v, want %v", len(f.Data[1]), expectedUV)
	}
}

func TestVideoFrameClone(t *testing.T) {
	original := NewI420Frame(1280, 720)
	original.PTS = 12345
	original.Timestamp = time.Second * 10

	// Fill with test data
	for i := range original.Data[0] {
		original.Data[0][i] = byte(i % 256)
	}

	clone := original.Clone()

	// Verify dimensions
	if clone.Width != original.Width || clone.Height != original.Height {
		t.Error("Clone dimensions don't match")
	}
	if clone.Format != original.Format {
		t.Error("Clone format doesn't match")
	}
	if clone.PTS != original.PTS {
		t.Error("Clone PTS doesn't match")
	}
	if clone.Timestamp != original.Timestamp {
		t.Error("Clone Timestamp doesn't match")
	}

	// Verify data is copied (not same slice)
	if &clone.Data[0][0] == &original.Data[0][0] {
		t.Error("Clone should have separate data buffer")
	}

	// Verify data content
	for i := range original.Data[0] {
		if clone.Data[0][i] != original.Data[0][i] {
			t.Errorf("Clone data mismatch at %d", i)
			break
		}
	}
}

func TestVideoFramePool(t *testing.T) {
	pool := NewVideoFramePool(1280, 720, PixelFormatI420, 4)

	// Get frames from pool
	frames := make([]*VideoFrame, 4)
	for i := 0; i < 4; i++ {
		frames[i] = pool.Get()
		if frames[i] == nil {
			t.Fatalf("Got nil frame at index %d", i)
		}
		if frames[i].Width != 1280 || frames[i].Height != 720 {
			t.Error("Frame dimensions don't match pool config")
		}
	}

	// Pool should be exhausted, next get allocates new
	extra := pool.Get()
	if extra == nil {
		t.Error("Pool should allocate new frame when exhausted")
	}

	// Return frames to pool
	for _, f := range frames {
		f.Release()
	}

	// Get again - should get from pool
	reused := pool.Get()
	if reused == nil {
		t.Error("Should get reused frame from pool")
	}

	// PTS should be reset
	if reused.PTS != 0 {
		t.Error("PTS should be reset on reuse")
	}
}

func TestAudioFrameS16(t *testing.T) {
	f := NewAudioFrameS16(48000, 2, 960) // 20ms at 48kHz stereo

	if f.SampleRate != 48000 {
		t.Errorf("SampleRate = %v, want 48000", f.SampleRate)
	}
	if f.Channels != 2 {
		t.Errorf("Channels = %v, want 2", f.Channels)
	}
	if f.Format != AudioFormatS16 {
		t.Errorf("Format = %v, want %v", f.Format, AudioFormatS16)
	}
	if f.NumSamples != 960 {
		t.Errorf("NumSamples = %v, want 960", f.NumSamples)
	}

	// S16: 2 bytes per sample, 2 channels
	expectedSize := 960 * 2 * 2
	if len(f.Samples) != expectedSize {
		t.Errorf("Samples size = %v, want %v", len(f.Samples), expectedSize)
	}
}

func TestAudioFrameF32(t *testing.T) {
	f := NewAudioFrameF32(48000, 2, 960)

	if f.Format != AudioFormatF32 {
		t.Errorf("Format = %v, want %v", f.Format, AudioFormatF32)
	}

	// F32: 4 bytes per sample, 2 channels
	expectedSize := 960 * 2 * 4
	if len(f.Samples) != expectedSize {
		t.Errorf("Samples size = %v, want %v", len(f.Samples), expectedSize)
	}
}

func TestAudioFrameDuration(t *testing.T) {
	f := NewAudioFrameS16(48000, 2, 960) // 20ms at 48kHz

	duration := f.Duration()
	expected := 20 * time.Millisecond

	if duration != expected {
		t.Errorf("Duration = %v, want %v", duration, expected)
	}
}

func TestAudioFramePool(t *testing.T) {
	pool := NewAudioFramePool(48000, 2, 960, AudioFormatS16, 4)

	// Get frames
	frames := make([]*AudioFrame, 4)
	for i := 0; i < 4; i++ {
		frames[i] = pool.Get()
		if frames[i] == nil {
			t.Fatalf("Got nil frame at index %d", i)
		}
	}

	// Return and reuse
	for _, f := range frames {
		f.Release()
	}

	reused := pool.Get()
	if reused == nil {
		t.Error("Should get reused frame")
	}
	if reused.PTS != 0 {
		t.Error("PTS should be reset")
	}
}

func TestPixelFormatString(t *testing.T) {
	tests := []struct {
		format PixelFormat
		str    string
	}{
		{PixelFormatI420, "I420"},
		{PixelFormatNV12, "NV12"},
		{PixelFormatRGBA, "RGBA"},
		{PixelFormatBGRA, "BGRA"},
	}

	for _, tt := range tests {
		if got := tt.format.String(); got != tt.str {
			t.Errorf("%v.String() = %v, want %v", tt.format, got, tt.str)
		}
	}
}

func TestAudioFormatString(t *testing.T) {
	if AudioFormatS16.String() != "S16" {
		t.Error("AudioFormatS16 should be 'S16'")
	}
	if AudioFormatF32.String() != "F32" {
		t.Error("AudioFormatF32 should be 'F32'")
	}
}

func BenchmarkNewI420Frame(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewI420Frame(1920, 1080)
	}
}

func BenchmarkVideoFramePool(b *testing.B) {
	pool := NewVideoFramePool(1920, 1080, PixelFormatI420, 8)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		f := pool.Get()
		f.Release()
	}
}

func BenchmarkVideoFrameClone(b *testing.B) {
	f := NewI420Frame(1920, 1080)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = f.Clone()
	}
}

func BenchmarkAudioFramePool(b *testing.B) {
	pool := NewAudioFramePool(48000, 2, 960, AudioFormatS16, 8)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		f := pool.Get()
		f.Release()
	}
}
