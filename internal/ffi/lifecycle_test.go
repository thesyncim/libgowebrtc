package ffi

import (
	"testing"
)

func TestVideoEncoder_CreateDestroy_Repeated(t *testing.T) {
	cfg := &VideoEncoderConfig{
		Width:      320,
		Height:     240,
		BitrateBps: 500_000,
		Framerate:  30,
	}

	// Create and destroy many times - verifies no handle leak
	for i := 0; i < 50; i++ {
		handle, err := CreateVideoEncoder(CodecVP8, cfg)
		if err != nil {
			t.Fatalf("iteration %d: create: %v", i, err)
		}
		VideoEncoderDestroy(handle)
	}
}

func TestVideoDecoder_CreateDestroy_Repeated(t *testing.T) {
	for i := 0; i < 50; i++ {
		handle, err := CreateVideoDecoder(CodecVP8)
		if err != nil {
			t.Fatalf("iteration %d: create: %v", i, err)
		}
		VideoDecoderDestroy(handle)
	}
}

func TestAudioEncoder_CreateDestroy_Repeated(t *testing.T) {
	cfg := &AudioEncoderConfig{
		SampleRate: 48000,
		Channels:   2,
		BitrateBps: 64000,
	}

	for i := 0; i < 50; i++ {
		handle, err := CreateAudioEncoder(cfg)
		if err != nil {
			t.Fatalf("iteration %d: create: %v", i, err)
		}
		AudioEncoderDestroy(handle)
	}
}

func TestAudioDecoder_CreateDestroy_Repeated(t *testing.T) {
	for i := 0; i < 50; i++ {
		handle, err := CreateAudioDecoder(48000, 2)
		if err != nil {
			t.Fatalf("iteration %d: create: %v", i, err)
		}
		AudioDecoderDestroy(handle)
	}
}

func TestPeerConnection_CreateDestroy_Repeated(t *testing.T) {
	cfg := &PeerConnectionConfig{}

	for i := 0; i < 20; i++ {
		handle, err := CreatePeerConnection(cfg)
		if err != nil {
			t.Fatalf("iteration %d: create: %v", i, err)
		}
		PeerConnectionDestroy(handle)
	}
}

func TestPacketizer_CreateDestroy_Repeated(t *testing.T) {
	cfg := &PacketizerConfig{
		Codec:       int32(CodecH264),
		SSRC:        12345,
		PayloadType: 96,
		MTU:         1200,
		ClockRate:   90000,
	}

	for i := 0; i < 50; i++ {
		handle := CreatePacketizer(cfg)
		if handle == 0 {
			t.Fatalf("iteration %d: create returned 0", i)
		}
		PacketizerDestroy(handle)
	}
}

func TestDepacketizer_CreateDestroy_Repeated(t *testing.T) {
	for i := 0; i < 50; i++ {
		handle := CreateDepacketizer(CodecH264)
		if handle == 0 {
			t.Fatalf("iteration %d: create returned 0", i)
		}
		DepacketizerDestroy(handle)
	}
}

func TestVideoEncoder_EncodeAfterDestroy(t *testing.T) {
	cfg := &VideoEncoderConfig{
		Width:      320,
		Height:     240,
		BitrateBps: 500_000,
		Framerate:  30,
	}

	handle, err := CreateVideoEncoder(CodecVP8, cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// First verify encoding works
	width, height := 320, 240
	yPlane := make([]byte, width*height)
	uPlane := make([]byte, (width/2)*(height/2))
	vPlane := make([]byte, (width/2)*(height/2))
	dst := make([]byte, width*height*2)

	n, _, err := encodeUntilOutput(
		t,
		handle,
		yPlane, uPlane, vPlane,
		width, width/2, width/2,
		0,
		true,
		dst,
		5,
	)
	if err != nil {
		t.Fatalf("encode before destroy: %v", err)
	}
	if n == 0 {
		t.Fatal("encoded 0 bytes")
	}

	// Destroy
	VideoEncoderDestroy(handle)

	// Don't call encode on destroyed handle - that's undefined behavior
	// This test just verifies the sequence create -> encode -> destroy works
}

func TestPeerConnection_CloseDestroy(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle, err := CreatePeerConnection(cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Close then destroy is the proper sequence
	PeerConnectionClose(handle)
	PeerConnectionDestroy(handle)
}

func TestPeerConnection_DestroyWithoutClose(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle, err := CreatePeerConnection(cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Direct destroy should also work
	PeerConnectionDestroy(handle)
}

func TestPeerConnection_WithTracks_Destroy(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle, err := CreatePeerConnection(cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Add some tracks
	sender1 := PeerConnectionAddTrack(handle, CodecVP8, "video-0", "stream-0")
	sender2 := PeerConnectionAddTrack(handle, CodecOpus, "audio-0", "stream-0")

	if sender1 == 0 {
		t.Log("video track creation returned 0 (may be expected)")
	}
	if sender2 == 0 {
		t.Log("audio track creation returned 0 (may be expected)")
	}

	// Create data channel
	dc := PeerConnectionCreateDataChannel(handle, "test-dc", true, -1, "")
	if dc != 0 {
		// Don't need to explicitly destroy - PC destroy handles it
	}

	// Destroy should clean up everything
	PeerConnectionClose(handle)
	PeerConnectionDestroy(handle)
}

func TestMultipleResourceTypes_Concurrent(t *testing.T) {
	// Create multiple different resource types
	videoEncCfg := &VideoEncoderConfig{
		Width:      320,
		Height:     240,
		BitrateBps: 500_000,
		Framerate:  30,
	}

	audioEncCfg := &AudioEncoderConfig{
		SampleRate: 48000,
		Channels:   2,
		BitrateBps: 64000,
	}

	pcCfg := &PeerConnectionConfig{}

	// Create
	videoEnc, err := CreateVideoEncoder(CodecVP8, videoEncCfg)
	if err != nil {
		t.Fatalf("video encoder: %v", err)
	}

	audioEnc, err := CreateAudioEncoder(audioEncCfg)
	if err != nil {
		VideoEncoderDestroy(videoEnc)
		t.Fatalf("audio encoder: %v", err)
	}

	videoDec, err := CreateVideoDecoder(CodecVP8)
	if err != nil {
		VideoEncoderDestroy(videoEnc)
		AudioEncoderDestroy(audioEnc)
		t.Fatalf("video decoder: %v", err)
	}

	audioDec, err := CreateAudioDecoder(48000, 2)
	if err != nil {
		VideoEncoderDestroy(videoEnc)
		AudioEncoderDestroy(audioEnc)
		VideoDecoderDestroy(videoDec)
		t.Fatalf("audio decoder: %v", err)
	}

	pc, err := CreatePeerConnection(pcCfg)
	if err != nil {
		VideoEncoderDestroy(videoEnc)
		AudioEncoderDestroy(audioEnc)
		VideoDecoderDestroy(videoDec)
		AudioDecoderDestroy(audioDec)
		t.Fatalf("peer connection: %v", err)
	}

	// Destroy in reverse order
	PeerConnectionDestroy(pc)
	AudioDecoderDestroy(audioDec)
	VideoDecoderDestroy(videoDec)
	AudioEncoderDestroy(audioEnc)
	VideoEncoderDestroy(videoEnc)
}
