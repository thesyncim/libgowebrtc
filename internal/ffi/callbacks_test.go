package ffi

import (
	"errors"
	"testing"
	"unsafe"
)

var ffiTestSerialLock = make(chan struct{}, 1)

func withFFITestSerialExecution(tb testing.TB) func() {
	tb.Helper()
	ffiTestSerialLock <- struct{}{}
	return func() {
		<-ffiTestSerialLock
	}
}

func TestSafeCallbackRecoversPanics(t *testing.T) {
	safeCallback(func() {
		panic("boom")
	})
}

func TestTrackCallbackRegistriesAndPointers(t *testing.T) {
	release := withFFITestSerialExecution(t)
	defer release()

	ResetForTesting()
	t.Cleanup(ResetForTesting)

	RegisterVideoCallback(11, func(width, height int, yPlane, uPlane, vPlane []byte, yStride, uStride, vStride int, timestampUs int64) {
	})
	RegisterAudioCallback(22, func(samples []int16, sampleRate, channels int, timestampUs int64) {})

	if videoCallbacks[11] == nil {
		t.Fatal("video callback was not registered")
	}
	if audioCallbacks[22] == nil {
		t.Fatal("audio callback was not registered")
	}
	if got := GetVideoSinkCallbackPtr(); got == 0 {
		t.Fatal("GetVideoSinkCallbackPtr() returned 0")
	}
	if got := GetAudioSinkCallbackPtr(); got == 0 {
		t.Fatal("GetAudioSinkCallbackPtr() returned 0")
	}

	UnregisterVideoCallback(11)
	UnregisterAudioCallback(22)
	if _, ok := videoCallbacks[11]; ok {
		t.Fatal("video callback was not removed")
	}
	if _, ok := audioCallbacks[22]; ok {
		t.Fatal("audio callback was not removed")
	}
}

func TestDataChannelAndBandwidthEstimateRegistries(t *testing.T) {
	release := withFFITestSerialExecution(t)
	defer release()

	ResetForTesting()
	t.Cleanup(ResetForTesting)

	pc, err := CreatePeerConnection(&PeerConnectionConfig{})
	if err != nil {
		t.Fatalf("CreatePeerConnection: %v", err)
	}
	defer PeerConnectionDestroy(pc)

	dc := PeerConnectionCreateDataChannel(pc, "ffi-callbacks", true, -1, -1, "", false, -1)
	if dc == 0 {
		t.Fatal("PeerConnectionCreateDataChannel returned 0")
	}
	defer DataChannelDestroy(dc)

	DataChannelSetOnMessage(dc, func(data []byte, isBinary bool) {})
	DataChannelSetOnOpen(dc, func() {})
	DataChannelSetOnClose(dc, func() {})
	if dcMessageCallbacks[dc] == nil {
		t.Fatal("data channel message callback was not registered")
	}
	if dcOpenCallbacks[dc] == nil {
		t.Fatal("data channel open callback was not registered")
	}
	if dcCloseCallbacks[dc] == nil {
		t.Fatal("data channel close callback was not registered")
	}
	if dcMessageCallbackPtr == 0 || dcOpenCallbackPtr == 0 || dcCloseCallbackPtr == 0 {
		t.Fatal("data channel callback pointers were not initialized")
	}

	UnregisterDataChannelCallbacks(dc)
	if _, ok := dcMessageCallbacks[dc]; ok {
		t.Fatal("data channel message callback was not removed")
	}
	if _, ok := dcOpenCallbacks[dc]; ok {
		t.Fatal("data channel open callback was not removed")
	}
	if _, ok := dcCloseCallbacks[dc]; ok {
		t.Fatal("data channel close callback was not removed")
	}

	if err := PeerConnectionSetOnBandwidthEstimate(pc, func(*BandwidthEstimate) {}); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("PeerConnectionSetOnBandwidthEstimate() error = %v, want %v", err, ErrNotSupported)
	}
	if _, ok := bweCallbacks[pc]; ok {
		t.Fatal("bandwidth estimate callback should not be registered when unsupported")
	}
	if _, err := PeerConnectionGetBandwidthEstimate(pc); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("PeerConnectionGetBandwidthEstimate() error = %v, want %v", err, ErrNotSupported)
	}

	UnregisterBandwidthEstimateCallback(pc)
	if _, ok := bweCallbacks[pc]; ok {
		t.Fatal("bandwidth estimate callback was not removed")
	}
}

func TestPeerConnectionDataChannelCallbackChurnRepeated(t *testing.T) {
	release := withFFITestSerialExecution(t)
	defer release()

	t.Cleanup(ResetForTesting)

	for i := 0; i < 20; i++ {
		ResetForTesting()

		pc, err := CreatePeerConnection(&PeerConnectionConfig{})
		if err != nil {
			t.Fatalf("iteration %d: CreatePeerConnection: %v", i, err)
		}

		dc := PeerConnectionCreateDataChannel(pc, "ffi-churn", true, -1, -1, "", false, -1)
		if dc == 0 {
			PeerConnectionDestroy(pc)
			t.Fatalf("iteration %d: PeerConnectionCreateDataChannel returned 0", i)
		}

		DataChannelSetOnMessage(dc, func(data []byte, isBinary bool) {})
		DataChannelSetOnOpen(dc, func() {})
		DataChannelSetOnClose(dc, func() {})
		PeerConnectionSetOnConnectionStateChange(pc, func(state int) {})
		PeerConnectionSetOnDataChannel(pc, func(dc uintptr) {})
		PeerConnectionSetOnSignalingStateChange(pc, func(state int) {})
		PeerConnectionSetOnNegotiationNeeded(pc, func() {})

		if dcMessageCallbacks[dc] == nil || dcOpenCallbacks[dc] == nil || dcCloseCallbacks[dc] == nil {
			DataChannelDestroy(dc)
			PeerConnectionDestroy(pc)
			t.Fatalf("iteration %d: expected data channel callbacks to be registered", i)
		}
		if connectionStateCallbacks[pc] == nil || onDataChannelCallbacks[pc] == nil || signalingStateCallbacks[pc] == nil || negotiationNeededCallbacks[pc] == nil {
			DataChannelDestroy(dc)
			PeerConnectionDestroy(pc)
			t.Fatalf("iteration %d: expected peer connection callbacks to be registered", i)
		}

		UnregisterDataChannelCallbacks(dc)
		UnregisterConnectionStateCallback(pc)
		UnregisterOnDataChannelCallback(pc)
		UnregisterSignalingStateCallback(pc)
		UnregisterNegotiationNeededCallback(pc)

		if _, ok := dcMessageCallbacks[dc]; ok {
			DataChannelDestroy(dc)
			PeerConnectionDestroy(pc)
			t.Fatalf("iteration %d: message callback was not removed", i)
		}
		if _, ok := dcOpenCallbacks[dc]; ok {
			DataChannelDestroy(dc)
			PeerConnectionDestroy(pc)
			t.Fatalf("iteration %d: open callback was not removed", i)
		}
		if _, ok := dcCloseCallbacks[dc]; ok {
			DataChannelDestroy(dc)
			PeerConnectionDestroy(pc)
			t.Fatalf("iteration %d: close callback was not removed", i)
		}
		if _, ok := connectionStateCallbacks[pc]; ok {
			DataChannelDestroy(dc)
			PeerConnectionDestroy(pc)
			t.Fatalf("iteration %d: connection state callback was not removed", i)
		}
		if _, ok := onDataChannelCallbacks[pc]; ok {
			DataChannelDestroy(dc)
			PeerConnectionDestroy(pc)
			t.Fatalf("iteration %d: on data channel callback was not removed", i)
		}
		if _, ok := signalingStateCallbacks[pc]; ok {
			DataChannelDestroy(dc)
			PeerConnectionDestroy(pc)
			t.Fatalf("iteration %d: signaling state callback was not removed", i)
		}
		if _, ok := negotiationNeededCallbacks[pc]; ok {
			DataChannelDestroy(dc)
			PeerConnectionDestroy(pc)
			t.Fatalf("iteration %d: negotiation callback was not removed", i)
		}

		PeerConnectionClose(pc)
		PeerConnectionDestroy(pc)
	}
}

func TestReadBandwidthEstimateFromC(t *testing.T) {
	if got := ReadBandwidthEstimateFromC(0); got != nil {
		t.Fatalf("ReadBandwidthEstimateFromC(0) = %#v, want nil", got)
	}

	estimate := BandwidthEstimate{
		TimestampUs:      123,
		TargetBitrateBps: 456,
		AvailableSendBps: 789,
		AvailableRecvBps: 1011,
		PacingRateBps:    1213,
		CongestionWindow: 14,
		LossRate:         0.25,
	}
	got := ReadBandwidthEstimateFromC(uintptr(unsafe.Pointer(&estimate)))
	if got == nil {
		t.Fatal("ReadBandwidthEstimateFromC returned nil")
	}
	if *got != estimate {
		t.Fatalf("ReadBandwidthEstimateFromC() = %#v, want %#v", *got, estimate)
	}
}

func TestVersionHelpersAndNotLoadedGuards(t *testing.T) {
	release := withFFITestSerialExecution(t)
	defer release()

	if got := ShimVersion(); got != ExpectedShimVersion {
		t.Fatalf("ShimVersion() = %q, want %q", got, ExpectedShimVersion)
	}
	if got := LibWebRTCVersion(); got != ExpectedLibWebRTCVersion {
		t.Fatalf("LibWebRTCVersion() = %q, want %q", got, ExpectedLibWebRTCVersion)
	}
	if err := CheckVersion(); err != nil {
		t.Fatalf("CheckVersion() unexpected error: %v", err)
	}

	libLoaded.Store(false)
	t.Cleanup(func() {
		libLoaded.Store(true)
	})

	if got := ShimVersion(); got != "" {
		t.Fatalf("ShimVersion() when not loaded = %q, want empty string", got)
	}
	if got := LibWebRTCVersion(); got != "" {
		t.Fatalf("LibWebRTCVersion() when not loaded = %q, want empty string", got)
	}
	if err := CheckVersion(); !errors.Is(err, ErrLibraryNotLoaded) {
		t.Fatalf("CheckVersion() when not loaded = %v, want %v", err, ErrLibraryNotLoaded)
	}
}

func TestSourcePushAndEncoderHelpers(t *testing.T) {
	release := withFFITestSerialExecution(t)
	defer release()

	pc, err := CreatePeerConnection(&PeerConnectionConfig{})
	if err != nil {
		t.Fatalf("CreatePeerConnection: %v", err)
	}
	defer PeerConnectionDestroy(pc)

	videoSource := VideoTrackSourceCreate(pc, 32, 32)
	if videoSource == 0 {
		t.Fatal("VideoTrackSourceCreate returned 0")
	}
	defer VideoTrackSourceDestroy(videoSource)

	yPlane := make([]byte, 32*32)
	uPlane := make([]byte, 16*16)
	vPlane := make([]byte, 16*16)
	if err := VideoTrackSourcePushFrame(videoSource, yPlane, uPlane, vPlane, 32, 16, 16, 90_000); err != nil {
		t.Fatalf("VideoTrackSourcePushFrame: %v", err)
	}

	audioSource := AudioTrackSourceCreate(pc, 48_000, 2)
	if audioSource == 0 {
		t.Fatal("AudioTrackSourceCreate returned 0")
	}
	defer AudioTrackSourceDestroy(audioSource)

	audioSamples := make([]int16, 960*2)
	if err := AudioTrackSourcePushFrame(audioSource, audioSamples, 20_000); err != nil {
		t.Fatalf("AudioTrackSourcePushFrame: %v", err)
	}

	videoEncoder, err := CreateVideoEncoder(CodecVP8, &VideoEncoderConfig{
		Width:      32,
		Height:     32,
		BitrateBps: 150_000,
		Framerate:  15,
	})
	if err != nil {
		t.Fatalf("CreateVideoEncoder: %v", err)
	}
	defer VideoEncoderDestroy(videoEncoder)

	if err := VideoEncoderRequestKeyframe(videoEncoder); err != nil {
		t.Fatalf("VideoEncoderRequestKeyframe: %v", err)
	}

	audioEncoder, err := CreateAudioEncoder(&AudioEncoderConfig{
		SampleRate: 48_000,
		Channels:   2,
		BitrateBps: 64_000,
	})
	if err != nil {
		t.Fatalf("CreateAudioEncoder: %v", err)
	}
	defer AudioEncoderDestroy(audioEncoder)

	if err := AudioEncoderSetBitrate(audioEncoder, 96_000); err != nil {
		t.Fatalf("AudioEncoderSetBitrate: %v", err)
	}

	packetizer := CreatePacketizer(&PacketizerConfig{
		Codec:       int32(CodecH264),
		SSRC:        1234,
		PayloadType: 96,
		MTU:         1200,
		ClockRate:   90_000,
	})
	if packetizer == 0 {
		t.Fatal("CreatePacketizer returned 0")
	}
	defer PacketizerDestroy(packetizer)

	before := PacketizerSequenceNumber(packetizer)
	dst := make([]byte, 4096)
	offsets := make([]int32, 16)
	sizes := make([]int32, 16)
	packetCount, err := PacketizerPacketizeInto(
		packetizer,
		[]byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84, 0x21},
		90_000,
		true,
		dst,
		offsets,
		sizes,
		16,
	)
	if err != nil {
		t.Fatalf("PacketizerPacketizeInto: %v", err)
	}
	if packetCount == 0 {
		t.Fatal("PacketizerPacketizeInto returned 0 packets")
	}
	if after := PacketizerSequenceNumber(packetizer); after <= before {
		t.Fatalf("PacketizerSequenceNumber did not advance: before=%d after=%d", before, after)
	}
}

func TestResetForTestingClearsRegistries(t *testing.T) {
	release := withFFITestSerialExecution(t)
	defer release()

	videoCallbacks[1] = func(width, height int, yPlane, uPlane, vPlane []byte, yStride, uStride, vStride int, timestampUs int64) {
	}
	audioCallbacks[2] = func(samples []int16, sampleRate, channels int, timestampUs int64) {}
	dcMessageCallbacks[3] = func(data []byte, isBinary bool) {}
	dcOpenCallbacks[3] = func() {}
	dcCloseCallbacks[3] = func() {}
	rtcpFeedbackCallbacks[4] = func(feedbackType int, ssrc uint32) {}
	connectionStateCallbacks[5] = func(state int) {}
	onTrackCallbacks[6] = func(track, receiver uintptr, streams string) {}
	onICECandidateCallbacks[7] = func(candidate, sdpMid string, sdpMLineIndex int) {}
	onDataChannelCallbacks[8] = func(dc uintptr) {}
	signalingStateCallbacks[9] = func(state int) {}
	iceConnectionStateCallbacks[10] = func(state int) {}
	iceGatheringStateCallbacks[11] = func(state int) {}
	negotiationNeededCallbacks[12] = func() {}
	bweCallbacks[13] = func(*BandwidthEstimate) {}

	ResetForTesting()

	if len(videoCallbacks) != 0 || len(audioCallbacks) != 0 || len(dcMessageCallbacks) != 0 ||
		len(dcOpenCallbacks) != 0 || len(dcCloseCallbacks) != 0 || len(rtcpFeedbackCallbacks) != 0 ||
		len(connectionStateCallbacks) != 0 || len(onTrackCallbacks) != 0 || len(onICECandidateCallbacks) != 0 ||
		len(onDataChannelCallbacks) != 0 || len(signalingStateCallbacks) != 0 ||
		len(iceConnectionStateCallbacks) != 0 || len(iceGatheringStateCallbacks) != 0 ||
		len(negotiationNeededCallbacks) != 0 || len(bweCallbacks) != 0 {
		t.Fatal("ResetForTesting did not clear all registries")
	}
}

func TestPeerConnectionNativeWrapperCoverage(t *testing.T) {
	release := withFFITestSerialExecution(t)
	defer release()

	ResetForTesting()
	t.Cleanup(ResetForTesting)

	pc, err := CreatePeerConnection(&PeerConnectionConfig{})
	if err != nil {
		t.Fatalf("CreatePeerConnection: %v", err)
	}
	defer PeerConnectionDestroy(pc)

	videoSource := VideoTrackSourceCreate(pc, 48, 48)
	if videoSource == 0 {
		t.Fatal("VideoTrackSourceCreate returned 0")
	}
	defer VideoTrackSourceDestroy(videoSource)

	audioSource := AudioTrackSourceCreate(pc, 48_000, 2)
	if audioSource == 0 {
		t.Fatal("AudioTrackSourceCreate returned 0")
	}
	defer AudioTrackSourceDestroy(audioSource)

	videoSender := PeerConnectionAddVideoTrackFromSource(pc, videoSource, "ffi-video", "ffi-stream")
	if videoSender == 0 {
		t.Fatal("PeerConnectionAddVideoTrackFromSource returned 0")
	}
	defer RTPSenderDestroy(videoSender)

	audioSender := PeerConnectionAddAudioTrackFromSource(pc, audioSource, "ffi-audio", "ffi-stream")
	if audioSender == 0 {
		t.Fatal("PeerConnectionAddAudioTrackFromSource returned 0")
	}
	defer RTPSenderDestroy(audioSender)

	dc := PeerConnectionCreateDataChannel(pc, "ffi-native", true, -1, -1, "", false, -1)
	if dc == 0 {
		t.Fatal("PeerConnectionCreateDataChannel returned 0")
	}
	defer DataChannelDestroy(dc)

	if got := DataChannelLabel(dc); got != "ffi-native" {
		t.Fatalf("DataChannelLabel() = %q, want %q", got, "ffi-native")
	}
	if got := DataChannelReadyState(dc); got < 0 {
		t.Fatalf("DataChannelReadyState() = %d, want non-negative", got)
	}
	DataChannelClose(dc)

	if err := PeerConnectionAddICECandidate(pc, "candidate:0 1 UDP 2122260223 127.0.0.1 5000 typ host", "0", 0); err != nil {
		t.Logf("PeerConnectionAddICECandidate returned %v before negotiation", err)
	}

	senders, err := PeerConnectionGetSenders(pc, 8)
	if err != nil {
		t.Fatalf("PeerConnectionGetSenders: %v", err)
	}
	if len(senders) < 2 {
		t.Fatalf("PeerConnectionGetSenders() len = %d, want at least 2", len(senders))
	}
	if _, err := PeerConnectionGetReceivers(pc, 8); err != nil {
		t.Fatalf("PeerConnectionGetReceivers: %v", err)
	}

	transceiver := PeerConnectionAddTransceiver(pc, MediaKindVideo, TransceiverDirectionSendRecv)
	if transceiver == 0 {
		t.Fatal("PeerConnectionAddTransceiver returned 0")
	}
	transceivers, err := PeerConnectionGetTransceivers(pc, 8)
	if err != nil {
		t.Fatalf("PeerConnectionGetTransceivers: %v", err)
	}
	if len(transceivers) == 0 {
		t.Fatal("PeerConnectionGetTransceivers returned no handles")
	}
	if got := TransceiverGetDirection(transceiver); got != TransceiverDirectionSendRecv {
		t.Fatalf("TransceiverGetDirection() = %v, want %v", got, TransceiverDirectionSendRecv)
	}
	if err := TransceiverSetDirection(transceiver, TransceiverDirectionRecvOnly); err != nil {
		t.Fatalf("TransceiverSetDirection: %v", err)
	}
	_ = TransceiverGetCurrentDirection(transceiver)
	_ = TransceiverMid(transceiver)
	if got := TransceiverGetSender(transceiver); got == 0 {
		t.Fatal("TransceiverGetSender returned 0")
	}
	if got := TransceiverGetReceiver(transceiver); got == 0 {
		t.Fatal("TransceiverGetReceiver returned 0")
	}
	if err := TransceiverSetCodecPreferences(transceiver, nil); err != nil {
		t.Fatalf("TransceiverSetCodecPreferences(nil): %v", err)
	}
	if _, err := TransceiverGetCodecPreferences(transceiver); err != nil {
		t.Fatalf("TransceiverGetCodecPreferences: %v", err)
	}

	trackHandle := RTPSenderGetTrack(videoSender)
	if trackHandle == 0 {
		t.Fatal("RTPSenderGetTrack returned 0")
	}
	if got := TrackKind(trackHandle); got != "video" {
		t.Fatalf("TrackKind() = %q, want %q", got, "video")
	}
	if got := TrackID(trackHandle); got != "ffi-video" {
		t.Fatalf("TrackID() = %q, want %q", got, "ffi-video")
	}

	encodings := make([]RTPEncodingParameters, 8)
	params, count, err := RTPSenderGetParameters(videoSender, encodings)
	if err != nil {
		t.Fatalf("RTPSenderGetParameters: %v", err)
	}
	if params == nil {
		t.Fatal("RTPSenderGetParameters returned nil params")
	}
	if count < 0 {
		t.Fatalf("RTPSenderGetParameters count = %d, want >= 0", count)
	}
	if err := RTPSenderSetParameters(videoSender, params); err != nil {
		t.Fatalf("RTPSenderSetParameters: %v", err)
	}

	if _, err := RTPSenderGetStats(videoSender); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("RTPSenderGetStats() error = %v, want %v", err, ErrNotSupported)
	}
	if err := RTPSenderSetOnRTCPFeedback(videoSender, func(feedbackType int, ssrc uint32) {}); !errors.Is(err, ErrNotSupported) {
		t.Fatalf("RTPSenderSetOnRTCPFeedback() error = %v, want %v", err, ErrNotSupported)
	}
	if _, ok := rtcpFeedbackCallbacks[videoSender]; ok {
		t.Fatal("RTCP feedback callback should not be registered when unsupported")
	}
	UnregisterRTCPFeedbackCallback(videoSender)

	if err := RTPSenderSetLayerActive(videoSender, "f", true); err != nil {
		t.Logf("RTPSenderSetLayerActive returned %v (acceptable before simulcast negotiation)", err)
	}
	if err := RTPSenderSetLayerBitrate(videoSender, "f", 300_000); err != nil {
		t.Logf("RTPSenderSetLayerBitrate returned %v (acceptable before simulcast negotiation)", err)
	}
	if _, _, err := RTPSenderGetActiveLayers(videoSender); err != nil {
		t.Logf("RTPSenderGetActiveLayers returned %v (acceptable before simulcast negotiation)", err)
	}
	if err := RTPSenderSetScalabilityMode(videoSender, "L1T2"); err != nil {
		t.Logf("RTPSenderSetScalabilityMode returned %v (acceptable before negotiation)", err)
	}
	if _, err := RTPSenderGetScalabilityMode(videoSender); err != nil {
		t.Logf("RTPSenderGetScalabilityMode returned %v (acceptable before negotiation)", err)
	}
	if _, err := RTPSenderGetNegotiatedCodecs(videoSender); err != nil {
		t.Logf("RTPSenderGetNegotiatedCodecs returned %v (acceptable before negotiation)", err)
	}
	if err := RTPSenderSetPreferredCodec(videoSender, "video/VP8", 96); err != nil &&
		!errors.Is(err, ErrNotFound) &&
		!errors.Is(err, ErrRenegotiationNeeded) {
		t.Fatalf("RTPSenderSetPreferredCodec: %v", err)
	}
	if err := RTPSenderReplaceTrack(videoSender, 0); err != nil {
		t.Fatalf("RTPSenderReplaceTrack(nil): %v", err)
	}

	if _, err := RTPReceiverGetStats(TransceiverGetReceiver(transceiver)); err != nil {
		t.Fatalf("RTPReceiverGetStats: %v", err)
	}
	if got := RTPReceiverGetTrack(TransceiverGetReceiver(transceiver)); got != 0 {
		_ = TrackID(got)
	}
	if err := RTPReceiverSetJitterBufferMinDelay(TransceiverGetReceiver(transceiver), 20); err != nil {
		t.Fatalf("RTPReceiverSetJitterBufferMinDelay: %v", err)
	}

	if _, err := PeerConnectionGetStats(pc); err != nil {
		t.Fatalf("PeerConnectionGetStats: %v", err)
	}
	if err := PeerConnectionRestartICE(pc); err != nil {
		t.Fatalf("PeerConnectionRestartICE: %v", err)
	}

	PeerConnectionSetOnConnectionStateChange(pc, func(state int) {})
	PeerConnectionSetOnTrack(pc, func(track uintptr, receiver uintptr, streams string) {})
	PeerConnectionSetOnICECandidate(pc, func(candidate, sdpMid string, sdpMLineIndex int) {})
	PeerConnectionSetOnDataChannel(pc, func(dc uintptr) {})
	PeerConnectionSetOnSignalingStateChange(pc, func(state int) {})
	PeerConnectionSetOnICEConnectionStateChange(pc, func(state int) {})
	PeerConnectionSetOnICEGatheringStateChange(pc, func(state int) {})
	PeerConnectionSetOnNegotiationNeeded(pc, func() {})
	if connectionStateCallbacks[pc] == nil || connectionStateCallbackPtr == 0 {
		t.Fatal("connection state callback was not registered")
	}
	if onTrackCallbacks[pc] == nil || onTrackCallbackPtr == 0 {
		t.Fatal("on track callback was not registered")
	}
	if onICECandidateCallbacks[pc] == nil || onICECandidateCallbackPtr == 0 {
		t.Fatal("on ICE callback was not registered")
	}
	if onDataChannelCallbacks[pc] == nil || onDataChannelCallbackPtr == 0 {
		t.Fatal("on data channel callback was not registered")
	}
	if signalingStateCallbacks[pc] == nil || signalingStateCallbackPtr == 0 {
		t.Fatal("signaling state callback was not registered")
	}
	if iceConnectionStateCallbacks[pc] == nil || iceConnectionStateCallbackPtr == 0 {
		t.Fatal("ICE connection state callback was not registered")
	}
	if iceGatheringStateCallbacks[pc] == nil || iceGatheringStateCallbackPtr == 0 {
		t.Fatal("ICE gathering state callback was not registered")
	}
	if negotiationNeededCallbacks[pc] == nil || negotiationNeededCallbackPtr == 0 {
		t.Fatal("negotiation needed callback was not registered")
	}

	UnregisterConnectionStateCallback(pc)
	UnregisterOnTrackCallback(pc)
	UnregisterOnICECandidateCallback(pc)
	UnregisterOnDataChannelCallback(pc)
	UnregisterSignalingStateCallback(pc)
	UnregisterICEConnectionStateCallback(pc)
	UnregisterICEGatheringStateCallback(pc)
	UnregisterNegotiationNeededCallback(pc)

	if _, ok := connectionStateCallbacks[pc]; ok {
		t.Fatal("connection state callback was not unregistered")
	}
	if _, ok := onTrackCallbacks[pc]; ok {
		t.Fatal("on track callback was not unregistered")
	}
	if _, ok := onICECandidateCallbacks[pc]; ok {
		t.Fatal("on ICE callback was not unregistered")
	}
	if _, ok := onDataChannelCallbacks[pc]; ok {
		t.Fatal("on data channel callback was not unregistered")
	}
	if _, ok := signalingStateCallbacks[pc]; ok {
		t.Fatal("signaling callback was not unregistered")
	}
	if _, ok := iceConnectionStateCallbacks[pc]; ok {
		t.Fatal("ICE connection callback was not unregistered")
	}
	if _, ok := iceGatheringStateCallbacks[pc]; ok {
		t.Fatal("ICE gathering callback was not unregistered")
	}
	if _, ok := negotiationNeededCallbacks[pc]; ok {
		t.Fatal("negotiation callback was not unregistered")
	}

	TrackRemoveVideoSink(trackHandle)
	TrackRemoveAudioSink(trackHandle)
	_ = TrackSetVideoSink(trackHandle, GetVideoSinkCallbackPtr(), trackHandle)
	_ = TrackSetAudioSink(trackHandle, GetAudioSinkCallbackPtr(), trackHandle)

	if err := TransceiverStop(transceiver); err != nil {
		t.Fatalf("TransceiverStop: %v", err)
	}
	PeerConnectionClose(pc)
}
