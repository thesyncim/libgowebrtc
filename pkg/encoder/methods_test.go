package encoder

import (
	"testing"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

func TestEncoderInvalidConfigs(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "H264",
			run: func() error {
				_, err := NewH264Encoder(codec.H264Config{})
				return err
			},
		},
		{
			name: "VP8",
			run: func() error {
				_, err := NewVP8Encoder(codec.VP8Config{})
				return err
			},
		},
		{
			name: "VP9",
			run: func() error {
				_, err := NewVP9Encoder(codec.VP9Config{})
				return err
			},
		},
		{
			name: "AV1",
			run: func() error {
				_, err := NewAV1Encoder(codec.AV1Config{})
				return err
			},
		},
		{
			name: "Opus",
			run: func() error {
				_, err := NewOpusEncoder(codec.OpusConfig{})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err != ErrInvalidConfig {
				t.Fatalf("%s invalid config error = %v, want %v", test.name, err, ErrInvalidConfig)
			}
		})
	}
}

func TestEncoderCodecMethods(t *testing.T) {
	if got := (&h264Encoder{}).Codec(); got != codec.H264 {
		t.Fatalf("h264 Codec() = %v, want %v", got, codec.H264)
	}
	if got := (&vp8Encoder{}).Codec(); got != codec.VP8 {
		t.Fatalf("vp8 Codec() = %v, want %v", got, codec.VP8)
	}
	if got := (&vp9Encoder{}).Codec(); got != codec.VP9 {
		t.Fatalf("vp9 Codec() = %v, want %v", got, codec.VP9)
	}
	if got := (&av1Encoder{}).Codec(); got != codec.AV1 {
		t.Fatalf("av1 Codec() = %v, want %v", got, codec.AV1)
	}
	if got := (&opusEncoder{}).Codec(); got != codec.Opus {
		t.Fatalf("opus Codec() = %v, want %v", got, codec.Opus)
	}
}

func TestVideoEncoderClosedHandleGuardPaths(t *testing.T) {
	if err := (&h264Encoder{}).SetFramerate(30); err != ErrEncoderClosed {
		t.Fatalf("h264 SetFramerate() error = %v, want %v", err, ErrEncoderClosed)
	}
	if err := (&av1Encoder{}).SetBitrate(1000); err != ErrEncoderClosed {
		t.Fatalf("av1 SetBitrate() error = %v, want %v", err, ErrEncoderClosed)
	}
	if err := (&av1Encoder{}).SetFramerate(30); err != ErrEncoderClosed {
		t.Fatalf("av1 SetFramerate() error = %v, want %v", err, ErrEncoderClosed)
	}
	if err := (&vp8Encoder{}).SetBitrate(1000); err != ErrEncoderClosed {
		t.Fatalf("vp8 SetBitrate() error = %v, want %v", err, ErrEncoderClosed)
	}
	if err := (&vp8Encoder{}).SetFramerate(30); err != ErrEncoderClosed {
		t.Fatalf("vp8 SetFramerate() error = %v, want %v", err, ErrEncoderClosed)
	}
	if err := (&vp9Encoder{}).SetBitrate(1000); err != ErrEncoderClosed {
		t.Fatalf("vp9 SetBitrate() error = %v, want %v", err, ErrEncoderClosed)
	}
	if err := (&vp9Encoder{}).SetFramerate(30); err != ErrEncoderClosed {
		t.Fatalf("vp9 SetFramerate() error = %v, want %v", err, ErrEncoderClosed)
	}
}

func TestAudioEncoderClosedHandleGuardPaths(t *testing.T) {
	if err := (&opusEncoder{}).SetBitrate(64000); err != ErrEncoderClosed {
		t.Fatalf("opus SetBitrate() error = %v, want %v", err, ErrEncoderClosed)
	}
}

func TestDependencyDescriptorAccessors(t *testing.T) {
	av1 := &av1Encoder{}
	if got := av1.LastDependencyDescriptor(); got != nil {
		t.Fatalf("av1 LastDependencyDescriptor() = %v, want nil", got)
	}
	av1.lastDD[0], av1.lastDD[1], av1.lastDD[2] = 1, 2, 3
	av1.lastDDLen = 3
	if got := av1.LastDependencyDescriptor(); len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("av1 LastDependencyDescriptor() = %v, want [1 2 3]", got)
	}

	vp9 := &vp9Encoder{}
	if got := vp9.LastDependencyDescriptor(); got != nil {
		t.Fatalf("vp9 LastDependencyDescriptor() = %v, want nil", got)
	}
	vp9.lastDD[0], vp9.lastDD[1] = 9, 8
	vp9.lastDDLen = 2
	if got := vp9.LastDependencyDescriptor(); len(got) != 2 || got[0] != 9 || got[1] != 8 {
		t.Fatalf("vp9 LastDependencyDescriptor() = %v, want [9 8]", got)
	}
}
