package ffi

// CreatePacketizer creates an RTP packetizer.
func CreatePacketizer(config *PacketizerConfig) uintptr {
	if !libLoaded {
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
	if !libLoaded {
		return 0, ErrLibraryNotLoaded
	}

	var outCount int

	keyframe := 0
	if isKeyframe {
		keyframe = 1
	}

	result := shimPacketizerPacketize(
		packetizer,
		ByteSlicePtr(data),
		len(data),
		timestamp,
		keyframe,
		ByteSlicePtr(dst),
		Int32SlicePtr(offsets),
		Int32SlicePtr(sizes),
		maxPackets,
		IntPtr(&outCount),
	)

	if err := ShimError(result); err != nil {
		return 0, err
	}

	return outCount, nil
}

// PacketizerSequenceNumber returns the current sequence number.
func PacketizerSequenceNumber(packetizer uintptr) uint16 {
	if !libLoaded {
		return 0
	}
	return shimPacketizerSeqNum(packetizer)
}

// PacketizerDestroy destroys a packetizer.
func PacketizerDestroy(packetizer uintptr) {
	if !libLoaded {
		return
	}
	shimPacketizerDestroy(packetizer)
}

// CreateDepacketizer creates an RTP depacketizer.
func CreateDepacketizer(codec CodecType) uintptr {
	if !libLoaded {
		return 0
	}
	return shimDepacketizerCreate(int(codec))
}

// DepacketizerPush pushes an RTP packet for reassembly.
func DepacketizerPush(depacketizer uintptr, packet []byte) error {
	if !libLoaded {
		return ErrLibraryNotLoaded
	}

	result := shimDepacketizerPush(depacketizer, ByteSlicePtr(packet), len(packet))
	return ShimError(result)
}

// DepacketizerPopInto pops a complete frame into a pre-allocated buffer.
func DepacketizerPopInto(depacketizer uintptr, dst []byte) (size int, timestamp uint32, isKeyframe bool, err error) {
	if !libLoaded {
		return 0, 0, false, ErrLibraryNotLoaded
	}

	var outSize int
	var outTimestamp uint32
	var outIsKeyframe int32

	result := shimDepacketizerPop(
		depacketizer,
		ByteSlicePtr(dst),
		IntPtr(&outSize),
		Uint32Ptr(&outTimestamp),
		BoolPtr(&outIsKeyframe),
	)

	if err := ShimError(result); err != nil {
		return 0, 0, false, err
	}

	return outSize, outTimestamp, outIsKeyframe != 0, nil
}

// DepacketizerDestroy destroys a depacketizer.
func DepacketizerDestroy(depacketizer uintptr) {
	if !libLoaded {
		return
	}
	shimDepacketizerDestroy(depacketizer)
}
