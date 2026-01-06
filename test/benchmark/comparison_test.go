package benchmark

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/pc"

	pionwebrtc "github.com/pion/webrtc/v4"
)

// TestMain initializes the libwebrtc library for benchmarks.
func TestMain(m *testing.M) {
	if err := ffi.LoadLibrary(); err != nil {
		// Skip if library not available
		return
	}
	defer ffi.Close()
	m.Run()
}

// ============================================================================
// PeerConnection Creation Benchmarks
// ============================================================================

// BenchmarkLibwebrtcPeerConnectionCreate benchmarks libwebrtc PC creation.
func BenchmarkLibwebrtcPeerConnectionCreate(b *testing.B) {
	cfg := pc.DefaultConfiguration()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pcConn, _ := pc.NewPeerConnection(cfg)
		pcConn.Close()
	}
}

// BenchmarkPionPeerConnectionCreate benchmarks pion PC creation.
func BenchmarkPionPeerConnectionCreate(b *testing.B) {
	cfg := pionwebrtc.Configuration{}
	api := pionwebrtc.NewAPI()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pcConn, _ := api.NewPeerConnection(cfg)
		pcConn.Close()
	}
}

// ============================================================================
// Offer Creation Benchmarks
// ============================================================================

// BenchmarkLibwebrtcCreateOffer benchmarks libwebrtc offer creation.
func BenchmarkLibwebrtcCreateOffer(b *testing.B) {
	cfg := pc.DefaultConfiguration()
	pcConn, _ := pc.NewPeerConnection(cfg)
	defer pcConn.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pcConn.CreateOffer(nil)
	}
}

// BenchmarkPionCreateOffer benchmarks pion offer creation.
func BenchmarkPionCreateOffer(b *testing.B) {
	cfg := pionwebrtc.Configuration{}
	api := pionwebrtc.NewAPI()
	pcConn, _ := api.NewPeerConnection(cfg)
	defer pcConn.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pcConn.CreateOffer(nil)
	}
}

// ============================================================================
// Full Offer/Answer Exchange Benchmarks
// ============================================================================

// BenchmarkLibwebrtcOfferAnswer benchmarks libwebrtc offer/answer exchange.
func BenchmarkLibwebrtcOfferAnswer(b *testing.B) {
	cfg := pc.DefaultConfiguration()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pc1, _ := pc.NewPeerConnection(cfg)
		pc2, _ := pc.NewPeerConnection(cfg)

		offer, _ := pc1.CreateOffer(nil)
		pc1.SetLocalDescription(offer)
		pc2.SetRemoteDescription(offer)

		answer, _ := pc2.CreateAnswer(nil)
		pc2.SetLocalDescription(answer)
		pc1.SetRemoteDescription(answer)

		pc1.Close()
		pc2.Close()
	}
}

// BenchmarkPionOfferAnswer benchmarks pion offer/answer exchange.
func BenchmarkPionOfferAnswer(b *testing.B) {
	cfg := pionwebrtc.Configuration{}
	api := pionwebrtc.NewAPI()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pc1, _ := api.NewPeerConnection(cfg)
		pc2, _ := api.NewPeerConnection(cfg)

		offer, _ := pc1.CreateOffer(nil)
		pc1.SetLocalDescription(offer)
		pc2.SetRemoteDescription(offer)

		answer, _ := pc2.CreateAnswer(nil)
		pc2.SetLocalDescription(answer)
		pc1.SetRemoteDescription(answer)

		pc1.Close()
		pc2.Close()
	}
}

// ============================================================================
// DataChannel Creation Benchmarks
// ============================================================================

// BenchmarkLibwebrtcDataChannelCreate benchmarks libwebrtc DC creation.
func BenchmarkLibwebrtcDataChannelCreate(b *testing.B) {
	cfg := pc.DefaultConfiguration()
	pcConn, _ := pc.NewPeerConnection(cfg)
	defer pcConn.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dc, _ := pcConn.CreateDataChannel("bench-dc", nil)
		_ = dc
	}
}

// BenchmarkPionDataChannelCreate benchmarks pion DC creation.
func BenchmarkPionDataChannelCreate(b *testing.B) {
	cfg := pionwebrtc.Configuration{}
	api := pionwebrtc.NewAPI()
	pcConn, _ := api.NewPeerConnection(cfg)
	defer pcConn.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dc, _ := pcConn.CreateDataChannel("bench-dc", nil)
		_ = dc
	}
}

// ============================================================================
// AddTrack Benchmarks
// ============================================================================

// BenchmarkLibwebrtcAddTrack benchmarks libwebrtc add track.
func BenchmarkLibwebrtcAddTrack(b *testing.B) {
	cfg := pc.DefaultConfiguration()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		pcConn, _ := pc.NewPeerConnection(cfg)
		b.StartTimer()

		// Add a track
		track, _ := pcConn.CreateVideoTrack("video-0", codec.H264, 640, 480)
		pcConn.AddTrack(track, "stream-0")

		b.StopTimer()
		pcConn.Close()
		b.StartTimer()
	}
}

// BenchmarkPionAddTrack benchmarks pion add track.
func BenchmarkPionAddTrack(b *testing.B) {
	cfg := pionwebrtc.Configuration{}
	api := pionwebrtc.NewAPI()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		pcConn, _ := api.NewPeerConnection(cfg)
		track, _ := pionwebrtc.NewTrackLocalStaticSample(
			pionwebrtc.RTPCodecCapability{MimeType: pionwebrtc.MimeTypeH264},
			"video-0",
			"stream-0",
		)
		b.StartTimer()

		pcConn.AddTrack(track)

		b.StopTimer()
		pcConn.Close()
		b.StartTimer()
	}
}

// ============================================================================
// FFI-level Benchmarks (libwebrtc only)
// ============================================================================

// BenchmarkFFIPeerConnectionCreate benchmarks raw FFI PC creation.
func BenchmarkFFIPeerConnectionCreate(b *testing.B) {
	cfg := &ffi.PeerConnectionConfig{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handle := ffi.CreatePeerConnection(cfg)
		if handle != 0 {
			ffi.PeerConnectionDestroy(handle)
		}
	}
}

// BenchmarkFFICreateOffer benchmarks raw FFI offer creation.
func BenchmarkFFICreateOffer(b *testing.B) {
	cfg := &ffi.PeerConnectionConfig{}
	handle := ffi.CreatePeerConnection(cfg)
	if handle == 0 {
		b.Fatal("Failed to create PC")
	}
	defer ffi.PeerConnectionDestroy(handle)

	sdpBuf := make([]byte, 64*1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ffi.PeerConnectionCreateOffer(handle, sdpBuf)
	}
}

// ============================================================================
// HOT PATH: Video Frame Push Benchmarks
// ============================================================================

// BenchmarkFFIVideoPushFrame benchmarks raw FFI video frame push (hot path).
func BenchmarkFFIVideoPushFrame(b *testing.B) {
	cfg := &ffi.PeerConnectionConfig{}
	handle := ffi.CreatePeerConnection(cfg)
	if handle == 0 {
		b.Fatal("Failed to create PC")
	}
	defer ffi.PeerConnectionDestroy(handle)

	// Create track source
	source := ffi.VideoTrackSourceCreate(handle, 640, 480)
	if source == 0 {
		b.Fatal("Failed to create video track source")
	}
	defer ffi.VideoTrackSourceDestroy(source)

	// Create I420 frame data (640x480)
	width, height := 640, 480
	ySize := width * height
	uvSize := (width / 2) * (height / 2)
	yPlane := make([]byte, ySize)
	uPlane := make([]byte, uvSize)
	vPlane := make([]byte, uvSize)

	// Fill with test pattern
	for i := range yPlane {
		yPlane[i] = byte(i % 256)
	}
	for i := range uPlane {
		uPlane[i] = 128
		vPlane[i] = 128
	}

	b.SetBytes(int64(ySize + uvSize*2))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ffi.VideoTrackSourcePushFrame(source, yPlane, uPlane, vPlane, width, width/2, width/2, int64(i)*33333)
	}
}

// BenchmarkPionVideoWriteSample benchmarks pion video sample write (hot path).
func BenchmarkPionVideoWriteSample(b *testing.B) {
	// Create a track
	track, err := pionwebrtc.NewTrackLocalStaticSample(
		pionwebrtc.RTPCodecCapability{MimeType: pionwebrtc.MimeTypeVP8},
		"video",
		"stream",
	)
	if err != nil {
		b.Fatal(err)
	}

	// Create sample data (simulating encoded frame ~10KB)
	sampleData := make([]byte, 10*1024)
	for i := range sampleData {
		sampleData[i] = byte(i % 256)
	}

	b.SetBytes(int64(len(sampleData)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Note: WriteSample without active connection just queues
		_ = track.WriteSample(media.Sample{
			Data:     sampleData,
			Duration: 33 * time.Millisecond,
		})
	}
}

// ============================================================================
// HOT PATH: Audio Frame Push Benchmarks
// ============================================================================

// BenchmarkFFIAudioPushFrame benchmarks raw FFI audio frame push (hot path).
func BenchmarkFFIAudioPushFrame(b *testing.B) {
	cfg := &ffi.PeerConnectionConfig{}
	handle := ffi.CreatePeerConnection(cfg)
	if handle == 0 {
		b.Fatal("Failed to create PC")
	}
	defer ffi.PeerConnectionDestroy(handle)

	// Create audio track source (48kHz stereo)
	source := ffi.AudioTrackSourceCreate(handle, 48000, 2)
	if source == 0 {
		b.Fatal("Failed to create audio track source")
	}
	defer ffi.AudioTrackSourceDestroy(source)

	// Create 10ms of audio at 48kHz stereo = 480 samples * 2 channels
	samples := make([]int16, 480*2)
	for i := range samples {
		samples[i] = int16(i % 32767)
	}

	b.SetBytes(int64(len(samples) * 2)) // 2 bytes per sample
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ffi.AudioTrackSourcePushFrame(source, samples, int64(i)*10000)
	}
}

// ============================================================================
// HOT PATH: DataChannel Send Benchmarks
// ============================================================================

// BenchmarkFFIDataChannelSend benchmarks raw FFI data channel send (hot path).
func BenchmarkFFIDataChannelSend(b *testing.B) {
	cfg := &ffi.PeerConnectionConfig{}
	handle := ffi.CreatePeerConnection(cfg)
	if handle == 0 {
		b.Fatal("Failed to create PC")
	}
	defer ffi.PeerConnectionDestroy(handle)

	// Create data channel
	dcHandle := ffi.PeerConnectionCreateDataChannel(handle, "bench", true, -1, "")
	if dcHandle == 0 {
		b.Fatal("Failed to create data channel")
	}

	// Test data (1KB message)
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Note: Send will fail without connection, but we're measuring the call overhead
		_ = ffi.DataChannelSend(dcHandle, data, true)
	}
}

// BenchmarkPionDataChannelSend benchmarks pion data channel send (hot path).
func BenchmarkPionDataChannelSend(b *testing.B) {
	cfg := pionwebrtc.Configuration{}
	api := pionwebrtc.NewAPI()
	pcConn, err := api.NewPeerConnection(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer pcConn.Close()

	dc, err := pcConn.CreateDataChannel("bench", nil)
	if err != nil {
		b.Fatal(err)
	}

	// Test data (1KB message)
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Note: Send will fail without connection, but we're measuring the call overhead
		_ = dc.Send(data)
	}
}
