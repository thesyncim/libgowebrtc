package ffi

import (
	"log"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// safeCallback wraps a callback invocation with panic recovery.
// This prevents panics in user callbacks from unwinding through C stack frames,
// which would cause undefined behavior.
func safeCallback(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[libgowebrtc] panic recovered in callback: %v", r)
		}
	}()
	fn()
}

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
//
//go:nocheckptr
func initCallbacks() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()

	if callbacksInitialized {
		return
	}

	// Create the video sink callback
	// Signature: void(ctx, width, height, y_plane, u_plane, v_plane, y_stride, u_stride, v_stride, timestamp_us)
	// NOTE: C uses 'int' (32-bit) for width/height/strides, so we must use int32 to match
	videoSinkCallbackPtr = purego.NewCallback(func(ctx uintptr, width, height int32, yPlane, uPlane, vPlane uintptr, yStride, uStride, vStride int32, timestampUs int64) {
		videoCallbackMu.RLock()
		cb, ok := videoCallbacks[ctx]
		videoCallbackMu.RUnlock()

		if !ok || cb == nil {
			return
		}

		// Validate dimensions to prevent panic from invalid data
		if width <= 0 || height <= 0 || width > 8192 || height > 8192 {
			return
		}
		if yStride <= 0 || uStride <= 0 || vStride <= 0 {
			return
		}
		if yStride > 16384 || uStride > 16384 || vStride > 16384 {
			return
		}

		// Calculate plane sizes
		ySize := int(yStride) * int(height)
		uvHeight := (int(height) + 1) / 2
		uSize := int(uStride) * uvHeight
		vSize := int(vStride) * uvHeight

		// Additional sanity check for total size
		const maxFrameSize = 64 * 1024 * 1024 // 64MB max
		if ySize > maxFrameSize || uSize > maxFrameSize || vSize > maxFrameSize {
			return
		}

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

		safeCallback(func() {
			cb(int(width), int(height), yData, uData, vData, int(yStride), int(uStride), int(vStride), timestampUs)
		})
	})

	// Create the audio sink callback
	// Signature: void(ctx, samples, num_samples, sample_rate, channels, timestamp_us)
	// NOTE: C uses 'int' (32-bit) for numSamples/sampleRate/channels, so we must use int32 to match
	audioSinkCallbackPtr = purego.NewCallback(func(ctx uintptr, samples uintptr, numSamples, sampleRate, channels int32, timestampUs int64) {
		audioCallbackMu.RLock()
		cb, ok := audioCallbacks[ctx]
		audioCallbackMu.RUnlock()

		if !ok || cb == nil {
			return
		}

		// Validate parameters
		if numSamples <= 0 || numSamples > 48000 || channels <= 0 || channels > 8 {
			return
		}

		// Total samples = numSamples * channels (interleaved)
		totalSamples := int(numSamples) * int(channels)

		// Copy data from C memory to Go slice
		samplesData := make([]int16, totalSamples)
		if samples != 0 {
			copy(samplesData, unsafe.Slice((*int16)(unsafe.Pointer(samples)), totalSamples))
		}

		safeCallback(func() {
			cb(samplesData, int(sampleRate), int(channels), timestampUs)
		})
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
	if !libLoaded.Load() || shimPeerConnectionCreate == nil {
		return 0
	}
	return shimPeerConnectionCreate(config.Ptr())
}

// PeerConnectionDestroy destroys a PeerConnection.
func PeerConnectionDestroy(pc uintptr) {
	if !libLoaded.Load() || shimPeerConnectionDestroy == nil {
		return
	}
	shimPeerConnectionDestroy(pc)
}

// PeerConnectionCreateOffer creates an SDP offer.
// Returns the SDP string written to the provided buffer.
func PeerConnectionCreateOffer(pc uintptr, sdpBuf []byte) (int, error) {
	if !libLoaded.Load() || shimPeerConnectionCreateOffer == nil {
		return 0, ErrLibraryNotLoaded
	}

	var sdpLen int32
	result := shimPeerConnectionCreateOffer(
		pc,
		ByteSlicePtr(sdpBuf),
		int32(len(sdpBuf)),
		Int32Ptr(&sdpLen),
	)

	if err := ShimError(result); err != nil {
		return 0, err
	}

	return int(sdpLen), nil
}

// PeerConnectionCreateAnswer creates an SDP answer.
func PeerConnectionCreateAnswer(pc uintptr, sdpBuf []byte) (int, error) {
	if !libLoaded.Load() || shimPeerConnectionCreateAnswer == nil {
		return 0, ErrLibraryNotLoaded
	}

	var sdpLen int32
	result := shimPeerConnectionCreateAnswer(
		pc,
		ByteSlicePtr(sdpBuf),
		int32(len(sdpBuf)),
		Int32Ptr(&sdpLen),
	)

	if err := ShimError(result); err != nil {
		return 0, err
	}

	return int(sdpLen), nil
}

// PeerConnectionSetLocalDescription sets the local SDP description.
func PeerConnectionSetLocalDescription(pc uintptr, sdpType int, sdp string) error {
	if !libLoaded.Load() || shimPeerConnectionSetLocalDescription == nil {
		return ErrLibraryNotLoaded
	}

	sdpCStr := CString(sdp)
	result := shimPeerConnectionSetLocalDescription(pc, int32(sdpType), ByteSlicePtr(sdpCStr))
	runtime.KeepAlive(sdpCStr)
	return ShimError(result)
}

// PeerConnectionSetRemoteDescription sets the remote SDP description.
func PeerConnectionSetRemoteDescription(pc uintptr, sdpType int, sdp string) error {
	if !libLoaded.Load() || shimPeerConnectionSetRemoteDescription == nil {
		return ErrLibraryNotLoaded
	}

	sdpCStr := CString(sdp)
	result := shimPeerConnectionSetRemoteDescription(pc, int32(sdpType), ByteSlicePtr(sdpCStr))
	runtime.KeepAlive(sdpCStr)
	return ShimError(result)
}

// PeerConnectionAddICECandidate adds an ICE candidate.
func PeerConnectionAddICECandidate(pc uintptr, candidate, sdpMid string, sdpMLineIndex int) error {
	if !libLoaded.Load() || shimPeerConnectionAddICECandidate == nil {
		return ErrLibraryNotLoaded
	}

	candidateCStr := CString(candidate)
	sdpMidCStr := CString(sdpMid)
	result := shimPeerConnectionAddICECandidate(
		pc,
		ByteSlicePtr(candidateCStr),
		ByteSlicePtr(sdpMidCStr),
		int32(sdpMLineIndex),
	)
	runtime.KeepAlive(candidateCStr)
	runtime.KeepAlive(sdpMidCStr)
	return ShimError(result)
}

// PeerConnectionSignalingState returns the signaling state.
func PeerConnectionSignalingState(pc uintptr) int {
	if !libLoaded.Load() || shimPeerConnectionSignalingState == nil {
		return -1
	}
	return int(shimPeerConnectionSignalingState(pc))
}

// PeerConnectionICEConnectionState returns the ICE connection state.
func PeerConnectionICEConnectionState(pc uintptr) int {
	if !libLoaded.Load() || shimPeerConnectionICEConnectionState == nil {
		return -1
	}
	return int(shimPeerConnectionICEConnectionState(pc))
}

// PeerConnectionICEGatheringState returns the ICE gathering state.
func PeerConnectionICEGatheringState(pc uintptr) int {
	if !libLoaded.Load() || shimPeerConnectionICEGatheringState == nil {
		return -1
	}
	return int(shimPeerConnectionICEGatheringState(pc))
}

// PeerConnectionConnectionState returns the connection state.
func PeerConnectionConnectionState(pc uintptr) int {
	if !libLoaded.Load() || shimPeerConnectionConnectionState == nil {
		return -1
	}
	return int(shimPeerConnectionConnectionState(pc))
}

// PeerConnectionAddTrack adds a track to the peer connection.
func PeerConnectionAddTrack(pc uintptr, codec CodecType, trackID, streamID string) uintptr {
	if !libLoaded.Load() || shimPeerConnectionAddTrack == nil {
		return 0
	}

	trackIDCStr := CString(trackID)
	streamIDCStr := CString(streamID)
	result := shimPeerConnectionAddTrack(
		pc,
		int32(codec),
		ByteSlicePtr(trackIDCStr),
		ByteSlicePtr(streamIDCStr),
	)
	runtime.KeepAlive(trackIDCStr)
	runtime.KeepAlive(streamIDCStr)
	return result
}

// PeerConnectionRemoveTrack removes a track from the peer connection.
func PeerConnectionRemoveTrack(pc uintptr, sender uintptr) error {
	if !libLoaded.Load() || shimPeerConnectionRemoveTrack == nil {
		return ErrLibraryNotLoaded
	}
	result := shimPeerConnectionRemoveTrack(pc, sender)
	return ShimError(result)
}

// PeerConnectionCreateDataChannel creates a data channel.
func PeerConnectionCreateDataChannel(pc uintptr, label string, ordered bool, maxRetransmits int, protocol string) uintptr {
	if !libLoaded.Load() || shimPeerConnectionCreateDataChannel == nil {
		return 0
	}

	labelCStr := CString(label)
	protocolCStr := CString(protocol)

	var orderedInt int32 = 0
	if ordered {
		orderedInt = 1
	}

	result := shimPeerConnectionCreateDataChannel(
		pc,
		ByteSlicePtr(labelCStr),
		orderedInt,
		int32(maxRetransmits),
		ByteSlicePtr(protocolCStr),
	)
	runtime.KeepAlive(labelCStr)
	runtime.KeepAlive(protocolCStr)
	return result
}

// PeerConnectionClose closes the peer connection.
func PeerConnectionClose(pc uintptr) {
	if !libLoaded.Load() || shimPeerConnectionClose == nil {
		return
	}
	shimPeerConnectionClose(pc)
}

// RTPSenderSetBitrate sets the bitrate for an RTP sender.
func RTPSenderSetBitrate(sender uintptr, bitrate uint32) error {
	if !libLoaded.Load() || shimRTPSenderSetBitrate == nil {
		return ErrLibraryNotLoaded
	}
	result := shimRTPSenderSetBitrate(sender, bitrate)
	return ShimError(result)
}

// RTPSenderDestroy destroys an RTP sender.
func RTPSenderDestroy(sender uintptr) {
	if !libLoaded.Load() || shimRTPSenderDestroy == nil {
		return
	}
	shimRTPSenderDestroy(sender)
}

// RTPSenderReplaceTrack replaces the track on an RTP sender.
func RTPSenderReplaceTrack(sender uintptr, track uintptr) error {
	if !libLoaded.Load() || shimRTPSenderReplaceTrack == nil {
		return ErrLibraryNotLoaded
	}
	result := shimRTPSenderReplaceTrack(sender, track)
	return ShimError(result)
}

// DataChannelSend sends data on a data channel.
func DataChannelSend(dc uintptr, data []byte, isBinary bool) error {
	if !libLoaded.Load() || shimDataChannelSend == nil {
		return ErrLibraryNotLoaded
	}

	var isBinaryInt int32 = 0
	if isBinary {
		isBinaryInt = 1
	}

	result := shimDataChannelSend(dc, ByteSlicePtr(data), int32(len(data)), isBinaryInt)
	return ShimError(result)
}

// DataChannelReadyState returns the ready state of a data channel.
func DataChannelReadyState(dc uintptr) int {
	if !libLoaded.Load() || shimDataChannelReadyState == nil {
		return -1
	}
	return int(shimDataChannelReadyState(dc))
}

// DataChannelLabel returns the label of a data channel.
func DataChannelLabel(dc uintptr) string {
	if !libLoaded.Load() || shimDataChannelLabel == nil {
		return ""
	}
	ptr := shimDataChannelLabel(dc)
	return GoString(unsafe.Pointer(ptr))
}

// DataChannelClose closes a data channel.
func DataChannelClose(dc uintptr) {
	if !libLoaded.Load() || shimDataChannelClose == nil {
		return
	}
	shimDataChannelClose(dc)
}

// DataChannelDestroy destroys a data channel.
func DataChannelDestroy(dc uintptr) {
	if !libLoaded.Load() || shimDataChannelDestroy == nil {
		return
	}
	shimDataChannelDestroy(dc)
}

// DataChannel callback types
type OnDataChannelMessageCallback func(data []byte, isBinary bool)
type OnDataChannelStateCallback func()

// DataChannel callback registries
var (
	dcMessageCallbackMu  sync.RWMutex
	dcMessageCallbacks   = make(map[uintptr]OnDataChannelMessageCallback)
	dcMessageCallbackPtr uintptr
	dcMessageInitialized bool

	dcOpenCallbackMu  sync.RWMutex
	dcOpenCallbacks   = make(map[uintptr]OnDataChannelStateCallback)
	dcOpenCallbackPtr uintptr
	dcOpenInitialized bool

	dcCloseCallbackMu  sync.RWMutex
	dcCloseCallbacks   = make(map[uintptr]OnDataChannelStateCallback)
	dcCloseCallbackPtr uintptr
	dcCloseInitialized bool
)

//go:nocheckptr
func initDCMessageCallback() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()
	if dcMessageInitialized {
		return
	}
	// NOTE: C uses 'int' (32-bit) for size/isBinary, so we must use int32 to match
	dcMessageCallbackPtr = purego.NewCallback(func(ctx uintptr, data uintptr, size int32, isBinary int32) {
		dcMessageCallbackMu.RLock()
		cb, ok := dcMessageCallbacks[ctx]
		dcMessageCallbackMu.RUnlock()
		if ok && cb != nil {
			// Bounds validation: DataChannel messages should be reasonably sized
			const maxMessageSize = 256 * 1024 * 1024 // 256MB max
			if size < 0 || size > maxMessageSize {
				return
			}
			// Copy data before callback (C memory may be reused)
			goData := make([]byte, size)
			if size > 0 && data != 0 {
				copy(goData, unsafe.Slice((*byte)(unsafe.Pointer(data)), size))
			}
			safeCallback(func() { cb(goData, isBinary != 0) })
		}
	})
	dcMessageInitialized = true
}

func initDCOpenCallback() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()
	if dcOpenInitialized {
		return
	}
	dcOpenCallbackPtr = purego.NewCallback(func(ctx uintptr) {
		dcOpenCallbackMu.RLock()
		cb, ok := dcOpenCallbacks[ctx]
		dcOpenCallbackMu.RUnlock()
		if ok && cb != nil {
			safeCallback(cb)
		}
	})
	dcOpenInitialized = true
}

func initDCCloseCallback() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()
	if dcCloseInitialized {
		return
	}
	dcCloseCallbackPtr = purego.NewCallback(func(ctx uintptr) {
		dcCloseCallbackMu.RLock()
		cb, ok := dcCloseCallbacks[ctx]
		dcCloseCallbackMu.RUnlock()
		if ok && cb != nil {
			safeCallback(cb)
		}
	})
	dcCloseInitialized = true
}

// DataChannelSetOnMessage sets the message callback for a data channel.
func DataChannelSetOnMessage(dc uintptr, cb OnDataChannelMessageCallback) {
	if !libLoaded.Load() || shimDataChannelSetOnMessage == nil {
		return
	}
	initDCMessageCallback()
	dcMessageCallbackMu.Lock()
	dcMessageCallbacks[dc] = cb
	dcMessageCallbackMu.Unlock()
	shimDataChannelSetOnMessage(dc, dcMessageCallbackPtr, dc)
}

// DataChannelSetOnOpen sets the open callback for a data channel.
func DataChannelSetOnOpen(dc uintptr, cb OnDataChannelStateCallback) {
	if !libLoaded.Load() || shimDataChannelSetOnOpen == nil {
		return
	}
	initDCOpenCallback()
	dcOpenCallbackMu.Lock()
	dcOpenCallbacks[dc] = cb
	dcOpenCallbackMu.Unlock()
	shimDataChannelSetOnOpen(dc, dcOpenCallbackPtr, dc)
}

// DataChannelSetOnClose sets the close callback for a data channel.
func DataChannelSetOnClose(dc uintptr, cb OnDataChannelStateCallback) {
	if !libLoaded.Load() || shimDataChannelSetOnClose == nil {
		return
	}
	initDCCloseCallback()
	dcCloseCallbackMu.Lock()
	dcCloseCallbacks[dc] = cb
	dcCloseCallbackMu.Unlock()
	shimDataChannelSetOnClose(dc, dcCloseCallbackPtr, dc)
}

// UnregisterDataChannelCallbacks removes all callbacks for a data channel.
func UnregisterDataChannelCallbacks(dc uintptr) {
	dcMessageCallbackMu.Lock()
	delete(dcMessageCallbacks, dc)
	dcMessageCallbackMu.Unlock()

	dcOpenCallbackMu.Lock()
	delete(dcOpenCallbacks, dc)
	dcOpenCallbackMu.Unlock()

	dcCloseCallbackMu.Lock()
	delete(dcCloseCallbacks, dc)
	dcCloseCallbackMu.Unlock()
}

// VideoTrackSourceCreate creates a video track source for frame injection.
func VideoTrackSourceCreate(pc uintptr, width, height int) uintptr {
	if !libLoaded.Load() || shimVideoTrackSourceCreate == nil {
		return 0
	}
	return shimVideoTrackSourceCreate(pc, int32(width), int32(height))
}

// VideoTrackSourcePushFrame pushes an I420 frame to the video track source.
func VideoTrackSourcePushFrame(source uintptr, yPlane, uPlane, vPlane []byte, yStride, uStride, vStride int, timestampUs int64) error {
	if !libLoaded.Load() || shimVideoTrackSourcePushFrame == nil {
		return ErrLibraryNotLoaded
	}

	// DEBUG: Log every 100th call
	static_counter++
	if static_counter%100 == 0 {
		println("DEBUG FFI: VideoTrackSourcePushFrame source=", source, "yLen=", len(yPlane), "ts=", timestampUs)
	}

	result := shimVideoTrackSourcePushFrame(
		source,
		ByteSlicePtr(yPlane),
		ByteSlicePtr(uPlane),
		ByteSlicePtr(vPlane),
		int32(yStride), int32(uStride), int32(vStride),
		timestampUs,
	)
	return ShimError(result)
}

var static_counter int

// PeerConnectionAddVideoTrackFromSource adds a video track using a source.
func PeerConnectionAddVideoTrackFromSource(pc, source uintptr, trackID, streamID string) uintptr {
	if !libLoaded.Load() || shimPeerConnectionAddVideoTrackFromSource == nil {
		return 0
	}

	trackIDCStr := CString(trackID)
	streamIDCStr := CString(streamID)
	result := shimPeerConnectionAddVideoTrackFromSource(
		pc,
		source,
		ByteSlicePtr(trackIDCStr),
		ByteSlicePtr(streamIDCStr),
	)
	runtime.KeepAlive(trackIDCStr)
	runtime.KeepAlive(streamIDCStr)
	return result
}

// VideoTrackSourceDestroy destroys a video track source.
func VideoTrackSourceDestroy(source uintptr) {
	if !libLoaded.Load() || shimVideoTrackSourceDestroy == nil {
		return
	}
	shimVideoTrackSourceDestroy(source)
}

// AudioTrackSourceCreate creates an audio track source for frame injection.
func AudioTrackSourceCreate(pc uintptr, sampleRate, channels int) uintptr {
	if !libLoaded.Load() || shimAudioTrackSourceCreate == nil {
		return 0
	}
	return shimAudioTrackSourceCreate(pc, int32(sampleRate), int32(channels))
}

// AudioTrackSourcePushFrame pushes audio samples to the audio track source.
func AudioTrackSourcePushFrame(source uintptr, samples []int16, timestampUs int64) error {
	if !libLoaded.Load() || shimAudioTrackSourcePushFrame == nil {
		return ErrLibraryNotLoaded
	}

	result := shimAudioTrackSourcePushFrame(
		source,
		Int16SlicePtr(samples),
		int32(len(samples)),
		timestampUs,
	)
	return ShimError(result)
}

// PeerConnectionAddAudioTrackFromSource adds an audio track using a source.
func PeerConnectionAddAudioTrackFromSource(pc, source uintptr, trackID, streamID string) uintptr {
	if !libLoaded.Load() || shimPeerConnectionAddAudioTrackFromSource == nil {
		return 0
	}

	trackIDCStr := CString(trackID)
	streamIDCStr := CString(streamID)
	result := shimPeerConnectionAddAudioTrackFromSource(
		pc,
		source,
		ByteSlicePtr(trackIDCStr),
		ByteSlicePtr(streamIDCStr),
	)
	runtime.KeepAlive(trackIDCStr)
	runtime.KeepAlive(streamIDCStr)
	return result
}

// AudioTrackSourceDestroy destroys an audio track source.
func AudioTrackSourceDestroy(source uintptr) {
	if !libLoaded.Load() || shimAudioTrackSourceDestroy == nil {
		return
	}
	shimAudioTrackSourceDestroy(source)
}

// TrackSetVideoSink sets a video frame callback on a remote track.
func TrackSetVideoSink(track uintptr, callback uintptr, ctx uintptr) error {
	if !libLoaded.Load() || shimTrackSetVideoSink == nil {
		return ErrLibraryNotLoaded
	}
	result := shimTrackSetVideoSink(track, callback, ctx)
	return ShimError(result)
}

// TrackSetAudioSink sets an audio frame callback on a remote track.
func TrackSetAudioSink(track uintptr, callback uintptr, ctx uintptr) error {
	if !libLoaded.Load() || shimTrackSetAudioSink == nil {
		return ErrLibraryNotLoaded
	}
	result := shimTrackSetAudioSink(track, callback, ctx)
	return ShimError(result)
}

// TrackRemoveVideoSink removes a video sink from a track.
func TrackRemoveVideoSink(track uintptr) {
	if !libLoaded.Load() || shimTrackRemoveVideoSink == nil {
		return
	}
	shimTrackRemoveVideoSink(track)
}

// TrackRemoveAudioSink removes an audio sink from a track.
func TrackRemoveAudioSink(track uintptr) {
	if !libLoaded.Load() || shimTrackRemoveAudioSink == nil {
		return
	}
	shimTrackRemoveAudioSink(track)
}

// TrackKind returns the track kind ("video" or "audio").
func TrackKind(track uintptr) string {
	if !libLoaded.Load() || shimTrackKind == nil {
		return ""
	}
	ptr := shimTrackKind(track)
	return GoString(unsafe.Pointer(ptr))
}

// TrackID returns the track ID.
func TrackID(track uintptr) string {
	if !libLoaded.Load() || shimTrackID == nil {
		return ""
	}
	ptr := shimTrackID(track)
	return GoString(unsafe.Pointer(ptr))
}

// ============================================================================
// RTPSender Parameters API
// ============================================================================

// RTPEncodingParameters matches ShimRTPEncodingParameters in shim.h
type RTPEncodingParameters struct {
	RID                   [64]byte
	MaxBitrateBps         uint32
	MinBitrateBps         uint32
	MaxFramerate          float64
	ScaleResolutionDownBy float64
	Active                int32
	ScalabilityMode       [32]byte
}

// RTPSendParameters matches ShimRTPSendParameters in shim.h
type RTPSendParameters struct {
	Encodings     uintptr
	EncodingCount int32
	TransactionID [64]byte
}

// RTPSenderGetParameters gets the current RTP send parameters.
func RTPSenderGetParameters(sender uintptr, encodings []RTPEncodingParameters) (*RTPSendParameters, int, error) {
	if !libLoaded.Load() || shimRTPSenderGetParameters == nil {
		return nil, 0, ErrLibraryNotLoaded
	}

	var params RTPSendParameters
	result := shimRTPSenderGetParameters(
		sender,
		uintptr(unsafe.Pointer(&params)),
		uintptr(unsafe.Pointer(&encodings[0])),
		int32(len(encodings)),
	)

	if err := ShimError(result); err != nil {
		return nil, 0, err
	}

	return &params, int(params.EncodingCount), nil
}

// RTPSenderSetParameters sets the RTP send parameters.
func RTPSenderSetParameters(sender uintptr, params *RTPSendParameters) error {
	if !libLoaded.Load() || shimRTPSenderSetParameters == nil {
		return ErrLibraryNotLoaded
	}

	result := shimRTPSenderSetParameters(sender, uintptr(unsafe.Pointer(params)))
	return ShimError(result)
}

// RTPSenderGetTrack gets the track associated with a sender.
func RTPSenderGetTrack(sender uintptr) uintptr {
	if !libLoaded.Load() || shimRTPSenderGetTrack == nil {
		return 0
	}
	return shimRTPSenderGetTrack(sender)
}

// RTCStats matches ShimRTCStats in shim.h
type RTCStats struct {
	TimestampUs              int64
	BytesSent                int64
	BytesReceived            int64
	PacketsSent              int64
	PacketsReceived          int64
	PacketsLost              int64
	RoundTripTimeMs          float64
	JitterMs                 float64
	AvailableOutgoingBitrate float64
	AvailableIncomingBitrate float64
	CurrentRTTMs             int64
	TotalRTTMs               int64
	ResponsesReceived        int64
	FramesEncoded            int32
	FramesDecoded            int32
	FramesDropped            int32
	KeyFramesEncoded         int32
	KeyFramesDecoded         int32
	NACKCount                int32
	PLICount                 int32
	FIRCount                 int32
	QPSum                    int32
	AudioLevel               float64
	TotalAudioEnergy         float64
	ConcealmentEvents        int32

	// SCTP/DataChannel stats
	DataChannelsOpened       int64
	DataChannelsClosed       int64
	MessagesSent             int64
	MessagesReceived         int64
	BytesSentDataChannel     int64
	BytesReceivedDataChannel int64

	// Quality limitation
	QualityLimitationReason     int32 // 0=none, 1=cpu, 2=bandwidth, 3=other
	QualityLimitationDurationMs int32

	// Remote inbound/outbound RTP stats
	RemotePacketsLost     int64
	RemoteJitterMs        float64
	RemoteRoundTripTimeMs float64

	// Jitter buffer stats (from RTCInboundRtpStreamStats)
	JitterBufferDelayMs        float64 // Total time spent in jitter buffer / emitted count
	JitterBufferTargetDelayMs  float64 // Target delay for adaptive buffer
	JitterBufferMinimumDelayMs float64 // User-configured minimum delay
	JitterBufferEmittedCount   int64   // Number of samples/frames emitted from buffer
}

// Quality limitation reason constants
const (
	QualityLimitationNone      = 0
	QualityLimitationCPU       = 1
	QualityLimitationBandwidth = 2
	QualityLimitationOther     = 3
)

// CodecCapability represents a supported codec.
type CodecCapability struct {
	MimeType    [64]byte
	ClockRate   int32
	Channels    int32
	SDPFmtpLine [256]byte
	PayloadType int32
}

// BandwidthEstimate represents bandwidth estimation info.
type BandwidthEstimate struct {
	TimestampUs      int64
	TargetBitrateBps int64
	AvailableSendBps int64
	AvailableRecvBps int64
	PacingRateBps    int64
	CongestionWindow int32
	_                int32 // padding
	LossRate         float64
}

// RTPSenderGetStats gets statistics for a sender.
func RTPSenderGetStats(sender uintptr) (*RTCStats, error) {
	if !libLoaded.Load() || shimRTPSenderGetStats == nil {
		return nil, ErrLibraryNotLoaded
	}

	var stats RTCStats
	result := shimRTPSenderGetStats(sender, uintptr(unsafe.Pointer(&stats)))
	if err := ShimError(result); err != nil {
		return nil, err
	}

	return &stats, nil
}

// RTCPFeedbackCallback is called when RTCP feedback is received.
type RTCPFeedbackCallback func(feedbackType int, ssrc uint32)

var (
	rtcpFeedbackCallbackMu  sync.RWMutex
	rtcpFeedbackCallbacks   = make(map[uintptr]RTCPFeedbackCallback)
	rtcpFeedbackCallbackPtr uintptr
	rtcpCallbackInitialized bool
)

func initRTCPCallback() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()

	if rtcpCallbackInitialized {
		return
	}

	// NOTE: C uses 'int' (32-bit) for feedbackType, so we must use int32 to match
	rtcpFeedbackCallbackPtr = purego.NewCallback(func(ctx uintptr, feedbackType int32, ssrc uint32) {
		rtcpFeedbackCallbackMu.RLock()
		cb, ok := rtcpFeedbackCallbacks[ctx]
		rtcpFeedbackCallbackMu.RUnlock()

		if ok && cb != nil {
			safeCallback(func() {
				cb(int(feedbackType), ssrc)
			})
		}
	})

	rtcpCallbackInitialized = true
}

// RTPSenderSetOnRTCPFeedback sets the RTCP feedback callback.
func RTPSenderSetOnRTCPFeedback(sender uintptr, cb RTCPFeedbackCallback) {
	if !libLoaded.Load() || shimRTPSenderSetOnRTCPFeedback == nil {
		return
	}

	initRTCPCallback()

	rtcpFeedbackCallbackMu.Lock()
	rtcpFeedbackCallbacks[sender] = cb
	rtcpFeedbackCallbackMu.Unlock()

	shimRTPSenderSetOnRTCPFeedback(sender, rtcpFeedbackCallbackPtr, sender)
}

// UnregisterRTCPFeedbackCallback removes the RTCP feedback callback for a sender.
func UnregisterRTCPFeedbackCallback(sender uintptr) {
	rtcpFeedbackCallbackMu.Lock()
	delete(rtcpFeedbackCallbacks, sender)
	rtcpFeedbackCallbackMu.Unlock()
}

// RTPSenderSetLayerActive enables or disables a simulcast layer.
func RTPSenderSetLayerActive(sender uintptr, rid string, active bool) error {
	if !libLoaded.Load() || shimRTPSenderSetLayerActive == nil {
		return ErrLibraryNotLoaded
	}

	ridCStr := CString(rid)
	var activeInt int32 = 0
	if active {
		activeInt = 1
	}

	result := shimRTPSenderSetLayerActive(sender, ByteSlicePtr(ridCStr), activeInt)
	runtime.KeepAlive(ridCStr)
	return ShimError(result)
}

// RTPSenderSetLayerBitrate sets the maximum bitrate for a layer.
func RTPSenderSetLayerBitrate(sender uintptr, rid string, maxBitrate uint32) error {
	if !libLoaded.Load() || shimRTPSenderSetLayerBitrate == nil {
		return ErrLibraryNotLoaded
	}

	ridCStr := CString(rid)
	result := shimRTPSenderSetLayerBitrate(sender, ByteSlicePtr(ridCStr), maxBitrate)
	runtime.KeepAlive(ridCStr)
	return ShimError(result)
}

// RTPSenderGetActiveLayers gets the number of active layers.
func RTPSenderGetActiveLayers(sender uintptr) (spatial, temporal int, err error) {
	if !libLoaded.Load() || shimRTPSenderGetActiveLayers == nil {
		return 0, 0, ErrLibraryNotLoaded
	}

	var spatialOut, temporalOut int32
	result := shimRTPSenderGetActiveLayers(sender, Int32Ptr(&spatialOut), Int32Ptr(&temporalOut))
	return int(spatialOut), int(temporalOut), ShimError(result)
}

// ============================================================================
// RTPReceiver API
// ============================================================================

// RTPReceiverGetTrack gets the track associated with a receiver.
func RTPReceiverGetTrack(receiver uintptr) uintptr {
	if !libLoaded.Load() || shimRTPReceiverGetTrack == nil {
		return 0
	}
	return shimRTPReceiverGetTrack(receiver)
}

// RTPReceiverGetStats gets statistics for a receiver.
func RTPReceiverGetStats(receiver uintptr) (*RTCStats, error) {
	if !libLoaded.Load() || shimRTPReceiverGetStats == nil {
		return nil, ErrLibraryNotLoaded
	}

	var stats RTCStats
	result := shimRTPReceiverGetStats(receiver, uintptr(unsafe.Pointer(&stats)))
	if err := ShimError(result); err != nil {
		return nil, err
	}

	return &stats, nil
}

// ============================================================================
// RTPTransceiver API
// ============================================================================

// TransceiverDirection represents the direction of a transceiver.
type TransceiverDirection int

const (
	TransceiverDirectionSendRecv TransceiverDirection = 0
	TransceiverDirectionSendOnly TransceiverDirection = 1
	TransceiverDirectionRecvOnly TransceiverDirection = 2
	TransceiverDirectionInactive TransceiverDirection = 3
	TransceiverDirectionStopped  TransceiverDirection = 4
)

// TransceiverGetDirection gets the current direction of a transceiver.
func TransceiverGetDirection(transceiver uintptr) TransceiverDirection {
	if !libLoaded.Load() || shimTransceiverGetDirection == nil {
		return TransceiverDirectionInactive
	}
	return TransceiverDirection(shimTransceiverGetDirection(transceiver))
}

// TransceiverSetDirection sets the direction of a transceiver.
func TransceiverSetDirection(transceiver uintptr, direction TransceiverDirection) error {
	if !libLoaded.Load() || shimTransceiverSetDirection == nil {
		return ErrLibraryNotLoaded
	}

	result := shimTransceiverSetDirection(transceiver, int32(direction))
	return ShimError(result)
}

// TransceiverGetCurrentDirection gets the current direction as negotiated in SDP.
func TransceiverGetCurrentDirection(transceiver uintptr) TransceiverDirection {
	if !libLoaded.Load() || shimTransceiverGetCurrentDirection == nil {
		return TransceiverDirectionInactive
	}
	return TransceiverDirection(shimTransceiverGetCurrentDirection(transceiver))
}

// TransceiverStop stops the transceiver.
func TransceiverStop(transceiver uintptr) error {
	if !libLoaded.Load() || shimTransceiverStop == nil {
		return ErrLibraryNotLoaded
	}

	result := shimTransceiverStop(transceiver)
	return ShimError(result)
}

// TransceiverMid gets the mid of a transceiver.
func TransceiverMid(transceiver uintptr) string {
	if !libLoaded.Load() || shimTransceiverMid == nil {
		return ""
	}
	ptr := shimTransceiverMid(transceiver)
	return GoString(unsafe.Pointer(ptr))
}

// TransceiverGetSender gets the sender associated with a transceiver.
func TransceiverGetSender(transceiver uintptr) uintptr {
	if !libLoaded.Load() || shimTransceiverGetSender == nil {
		return 0
	}
	return shimTransceiverGetSender(transceiver)
}

// TransceiverGetReceiver gets the receiver associated with a transceiver.
func TransceiverGetReceiver(transceiver uintptr) uintptr {
	if !libLoaded.Load() || shimTransceiverGetReceiver == nil {
		return 0
	}
	return shimTransceiverGetReceiver(transceiver)
}

// ============================================================================
// PeerConnection Extended API
// ============================================================================

// MediaKind represents the kind of media.
type MediaKind int

const (
	MediaKindAudio MediaKind = 0
	MediaKindVideo MediaKind = 1
)

// PeerConnectionAddTransceiver adds a transceiver with the specified kind and direction.
func PeerConnectionAddTransceiver(pc uintptr, kind MediaKind, direction TransceiverDirection) uintptr {
	if !libLoaded.Load() || shimPeerConnectionAddTransceiver == nil {
		return 0
	}
	return shimPeerConnectionAddTransceiver(pc, int32(kind), int32(direction))
}

// PeerConnectionGetSenders gets all senders associated with a PeerConnection.
func PeerConnectionGetSenders(pc uintptr, maxSenders int) ([]uintptr, error) {
	if !libLoaded.Load() || shimPeerConnectionGetSenders == nil {
		return nil, ErrLibraryNotLoaded
	}

	senders := make([]uintptr, maxSenders)
	var count int32
	result := shimPeerConnectionGetSenders(pc, uintptr(unsafe.Pointer(&senders[0])), int32(maxSenders), Int32Ptr(&count))

	if err := ShimError(result); err != nil {
		return nil, err
	}

	return senders[:count], nil
}

// PeerConnectionGetReceivers gets all receivers associated with a PeerConnection.
func PeerConnectionGetReceivers(pc uintptr, maxReceivers int) ([]uintptr, error) {
	if !libLoaded.Load() || shimPeerConnectionGetReceivers == nil {
		return nil, ErrLibraryNotLoaded
	}

	receivers := make([]uintptr, maxReceivers)
	var count int32
	result := shimPeerConnectionGetReceivers(pc, uintptr(unsafe.Pointer(&receivers[0])), int32(maxReceivers), Int32Ptr(&count))

	if err := ShimError(result); err != nil {
		return nil, err
	}

	return receivers[:count], nil
}

// PeerConnectionGetTransceivers gets all transceivers associated with a PeerConnection.
func PeerConnectionGetTransceivers(pc uintptr, maxTransceivers int) ([]uintptr, error) {
	if !libLoaded.Load() || shimPeerConnectionGetTransceivers == nil {
		return nil, ErrLibraryNotLoaded
	}

	transceivers := make([]uintptr, maxTransceivers)
	var count int32
	result := shimPeerConnectionGetTransceivers(pc, uintptr(unsafe.Pointer(&transceivers[0])), int32(maxTransceivers), Int32Ptr(&count))

	if err := ShimError(result); err != nil {
		return nil, err
	}

	return transceivers[:count], nil
}

// PeerConnectionRestartICE triggers an ICE restart on the next offer.
func PeerConnectionRestartICE(pc uintptr) error {
	if !libLoaded.Load() || shimPeerConnectionRestartICE == nil {
		return ErrLibraryNotLoaded
	}

	result := shimPeerConnectionRestartICE(pc)
	return ShimError(result)
}

// PeerConnectionGetStats gets connection statistics.
func PeerConnectionGetStats(pc uintptr) (*RTCStats, error) {
	if !libLoaded.Load() || shimPeerConnectionGetStats == nil {
		return nil, ErrLibraryNotLoaded
	}

	var stats RTCStats
	result := shimPeerConnectionGetStats(pc, uintptr(unsafe.Pointer(&stats)))
	if err := ShimError(result); err != nil {
		return nil, err
	}

	return &stats, nil
}

// ============================================================================
// Connection State Change Callback
// ============================================================================

// ConnectionStateCallback is called when the connection state changes.
type ConnectionStateCallback func(state int)

var (
	connectionStateCallbackMu  sync.RWMutex
	connectionStateCallbacks   = make(map[uintptr]ConnectionStateCallback)
	connectionStateCallbackPtr uintptr
	connectionStateInitialized bool
)

func initConnectionStateCallback() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()

	if connectionStateInitialized {
		return
	}

	// NOTE: C uses 'int' (32-bit) for state, so we must use int32 to match
	connectionStateCallbackPtr = purego.NewCallback(func(ctx uintptr, state int32) {
		connectionStateCallbackMu.RLock()
		cb, ok := connectionStateCallbacks[ctx]
		connectionStateCallbackMu.RUnlock()

		if ok && cb != nil {
			safeCallback(func() {
				cb(int(state))
			})
		}
	})

	connectionStateInitialized = true
}

// PeerConnectionSetOnConnectionStateChange sets the connection state change callback.
func PeerConnectionSetOnConnectionStateChange(pc uintptr, cb ConnectionStateCallback) {
	if !libLoaded.Load() || shimPeerConnectionSetOnConnectionStateChange == nil {
		return
	}

	initConnectionStateCallback()

	connectionStateCallbackMu.Lock()
	connectionStateCallbacks[pc] = cb
	connectionStateCallbackMu.Unlock()

	shimPeerConnectionSetOnConnectionStateChange(pc, connectionStateCallbackPtr, pc)
}

// UnregisterConnectionStateCallback removes the connection state callback for a PC.
func UnregisterConnectionStateCallback(pc uintptr) {
	connectionStateCallbackMu.Lock()
	delete(connectionStateCallbacks, pc)
	connectionStateCallbackMu.Unlock()
}

// ============================================================================
// OnTrack Callback
// ============================================================================

// OnTrackCallback is called when a remote track is received.
type OnTrackCallback func(track uintptr, receiver uintptr, streams string)

var (
	onTrackCallbackMu  sync.RWMutex
	onTrackCallbacks   = make(map[uintptr]OnTrackCallback)
	onTrackCallbackPtr uintptr
	onTrackInitialized bool
)

func initOnTrackCallback() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()

	if onTrackInitialized {
		return
	}

	onTrackCallbackPtr = purego.NewCallback(func(ctx uintptr, track, receiver uintptr, streams uintptr) {
		onTrackCallbackMu.RLock()
		cb, ok := onTrackCallbacks[ctx]
		onTrackCallbackMu.RUnlock()

		if ok && cb != nil {
			streamsStr := GoString(unsafe.Pointer(streams))
			safeCallback(func() {
				cb(track, receiver, streamsStr)
			})
		}
	})

	onTrackInitialized = true
}

// PeerConnectionSetOnTrack sets the on track callback.
func PeerConnectionSetOnTrack(pc uintptr, cb OnTrackCallback) {
	if !libLoaded.Load() || shimPeerConnectionSetOnTrack == nil {
		return
	}

	initOnTrackCallback()

	onTrackCallbackMu.Lock()
	onTrackCallbacks[pc] = cb
	onTrackCallbackMu.Unlock()

	shimPeerConnectionSetOnTrack(pc, onTrackCallbackPtr, pc)
}

// UnregisterOnTrackCallback removes the on track callback for a PC.
func UnregisterOnTrackCallback(pc uintptr) {
	onTrackCallbackMu.Lock()
	delete(onTrackCallbacks, pc)
	onTrackCallbackMu.Unlock()
}

// ============================================================================
// OnICECandidate Callback
// ============================================================================

// OnICECandidateCallback is called when an ICE candidate is generated.
type OnICECandidateCallback func(candidate, sdpMid string, sdpMLineIndex int)

var (
	onICECandidateCallbackMu  sync.RWMutex
	onICECandidateCallbacks   = make(map[uintptr]OnICECandidateCallback)
	onICECandidateCallbackPtr uintptr
	onICECandidateInitialized bool
)

func initOnICECandidateCallback() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()

	if onICECandidateInitialized {
		return
	}

	// Callback receives pointer to ShimICECandidate struct
	onICECandidateCallbackPtr = purego.NewCallback(func(ctx uintptr, candidatePtr uintptr) {
		onICECandidateCallbackMu.RLock()
		cb, ok := onICECandidateCallbacks[ctx]
		onICECandidateCallbackMu.RUnlock()

		if ok && cb != nil && candidatePtr != 0 {
			// Read ICECandidate fields from memory
			// struct layout: const char* candidate; const char* sdp_mid; int sdp_mline_index;
			candidateStrPtr := *(*uintptr)(unsafe.Pointer(candidatePtr))
			sdpMidPtr := *(*uintptr)(unsafe.Pointer(candidatePtr + unsafe.Sizeof(uintptr(0))))
			sdpMLineIndex := *(*int32)(unsafe.Pointer(candidatePtr + 2*unsafe.Sizeof(uintptr(0))))

			candidate := GoString(unsafe.Pointer(candidateStrPtr))
			sdpMid := GoString(unsafe.Pointer(sdpMidPtr))
			safeCallback(func() {
				cb(candidate, sdpMid, int(sdpMLineIndex))
			})
		}
	})

	onICECandidateInitialized = true
}

// PeerConnectionSetOnICECandidate sets the on ICE candidate callback.
func PeerConnectionSetOnICECandidate(pc uintptr, cb OnICECandidateCallback) {
	if !libLoaded.Load() || shimPeerConnectionSetOnICECandidate == nil {
		return
	}

	initOnICECandidateCallback()

	onICECandidateCallbackMu.Lock()
	onICECandidateCallbacks[pc] = cb
	onICECandidateCallbackMu.Unlock()

	shimPeerConnectionSetOnICECandidate(pc, onICECandidateCallbackPtr, pc)
}

// UnregisterOnICECandidateCallback removes the on ICE candidate callback for a PC.
func UnregisterOnICECandidateCallback(pc uintptr) {
	onICECandidateCallbackMu.Lock()
	delete(onICECandidateCallbacks, pc)
	onICECandidateCallbackMu.Unlock()
}

// ============================================================================
// OnDataChannel Callback
// ============================================================================

// OnDataChannelCallback is called when a data channel is received.
type OnDataChannelCallback func(dc uintptr)

var (
	onDataChannelCallbackMu  sync.RWMutex
	onDataChannelCallbacks   = make(map[uintptr]OnDataChannelCallback)
	onDataChannelCallbackPtr uintptr
	onDataChannelInitialized bool
)

func initOnDataChannelCallback() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()

	if onDataChannelInitialized {
		return
	}

	onDataChannelCallbackPtr = purego.NewCallback(func(ctx uintptr, dc uintptr) {
		onDataChannelCallbackMu.RLock()
		cb, ok := onDataChannelCallbacks[ctx]
		onDataChannelCallbackMu.RUnlock()

		if ok && cb != nil {
			safeCallback(func() {
				cb(dc)
			})
		}
	})

	onDataChannelInitialized = true
}

// PeerConnectionSetOnDataChannel sets the on data channel callback.
func PeerConnectionSetOnDataChannel(pc uintptr, cb OnDataChannelCallback) {
	if !libLoaded.Load() || shimPeerConnectionSetOnDataChannel == nil {
		return
	}

	initOnDataChannelCallback()

	onDataChannelCallbackMu.Lock()
	onDataChannelCallbacks[pc] = cb
	onDataChannelCallbackMu.Unlock()

	shimPeerConnectionSetOnDataChannel(pc, onDataChannelCallbackPtr, pc)
}

// UnregisterOnDataChannelCallback removes the on data channel callback for a PC.
func UnregisterOnDataChannelCallback(pc uintptr) {
	onDataChannelCallbackMu.Lock()
	delete(onDataChannelCallbacks, pc)
	onDataChannelCallbackMu.Unlock()
}

// ============================================================================
// OnSignalingStateChange Callback
// ============================================================================

// SignalingStateCallback is called when the signaling state changes.
type SignalingStateCallback func(state int)

var (
	signalingStateCallbackMu  sync.RWMutex
	signalingStateCallbacks   = make(map[uintptr]SignalingStateCallback)
	signalingStateCallbackPtr uintptr
	signalingStateInitialized bool
)

func initSignalingStateCallback() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()

	if signalingStateInitialized {
		return
	}

	// NOTE: C uses 'int' (32-bit) for state, so we must use int32 to match
	signalingStateCallbackPtr = purego.NewCallback(func(ctx uintptr, state int32) {
		signalingStateCallbackMu.RLock()
		cb, ok := signalingStateCallbacks[ctx]
		signalingStateCallbackMu.RUnlock()

		if ok && cb != nil {
			safeCallback(func() {
				cb(int(state))
			})
		}
	})

	signalingStateInitialized = true
}

// PeerConnectionSetOnSignalingStateChange sets the signaling state change callback.
func PeerConnectionSetOnSignalingStateChange(pc uintptr, cb SignalingStateCallback) {
	if !libLoaded.Load() || shimPeerConnectionSetOnSignalingStateChange == nil {
		return
	}

	initSignalingStateCallback()

	signalingStateCallbackMu.Lock()
	signalingStateCallbacks[pc] = cb
	signalingStateCallbackMu.Unlock()

	shimPeerConnectionSetOnSignalingStateChange(pc, signalingStateCallbackPtr, pc)
}

// UnregisterSignalingStateCallback removes the signaling state callback for a PC.
func UnregisterSignalingStateCallback(pc uintptr) {
	signalingStateCallbackMu.Lock()
	delete(signalingStateCallbacks, pc)
	signalingStateCallbackMu.Unlock()
}

// ============================================================================
// OnICEConnectionStateChange Callback
// ============================================================================

// ICEConnectionStateCallback is called when the ICE connection state changes.
type ICEConnectionStateCallback func(state int)

var (
	iceConnectionStateCallbackMu  sync.RWMutex
	iceConnectionStateCallbacks   = make(map[uintptr]ICEConnectionStateCallback)
	iceConnectionStateCallbackPtr uintptr
	iceConnectionStateInitialized bool
)

func initICEConnectionStateCallback() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()

	if iceConnectionStateInitialized {
		return
	}

	// NOTE: C uses 'int' (32-bit) for state, so we must use int32 to match
	iceConnectionStateCallbackPtr = purego.NewCallback(func(ctx uintptr, state int32) {
		iceConnectionStateCallbackMu.RLock()
		cb, ok := iceConnectionStateCallbacks[ctx]
		iceConnectionStateCallbackMu.RUnlock()

		if ok && cb != nil {
			safeCallback(func() {
				cb(int(state))
			})
		}
	})

	iceConnectionStateInitialized = true
}

// PeerConnectionSetOnICEConnectionStateChange sets the ICE connection state change callback.
func PeerConnectionSetOnICEConnectionStateChange(pc uintptr, cb ICEConnectionStateCallback) {
	if !libLoaded.Load() || shimPeerConnectionSetOnICEConnectionStateChange == nil {
		return
	}

	initICEConnectionStateCallback()

	iceConnectionStateCallbackMu.Lock()
	iceConnectionStateCallbacks[pc] = cb
	iceConnectionStateCallbackMu.Unlock()

	shimPeerConnectionSetOnICEConnectionStateChange(pc, iceConnectionStateCallbackPtr, pc)
}

// UnregisterICEConnectionStateCallback removes the ICE connection state callback for a PC.
func UnregisterICEConnectionStateCallback(pc uintptr) {
	iceConnectionStateCallbackMu.Lock()
	delete(iceConnectionStateCallbacks, pc)
	iceConnectionStateCallbackMu.Unlock()
}

// ============================================================================
// OnICEGatheringStateChange Callback
// ============================================================================

// ICEGatheringStateCallback is called when the ICE gathering state changes.
type ICEGatheringStateCallback func(state int)

var (
	iceGatheringStateCallbackMu  sync.RWMutex
	iceGatheringStateCallbacks   = make(map[uintptr]ICEGatheringStateCallback)
	iceGatheringStateCallbackPtr uintptr
	iceGatheringStateInitialized bool
)

func initICEGatheringStateCallback() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()

	if iceGatheringStateInitialized {
		return
	}

	// NOTE: C uses 'int' (32-bit) for state, so we must use int32 to match
	iceGatheringStateCallbackPtr = purego.NewCallback(func(ctx uintptr, state int32) {
		iceGatheringStateCallbackMu.RLock()
		cb, ok := iceGatheringStateCallbacks[ctx]
		iceGatheringStateCallbackMu.RUnlock()

		if ok && cb != nil {
			safeCallback(func() {
				cb(int(state))
			})
		}
	})

	iceGatheringStateInitialized = true
}

// PeerConnectionSetOnICEGatheringStateChange sets the ICE gathering state change callback.
func PeerConnectionSetOnICEGatheringStateChange(pc uintptr, cb ICEGatheringStateCallback) {
	if !libLoaded.Load() || shimPeerConnectionSetOnICEGatheringStateChange == nil {
		return
	}

	initICEGatheringStateCallback()

	iceGatheringStateCallbackMu.Lock()
	iceGatheringStateCallbacks[pc] = cb
	iceGatheringStateCallbackMu.Unlock()

	shimPeerConnectionSetOnICEGatheringStateChange(pc, iceGatheringStateCallbackPtr, pc)
}

// UnregisterICEGatheringStateCallback removes the ICE gathering state callback for a PC.
func UnregisterICEGatheringStateCallback(pc uintptr) {
	iceGatheringStateCallbackMu.Lock()
	delete(iceGatheringStateCallbacks, pc)
	iceGatheringStateCallbackMu.Unlock()
}

// ============================================================================
// OnNegotiationNeeded Callback
// ============================================================================

// NegotiationNeededCallback is called when negotiation is needed.
type NegotiationNeededCallback func()

var (
	negotiationNeededCallbackMu  sync.RWMutex
	negotiationNeededCallbacks   = make(map[uintptr]NegotiationNeededCallback)
	negotiationNeededCallbackPtr uintptr
	negotiationNeededInitialized bool
)

func initNegotiationNeededCallback() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()

	if negotiationNeededInitialized {
		return
	}

	negotiationNeededCallbackPtr = purego.NewCallback(func(ctx uintptr) {
		negotiationNeededCallbackMu.RLock()
		cb, ok := negotiationNeededCallbacks[ctx]
		negotiationNeededCallbackMu.RUnlock()

		if ok && cb != nil {
			safeCallback(func() {
				cb()
			})
		}
	})

	negotiationNeededInitialized = true
}

// PeerConnectionSetOnNegotiationNeeded sets the negotiation needed callback.
func PeerConnectionSetOnNegotiationNeeded(pc uintptr, cb NegotiationNeededCallback) {
	if !libLoaded.Load() || shimPeerConnectionSetOnNegotiationNeeded == nil {
		return
	}

	initNegotiationNeededCallback()

	negotiationNeededCallbackMu.Lock()
	negotiationNeededCallbacks[pc] = cb
	negotiationNeededCallbackMu.Unlock()

	shimPeerConnectionSetOnNegotiationNeeded(pc, negotiationNeededCallbackPtr, pc)
}

// UnregisterNegotiationNeededCallback removes the negotiation needed callback for a PC.
func UnregisterNegotiationNeededCallback(pc uintptr) {
	negotiationNeededCallbackMu.Lock()
	delete(negotiationNeededCallbacks, pc)
	negotiationNeededCallbackMu.Unlock()
}

// ============================================================================
// RTPSender Scalability Mode
// ============================================================================

// RTPSenderSetScalabilityMode sets the scalability mode for a sender.
func RTPSenderSetScalabilityMode(sender uintptr, mode string) error {
	if !libLoaded.Load() || shimRTPSenderSetScalabilityMode == nil {
		return ErrLibraryNotLoaded
	}

	modeCStr := CString(mode)
	result := shimRTPSenderSetScalabilityMode(sender, ByteSlicePtr(modeCStr))
	runtime.KeepAlive(modeCStr)
	return ShimError(result)
}

// RTPSenderGetScalabilityMode gets the current scalability mode for a sender.
func RTPSenderGetScalabilityMode(sender uintptr) (string, error) {
	if !libLoaded.Load() || shimRTPSenderGetScalabilityMode == nil {
		return "", ErrLibraryNotLoaded
	}

	modeBuf := make([]byte, 64)
	result := shimRTPSenderGetScalabilityMode(sender, ByteSlicePtr(modeBuf), int32(len(modeBuf)))
	runtime.KeepAlive(modeBuf)
	if err := ShimError(result); err != nil {
		return "", err
	}

	// Find null terminator
	for i, b := range modeBuf {
		if b == 0 {
			return string(modeBuf[:i]), nil
		}
	}
	return string(modeBuf), nil
}

// ============================================================================
// Codec Capability API
// ============================================================================

// GetSupportedVideoCodecs returns a list of supported video codecs.
func GetSupportedVideoCodecs() ([]CodecCapability, error) {
	if !libLoaded.Load() || shimGetSupportedVideoCodecs == nil {
		return nil, ErrLibraryNotLoaded
	}

	codecs := make([]CodecCapability, 16)
	var count int32
	result := shimGetSupportedVideoCodecs(
		uintptr(unsafe.Pointer(&codecs[0])),
		int32(len(codecs)),
		Int32Ptr(&count),
	)
	runtime.KeepAlive(&count)
	runtime.KeepAlive(&codecs)
	if err := ShimError(result); err != nil {
		return nil, err
	}

	// Workaround for purego FFI output pointer issue:
	// The count value isn't propagating through purego correctly,
	// so we count codecs by checking which ones have non-empty mime types.
	actualCount := 0
	for i := range codecs {
		if codecs[i].MimeType[0] != 0 {
			actualCount++
		}
	}

	return codecs[:actualCount], nil
}

// GetSupportedAudioCodecs returns a list of supported audio codecs.
func GetSupportedAudioCodecs() ([]CodecCapability, error) {
	if !libLoaded.Load() || shimGetSupportedAudioCodecs == nil {
		return nil, ErrLibraryNotLoaded
	}

	codecs := make([]CodecCapability, 16)
	var count int32
	result := shimGetSupportedAudioCodecs(
		uintptr(unsafe.Pointer(&codecs[0])),
		int32(len(codecs)),
		Int32Ptr(&count),
	)
	runtime.KeepAlive(&count)
	runtime.KeepAlive(&codecs)
	if err := ShimError(result); err != nil {
		return nil, err
	}

	// Workaround for purego FFI output pointer issue
	actualCount := 0
	for i := range codecs {
		if codecs[i].MimeType[0] != 0 {
			actualCount++
		}
	}

	return codecs[:actualCount], nil
}

// IsCodecSupported checks if a specific codec is supported.
func IsCodecSupported(mimeType string) bool {
	if !libLoaded.Load() || shimIsCodecSupported == nil {
		return false
	}

	cstr := CString(mimeType)
	result := shimIsCodecSupported(ByteSlicePtr(cstr)) != 0
	runtime.KeepAlive(cstr)
	return result
}

// ============================================================================
// Bandwidth Estimation API
// ============================================================================

// BandwidthEstimateCallback is called when the bandwidth estimate changes.
type BandwidthEstimateCallback func(estimate *BandwidthEstimate)

var (
	bweCallbackMu  sync.RWMutex
	bweCallbacks   = make(map[uintptr]BandwidthEstimateCallback)
	bweCallbackPtr uintptr
	bweInitialized bool
)

func initBWECallback() {
	callbackInitMu.Lock()
	defer callbackInitMu.Unlock()

	if bweInitialized {
		return
	}

	bweCallbackPtr = purego.NewCallback(func(ctx uintptr, estimatePtr uintptr) {
		bweCallbackMu.RLock()
		cb, ok := bweCallbacks[ctx]
		bweCallbackMu.RUnlock()

		if ok && cb != nil && estimatePtr != 0 {
			estimate := (*BandwidthEstimate)(unsafe.Pointer(estimatePtr))
			safeCallback(func() {
				cb(estimate)
			})
		}
	})

	bweInitialized = true
}

// PeerConnectionSetOnBandwidthEstimate sets the bandwidth estimate callback.
func PeerConnectionSetOnBandwidthEstimate(pc uintptr, cb BandwidthEstimateCallback) {
	if !libLoaded.Load() || shimPeerConnectionSetOnBandwidthEstimate == nil {
		return
	}

	initBWECallback()

	bweCallbackMu.Lock()
	bweCallbacks[pc] = cb
	bweCallbackMu.Unlock()

	shimPeerConnectionSetOnBandwidthEstimate(pc, bweCallbackPtr, pc)
}

// UnregisterBandwidthEstimateCallback removes the bandwidth estimate callback for a PC.
func UnregisterBandwidthEstimateCallback(pc uintptr) {
	bweCallbackMu.Lock()
	delete(bweCallbacks, pc)
	bweCallbackMu.Unlock()
}

// PeerConnectionGetBandwidthEstimate gets the current bandwidth estimate.
func PeerConnectionGetBandwidthEstimate(pc uintptr) (*BandwidthEstimate, error) {
	if !libLoaded.Load() || shimPeerConnectionGetBandwidthEstimate == nil {
		return nil, ErrLibraryNotLoaded
	}

	var estimate BandwidthEstimate
	result := shimPeerConnectionGetBandwidthEstimate(pc, uintptr(unsafe.Pointer(&estimate)))
	if err := ShimError(result); err != nil {
		return nil, err
	}

	return &estimate, nil
}

// ============================================================================
// Jitter Buffer Control API
//
// NOTE: libwebrtc provides limited jitter buffer control via RtpReceiverInterface.
// Only SetJitterBufferMinimumDelay() is available - this sets a floor for the
// adaptive jitter buffer algorithm.
//
// For full jitter buffer stats, use PeerConnectionGetStats() or RTPReceiverGetStats()
// which provides RTCStats with jitter buffer fields (JitterBufferDelayMs,
// JitterBufferTargetDelayMs, JitterBufferMinimumDelayMs, JitterBufferEmittedCount).
// ============================================================================

// RTPReceiverSetJitterBufferMinDelay sets the minimum jitter buffer delay.
// This sets a floor for libwebrtc's adaptive jitter buffer. The actual delay
// may be higher based on network conditions, but won't go below this value.
// Pass 0 to let libwebrtc's adaptive algorithm decide without a minimum floor.
func RTPReceiverSetJitterBufferMinDelay(receiver uintptr, minDelayMs int) error {
	if !libLoaded.Load() || shimRTPReceiverSetJitterBufferMinDelay == nil {
		return ErrLibraryNotLoaded
	}

	result := shimRTPReceiverSetJitterBufferMinDelay(receiver, int32(minDelayMs))
	return ShimError(result)
}
