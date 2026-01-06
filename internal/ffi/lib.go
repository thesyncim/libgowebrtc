// Package ffi provides purego-based FFI bindings to the libwebrtc shim library.
package ffi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/ebitengine/purego"
)

var (
	// ErrLibraryNotLoaded is returned when the shim library hasn't been loaded.
	ErrLibraryNotLoaded = errors.New("libwebrtc_shim library not loaded")

	// ErrLibraryNotFound is returned when the shim library cannot be found.
	ErrLibraryNotFound = errors.New("libwebrtc_shim library not found")
)

// Error codes from shim
const (
	ShimOK              = 0
	ShimErrInvalidParam = -1
	ShimErrInitFailed   = -2
	ShimErrEncodeFailed = -3
	ShimErrDecodeFailed = -4
	ShimErrOutOfMemory  = -5
	ShimErrNotSupported = -6
	ShimErrNeedMoreData = -7
)

// CodecType matches ShimCodecType in shim.h
type CodecType int

const (
	CodecH264 CodecType = 0
	CodecVP8  CodecType = 1
	CodecVP9  CodecType = 2
	CodecAV1  CodecType = 3
	CodecOpus CodecType = 10
)

var (
	libHandle uintptr
	libLoaded bool
	libMu     sync.Mutex
)

// Function pointers - populated by LoadLibrary
var (
	// Video Encoder
	shimVideoEncoderCreate          func(codec int, configPtr uintptr) uintptr
	shimVideoEncoderEncode          func(encoder uintptr, yPlane, uPlane, vPlane uintptr, yStride, uStride, vStride int, timestamp uint32, forceKeyframe int, outData, outSize, outIsKeyframe uintptr) int
	shimVideoEncoderSetBitrate      func(encoder uintptr, bitrate uint32) int
	shimVideoEncoderSetFramerate    func(encoder uintptr, framerate float32) int
	shimVideoEncoderRequestKeyframe func(encoder uintptr) int
	shimVideoEncoderDestroy         func(encoder uintptr)

	// Video Decoder
	shimVideoDecoderCreate  func(codec int) uintptr
	shimVideoDecoderDecode  func(decoder uintptr, data uintptr, size int, timestamp uint32, isKeyframe int, outY, outU, outV, outW, outH, outYStride, outUStride, outVStride uintptr) int
	shimVideoDecoderDestroy func(decoder uintptr)

	// Audio Encoder
	shimAudioEncoderCreate     func(configPtr uintptr) uintptr
	shimAudioEncoderEncode     func(encoder uintptr, samples uintptr, numSamples int, outData, outSize uintptr) int
	shimAudioEncoderSetBitrate func(encoder uintptr, bitrate uint32) int
	shimAudioEncoderDestroy    func(encoder uintptr)

	// Audio Decoder
	shimAudioDecoderCreate  func(sampleRate, channels int) uintptr
	shimAudioDecoderDecode  func(decoder uintptr, data uintptr, size int, outSamples, outNumSamples uintptr) int
	shimAudioDecoderDestroy func(decoder uintptr)

	// Packetizer
	shimPacketizerCreate    func(configPtr uintptr) uintptr
	shimPacketizerPacketize func(packetizer uintptr, data uintptr, size int, timestamp uint32, isKeyframe int, dst uintptr, offsets uintptr, sizes uintptr, maxPackets int, outCount uintptr) int
	shimPacketizerSeqNum    func(packetizer uintptr) uint16
	shimPacketizerDestroy   func(packetizer uintptr)

	// Depacketizer
	shimDepacketizerCreate  func(codec int) uintptr
	shimDepacketizerPush    func(depacketizer uintptr, data uintptr, size int) int
	shimDepacketizerPop     func(depacketizer uintptr, outData, outSize, outTimestamp, outIsKeyframe uintptr) int
	shimDepacketizerDestroy func(depacketizer uintptr)

	// Memory
	shimFreeBuffer  func(buffer uintptr)
	shimFreePackets func(packets, sizes uintptr, count int)

	// Version
	shimLibwebrtcVersion func() uintptr
	shimVersion          func() uintptr
)

// PeerConnection FFI function pointers - populated by registerFunctions
var (
	shimPeerConnectionCreate                     func(configPtr uintptr) uintptr
	shimPeerConnectionDestroy                    func(pc uintptr)
	shimPeerConnectionSetOnICECandidate          func(pc uintptr, callback uintptr, ctx uintptr)
	shimPeerConnectionSetOnConnectionStateChange func(pc uintptr, callback uintptr, ctx uintptr)
	shimPeerConnectionSetOnTrack                 func(pc uintptr, callback uintptr, ctx uintptr)
	shimPeerConnectionSetOnDataChannel           func(pc uintptr, callback uintptr, ctx uintptr)
	shimPeerConnectionCreateOffer                func(pc uintptr, sdpOut uintptr, sdpOutSize int, outSdpLen uintptr) int
	shimPeerConnectionCreateAnswer               func(pc uintptr, sdpOut uintptr, sdpOutSize int, outSdpLen uintptr) int
	shimPeerConnectionSetLocalDescription        func(pc uintptr, sdpType int, sdp uintptr) int
	shimPeerConnectionSetRemoteDescription       func(pc uintptr, sdpType int, sdp uintptr) int
	shimPeerConnectionAddICECandidate            func(pc uintptr, candidate, sdpMid uintptr, sdpMLineIndex int) int
	shimPeerConnectionSignalingState             func(pc uintptr) int
	shimPeerConnectionICEConnectionState         func(pc uintptr) int
	shimPeerConnectionICEGatheringState          func(pc uintptr) int
	shimPeerConnectionConnectionState            func(pc uintptr) int
	shimPeerConnectionAddTrack                   func(pc uintptr, codec int, trackID, streamID uintptr) uintptr
	shimPeerConnectionRemoveTrack                func(pc uintptr, sender uintptr) int
	shimPeerConnectionCreateDataChannel          func(pc uintptr, label uintptr, ordered, maxRetransmits int, protocol uintptr) uintptr
	shimPeerConnectionClose                      func(pc uintptr)

	shimRTPSenderSetBitrate   func(sender uintptr, bitrate uint32) int
	shimRTPSenderReplaceTrack func(sender uintptr, track uintptr) int
	shimRTPSenderDestroy      func(sender uintptr)

	shimDataChannelSetOnMessage func(dc uintptr, callback uintptr, ctx uintptr)
	shimDataChannelSetOnOpen    func(dc uintptr, callback uintptr, ctx uintptr)
	shimDataChannelSetOnClose   func(dc uintptr, callback uintptr, ctx uintptr)
	shimDataChannelSend         func(dc uintptr, data uintptr, size int, isBinary int) int
	shimDataChannelLabel        func(dc uintptr) uintptr
	shimDataChannelReadyState   func(dc uintptr) int
	shimDataChannelClose        func(dc uintptr)
	shimDataChannelDestroy      func(dc uintptr)

	// Video Track Source (for frame injection)
	shimVideoTrackSourceCreate                func(pc uintptr, width, height int) uintptr
	shimVideoTrackSourcePushFrame             func(source uintptr, yPlane, uPlane, vPlane uintptr, yStride, uStride, vStride int, timestampUs int64) int
	shimPeerConnectionAddVideoTrackFromSource func(pc, source uintptr, trackID, streamID uintptr) uintptr
	shimVideoTrackSourceDestroy               func(source uintptr)

	// Audio Track Source (for frame injection)
	shimAudioTrackSourceCreate                func(pc uintptr, sampleRate, channels int) uintptr
	shimAudioTrackSourcePushFrame             func(source uintptr, samples uintptr, numSamples int, timestampUs int64) int
	shimPeerConnectionAddAudioTrackFromSource func(pc, source uintptr, trackID, streamID uintptr) uintptr
	shimAudioTrackSourceDestroy               func(source uintptr)

	// Remote Track Sink (for receiving frames from remote tracks)
	shimTrackSetVideoSink    func(track uintptr, callback uintptr, ctx uintptr) int
	shimTrackSetAudioSink    func(track uintptr, callback uintptr, ctx uintptr) int
	shimTrackRemoveVideoSink func(track uintptr)
	shimTrackRemoveAudioSink func(track uintptr)
	shimTrackKind            func(track uintptr) uintptr
	shimTrackID              func(track uintptr) uintptr

	// RTPSender Parameters
	shimRTPSenderGetParameters     func(sender uintptr, outParams uintptr, encodings uintptr, maxEncodings int) int
	shimRTPSenderSetParameters     func(sender uintptr, params uintptr) int
	shimRTPSenderGetTrack          func(sender uintptr) uintptr
	shimRTPSenderGetStats          func(sender uintptr, outStats uintptr) int
	shimRTPSenderSetOnRTCPFeedback func(sender uintptr, callback uintptr, ctx uintptr)
	shimRTPSenderSetLayerActive    func(sender uintptr, rid uintptr, active int) int
	shimRTPSenderSetLayerBitrate   func(sender uintptr, rid uintptr, maxBitrate uint32) int
	shimRTPSenderGetActiveLayers   func(sender uintptr, outSpatial uintptr, outTemporal uintptr) int

	// RTPReceiver
	shimRTPReceiverGetTrack        func(receiver uintptr) uintptr
	shimRTPReceiverGetStats        func(receiver uintptr, outStats uintptr) int
	shimRTPReceiverRequestKeyframe func(receiver uintptr) int

	// RTPTransceiver
	shimTransceiverGetDirection        func(transceiver uintptr) int
	shimTransceiverSetDirection        func(transceiver uintptr, direction int) int
	shimTransceiverGetCurrentDirection func(transceiver uintptr) int
	shimTransceiverStop                func(transceiver uintptr) int
	shimTransceiverMid                 func(transceiver uintptr) uintptr
	shimTransceiverGetSender           func(transceiver uintptr) uintptr
	shimTransceiverGetReceiver         func(transceiver uintptr) uintptr

	// PeerConnection Extended
	shimPeerConnectionAddTransceiver                func(pc uintptr, kind int, direction int) uintptr
	shimPeerConnectionGetSenders                    func(pc uintptr, senders uintptr, maxSenders int, outCount uintptr) int
	shimPeerConnectionGetReceivers                  func(pc uintptr, receivers uintptr, maxReceivers int, outCount uintptr) int
	shimPeerConnectionGetTransceivers               func(pc uintptr, transceivers uintptr, maxTransceivers int, outCount uintptr) int
	shimPeerConnectionRestartICE                    func(pc uintptr) int
	shimPeerConnectionGetStats                      func(pc uintptr, outStats uintptr) int
	shimPeerConnectionSetOnSignalingStateChange     func(pc uintptr, callback uintptr, ctx uintptr)
	shimPeerConnectionSetOnICEConnectionStateChange func(pc uintptr, callback uintptr, ctx uintptr)
	shimPeerConnectionSetOnICEGatheringStateChange  func(pc uintptr, callback uintptr, ctx uintptr)
	shimPeerConnectionSetOnNegotiationNeeded        func(pc uintptr, callback uintptr, ctx uintptr)

	// RTPSender Scalability Mode
	shimRTPSenderSetScalabilityMode func(sender uintptr, mode uintptr) int
	shimRTPSenderGetScalabilityMode func(sender uintptr, modeOut uintptr, modeOutSize int) int

	// Codec Capabilities
	shimGetSupportedVideoCodecs func(codecs uintptr, maxCodecs int, outCount uintptr) int
	shimGetSupportedAudioCodecs func(codecs uintptr, maxCodecs int, outCount uintptr) int
	shimIsCodecSupported        func(mimeType uintptr) int

	// Bandwidth Estimation
	shimPeerConnectionSetOnBandwidthEstimate func(pc uintptr, callback uintptr, ctx uintptr)
	shimPeerConnectionGetBandwidthEstimate   func(pc uintptr, outEstimate uintptr) int
)

// LoadLibrary loads the libwebrtc_shim shared library.
// It searches in the following locations:
// 1. Path specified by LIBWEBRTC_SHIM_PATH environment variable
// 2. ./lib/{os}_{arch}/
// 3. System library paths
func LoadLibrary() error {
	libMu.Lock()
	defer libMu.Unlock()

	if libLoaded {
		return nil
	}

	libPath, err := findLibrary()
	if err != nil {
		return err
	}

	handle, err := purego.Dlopen(libPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", libPath, err)
	}

	libHandle = handle
	if err := registerFunctions(); err != nil {
		purego.Dlclose(handle)
		return err
	}

	libLoaded = true
	return nil
}

// MustLoadLibrary loads the library and panics on failure.
func MustLoadLibrary() {
	if err := LoadLibrary(); err != nil {
		panic(fmt.Sprintf("libgowebrtc: %v", err))
	}
}

// IsLoaded returns true if the shim library is loaded.
func IsLoaded() bool {
	libMu.Lock()
	defer libMu.Unlock()
	return libLoaded
}

// Close unloads the shim library.
func Close() error {
	libMu.Lock()
	defer libMu.Unlock()

	if !libLoaded {
		return nil
	}

	if err := purego.Dlclose(libHandle); err != nil {
		return err
	}

	libLoaded = false
	libHandle = 0
	return nil
}

func findLibrary() (string, error) {
	// Check environment variable first
	if path := os.Getenv("LIBWEBRTC_SHIM_PATH"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	libName := getLibraryName()
	platformDir := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)

	// Build search paths
	var searchPaths []string

	// Check relative to executable
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		searchPaths = append(searchPaths, filepath.Join(execDir, "lib", platformDir, libName))
	}

	// Check working directory
	if wd, err := os.Getwd(); err == nil {
		searchPaths = append(searchPaths,
			filepath.Join(wd, "lib", platformDir, libName),
			filepath.Join(wd, "..", "lib", platformDir, libName),
			filepath.Join(wd, "..", "..", "lib", platformDir, libName),
		)
	}

	// Check relative to this source file (for development/testing)
	// This finds lib/ relative to the Go module root
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		// thisFile is .../internal/ffi/lib.go, go up to module root
		moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
		searchPaths = append(searchPaths, filepath.Join(moduleRoot, "lib", platformDir, libName))
	}

	// Standard paths
	searchPaths = append(searchPaths,
		filepath.Join(".", "lib", platformDir, libName),
		filepath.Join("..", "lib", platformDir, libName),
	)

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath, nil
		}
	}

	// Try system library paths (let dlopen find it)
	return libName, nil
}

func getLibraryName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libwebrtc_shim.dylib"
	case "windows":
		return "webrtc_shim.dll"
	default:
		return "libwebrtc_shim.so"
	}
}

func registerFunctions() error {
	var err error

	// Video Encoder
	purego.RegisterLibFunc(&shimVideoEncoderCreate, libHandle, "shim_video_encoder_create")
	purego.RegisterLibFunc(&shimVideoEncoderEncode, libHandle, "shim_video_encoder_encode")
	purego.RegisterLibFunc(&shimVideoEncoderSetBitrate, libHandle, "shim_video_encoder_set_bitrate")
	purego.RegisterLibFunc(&shimVideoEncoderSetFramerate, libHandle, "shim_video_encoder_set_framerate")
	purego.RegisterLibFunc(&shimVideoEncoderRequestKeyframe, libHandle, "shim_video_encoder_request_keyframe")
	purego.RegisterLibFunc(&shimVideoEncoderDestroy, libHandle, "shim_video_encoder_destroy")

	// Video Decoder
	purego.RegisterLibFunc(&shimVideoDecoderCreate, libHandle, "shim_video_decoder_create")
	purego.RegisterLibFunc(&shimVideoDecoderDecode, libHandle, "shim_video_decoder_decode")
	purego.RegisterLibFunc(&shimVideoDecoderDestroy, libHandle, "shim_video_decoder_destroy")

	// Audio Encoder
	purego.RegisterLibFunc(&shimAudioEncoderCreate, libHandle, "shim_audio_encoder_create")
	purego.RegisterLibFunc(&shimAudioEncoderEncode, libHandle, "shim_audio_encoder_encode")
	purego.RegisterLibFunc(&shimAudioEncoderSetBitrate, libHandle, "shim_audio_encoder_set_bitrate")
	purego.RegisterLibFunc(&shimAudioEncoderDestroy, libHandle, "shim_audio_encoder_destroy")

	// Audio Decoder
	purego.RegisterLibFunc(&shimAudioDecoderCreate, libHandle, "shim_audio_decoder_create")
	purego.RegisterLibFunc(&shimAudioDecoderDecode, libHandle, "shim_audio_decoder_decode")
	purego.RegisterLibFunc(&shimAudioDecoderDestroy, libHandle, "shim_audio_decoder_destroy")

	// Packetizer
	purego.RegisterLibFunc(&shimPacketizerCreate, libHandle, "shim_packetizer_create")
	purego.RegisterLibFunc(&shimPacketizerPacketize, libHandle, "shim_packetizer_packetize")
	purego.RegisterLibFunc(&shimPacketizerSeqNum, libHandle, "shim_packetizer_sequence_number")
	purego.RegisterLibFunc(&shimPacketizerDestroy, libHandle, "shim_packetizer_destroy")

	// Depacketizer
	purego.RegisterLibFunc(&shimDepacketizerCreate, libHandle, "shim_depacketizer_create")
	purego.RegisterLibFunc(&shimDepacketizerPush, libHandle, "shim_depacketizer_push")
	purego.RegisterLibFunc(&shimDepacketizerPop, libHandle, "shim_depacketizer_pop")
	purego.RegisterLibFunc(&shimDepacketizerDestroy, libHandle, "shim_depacketizer_destroy")

	// Memory
	purego.RegisterLibFunc(&shimFreeBuffer, libHandle, "shim_free_buffer")
	purego.RegisterLibFunc(&shimFreePackets, libHandle, "shim_free_packets")

	// Version
	purego.RegisterLibFunc(&shimLibwebrtcVersion, libHandle, "shim_libwebrtc_version")
	purego.RegisterLibFunc(&shimVersion, libHandle, "shim_version")

	// PeerConnection
	purego.RegisterLibFunc(&shimPeerConnectionCreate, libHandle, "shim_peer_connection_create")
	purego.RegisterLibFunc(&shimPeerConnectionDestroy, libHandle, "shim_peer_connection_destroy")
	purego.RegisterLibFunc(&shimPeerConnectionSetOnICECandidate, libHandle, "shim_peer_connection_set_on_ice_candidate")
	purego.RegisterLibFunc(&shimPeerConnectionSetOnConnectionStateChange, libHandle, "shim_peer_connection_set_on_connection_state_change")
	purego.RegisterLibFunc(&shimPeerConnectionSetOnTrack, libHandle, "shim_peer_connection_set_on_track")
	purego.RegisterLibFunc(&shimPeerConnectionSetOnDataChannel, libHandle, "shim_peer_connection_set_on_data_channel")
	purego.RegisterLibFunc(&shimPeerConnectionCreateOffer, libHandle, "shim_peer_connection_create_offer")
	purego.RegisterLibFunc(&shimPeerConnectionCreateAnswer, libHandle, "shim_peer_connection_create_answer")
	purego.RegisterLibFunc(&shimPeerConnectionSetLocalDescription, libHandle, "shim_peer_connection_set_local_description")
	purego.RegisterLibFunc(&shimPeerConnectionSetRemoteDescription, libHandle, "shim_peer_connection_set_remote_description")
	purego.RegisterLibFunc(&shimPeerConnectionAddICECandidate, libHandle, "shim_peer_connection_add_ice_candidate")
	purego.RegisterLibFunc(&shimPeerConnectionSignalingState, libHandle, "shim_peer_connection_signaling_state")
	purego.RegisterLibFunc(&shimPeerConnectionICEConnectionState, libHandle, "shim_peer_connection_ice_connection_state")
	purego.RegisterLibFunc(&shimPeerConnectionICEGatheringState, libHandle, "shim_peer_connection_ice_gathering_state")
	purego.RegisterLibFunc(&shimPeerConnectionConnectionState, libHandle, "shim_peer_connection_connection_state")
	purego.RegisterLibFunc(&shimPeerConnectionAddTrack, libHandle, "shim_peer_connection_add_track")
	purego.RegisterLibFunc(&shimPeerConnectionRemoveTrack, libHandle, "shim_peer_connection_remove_track")
	purego.RegisterLibFunc(&shimPeerConnectionCreateDataChannel, libHandle, "shim_peer_connection_create_data_channel")
	purego.RegisterLibFunc(&shimPeerConnectionClose, libHandle, "shim_peer_connection_close")

	// RTPSender
	purego.RegisterLibFunc(&shimRTPSenderSetBitrate, libHandle, "shim_rtp_sender_set_bitrate")
	purego.RegisterLibFunc(&shimRTPSenderReplaceTrack, libHandle, "shim_rtp_sender_replace_track")
	purego.RegisterLibFunc(&shimRTPSenderDestroy, libHandle, "shim_rtp_sender_destroy")

	// DataChannel
	purego.RegisterLibFunc(&shimDataChannelSetOnMessage, libHandle, "shim_data_channel_set_on_message")
	purego.RegisterLibFunc(&shimDataChannelSetOnOpen, libHandle, "shim_data_channel_set_on_open")
	purego.RegisterLibFunc(&shimDataChannelSetOnClose, libHandle, "shim_data_channel_set_on_close")
	purego.RegisterLibFunc(&shimDataChannelSend, libHandle, "shim_data_channel_send")
	purego.RegisterLibFunc(&shimDataChannelLabel, libHandle, "shim_data_channel_label")
	purego.RegisterLibFunc(&shimDataChannelReadyState, libHandle, "shim_data_channel_ready_state")
	purego.RegisterLibFunc(&shimDataChannelClose, libHandle, "shim_data_channel_close")
	purego.RegisterLibFunc(&shimDataChannelDestroy, libHandle, "shim_data_channel_destroy")

	// Device Enumeration
	purego.RegisterLibFunc(&shimEnumerateDevices, libHandle, "shim_enumerate_devices")

	// Video Capture
	purego.RegisterLibFunc(&shimVideoCaptureCreate, libHandle, "shim_video_capture_create")
	purego.RegisterLibFunc(&shimVideoCaptureStart, libHandle, "shim_video_capture_start")
	purego.RegisterLibFunc(&shimVideoCaptureStop, libHandle, "shim_video_capture_stop")
	purego.RegisterLibFunc(&shimVideoCaptureDestroy, libHandle, "shim_video_capture_destroy")

	// Audio Capture
	purego.RegisterLibFunc(&shimAudioCaptureCreate, libHandle, "shim_audio_capture_create")
	purego.RegisterLibFunc(&shimAudioCaptureStart, libHandle, "shim_audio_capture_start")
	purego.RegisterLibFunc(&shimAudioCaptureStop, libHandle, "shim_audio_capture_stop")
	purego.RegisterLibFunc(&shimAudioCaptureDestroy, libHandle, "shim_audio_capture_destroy")

	// Screen Capture
	purego.RegisterLibFunc(&shimEnumerateScreens, libHandle, "shim_enumerate_screens")
	purego.RegisterLibFunc(&shimScreenCaptureCreate, libHandle, "shim_screen_capture_create")
	purego.RegisterLibFunc(&shimScreenCaptureStart, libHandle, "shim_screen_capture_start")
	purego.RegisterLibFunc(&shimScreenCaptureStop, libHandle, "shim_screen_capture_stop")
	purego.RegisterLibFunc(&shimScreenCaptureDestroy, libHandle, "shim_screen_capture_destroy")

	// Video Track Source
	purego.RegisterLibFunc(&shimVideoTrackSourceCreate, libHandle, "shim_video_track_source_create")
	purego.RegisterLibFunc(&shimVideoTrackSourcePushFrame, libHandle, "shim_video_track_source_push_frame")
	purego.RegisterLibFunc(&shimPeerConnectionAddVideoTrackFromSource, libHandle, "shim_peer_connection_add_video_track_from_source")
	purego.RegisterLibFunc(&shimVideoTrackSourceDestroy, libHandle, "shim_video_track_source_destroy")

	// Audio Track Source
	purego.RegisterLibFunc(&shimAudioTrackSourceCreate, libHandle, "shim_audio_track_source_create")
	purego.RegisterLibFunc(&shimAudioTrackSourcePushFrame, libHandle, "shim_audio_track_source_push_frame")
	purego.RegisterLibFunc(&shimPeerConnectionAddAudioTrackFromSource, libHandle, "shim_peer_connection_add_audio_track_from_source")
	purego.RegisterLibFunc(&shimAudioTrackSourceDestroy, libHandle, "shim_audio_track_source_destroy")

	// Remote Track Sink
	purego.RegisterLibFunc(&shimTrackSetVideoSink, libHandle, "shim_track_set_video_sink")
	purego.RegisterLibFunc(&shimTrackSetAudioSink, libHandle, "shim_track_set_audio_sink")
	purego.RegisterLibFunc(&shimTrackRemoveVideoSink, libHandle, "shim_track_remove_video_sink")
	purego.RegisterLibFunc(&shimTrackRemoveAudioSink, libHandle, "shim_track_remove_audio_sink")
	purego.RegisterLibFunc(&shimTrackKind, libHandle, "shim_track_kind")
	purego.RegisterLibFunc(&shimTrackID, libHandle, "shim_track_id")

	// RTPSender Parameters
	purego.RegisterLibFunc(&shimRTPSenderGetParameters, libHandle, "shim_rtp_sender_get_parameters")
	purego.RegisterLibFunc(&shimRTPSenderSetParameters, libHandle, "shim_rtp_sender_set_parameters")
	purego.RegisterLibFunc(&shimRTPSenderGetTrack, libHandle, "shim_rtp_sender_get_track")
	purego.RegisterLibFunc(&shimRTPSenderGetStats, libHandle, "shim_rtp_sender_get_stats")
	purego.RegisterLibFunc(&shimRTPSenderSetOnRTCPFeedback, libHandle, "shim_rtp_sender_set_on_rtcp_feedback")
	purego.RegisterLibFunc(&shimRTPSenderSetLayerActive, libHandle, "shim_rtp_sender_set_layer_active")
	purego.RegisterLibFunc(&shimRTPSenderSetLayerBitrate, libHandle, "shim_rtp_sender_set_layer_bitrate")
	purego.RegisterLibFunc(&shimRTPSenderGetActiveLayers, libHandle, "shim_rtp_sender_get_active_layers")

	// RTPReceiver
	purego.RegisterLibFunc(&shimRTPReceiverGetTrack, libHandle, "shim_rtp_receiver_get_track")
	purego.RegisterLibFunc(&shimRTPReceiverGetStats, libHandle, "shim_rtp_receiver_get_stats")
	purego.RegisterLibFunc(&shimRTPReceiverRequestKeyframe, libHandle, "shim_rtp_receiver_request_keyframe")

	// RTPTransceiver
	purego.RegisterLibFunc(&shimTransceiverGetDirection, libHandle, "shim_transceiver_get_direction")
	purego.RegisterLibFunc(&shimTransceiverSetDirection, libHandle, "shim_transceiver_set_direction")
	purego.RegisterLibFunc(&shimTransceiverGetCurrentDirection, libHandle, "shim_transceiver_get_current_direction")
	purego.RegisterLibFunc(&shimTransceiverStop, libHandle, "shim_transceiver_stop")
	purego.RegisterLibFunc(&shimTransceiverMid, libHandle, "shim_transceiver_mid")
	purego.RegisterLibFunc(&shimTransceiverGetSender, libHandle, "shim_transceiver_get_sender")
	purego.RegisterLibFunc(&shimTransceiverGetReceiver, libHandle, "shim_transceiver_get_receiver")

	// PeerConnection Extended
	purego.RegisterLibFunc(&shimPeerConnectionAddTransceiver, libHandle, "shim_peer_connection_add_transceiver")
	purego.RegisterLibFunc(&shimPeerConnectionGetSenders, libHandle, "shim_peer_connection_get_senders")
	purego.RegisterLibFunc(&shimPeerConnectionGetReceivers, libHandle, "shim_peer_connection_get_receivers")
	purego.RegisterLibFunc(&shimPeerConnectionGetTransceivers, libHandle, "shim_peer_connection_get_transceivers")
	purego.RegisterLibFunc(&shimPeerConnectionRestartICE, libHandle, "shim_peer_connection_restart_ice")
	purego.RegisterLibFunc(&shimPeerConnectionGetStats, libHandle, "shim_peer_connection_get_stats")
	purego.RegisterLibFunc(&shimPeerConnectionSetOnSignalingStateChange, libHandle, "shim_peer_connection_set_on_signaling_state_change")
	purego.RegisterLibFunc(&shimPeerConnectionSetOnICEConnectionStateChange, libHandle, "shim_peer_connection_set_on_ice_connection_state_change")
	purego.RegisterLibFunc(&shimPeerConnectionSetOnICEGatheringStateChange, libHandle, "shim_peer_connection_set_on_ice_gathering_state_change")
	purego.RegisterLibFunc(&shimPeerConnectionSetOnNegotiationNeeded, libHandle, "shim_peer_connection_set_on_negotiation_needed")

	// RTPSender Scalability Mode
	purego.RegisterLibFunc(&shimRTPSenderSetScalabilityMode, libHandle, "shim_rtp_sender_set_scalability_mode")
	purego.RegisterLibFunc(&shimRTPSenderGetScalabilityMode, libHandle, "shim_rtp_sender_get_scalability_mode")

	// Codec Capabilities
	purego.RegisterLibFunc(&shimGetSupportedVideoCodecs, libHandle, "shim_get_supported_video_codecs")
	purego.RegisterLibFunc(&shimGetSupportedAudioCodecs, libHandle, "shim_get_supported_audio_codecs")
	purego.RegisterLibFunc(&shimIsCodecSupported, libHandle, "shim_is_codec_supported")

	// Bandwidth Estimation
	purego.RegisterLibFunc(&shimPeerConnectionSetOnBandwidthEstimate, libHandle, "shim_peer_connection_set_on_bandwidth_estimate")
	purego.RegisterLibFunc(&shimPeerConnectionGetBandwidthEstimate, libHandle, "shim_peer_connection_get_bandwidth_estimate")

	return err
}

// ShimError converts a shim error code to a Go error.
func ShimError(code int) error {
	switch code {
	case ShimOK:
		return nil
	case ShimErrInvalidParam:
		return errors.New("invalid parameter")
	case ShimErrInitFailed:
		return errors.New("initialization failed")
	case ShimErrEncodeFailed:
		return errors.New("encode failed")
	case ShimErrDecodeFailed:
		return errors.New("decode failed")
	case ShimErrOutOfMemory:
		return errors.New("out of memory")
	case ShimErrNotSupported:
		return errors.New("not supported")
	case ShimErrNeedMoreData:
		return errors.New("need more data")
	default:
		return fmt.Errorf("unknown error: %d", code)
	}
}
