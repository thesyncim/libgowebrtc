package encoder

import (
	"errors"
	"fmt"

	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

var (
	// ErrUnsupportedPixelFormat reports video input that does not use the
	// currently supported encoder pixel layout.
	ErrUnsupportedPixelFormat = errors.New("unsupported pixel format")
	// ErrInvalidFrameLayout reports malformed plane/stride layout in a raw
	// video frame.
	ErrInvalidFrameLayout = errors.New("invalid video frame layout")
)

// ValidateI420Frame ensures a raw video frame is safe to pass into the current
// libwebrtc-backed encoders, which accept I420 planes.
func ValidateI420Frame(src *frame.VideoFrame) error {
	if src == nil {
		return ErrInvalidFrame
	}
	if src.Format != frame.PixelFormatI420 {
		return fmt.Errorf("%w: got %s, want I420", ErrUnsupportedPixelFormat, src.Format)
	}
	if src.Width <= 0 || src.Height <= 0 {
		return fmt.Errorf("%w: invalid dimensions %dx%d", ErrInvalidFrameLayout, src.Width, src.Height)
	}
	if len(src.Data) < 3 || len(src.Stride) < 3 {
		return fmt.Errorf("%w: expected 3 planes and 3 strides, got %d planes and %d strides", ErrInvalidFrameLayout, len(src.Data), len(src.Stride))
	}

	if err := validatePlane(src.Data[0], src.Stride[0], src.Width, src.Height, "Y"); err != nil {
		return err
	}

	chromaWidth := (src.Width + 1) / 2
	chromaHeight := (src.Height + 1) / 2
	if err := validatePlane(src.Data[1], src.Stride[1], chromaWidth, chromaHeight, "U"); err != nil {
		return err
	}
	return validatePlane(src.Data[2], src.Stride[2], chromaWidth, chromaHeight, "V")
}

func validatePlane(data []byte, stride, width, height int, name string) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("%w: %s plane has invalid dimensions %dx%d", ErrInvalidFrameLayout, name, width, height)
	}
	if stride < width {
		return fmt.Errorf("%w: %s plane stride %d is smaller than width %d", ErrInvalidFrameLayout, name, stride, width)
	}

	required := planeBytes(stride, width, height)
	if len(data) < required {
		return fmt.Errorf("%w: %s plane has %d bytes, need at least %d", ErrInvalidFrameLayout, name, len(data), required)
	}

	return nil
}

func planeBytes(stride, width, height int) int {
	if width <= 0 || height <= 0 {
		return 0
	}
	return (height-1)*stride + width
}
