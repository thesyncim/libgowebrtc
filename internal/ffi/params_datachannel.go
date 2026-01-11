package ffi

// shimDataChannelSetOnMessageParams matches ShimDataChannelSetOnMessageParams in shim.h.
type shimDataChannelSetOnMessageParams struct {
	DC       uintptr
	Callback uintptr
	Ctx      uintptr
}

// shimDataChannelSetOnOpenParams matches ShimDataChannelSetOnOpenParams in shim.h.
type shimDataChannelSetOnOpenParams struct {
	DC       uintptr
	Callback uintptr
	Ctx      uintptr
}

// shimDataChannelSetOnCloseParams matches ShimDataChannelSetOnCloseParams in shim.h.
type shimDataChannelSetOnCloseParams struct {
	DC       uintptr
	Callback uintptr
	Ctx      uintptr
}

// shimDataChannelSendParams matches ShimDataChannelSendParams in shim.h.
type shimDataChannelSendParams struct {
	DC       uintptr
	Data     uintptr
	Size     int32
	IsBinary int32
	ErrorOut uintptr
}
