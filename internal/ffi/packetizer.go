package ffi

import (
	"runtime"
	"unsafe"
)

// CreatePacketizer creates an RTP packetizer.
func CreatePacketizer(config *PacketizerConfig) uintptr {
	if !libLoaded.Load() {
		return 0
	}
	return shimPacketizerCreate(config.Ptr())
}

// PacketizerPacketizeInto packetizes encoded data into RTP packets.
// Writes packets into dst buffer, returns packet count.
func PacketizerPacketizeInto(
	packetizer uintptr,
	data []byte,
	timestamp uint32,
	isKeyframe bool,
	dst []byte,
	offsets []int32,
	sizes []int32,
	maxPackets int,
) (int, error) {
	if !libLoaded.Load() {
		return 0, ErrLibraryNotLoaded
	}

	var keyframe int32
	if isKeyframe {
		keyframe = 1
	}

	params := shimPacketizerPacketizeParams{
		Packetizer: packetizer,
		Data:       ByteSlicePtr(data),
		Size:       int32(len(data)),
		Timestamp:  timestamp,
		IsKeyframe: keyframe,
		DstBuffer:  ByteSlicePtr(dst),
		DstOffsets: Int32SlicePtr(offsets),
		DstSizes:   Int32SlicePtr(sizes),
		MaxPackets: int32(maxPackets),
	}
	result := shimPacketizerPacketize(uintptr(unsafe.Pointer(&params)))
	runtime.KeepAlive(data)
	runtime.KeepAlive(dst)
	runtime.KeepAlive(offsets)
	runtime.KeepAlive(sizes)
	runtime.KeepAlive(&params)

	if err := ShimError(result); err != nil {
		return 0, err
	}

	return int(params.OutCount), nil
}

// PacketizerSequenceNumber returns the current sequence number.
func PacketizerSequenceNumber(packetizer uintptr) uint16 {
	if !libLoaded.Load() {
		return 0
	}
	return shimPacketizerSeqNum(packetizer)
}

// PacketizerDestroy destroys a packetizer.
func PacketizerDestroy(packetizer uintptr) {
	if !libLoaded.Load() {
		return
	}
	shimPacketizerDestroy(packetizer)
}

// CreateDepacketizer creates an RTP depacketizer.
func CreateDepacketizer(codec CodecType) uintptr {
	if !libLoaded.Load() {
		return 0
	}
	return shimDepacketizerCreate(int32(codec))
}

// DepacketizerPush pushes an RTP packet for reassembly.
func DepacketizerPush(depacketizer uintptr, packet []byte) error {
	if !libLoaded.Load() {
		return ErrLibraryNotLoaded
	}

	params := shimDepacketizerPushParams{
		Depacketizer: depacketizer,
		Data:         ByteSlicePtr(packet),
		Size:         int32(len(packet)),
	}
	result := shimDepacketizerPush(uintptr(unsafe.Pointer(&params)))
	runtime.KeepAlive(packet)
	runtime.KeepAlive(&params)
	return ShimError(result)
}

// DepacketizerPopInto pops a complete frame into a pre-allocated buffer.
func DepacketizerPopInto(depacketizer uintptr, dst []byte) (size int, timestamp uint32, isKeyframe bool, err error) {
	if !libLoaded.Load() {
		return 0, 0, false, ErrLibraryNotLoaded
	}

	params := shimDepacketizerPopParams{
		Depacketizer: depacketizer,
		DstBuffer:    ByteSlicePtr(dst),
	}
	result := shimDepacketizerPop(uintptr(unsafe.Pointer(&params)))
	runtime.KeepAlive(dst)
	runtime.KeepAlive(&params)

	if err := ShimError(result); err != nil {
		return 0, 0, false, err
	}

	return int(params.OutSize), params.OutTimestamp, params.OutIsKeyframe != 0, nil
}

// DepacketizerDestroy destroys a depacketizer.
func DepacketizerDestroy(depacketizer uintptr) {
	if !libLoaded.Load() {
		return
	}
	shimDepacketizerDestroy(depacketizer)
}
