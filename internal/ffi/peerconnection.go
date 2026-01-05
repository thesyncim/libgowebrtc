package ffi

import (
	"unsafe"
)

// PeerConnection FFI function pointers are defined in lib.go

// PeerConnectionConfig matches ShimPeerConnectionConfig in shim.h
type PeerConnectionConfig struct {
	ICEServers           uintptr // Pointer to array of ICEServerConfig
	ICEServerCount       int32
	ICECandidatePoolSize int32
	BundlePolicy         *byte // C string
	RTCPMuxPolicy        *byte // C string
	SDPSemantics         *byte // C string
}

// ICEServerConfig matches ShimICEServer in shim.h
type ICEServerConfig struct {
	URLs       uintptr // Pointer to array of C strings
	URLCount   int32
	Username   *byte // C string
	Credential *byte // C string
}

// Ptr returns a pointer to the config as uintptr for FFI calls.
func (c *PeerConnectionConfig) Ptr() uintptr {
	return uintptr(unsafe.Pointer(c))
}

// Ptr returns a pointer to the config as uintptr for FFI calls.
func (c *ICEServerConfig) Ptr() uintptr {
	return uintptr(unsafe.Pointer(c))
}

// CreatePeerConnection creates a new PeerConnection.
func CreatePeerConnection(config *PeerConnectionConfig) uintptr {
	if !libLoaded || shimPeerConnectionCreate == nil {
		return 0
	}
	return shimPeerConnectionCreate(config.Ptr())
}

// PeerConnectionDestroy destroys a PeerConnection.
func PeerConnectionDestroy(pc uintptr) {
	if !libLoaded || shimPeerConnectionDestroy == nil {
		return
	}
	shimPeerConnectionDestroy(pc)
}

// PeerConnectionCreateOffer creates an SDP offer.
// Returns the SDP string written to the provided buffer.
func PeerConnectionCreateOffer(pc uintptr, sdpBuf []byte) (int, error) {
	if !libLoaded || shimPeerConnectionCreateOffer == nil {
		return 0, ErrLibraryNotLoaded
	}

	var sdpLen int
	result := shimPeerConnectionCreateOffer(
		pc,
		ByteSlicePtr(sdpBuf),
		len(sdpBuf),
		IntPtr(&sdpLen),
	)

	if err := ShimError(result); err != nil {
		return 0, err
	}

	return sdpLen, nil
}

// PeerConnectionCreateAnswer creates an SDP answer.
func PeerConnectionCreateAnswer(pc uintptr, sdpBuf []byte) (int, error) {
	if !libLoaded || shimPeerConnectionCreateAnswer == nil {
		return 0, ErrLibraryNotLoaded
	}

	var sdpLen int
	result := shimPeerConnectionCreateAnswer(
		pc,
		ByteSlicePtr(sdpBuf),
		len(sdpBuf),
		IntPtr(&sdpLen),
	)

	if err := ShimError(result); err != nil {
		return 0, err
	}

	return sdpLen, nil
}

// PeerConnectionSetLocalDescription sets the local SDP description.
func PeerConnectionSetLocalDescription(pc uintptr, sdpType int, sdp string) error {
	if !libLoaded || shimPeerConnectionSetLocalDescription == nil {
		return ErrLibraryNotLoaded
	}

	sdpCStr := CString(sdp)
	result := shimPeerConnectionSetLocalDescription(pc, sdpType, ByteSlicePtr(sdpCStr))
	return ShimError(result)
}

// PeerConnectionSetRemoteDescription sets the remote SDP description.
func PeerConnectionSetRemoteDescription(pc uintptr, sdpType int, sdp string) error {
	if !libLoaded || shimPeerConnectionSetRemoteDescription == nil {
		return ErrLibraryNotLoaded
	}

	sdpCStr := CString(sdp)
	result := shimPeerConnectionSetRemoteDescription(pc, sdpType, ByteSlicePtr(sdpCStr))
	return ShimError(result)
}

// PeerConnectionAddICECandidate adds an ICE candidate.
func PeerConnectionAddICECandidate(pc uintptr, candidate, sdpMid string, sdpMLineIndex int) error {
	if !libLoaded || shimPeerConnectionAddICECandidate == nil {
		return ErrLibraryNotLoaded
	}

	candidateCStr := CString(candidate)
	sdpMidCStr := CString(sdpMid)
	result := shimPeerConnectionAddICECandidate(
		pc,
		ByteSlicePtr(candidateCStr),
		ByteSlicePtr(sdpMidCStr),
		sdpMLineIndex,
	)
	return ShimError(result)
}

// PeerConnectionSignalingState returns the signaling state.
func PeerConnectionSignalingState(pc uintptr) int {
	if !libLoaded || shimPeerConnectionSignalingState == nil {
		return -1
	}
	return shimPeerConnectionSignalingState(pc)
}

// PeerConnectionICEConnectionState returns the ICE connection state.
func PeerConnectionICEConnectionState(pc uintptr) int {
	if !libLoaded || shimPeerConnectionICEConnectionState == nil {
		return -1
	}
	return shimPeerConnectionICEConnectionState(pc)
}

// PeerConnectionICEGatheringState returns the ICE gathering state.
func PeerConnectionICEGatheringState(pc uintptr) int {
	if !libLoaded || shimPeerConnectionICEGatheringState == nil {
		return -1
	}
	return shimPeerConnectionICEGatheringState(pc)
}

// PeerConnectionConnectionState returns the connection state.
func PeerConnectionConnectionState(pc uintptr) int {
	if !libLoaded || shimPeerConnectionConnectionState == nil {
		return -1
	}
	return shimPeerConnectionConnectionState(pc)
}

// PeerConnectionAddTrack adds a track to the peer connection.
func PeerConnectionAddTrack(pc uintptr, codec CodecType, trackID, streamID string) uintptr {
	if !libLoaded || shimPeerConnectionAddTrack == nil {
		return 0
	}

	trackIDCStr := CString(trackID)
	streamIDCStr := CString(streamID)
	return shimPeerConnectionAddTrack(
		pc,
		int(codec),
		ByteSlicePtr(trackIDCStr),
		ByteSlicePtr(streamIDCStr),
	)
}

// PeerConnectionRemoveTrack removes a track from the peer connection.
func PeerConnectionRemoveTrack(pc uintptr, sender uintptr) error {
	if !libLoaded || shimPeerConnectionRemoveTrack == nil {
		return ErrLibraryNotLoaded
	}
	result := shimPeerConnectionRemoveTrack(pc, sender)
	return ShimError(result)
}

// PeerConnectionCreateDataChannel creates a data channel.
func PeerConnectionCreateDataChannel(pc uintptr, label string, ordered bool, maxRetransmits int, protocol string) uintptr {
	if !libLoaded || shimPeerConnectionCreateDataChannel == nil {
		return 0
	}

	labelCStr := CString(label)
	protocolCStr := CString(protocol)

	orderedInt := 0
	if ordered {
		orderedInt = 1
	}

	return shimPeerConnectionCreateDataChannel(
		pc,
		ByteSlicePtr(labelCStr),
		orderedInt,
		maxRetransmits,
		ByteSlicePtr(protocolCStr),
	)
}

// PeerConnectionClose closes the peer connection.
func PeerConnectionClose(pc uintptr) {
	if !libLoaded || shimPeerConnectionClose == nil {
		return
	}
	shimPeerConnectionClose(pc)
}

// RTPSenderSetBitrate sets the bitrate for an RTP sender.
func RTPSenderSetBitrate(sender uintptr, bitrate uint32) error {
	if !libLoaded || shimRTPSenderSetBitrate == nil {
		return ErrLibraryNotLoaded
	}
	result := shimRTPSenderSetBitrate(sender, bitrate)
	return ShimError(result)
}

// RTPSenderDestroy destroys an RTP sender.
func RTPSenderDestroy(sender uintptr) {
	if !libLoaded || shimRTPSenderDestroy == nil {
		return
	}
	shimRTPSenderDestroy(sender)
}

// DataChannelSend sends data on a data channel.
func DataChannelSend(dc uintptr, data []byte, isBinary bool) error {
	if !libLoaded || shimDataChannelSend == nil {
		return ErrLibraryNotLoaded
	}

	isBinaryInt := 0
	if isBinary {
		isBinaryInt = 1
	}

	result := shimDataChannelSend(dc, ByteSlicePtr(data), len(data), isBinaryInt)
	return ShimError(result)
}

// DataChannelReadyState returns the ready state of a data channel.
func DataChannelReadyState(dc uintptr) int {
	if !libLoaded || shimDataChannelReadyState == nil {
		return -1
	}
	return shimDataChannelReadyState(dc)
}

// DataChannelClose closes a data channel.
func DataChannelClose(dc uintptr) {
	if !libLoaded || shimDataChannelClose == nil {
		return
	}
	shimDataChannelClose(dc)
}

// DataChannelDestroy destroys a data channel.
func DataChannelDestroy(dc uintptr) {
	if !libLoaded || shimDataChannelDestroy == nil {
		return
	}
	shimDataChannelDestroy(dc)
}
