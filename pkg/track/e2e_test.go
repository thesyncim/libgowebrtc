package track

import (
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

func TestMain(m *testing.M) {
	if err := ffi.LoadLibrary(); err != nil {
		// Skip if library not available
		return
	}
	defer ffi.Close()
	m.Run()
}

func TestVideoTrackFullPipeline(t *testing.T) {
	vt, err := NewVideoTrack(VideoTrackConfig{
		ID:      "test-video",
		Codec:   codec.H264,
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("Failed to create video track: %v", err)
	}
	defer vt.Close()

	// Create test frame
	f := frame.NewI420Frame(640, 480)

	// Fill with gray using Data slices
	for i := range f.Data[0] {
		f.Data[0][i] = 128
	}
	for i := range f.Data[1] {
		f.Data[1][i] = 128
		f.Data[2][i] = 128
	}

	// Write multiple frames - expect ErrNotBound when track is not added to PeerConnection
	for i := 0; i < 10; i++ {
		f.PTS = uint32(i * 33) // 30fps
		forceKeyframe := i == 0

		err := vt.WriteFrame(f, forceKeyframe)
		if err != nil && err != ErrNotBound {
			t.Fatalf("Frame %d: unexpected error: %v", i, err)
		}
	}

	t.Log("Video track pipeline test passed (not bound to PeerConnection)")
}

func TestAudioTrackFullPipeline(t *testing.T) {
	at, err := NewAudioTrack(AudioTrackConfig{
		ID:         "test-audio",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		t.Fatalf("Failed to create audio track: %v", err)
	}
	defer at.Close()

	// Create test audio frame (20ms at 48kHz stereo)
	// Browser WebRTC uses 20ms Opus frames; shim handles chunking internally
	f := frame.NewAudioFrameS16(48000, 2, 960)

	// Fill with silence (already zeroed)

	// Write multiple frames - expect ErrNotBound when track is not added to PeerConnection
	for i := 0; i < 10; i++ {
		f.PTS = uint32(i * 20) // 20ms per frame

		err := at.WriteFrame(f)
		if err != nil && err != ErrNotBound {
			t.Fatalf("Frame %d: unexpected error: %v", i, err)
		}
	}

	t.Log("Audio track pipeline test passed (not bound to PeerConnection)")
}

func TestVideoTrackBitrateChange(t *testing.T) {
	vt, err := NewVideoTrack(VideoTrackConfig{
		ID:      "bitrate-test",
		Codec:   codec.VP8,
		Width:   1280,
		Height:  720,
		Bitrate: 2_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("Failed to create video track: %v", err)
	}
	defer vt.Close()

	// Test runtime bitrate changes
	bitrates := []uint32{500_000, 1_000_000, 4_000_000, 2_000_000}
	for _, bitrate := range bitrates {
		if err := vt.SetBitrate(bitrate); err != nil {
			t.Errorf("SetBitrate(%d) failed: %v", bitrate, err)
		}
		t.Logf("Set video track bitrate to %d bps", bitrate)
	}

	// Test framerate change
	if err := vt.SetFramerate(60); err != nil {
		t.Errorf("SetFramerate(60) failed: %v", err)
	}
	t.Log("Set video track framerate to 60 fps")

	t.Log("Bitrate change test passed")
}

func TestVideoTrackKeyframeRequest(t *testing.T) {
	vt, err := NewVideoTrack(VideoTrackConfig{
		ID:      "keyframe-test",
		Codec:   codec.H264,
		Width:   640,
		Height:  480,
		Bitrate: 1_000_000,
		FPS:     30,
	})
	if err != nil {
		t.Fatalf("Failed to create video track: %v", err)
	}
	defer vt.Close()

	// Request keyframe
	vt.RequestKeyFrame()
	t.Log("Requested keyframe")

	// Write a frame (should be keyframe)
	f := frame.NewI420Frame(640, 480)
	for i := range f.Data[0] {
		f.Data[0][i] = 128
	}
	for i := range f.Data[1] {
		f.Data[1][i] = 128
		f.Data[2][i] = 128
	}

	err = vt.WriteFrame(f, false)
	if err != nil && err != ErrNotBound {
		t.Fatalf("WriteFrame unexpected error: %v", err)
	}

	t.Log("Keyframe request test passed (not bound to PeerConnection)")
}

func TestMultipleCodecs(t *testing.T) {
	codecs := []struct {
		name  string
		codec codec.Type
	}{
		{"H264", codec.H264},
		{"VP8", codec.VP8},
		{"VP9", codec.VP9},
	}

	for _, tc := range codecs {
		t.Run(tc.name, func(t *testing.T) {
			vt, err := NewVideoTrack(VideoTrackConfig{
				ID:      "test-" + tc.name,
				Codec:   tc.codec,
				Width:   640,
				Height:  480,
				Bitrate: 1_000_000,
				FPS:     30,
			})
			if err != nil {
				t.Fatalf("Failed to create %s track: %v", tc.name, err)
			}
			defer vt.Close()

			f := frame.NewI420Frame(640, 480)
			for i := range f.Data[0] {
				f.Data[0][i] = 128
			}
			for i := range f.Data[1] {
				f.Data[1][i] = 128
				f.Data[2][i] = 128
			}

			err = vt.WriteFrame(f, true)
			if err != nil && err != ErrNotBound {
				t.Fatalf("%s WriteFrame unexpected error: %v", tc.name, err)
			}

			// Success - track created and encoder works
		})
	}
}

func BenchmarkVideoTrackWrite(b *testing.B) {
	vt, err := NewVideoTrack(VideoTrackConfig{
		ID:      "bench-video",
		Codec:   codec.H264,
		Width:   1280,
		Height:  720,
		Bitrate: 2_000_000,
		FPS:     30,
	})
	if err != nil {
		b.Fatalf("Failed to create video track: %v", err)
	}
	defer vt.Close()

	f := frame.NewI420Frame(1280, 720)
	for i := range f.Data[0] {
		f.Data[0][i] = 128
	}
	for i := range f.Data[1] {
		f.Data[1][i] = 128
		f.Data[2][i] = 128
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.PTS = uint32(i * 33)
		_ = vt.WriteFrame(f, i%30 == 0)
	}
}
