package ffi

import (
	"testing"
	"unsafe"
)

// Integration tests for PeerConnection FFI bindings.
// Run with: go test -tags=integration ./internal/ffi/...

func TestCreatePeerConnection(t *testing.T) {
	cfg := &PeerConnectionConfig{
		ICECandidatePoolSize: 0,
	}

	handle := CreatePeerConnection(cfg)
	if handle == 0 {
		t.Fatal("CreatePeerConnection returned 0 handle")
	}
	defer PeerConnectionDestroy(handle)

	t.Logf("Created PeerConnection: handle=%d", handle)
}

func TestPeerConnectionCreateOffer(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle := CreatePeerConnection(cfg)
	if handle == 0 {
		t.Fatal("CreatePeerConnection failed")
	}
	defer PeerConnectionDestroy(handle)

	// Create offer
	sdpBuf := make([]byte, 64*1024)
	sdpLen, err := PeerConnectionCreateOffer(handle, sdpBuf)
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}

	if sdpLen == 0 {
		t.Error("SDP length should be > 0")
	}

	sdp := string(sdpBuf[:sdpLen])
	t.Logf("Created offer: %d bytes", sdpLen)
	t.Logf("SDP preview: %.200s...", sdp)
}

func TestPeerConnectionSetLocalDescription(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle := CreatePeerConnection(cfg)
	if handle == 0 {
		t.Fatal("CreatePeerConnection failed")
	}
	defer PeerConnectionDestroy(handle)

	// Create offer
	sdpBuf := make([]byte, 64*1024)
	sdpLen, err := PeerConnectionCreateOffer(handle, sdpBuf)
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}

	sdp := string(sdpBuf[:sdpLen])

	// Set local description (type 0 = offer)
	err = PeerConnectionSetLocalDescription(handle, 0, sdp)
	if err != nil {
		t.Fatalf("SetLocalDescription failed: %v", err)
	}

	t.Log("SetLocalDescription succeeded")
}

func TestPeerConnectionSignalingState(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle := CreatePeerConnection(cfg)
	if handle == 0 {
		t.Fatal("CreatePeerConnection failed")
	}
	defer PeerConnectionDestroy(handle)

	// Initial state should be stable (0)
	state := PeerConnectionSignalingState(handle)
	if state != 0 {
		t.Errorf("Initial signaling state = %d, want 0 (stable)", state)
	}

	t.Logf("Signaling state: %d", state)
}

func TestPeerConnectionICEConnectionState(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle := CreatePeerConnection(cfg)
	if handle == 0 {
		t.Fatal("CreatePeerConnection failed")
	}
	defer PeerConnectionDestroy(handle)

	// Initial state should be new (0)
	state := PeerConnectionICEConnectionState(handle)
	if state != 0 {
		t.Errorf("Initial ICE connection state = %d, want 0 (new)", state)
	}

	t.Logf("ICE connection state: %d", state)
}

func TestPeerConnectionConnectionState(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle := CreatePeerConnection(cfg)
	if handle == 0 {
		t.Fatal("CreatePeerConnection failed")
	}
	defer PeerConnectionDestroy(handle)

	// Initial state should be new (0)
	state := PeerConnectionConnectionState(handle)
	if state != 0 {
		t.Errorf("Initial connection state = %d, want 0 (new)", state)
	}

	t.Logf("Connection state: %d", state)
}

func TestPeerConnectionAddTrack(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle := CreatePeerConnection(cfg)
	if handle == 0 {
		t.Fatal("CreatePeerConnection failed")
	}
	defer PeerConnectionDestroy(handle)

	// Add H264 track
	senderHandle := PeerConnectionAddTrack(handle, CodecH264, "video-0", "stream-0")
	if senderHandle == 0 {
		t.Fatal("AddTrack returned 0 handle")
	}

	t.Logf("Added track: sender handle=%d", senderHandle)
}

func TestPeerConnectionRemoveTrack(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle := CreatePeerConnection(cfg)
	if handle == 0 {
		t.Fatal("CreatePeerConnection failed")
	}
	defer PeerConnectionDestroy(handle)

	// Add track
	senderHandle := PeerConnectionAddTrack(handle, CodecVP8, "video-0", "stream-0")
	if senderHandle == 0 {
		t.Fatal("AddTrack failed")
	}

	// Remove track
	err := PeerConnectionRemoveTrack(handle, senderHandle)
	if err != nil {
		t.Fatalf("RemoveTrack failed: %v", err)
	}

	t.Log("RemoveTrack succeeded")
}

func TestPeerConnectionCreateDataChannel(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle := CreatePeerConnection(cfg)
	if handle == 0 {
		t.Fatal("CreatePeerConnection failed")
	}
	defer PeerConnectionDestroy(handle)

	// Create data channel
	dcHandle := PeerConnectionCreateDataChannel(handle, "test-dc", true, -1, "")
	if dcHandle == 0 {
		t.Fatal("CreateDataChannel returned 0 handle")
	}

	t.Logf("Created data channel: handle=%d", dcHandle)
}

func TestPeerConnectionClose(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle := CreatePeerConnection(cfg)
	if handle == 0 {
		t.Fatal("CreatePeerConnection failed")
	}

	// Close
	PeerConnectionClose(handle)

	// Destroy
	PeerConnectionDestroy(handle)

	t.Log("Close and Destroy succeeded")
}

func TestOfferAnswerExchange(t *testing.T) {
	// Create two peer connections
	cfg := &PeerConnectionConfig{}

	pc1 := CreatePeerConnection(cfg)
	if pc1 == 0 {
		t.Fatal("CreatePeerConnection (offerer) failed")
	}
	defer PeerConnectionDestroy(pc1)

	pc2 := CreatePeerConnection(cfg)
	if pc2 == 0 {
		t.Fatal("CreatePeerConnection (answerer) failed")
	}
	defer PeerConnectionDestroy(pc2)

	// PC1 creates offer
	offerBuf := make([]byte, 64*1024)
	offerLen, err := PeerConnectionCreateOffer(pc1, offerBuf)
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}
	offer := string(offerBuf[:offerLen])

	// PC1 sets local description
	err = PeerConnectionSetLocalDescription(pc1, 0, offer) // 0 = offer
	if err != nil {
		t.Fatalf("PC1 SetLocalDescription failed: %v", err)
	}

	// PC2 sets remote description
	err = PeerConnectionSetRemoteDescription(pc2, 0, offer) // 0 = offer
	if err != nil {
		t.Fatalf("PC2 SetRemoteDescription failed: %v", err)
	}

	// PC2 creates answer
	answerBuf := make([]byte, 64*1024)
	answerLen, err := PeerConnectionCreateAnswer(pc2, answerBuf)
	if err != nil {
		t.Fatalf("CreateAnswer failed: %v", err)
	}
	answer := string(answerBuf[:answerLen])

	// PC2 sets local description
	err = PeerConnectionSetLocalDescription(pc2, 2, answer) // 2 = answer
	if err != nil {
		t.Fatalf("PC2 SetLocalDescription failed: %v", err)
	}

	// PC1 sets remote description
	err = PeerConnectionSetRemoteDescription(pc1, 2, answer) // 2 = answer
	if err != nil {
		t.Fatalf("PC1 SetRemoteDescription failed: %v", err)
	}

	t.Log("Offer/Answer exchange completed successfully")
}

func TestDataChannelSend(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle := CreatePeerConnection(cfg)
	if handle == 0 {
		t.Fatal("CreatePeerConnection failed")
	}
	defer PeerConnectionDestroy(handle)

	// Create data channel
	dcHandle := PeerConnectionCreateDataChannel(handle, "test-dc", true, -1, "")
	if dcHandle == 0 {
		t.Fatal("CreateDataChannel failed")
	}

	// Try to send data (may fail if not connected, but shouldn't crash)
	data := []byte("hello world")
	err := DataChannelSend(dcHandle, data, false)
	if err != nil {
		t.Logf("DataChannelSend: %v (expected if not connected)", err)
	}

	t.Log("DataChannel test completed")
}

func TestRTPSenderSetBitrate(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle := CreatePeerConnection(cfg)
	if handle == 0 {
		t.Fatal("CreatePeerConnection failed")
	}
	defer PeerConnectionDestroy(handle)

	// Add track
	senderHandle := PeerConnectionAddTrack(handle, CodecH264, "video-0", "stream-0")
	if senderHandle == 0 {
		t.Fatal("AddTrack failed")
	}

	// Set bitrate
	err := RTPSenderSetBitrate(senderHandle, 2_000_000)
	if err != nil {
		t.Logf("SetBitrate: %v (may be expected)", err)
	}

	t.Log("RTPSender test completed")
}

// Benchmark PeerConnection creation at FFI level
func BenchmarkFFICreatePeerConnection(b *testing.B) {
	cfg := &PeerConnectionConfig{}
	for i := 0; i < b.N; i++ {
		handle := CreatePeerConnection(cfg)
		if handle != 0 {
			PeerConnectionDestroy(handle)
		}
	}
}

// Benchmark offer creation at FFI level
func BenchmarkFFICreateOffer(b *testing.B) {
	cfg := &PeerConnectionConfig{}
	handle := CreatePeerConnection(cfg)
	if handle == 0 {
		b.Fatal("CreatePeerConnection failed")
	}
	defer PeerConnectionDestroy(handle)

	sdpBuf := make([]byte, 64*1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = PeerConnectionCreateOffer(handle, sdpBuf)
	}
}

func TestGetSupportedVideoCodecsFFI(t *testing.T) {
	t.Logf("CodecCapability struct size: %d", unsafe.Sizeof(CodecCapability{}))

	codecs, err := GetSupportedVideoCodecs()
	if err != nil {
		t.Fatalf("GetSupportedVideoCodecs failed: %v", err)
	}

	t.Logf("Got %d video codecs", len(codecs))
	for i, c := range codecs {
		t.Logf("  [%d] mime=%s clock=%d pt=%d", i,
			CStringToGo(c.MimeType[:]),
			c.ClockRate,
			c.PayloadType)
	}

	if len(codecs) == 0 {
		t.Error("Expected at least one video codec")
	}
}

func TestGetSupportedAudioCodecsFFI(t *testing.T) {
	codecs, err := GetSupportedAudioCodecs()
	if err != nil {
		t.Fatalf("GetSupportedAudioCodecs failed: %v", err)
	}

	t.Logf("Got %d audio codecs", len(codecs))
	for i, c := range codecs {
		t.Logf("  [%d] mime=%s clock=%d ch=%d pt=%d", i,
			CStringToGo(c.MimeType[:]),
			c.ClockRate,
			c.Channels,
			c.PayloadType)
	}

	if len(codecs) == 0 {
		t.Error("Expected at least one audio codec")
	}
}
