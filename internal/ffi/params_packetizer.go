package ffi

// shimPacketizerPacketizeParams matches ShimPacketizerPacketizeParams in shim.h.
type shimPacketizerPacketizeParams struct {
	Packetizer uintptr
	Data       uintptr
	Size       int32
	Timestamp  uint32
	IsKeyframe int32
	DstBuffer  uintptr
	DstOffsets uintptr
	DstSizes   uintptr
	MaxPackets int32
	OutCount   int32
}

// shimDepacketizerPushParams matches ShimDepacketizerPushParams in shim.h.
type shimDepacketizerPushParams struct {
	Depacketizer uintptr
	Data         uintptr
	Size         int32
}

// shimDepacketizerPopParams matches ShimDepacketizerPopParams in shim.h.
type shimDepacketizerPopParams struct {
	Depacketizer  uintptr
	DstBuffer     uintptr
	OutSize       int32
	OutTimestamp  uint32
	OutIsKeyframe int32
}
