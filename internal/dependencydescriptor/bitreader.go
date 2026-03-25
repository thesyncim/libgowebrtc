package dependencydescriptor

import (
	"errors"
	"io"
)

type bitReader struct {
	buf           []byte
	pos           int
	remainingBits int
}

func newBitReader(buf []byte) *bitReader {
	return &bitReader{
		buf:           buf,
		remainingBits: len(buf) * 8,
	}
}

func (r *bitReader) ReadBits(bits int) (uint64, error) {
	if bits < 0 || bits > 64 {
		return 0, errors.New("dependency descriptor: invalid bit count")
	}
	if r.remainingBits < bits {
		r.remainingBits -= bits
		return 0, io.EOF
	}

	remainingBitsInFirstByte := r.remainingBits % 8
	r.remainingBits -= bits
	if bits < remainingBitsInFirstByte {
		offset := remainingBitsInFirstByte - bits
		return uint64((r.buf[r.pos] >> offset) & ((1 << bits) - 1)), nil
	}

	var out uint64
	if remainingBitsInFirstByte > 0 {
		bits -= remainingBitsInFirstByte
		mask := byte((1 << remainingBitsInFirstByte) - 1)
		out = uint64(r.buf[r.pos]&mask) << bits
		r.pos++
	}

	for bits >= 8 {
		bits -= 8
		out |= uint64(r.buf[r.pos]) << bits
		r.pos++
	}
	if bits > 0 {
		out |= uint64(r.buf[r.pos] >> (8 - bits))
	}

	return out, nil
}

func (r *bitReader) ReadBool() (bool, error) {
	value, err := r.ReadBits(1)
	return value != 0, err
}

func (r *bitReader) ReadNonSymmetric(numValues uint32) (uint32, error) {
	if numValues >= (uint32(1) << 31) {
		return 0, errors.New("dependency descriptor: invalid non-symmetric range")
	}

	width := bitWidth(numValues)
	numMinBitsValues := (uint32(1) << width) - numValues

	value, err := r.ReadBits(width - 1)
	if err != nil {
		return 0, err
	}
	if value < uint64(numMinBitsValues) {
		return uint32(value), nil
	}

	extra, err := r.ReadBits(1)
	if err != nil {
		return 0, err
	}
	return uint32((value << 1) + extra - uint64(numMinBitsValues)), nil
}

func bitWidth(value uint32) int {
	width := 0
	for value != 0 {
		value >>= 1
		width++
	}
	return width
}
