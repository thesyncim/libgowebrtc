package ffi

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

	var outCount int32

	var keyframe int32
	if isKeyframe {
		keyframe = 1
	}

	result := shimPacketizerPacketize(
		packetizer,
		ByteSlicePtr(data),
		int32(len(data)),
		timestamp,
		keyframe,
		ByteSlicePtr(dst),
		Int32SlicePtr(offsets),
		Int32SlicePtr(sizes),
		int32(maxPackets),
		Int32Ptr(&outCount),
	)

	if err := ShimError(result); err != nil {
		return 0, err
	}

	return int(outCount), nil
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

	result := shimDepacketizerPush(depacketizer, ByteSlicePtr(packet), int32(len(packet)))
	return ShimError(result)
}

// DepacketizerPopInto pops a complete frame into a pre-allocated buffer.
func DepacketizerPopInto(depacketizer uintptr, dst []byte) (size int, timestamp uint32, isKeyframe bool, err error) {
	if !libLoaded.Load() {
		return 0, 0, false, ErrLibraryNotLoaded
	}

	var outSize int32
	var outTimestamp uint32
	var outIsKeyframe int32

	result := shimDepacketizerPop(
		depacketizer,
		ByteSlicePtr(dst),
		Int32Ptr(&outSize),
		Uint32Ptr(&outTimestamp),
		Int32Ptr(&outIsKeyframe),
	)

	if err := ShimError(result); err != nil {
		return 0, 0, false, err
	}

	return int(outSize), outTimestamp, outIsKeyframe != 0, nil
}

// DepacketizerDestroy destroys a depacketizer.
func DepacketizerDestroy(depacketizer uintptr) {
	if !libLoaded.Load() {
		return
	}
	shimDepacketizerDestroy(depacketizer)
}
