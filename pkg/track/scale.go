package track

import (
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

// ScaleI420Frame scales an I420 video frame by the given factor using box filter (area averaging).
// A scale factor of 2.0 means the output is half the size of the input.
// The dst frame must already be allocated with the correct dimensions.
func ScaleI420Frame(src, dst *frame.VideoFrame, scaleFactor float64) {
	if scaleFactor <= 1.0 {
		// No scaling needed, just copy
		copy(dst.Data[0], src.Data[0])
		copy(dst.Data[1], src.Data[1])
		copy(dst.Data[2], src.Data[2])
		return
	}

	srcW, srcH := src.Width, src.Height
	dstW, dstH := dst.Width, dst.Height

	// Scale Y plane
	scalePlane(src.Data[0], dst.Data[0], srcW, srcH, dstW, dstH, src.Stride[0], dst.Stride[0])

	// Scale U and V planes (chroma is quarter size for I420)
	srcUW, srcUH := srcW/2, srcH/2
	dstUW, dstUH := dstW/2, dstH/2
	scalePlane(src.Data[1], dst.Data[1], srcUW, srcUH, dstUW, dstUH, src.Stride[1], dst.Stride[1])
	scalePlane(src.Data[2], dst.Data[2], srcUW, srcUH, dstUW, dstUH, src.Stride[2], dst.Stride[2])
}

// scalePlane scales a plane (Y, U, or V) using box filter downsampling.
func scalePlane(src, dst []byte, srcW, srcH, dstW, dstH, srcStride, dstStride int) {
	xRatio := float64(srcW) / float64(dstW)
	yRatio := float64(srcH) / float64(dstH)

	for dstY := 0; dstY < dstH; dstY++ {
		srcY0 := int(float64(dstY) * yRatio)
		srcY1 := int(float64(dstY+1) * yRatio)
		if srcY1 > srcH {
			srcY1 = srcH
		}

		dstRow := dstY * dstStride

		for dstX := 0; dstX < dstW; dstX++ {
			srcX0 := int(float64(dstX) * xRatio)
			srcX1 := int(float64(dstX+1) * xRatio)
			if srcX1 > srcW {
				srcX1 = srcW
			}

			// Box filter: average all pixels in the source region
			var sum int
			count := 0
			for sy := srcY0; sy < srcY1; sy++ {
				srcRow := sy * srcStride
				for sx := srcX0; sx < srcX1; sx++ {
					sum += int(src[srcRow+sx])
					count++
				}
			}

			if count > 0 {
				dst[dstRow+dstX] = byte(sum / count)
			}
		}
	}
}

// ScaleI420FrameFast scales an I420 frame using nearest neighbor (fast but lower quality).
// Use this when performance is critical.
func ScaleI420FrameFast(src, dst *frame.VideoFrame, scaleFactor float64) {
	if scaleFactor <= 1.0 {
		copy(dst.Data[0], src.Data[0])
		copy(dst.Data[1], src.Data[1])
		copy(dst.Data[2], src.Data[2])
		return
	}

	srcW, srcH := src.Width, src.Height
	dstW, dstH := dst.Width, dst.Height

	// Scale Y plane (nearest neighbor)
	xRatio := float64(srcW) / float64(dstW)
	yRatio := float64(srcH) / float64(dstH)

	for dstY := 0; dstY < dstH; dstY++ {
		srcY := int(float64(dstY) * yRatio)
		dstRow := dstY * dst.Stride[0]
		srcRow := srcY * src.Stride[0]

		for dstX := 0; dstX < dstW; dstX++ {
			srcX := int(float64(dstX) * xRatio)
			dst.Data[0][dstRow+dstX] = src.Data[0][srcRow+srcX]
		}
	}

	// Scale U and V planes
	srcUW, srcUH := srcW/2, srcH/2
	dstUW, dstUH := dstW/2, dstH/2
	xRatioUV := float64(srcUW) / float64(dstUW)
	yRatioUV := float64(srcUH) / float64(dstUH)

	for dstY := 0; dstY < dstUH; dstY++ {
		srcY := int(float64(dstY) * yRatioUV)
		dstRowU := dstY * dst.Stride[1]
		dstRowV := dstY * dst.Stride[2]
		srcRowU := srcY * src.Stride[1]
		srcRowV := srcY * src.Stride[2]

		for dstX := 0; dstX < dstUW; dstX++ {
			srcX := int(float64(dstX) * xRatioUV)
			dst.Data[1][dstRowU+dstX] = src.Data[1][srcRowU+srcX]
			dst.Data[2][dstRowV+dstX] = src.Data[2][srcRowV+srcX]
		}
	}
}
