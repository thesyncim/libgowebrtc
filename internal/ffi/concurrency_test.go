package ffi

import (
	"errors"
	"sync"
	"testing"
)

func TestConcurrent_VideoEncoderCreate(t *testing.T) {
	// Keep concurrency moderate to avoid exhausting encoder pool
	const numGoroutines = 4

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)
	handles := make(chan uintptr, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := &VideoEncoderConfig{
				Width:      320,
				Height:     240,
				BitrateBps: 500_000,
				Framerate:  30,
			}
			handle, err := CreateVideoEncoder(CodecVP8, cfg)
			if err != nil {
				errCh <- err
				return
			}
			handles <- handle
		}()
	}

	wg.Wait()
	close(errCh)
	close(handles)

	for err := range errCh {
		t.Errorf("concurrent create error: %v", err)
	}

	// Clean up encoders
	for handle := range handles {
		VideoEncoderDestroy(handle)
	}
}

func TestConcurrent_VideoEncoderEncode(t *testing.T) {
	cfg := &VideoEncoderConfig{
		Width:      320,
		Height:     240,
		BitrateBps: 500_000,
		Framerate:  30,
	}

	handle, err := CreateVideoEncoder(CodecVP8, cfg)
	if err != nil {
		t.Fatalf("create encoder: %v", err)
	}
	defer VideoEncoderDestroy(handle)

	const numGoroutines = 4
	const framesPerGoroutine = 20

	// Create test frame data
	width, height := 320, 240
	ySize := width * height
	uvSize := (width / 2) * (height / 2)
	yPlane := make([]byte, ySize)
	uPlane := make([]byte, uvSize)
	vPlane := make([]byte, uvSize)

	for i := range yPlane {
		yPlane[i] = 128
	}
	for i := range uPlane {
		uPlane[i] = 128
		vPlane[i] = 128
	}

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines*framesPerGoroutine)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			dst := make([]byte, width*height*2)

			for i := 0; i < framesPerGoroutine; i++ {
				_, _, err := VideoEncoderEncodeInto(
					handle,
					yPlane, uPlane, vPlane,
					width, width/2, width/2,
					uint32(id*1000+i),
					i == 0,
					dst,
				)
				if err != nil && !errors.Is(err, ErrNeedMoreData) {
					errCh <- err
				}
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent encode error: %v", err)
	}
}

func TestConcurrent_VideoDecoderCreate(t *testing.T) {
	// Keep concurrency moderate to avoid exhausting decoder pool
	const numGoroutines = 4

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)
	handles := make(chan uintptr, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handle, err := CreateVideoDecoder(CodecVP8)
			if err != nil {
				errCh <- err
				return
			}
			handles <- handle
		}()
	}

	wg.Wait()
	close(errCh)
	close(handles)

	for err := range errCh {
		t.Errorf("concurrent create error: %v", err)
	}

	// Clean up decoders
	for handle := range handles {
		VideoDecoderDestroy(handle)
	}
}

func TestConcurrent_AudioEncoderCreate(t *testing.T) {
	// Keep concurrency moderate to avoid exhausting encoder pool
	const numGoroutines = 4

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)
	handles := make(chan uintptr, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := &AudioEncoderConfig{
				SampleRate: 48000,
				Channels:   2,
				BitrateBps: 64000,
			}
			handle, err := CreateAudioEncoder(cfg)
			if err != nil {
				errCh <- err
				return
			}
			handles <- handle
		}()
	}

	wg.Wait()
	close(errCh)
	close(handles)

	for err := range errCh {
		t.Errorf("concurrent create error: %v", err)
	}

	for handle := range handles {
		AudioEncoderDestroy(handle)
	}
}

func TestConcurrent_PeerConnectionCreate(t *testing.T) {
	const numGoroutines = 5

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)
	handles := make(chan uintptr, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := &PeerConnectionConfig{}
			handle, err := CreatePeerConnection(cfg)
			if err != nil {
				errCh <- err
				return
			}
			handles <- handle
		}()
	}

	wg.Wait()
	close(errCh)
	close(handles)

	for err := range errCh {
		t.Errorf("concurrent create error: %v", err)
	}

	for handle := range handles {
		PeerConnectionDestroy(handle)
	}
}

func TestConcurrent_PeerConnectionOperations(t *testing.T) {
	cfg := &PeerConnectionConfig{}
	handle, err := CreatePeerConnection(cfg)
	if err != nil {
		t.Fatalf("create peer connection: %v", err)
	}
	defer PeerConnectionDestroy(handle)

	var wg sync.WaitGroup

	// Concurrent state queries
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = PeerConnectionSignalingState(handle)
			_ = PeerConnectionConnectionState(handle)
			_ = PeerConnectionICEConnectionState(handle)
			_ = PeerConnectionICEGatheringState(handle)
		}
	}()

	// Concurrent offer creation (this may fail due to state, but shouldn't crash)
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 64*1024)
		for i := 0; i < 5; i++ {
			PeerConnectionCreateOffer(handle, buf)
		}
	}()

	// Concurrent track additions
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			_ = PeerConnectionAddTrack(handle, CodecVP8, "video-"+string(rune('0'+i)), "stream-0")
		}
	}()

	wg.Wait()
	// Success = no deadlock, no panic
}

func TestConcurrent_CallbackRegistration(t *testing.T) {
	const numGoroutines = 10

	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			trackID := uintptr(id)

			for j := 0; j < 10; j++ {
				RegisterVideoCallback(trackID, func(width, height int, yPlane, uPlane, vPlane []byte, yStride, uStride, vStride int, timestampUs int64) {
					// no-op callback
				})
				UnregisterVideoCallback(trackID)
			}
		}(i)
	}

	wg.Wait()
}

func TestConcurrent_MixedOperations(t *testing.T) {
	// Create resources
	encCfg := &VideoEncoderConfig{
		Width:      320,
		Height:     240,
		BitrateBps: 500_000,
		Framerate:  30,
	}
	encoder, err := CreateVideoEncoder(CodecVP8, encCfg)
	if err != nil {
		t.Fatalf("create encoder: %v", err)
	}
	defer VideoEncoderDestroy(encoder)

	decoder, err := CreateVideoDecoder(CodecVP8)
	if err != nil {
		t.Fatalf("create decoder: %v", err)
	}
	defer VideoDecoderDestroy(decoder)

	pcCfg := &PeerConnectionConfig{}
	pc, err := CreatePeerConnection(pcCfg)
	if err != nil {
		t.Fatalf("create peer connection: %v", err)
	}
	defer PeerConnectionDestroy(pc)

	// Test frame data
	width, height := 320, 240
	yPlane := make([]byte, width*height)
	uPlane := make([]byte, (width/2)*(height/2))
	vPlane := make([]byte, (width/2)*(height/2))
	for i := range yPlane {
		yPlane[i] = 128
	}
	for i := range uPlane {
		uPlane[i] = 128
		vPlane[i] = 128
	}

	var wg sync.WaitGroup

	// Encoding goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		dst := make([]byte, width*height*2)
		for i := 0; i < 20; i++ {
			VideoEncoderEncodeInto(encoder, yPlane, uPlane, vPlane, width, width/2, width/2, uint32(i), i == 0, dst)
		}
	}()

	// Bitrate changes goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			VideoEncoderSetBitrate(encoder, 500_000+uint32(i)*100_000)
		}
	}()

	// PC state queries goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = PeerConnectionConnectionState(pc)
		}
	}()

	wg.Wait()
}
