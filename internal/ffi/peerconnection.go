package ffi

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// VideoFrameCallback is called when a video frame is received from a remote track.
type VideoFrameCallback func(width, height int, yPlane, uPlane, vPlane []byte, yStride, uStride, vStride int, timestampUs int64)

// AudioFrameCallback is called when audio samples are received from a remote track.
type AudioFrameCallback func(samples []int16, sampleRate, channels int, timestampUs int64)

// Global callback registry for remote track sinks
var (
	videoCallbackMu sync.RWMutex
	videoCallbacks  = make(map[uintptr]VideoFrameCallback)

	audioCallbackMu sync.RWMutex
	audioCallbacks  = make(map[uintptr]AudioFrameCallback)

	// purego callback function pointers (must be kept alive)
	videoSinkCallbackPtr uintptr
	audioSinkCallbackPtr uintptr
	callbacksInitialized bool
	callbackInitMu       sync.Mutex
)

// initCallbacks initializes the purego callbacks for remote track sinks.
// This must be called before setting any track sinks.
func initCallbacks() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()

	if callbacksInitialized {
		return
	}

	// Create the video sink callback
	// Signature: void(ctx, width, height, y_plane, u_plane, v_plane, y_stride, u_stride, v_stride, timestamp_us)
	videoSinkCallbackPtr = purego.NewCallback(func(ctx uintptr, width, height int, yPlane, uPlane, vPlane uintptr, yStride, uStride, vStride int, timestampUs int64) {
		videoCallbackMu.RLock()
		cb, ok := videoCallbacks[ctx]
		videoCallbackMu.RUnlock()

		if !ok || cb == nil {
			return
		}

		// Calculate plane sizes
		ySize := yStride * height
		uvHeight := (height + 1) / 2
		uSize := uStride * uvHeight
		vSize := vStride * uvHeight

		// Copy data from C memory to Go slices (avoid holding pointers across calls)
		yData := make([]byte, ySize)
		uData := make([]byte, uSize)
		vData := make([]byte, vSize)

		if yPlane != 0 {
			copy(yData, unsafe.Slice((*byte)(unsafe.Pointer(yPlane)), ySize))
		}
		if uPlane != 0 {
			copy(uData, unsafe.Slice((*byte)(unsafe.Pointer(uPlane)), uSize))
		}
		if vPlane != 0 {
			copy(vData, unsafe.Slice((*byte)(unsafe.Pointer(vPlane)), vSize))
		}

		cb(width, height, yData, uData, vData, yStride, uStride, vStride, timestampUs)
	})

	// Create the audio sink callback
	// Signature: void(ctx, samples, num_samples, sample_rate, channels, timestamp_us)
	audioSinkCallbackPtr = purego.NewCallback(func(ctx uintptr, samples uintptr, numSamples, sampleRate, channels int, timestampUs int64) {
		audioCallbackMu.RLock()
		cb, ok := audioCallbacks[ctx]
		audioCallbackMu.RUnlock()

		if !ok || cb == nil {
			return
		}

		// Total samples = numSamples * channels (interleaved)
		totalSamples := numSamples * channels

		// Copy data from C memory to Go slice
		samplesData := make([]int16, totalSamples)
		if samples != 0 {
			copy(samplesData, unsafe.Slice((*int16)(unsafe.Pointer(samples)), totalSamples))
		}

		cb(samplesData, sampleRate, channels, timestampUs)
	})

	callbacksInitialized = true
}

// RegisterVideoCallback registers a video frame callback for a track.
// The trackID should be the track handle pointer value.
func RegisterVideoCallback(trackID uintptr, cb VideoFrameCallback) {
	initCallbacks()

	videoCallbackMu.Lock()
	videoCallbacks[trackID] = cb
	videoCallbackMu.Unlock()
}

// UnregisterVideoCallback removes a video frame callback for a track.
func UnregisterVideoCallback(trackID uintptr) {
	videoCallbackMu.Lock()
	delete(videoCallbacks, trackID)
	videoCallbackMu.Unlock()
}

// RegisterAudioCallback registers an audio frame callback for a track.
// The trackID should be the track handle pointer value.
func RegisterAudioCallback(trackID uintptr, cb AudioFrameCallback) {
	initCallbacks()

	audioCallbackMu.Lock()
	audioCallbacks[trackID] = cb
	audioCallbackMu.Unlock()
}

// UnregisterAudioCallback removes an audio frame callback for a track.
func UnregisterAudioCallback(trackID uintptr) {
	audioCallbackMu.Lock()
	delete(audioCallbacks, trackID)
	audioCallbackMu.Unlock()
}

// GetVideoSinkCallbackPtr returns the purego callback pointer for video sinks.
func GetVideoSinkCallbackPtr() uintptr {
	initCallbacks()
	return videoSinkCallbackPtr
}

// GetAudioSinkCallbackPtr returns the purego callback pointer for audio sinks.
func GetAudioSinkCallbackPtr() uintptr {
	initCallbacks()
	return audioSinkCallbackPtr
}

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

// RTPSenderReplaceTrack replaces the track on an RTP sender.
func RTPSenderReplaceTrack(sender uintptr, track uintptr) error {
	if !libLoaded || shimRTPSenderReplaceTrack == nil {
		return ErrLibraryNotLoaded
	}
	result := shimRTPSenderReplaceTrack(sender, track)
	return ShimError(result)
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

// VideoTrackSourceCreate creates a video track source for frame injection.
func VideoTrackSourceCreate(pc uintptr, width, height int) uintptr {
	if !libLoaded || shimVideoTrackSourceCreate == nil {
		return 0
	}
	return shimVideoTrackSourceCreate(pc, width, height)
}

// VideoTrackSourcePushFrame pushes an I420 frame to the video track source.
func VideoTrackSourcePushFrame(source uintptr, yPlane, uPlane, vPlane []byte, yStride, uStride, vStride int, timestampUs int64) error {
	if !libLoaded || shimVideoTrackSourcePushFrame == nil {
		return ErrLibraryNotLoaded
	}

	result := shimVideoTrackSourcePushFrame(
		source,
		ByteSlicePtr(yPlane),
		ByteSlicePtr(uPlane),
		ByteSlicePtr(vPlane),
		yStride, uStride, vStride,
		timestampUs,
	)
	return ShimError(result)
}

// PeerConnectionAddVideoTrackFromSource adds a video track using a source.
func PeerConnectionAddVideoTrackFromSource(pc, source uintptr, trackID, streamID string) uintptr {
	if !libLoaded || shimPeerConnectionAddVideoTrackFromSource == nil {
		return 0
	}

	trackIDCStr := CString(trackID)
	streamIDCStr := CString(streamID)
	return shimPeerConnectionAddVideoTrackFromSource(
		pc,
		source,
		ByteSlicePtr(trackIDCStr),
		ByteSlicePtr(streamIDCStr),
	)
}

// VideoTrackSourceDestroy destroys a video track source.
func VideoTrackSourceDestroy(source uintptr) {
	if !libLoaded || shimVideoTrackSourceDestroy == nil {
		return
	}
	shimVideoTrackSourceDestroy(source)
}

// AudioTrackSourceCreate creates an audio track source for frame injection.
func AudioTrackSourceCreate(pc uintptr, sampleRate, channels int) uintptr {
	if !libLoaded || shimAudioTrackSourceCreate == nil {
		return 0
	}
	return shimAudioTrackSourceCreate(pc, sampleRate, channels)
}

// AudioTrackSourcePushFrame pushes audio samples to the audio track source.
func AudioTrackSourcePushFrame(source uintptr, samples []int16, timestampUs int64) error {
	if !libLoaded || shimAudioTrackSourcePushFrame == nil {
		return ErrLibraryNotLoaded
	}

	result := shimAudioTrackSourcePushFrame(
		source,
		Int16SlicePtr(samples),
		len(samples),
		timestampUs,
	)
	return ShimError(result)
}

// PeerConnectionAddAudioTrackFromSource adds an audio track using a source.
func PeerConnectionAddAudioTrackFromSource(pc, source uintptr, trackID, streamID string) uintptr {
	if !libLoaded || shimPeerConnectionAddAudioTrackFromSource == nil {
		return 0
	}

	trackIDCStr := CString(trackID)
	streamIDCStr := CString(streamID)
	return shimPeerConnectionAddAudioTrackFromSource(
		pc,
		source,
		ByteSlicePtr(trackIDCStr),
		ByteSlicePtr(streamIDCStr),
	)
}

// AudioTrackSourceDestroy destroys an audio track source.
func AudioTrackSourceDestroy(source uintptr) {
	if !libLoaded || shimAudioTrackSourceDestroy == nil {
		return
	}
	shimAudioTrackSourceDestroy(source)
}

// TrackSetVideoSink sets a video frame callback on a remote track.
func TrackSetVideoSink(track uintptr, callback uintptr, ctx uintptr) error {
	if !libLoaded || shimTrackSetVideoSink == nil {
		return ErrLibraryNotLoaded
	}
	result := shimTrackSetVideoSink(track, callback, ctx)
	return ShimError(result)
}

// TrackSetAudioSink sets an audio frame callback on a remote track.
func TrackSetAudioSink(track uintptr, callback uintptr, ctx uintptr) error {
	if !libLoaded || shimTrackSetAudioSink == nil {
		return ErrLibraryNotLoaded
	}
	result := shimTrackSetAudioSink(track, callback, ctx)
	return ShimError(result)
}

// TrackRemoveVideoSink removes a video sink from a track.
func TrackRemoveVideoSink(track uintptr) {
	if !libLoaded || shimTrackRemoveVideoSink == nil {
		return
	}
	shimTrackRemoveVideoSink(track)
}

// TrackRemoveAudioSink removes an audio sink from a track.
func TrackRemoveAudioSink(track uintptr) {
	if !libLoaded || shimTrackRemoveAudioSink == nil {
		return
	}
	shimTrackRemoveAudioSink(track)
}

// TrackKind returns the track kind ("video" or "audio").
func TrackKind(track uintptr) string {
	if !libLoaded || shimTrackKind == nil {
		return ""
	}
	ptr := shimTrackKind(track)
	return GoString(ptr)
}

// TrackID returns the track ID.
func TrackID(track uintptr) string {
	if !libLoaded || shimTrackID == nil {
		return ""
	}
	ptr := shimTrackID(track)
	return GoString(ptr)
}
