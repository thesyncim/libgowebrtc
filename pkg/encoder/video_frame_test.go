package encoder

import (
	"errors"
	"testing"

	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

func TestValidateI420Frame(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if err := ValidateI420Frame(nil); !errors.Is(err, ErrInvalidFrame) {
			t.Fatalf("ValidateI420Frame(nil) error = %v, want %v", err, ErrInvalidFrame)
		}
	})

	t.Run("unsupported format", func(t *testing.T) {
		src := frame.NewNV12Frame(320, 240)
		if err := ValidateI420Frame(src); !errors.Is(err, ErrUnsupportedPixelFormat) {
			t.Fatalf("ValidateI420Frame(NV12) error = %v, want %v", err, ErrUnsupportedPixelFormat)
		}
	})

	t.Run("missing planes", func(t *testing.T) {
		src := frame.NewI420Frame(320, 240)
		src.Data = src.Data[:2]
		if err := ValidateI420Frame(src); !errors.Is(err, ErrInvalidFrameLayout) {
			t.Fatalf("ValidateI420Frame(missing planes) error = %v, want %v", err, ErrInvalidFrameLayout)
		}
	})

	t.Run("short plane", func(t *testing.T) {
		src := frame.NewI420Frame(320, 240)
		src.Data[0] = src.Data[0][:100]
		if err := ValidateI420Frame(src); !errors.Is(err, ErrInvalidFrameLayout) {
			t.Fatalf("ValidateI420Frame(short plane) error = %v, want %v", err, ErrInvalidFrameLayout)
		}
	})

	t.Run("short stride", func(t *testing.T) {
		src := frame.NewI420Frame(320, 240)
		src.Stride[0] = 100
		if err := ValidateI420Frame(src); !errors.Is(err, ErrInvalidFrameLayout) {
			t.Fatalf("ValidateI420Frame(short stride) error = %v, want %v", err, ErrInvalidFrameLayout)
		}
	})

	t.Run("odd dimensions", func(t *testing.T) {
		src := frame.NewI420Frame(321, 241)
		if err := ValidateI420Frame(src); err != nil {
			t.Fatalf("ValidateI420Frame(odd dimensions) error = %v, want nil", err)
		}
	})
}

func TestPlaneBytes(t *testing.T) {
	if got := planeBytes(320, 320, 240); got != 320*240 {
		t.Fatalf("planeBytes(320,320,240) = %d, want %d", got, 320*240)
	}
	if got := planeBytes(161, 160, 2); got != 321 {
		t.Fatalf("planeBytes(161,160,2) = %d, want 321", got)
	}
	if got := planeBytes(10, 0, 2); got != 0 {
		t.Fatalf("planeBytes(width=0) = %d, want 0", got)
	}
}
