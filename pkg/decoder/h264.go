package decoder

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

type h264Decoder struct {
	handle uintptr
	closed atomic.Bool
	mu     sync.Mutex

	// H.264 parameter set cache for decoding frames that don't include SPS/PPS.
	// OpenH264 encoder may not include these with every keyframe.
	lastSPS []byte
	lastPPS []byte
}

func NewH264Decoder() (VideoDecoder, error) {
	if err := ffi.LoadLibrary(); err != nil {
		return nil, err
	}

	dec := &h264Decoder{}
	if err := dec.init(); err != nil {
		return nil, err
	}

	return dec, nil
}

func (d *h264Decoder) init() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	handle, err := ffi.CreateVideoDecoder(ffi.CodecH264)
	if err != nil {
		return err
	}

	d.handle = handle
	return nil
}

func (d *h264Decoder) DecodeInto(src []byte, dst *frame.VideoFrame, timestamp uint32, isKeyframe bool) error {
	if d.closed.Load() {
		return ErrDecoderClosed
	}
	if len(src) == 0 {
		return ErrInvalidData
	}
	if dst == nil || len(dst.Data) < 3 || len(dst.Stride) < 3 {
		return ErrBufferTooSmall
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.handle == 0 {
		return ErrDecoderClosed
	}

	// Convert AVCC to Annex B if needed (VideoToolbox on macOS outputs AVCC format)
	data := d.ensureAnnexB(src)

	// Scan for and cache SPS/PPS NAL units
	hasSPS, hasPPS := d.scanForParameterSets(data)
	if hasSPS {
		d.lastSPS = d.extractNALUnit(data, 7)
	}
	if hasPPS {
		d.lastPPS = d.extractNALUnit(data, 8)
	}

	// If data doesn't have SPS/PPS but we have cached ones, prepend them
	dataToUse := data
	if !hasSPS && !hasPPS && len(d.lastSPS) > 0 && len(d.lastPPS) > 0 {
		dataToUse = make([]byte, 0, len(d.lastSPS)+len(d.lastPPS)+len(data))
		dataToUse = append(dataToUse, d.lastSPS...)
		dataToUse = append(dataToUse, d.lastPPS...)
		dataToUse = append(dataToUse, data...)
	}

	width, height, yStride, uStride, vStride, err := ffi.VideoDecoderDecodeInto(
		d.handle, dataToUse, timestamp, isKeyframe,
		dst.Data[0], dst.Data[1], dst.Data[2],
	)
	if err != nil {
		if errors.Is(err, ffi.ErrNeedMoreData) {
			return ErrNeedMoreData
		}
		return err
	}

	dst.Width = width
	dst.Height = height
	dst.Stride[0] = yStride
	dst.Stride[1] = uStride
	dst.Stride[2] = vStride
	dst.PTS = timestamp
	dst.Format = frame.PixelFormatI420

	return nil
}

// isAnnexB checks if the data starts with an Annex B start code.
func (d *h264Decoder) isAnnexB(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	// Check for 4-byte start code
	if data[0] == 0 && data[1] == 0 && data[2] == 0 && data[3] == 1 {
		return true
	}
	// Check for 3-byte start code
	if data[0] == 0 && data[1] == 0 && data[2] == 1 {
		return true
	}
	return false
}

// ensureAnnexB converts AVCC format to Annex B if needed.
// AVCC uses 4-byte length prefixes, Annex B uses start codes (00 00 00 01).
func (d *h264Decoder) ensureAnnexB(data []byte) []byte {
	if len(data) < 5 {
		return data
	}

	// If already Annex B, return as-is
	if d.isAnnexB(data) {
		return data
	}

	// Convert AVCC to Annex B
	// AVCC format: [4-byte length][NAL unit][4-byte length][NAL unit]...
	result := make([]byte, 0, len(data)+16) // Extra space for start codes
	startCode := []byte{0, 0, 0, 1}

	pos := 0
	for pos+4 <= len(data) {
		// Read 4-byte big-endian length
		nalLen := int(data[pos])<<24 | int(data[pos+1])<<16 | int(data[pos+2])<<8 | int(data[pos+3])
		pos += 4

		// Validate NAL length
		if nalLen <= 0 || pos+nalLen > len(data) {
			// Invalid length, might not be AVCC format after all
			// Return original data
			return data
		}

		// Add start code and NAL unit
		result = append(result, startCode...)
		result = append(result, data[pos:pos+nalLen]...)
		pos += nalLen
	}

	// If we didn't consume all data, it might not be valid AVCC
	if pos != len(data) && len(result) == 0 {
		return data
	}

	return result
}

// scanForParameterSets checks if the data contains SPS (type 7) and PPS (type 8) NAL units.
func (d *h264Decoder) scanForParameterSets(data []byte) (hasSPS, hasPPS bool) {
	for i := 0; i < len(data)-4; i++ {
		// Look for 4-byte start code (00 00 00 01)
		if data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
			if i+4 < len(data) {
				nalType := data[i+4] & 0x1F
				if nalType == 7 {
					hasSPS = true
				}
				if nalType == 8 {
					hasPPS = true
				}
			}
		}
		// Also check 3-byte start code (00 00 01)
		if data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
			if i+3 < len(data) {
				nalType := data[i+3] & 0x1F
				if nalType == 7 {
					hasSPS = true
				}
				if nalType == 8 {
					hasPPS = true
				}
			}
		}
	}
	return
}

// extractNALUnit finds and extracts a NAL unit of the given type (with start code).
func (d *h264Decoder) extractNALUnit(data []byte, targetType byte) []byte {
	for i := 0; i < len(data)-4; i++ {
		// Look for 4-byte start code
		if data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
			if i+4 < len(data) && (data[i+4]&0x1F) == targetType {
				start := i
				end := i + 5
				// Find next start code
				for end < len(data)-3 {
					if data[end] == 0 && data[end+1] == 0 &&
						(data[end+2] == 1 || (data[end+2] == 0 && end+3 < len(data) && data[end+3] == 1)) {
						break
					}
					end++
				}
				return append([]byte{}, data[start:end]...)
			}
		}
		// Also check 3-byte start code
		if data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
			if i+3 < len(data) && (data[i+3]&0x1F) == targetType {
				start := i
				end := i + 4
				// Find next start code
				for end < len(data)-2 {
					if data[end] == 0 && data[end+1] == 0 &&
						(data[end+2] == 1 || (data[end+2] == 0 && end+3 < len(data) && data[end+3] == 1)) {
						break
					}
					end++
				}
				return append([]byte{}, data[start:end]...)
			}
		}
	}
	return nil
}

func (d *h264Decoder) Codec() codec.Type {
	return codec.H264
}

func (d *h264Decoder) Close() error {
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle != 0 {
		ffi.VideoDecoderDestroy(d.handle)
		d.handle = 0
	}
	return nil
}
