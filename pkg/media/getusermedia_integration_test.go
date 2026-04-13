package media

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/internal/testutil"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
)

type syntheticVideoCapture struct {
	deviceID string
	width    int
	height   int
	fps      int

	mu      sync.Mutex
	stopCh  chan struct{}
	stopped bool
}

func newSyntheticVideoCapture(deviceID string, width, height, fps int) *syntheticVideoCapture {
	return &syntheticVideoCapture{
		deviceID: deviceID,
		width:    width,
		height:   height,
		fps:      fps,
		stopCh:   make(chan struct{}),
	}
}

func (c *syntheticVideoCapture) Start(callback ffi.VideoCaptureCallback) error {
	interval := time.Second / 30
	if c.fps > 0 {
		interval = time.Second / time.Duration(c.fps)
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for frameIndex := int64(0); ; frameIndex++ {
			frame := testutil.CreateTestVideoFrame(c.width, c.height)
			if len(frame.Data) > 0 && len(frame.Data[0]) > 0 {
				frame.Data[0][0] = byte(len(c.deviceID) % 255)
			}
			callback(&ffi.CapturedVideoFrame{
				YPlane:      frame.Data[0],
				UPlane:      frame.Data[1],
				VPlane:      frame.Data[2],
				Width:       int32(c.width),
				Height:      int32(c.height),
				YStride:     int32(frame.Stride[0]),
				UStride:     int32(frame.Stride[1]),
				VStride:     int32(frame.Stride[2]),
				TimestampUs: frameIndex * int64(interval/time.Microsecond),
			})

			select {
			case <-ticker.C:
			case <-c.stopCh:
				return
			}
		}
	}()

	return nil
}

func (c *syntheticVideoCapture) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return
	}
	c.stopped = true
	close(c.stopCh)
}

type syntheticAudioCapture struct {
	deviceID   string
	sampleRate int
	channels   int

	mu      sync.Mutex
	stopCh  chan struct{}
	stopped bool
}

func newSyntheticAudioCapture(deviceID string, sampleRate, channels int) *syntheticAudioCapture {
	return &syntheticAudioCapture{
		deviceID:   deviceID,
		sampleRate: sampleRate,
		channels:   channels,
		stopCh:     make(chan struct{}),
	}
}

func (c *syntheticAudioCapture) Start(callback ffi.AudioCaptureCallback) error {
	const interval = 20 * time.Millisecond
	samplesPerChannel := c.sampleRate / 50

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for frameIndex := int64(0); ; frameIndex++ {
			frame := testutil.CreateTestAudioFrame(c.sampleRate, c.channels, samplesPerChannel)
			callback(&ffi.CapturedAudioFrame{
				Samples:     frame.SamplesS16(),
				NumChannels: int32(c.channels),
				SampleRate:  int32(c.sampleRate),
				TimestampUs: frameIndex * int64(interval/time.Microsecond),
			})

			select {
			case <-ticker.C:
			case <-c.stopCh:
				return
			}
		}
	}()

	return nil
}

func (c *syntheticAudioCapture) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return
	}
	c.stopped = true
	close(c.stopCh)
}

func TestGetUserMediaFacingModeSelectsMatchingCameraAndFlowsToPion(t *testing.T) {
	testutil.RequireShim(t)

	installMediaFFIStubs(t, mediaFFIStubs{
		loadLibrary: func() error { return nil },
		enumerateDevice: func() ([]ffi.DeviceInfo, error) {
			return []ffi.DeviceInfo{
				{DeviceID: "camera-front", Label: "Front Camera", Kind: ffi.DeviceKindVideoInput},
				{DeviceID: "camera-rear", Label: "Rear Camera", Kind: ffi.DeviceKindVideoInput},
			}, nil
		},
		newVideo: func(deviceID string, width, height, fps int) (videoCaptureHandle, error) {
			return newSyntheticVideoCapture(deviceID, width, height, fps), nil
		},
	})

	stream, err := GetUserMedia(Constraints{
		Video: &VideoConstraints{
			FacingMode: FacingModeEnvironment,
			Width:      IdealInt(320),
			Height:     IdealInt(240),
			FrameRate:  ExactFloat(10),
			Codec:      codec.VP8,
		},
	})
	if err != nil {
		t.Fatalf("GetUserMedia() error = %v", err)
	}
	defer stopSyntheticMediaStreamTracks(stream)

	videoTracks := stream.GetVideoTracks()
	if len(videoTracks) != 1 {
		t.Fatalf("GetVideoTracks() len = %d, want 1", len(videoTracks))
	}

	settings := videoTracks[0].GetSettings()
	if settings.DeviceID != "camera-rear" {
		t.Fatalf("GetSettings().DeviceID = %q, want %q", settings.DeviceID, "camera-rear")
	}
	if settings.FacingMode != FacingModeEnvironment {
		t.Fatalf("GetSettings().FacingMode = %q, want %q", settings.FacingMode, FacingModeEnvironment)
	}
	if videoTracks[0].Label() != "Rear Camera" {
		t.Fatalf("Label() = %q, want %q", videoTracks[0].Label(), "Rear Camera")
	}

	remoteTrack, packetCount := requireSyntheticMediaStreamInterop(t, stream)
	if got := remoteTrack.Kind(); got != webrtc.RTPCodecTypeVideo {
		t.Fatalf("remote track kind = %v, want %v", got, webrtc.RTPCodecTypeVideo)
	}
	if got := remoteTrack.StreamID(); got != stream.ID() {
		t.Fatalf("remote StreamID() = %q, want %q", got, stream.ID())
	}
	if packetCount.Load() == 0 {
		t.Fatal("remote RTP packet count = 0, want synthetic capture to flow end to end")
	}
}

func TestGetUserMediaFallsBackToDefaultDevicesWhenEnumerationIsEmpty(t *testing.T) {
	testutil.RequireShim(t)

	var videoDeviceID string
	var audioDeviceID string

	installMediaFFIStubs(t, mediaFFIStubs{
		loadLibrary: func() error { return nil },
		enumerateDevice: func() ([]ffi.DeviceInfo, error) {
			return nil, nil
		},
		newVideo: func(deviceID string, width, height, fps int) (videoCaptureHandle, error) {
			videoDeviceID = deviceID
			return newSyntheticVideoCapture(deviceID, width, height, fps), nil
		},
		newAudio: func(deviceID string, sampleRate, channels int) (audioCaptureHandle, error) {
			audioDeviceID = deviceID
			return newSyntheticAudioCapture(deviceID, sampleRate, channels), nil
		},
	})

	stream, err := GetUserMedia(Constraints{
		Video: &VideoConstraints{
			Width:     ExactInt(320),
			Height:    ExactInt(240),
			FrameRate: ExactFloat(10),
			Codec:     codec.VP8,
		},
		Audio: &AudioConstraints{
			SampleRate:   ExactInt(48_000),
			ChannelCount: ExactInt(2),
		},
	})
	if err != nil {
		t.Fatalf("GetUserMedia() error = %v", err)
	}
	defer stopSyntheticMediaStreamTracks(stream)

	if videoDeviceID != "" {
		t.Fatalf("newVideoCapture deviceID = %q, want empty default-device selection", videoDeviceID)
	}
	if audioDeviceID != "" {
		t.Fatalf("newAudioCapture deviceID = %q, want empty default-device selection", audioDeviceID)
	}

	videoTracks := stream.GetVideoTracks()
	audioTracks := stream.GetAudioTracks()
	if len(videoTracks) != 1 || len(audioTracks) != 1 {
		t.Fatalf("track counts = (%d video, %d audio), want (1, 1)", len(videoTracks), len(audioTracks))
	}
	if got := videoTracks[0].Label(); got != "camera" {
		t.Fatalf("video Label() = %q, want %q", got, "camera")
	}
	if got := audioTracks[0].Label(); got != "microphone" {
		t.Fatalf("audio Label() = %q, want %q", got, "microphone")
	}
	if got := videoTracks[0].GetSettings().DeviceID; got != "" {
		t.Fatalf("video DeviceID = %q, want empty default-device ID", got)
	}
	if got := audioTracks[0].GetSettings().DeviceID; got != "" {
		t.Fatalf("audio DeviceID = %q, want empty default-device ID", got)
	}

	remoteTracks, packetCounts := requireSyntheticMediaStreamInteropKinds(
		t,
		stream,
		[]webrtc.RTPCodecType{webrtc.RTPCodecTypeAudio, webrtc.RTPCodecTypeVideo},
	)
	for _, kind := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeAudio, webrtc.RTPCodecTypeVideo} {
		if packetCounts[kind].Load() == 0 {
			t.Fatalf("remote %v RTP packet count = 0, want fallback capture to flow end to end", kind)
		}
		if got := remoteTracks[kind].StreamID(); got != stream.ID() {
			t.Fatalf("remote %v StreamID() = %q, want %q", kind, got, stream.ID())
		}
	}
}

func TestGetUserMediaDoesNotReportRequestedFacingModeWhenEnumerationIsEmpty(t *testing.T) {
	testutil.RequireShim(t)

	installMediaFFIStubs(t, mediaFFIStubs{
		loadLibrary: func() error { return nil },
		enumerateDevice: func() ([]ffi.DeviceInfo, error) {
			return nil, nil
		},
		newVideo: func(deviceID string, width, height, fps int) (videoCaptureHandle, error) {
			return newSyntheticVideoCapture(deviceID, width, height, fps), nil
		},
	})

	stream, err := GetUserMedia(Constraints{
		Video: &VideoConstraints{
			FacingMode: FacingModeEnvironment,
			Width:      ExactInt(320),
			Height:     ExactInt(240),
			FrameRate:  ExactFloat(10),
			Codec:      codec.VP8,
		},
	})
	if err != nil {
		t.Fatalf("GetUserMedia() error = %v", err)
	}
	defer stopSyntheticMediaStreamTracks(stream)

	videoTracks := stream.GetVideoTracks()
	if len(videoTracks) != 1 {
		t.Fatalf("GetVideoTracks() len = %d, want 1", len(videoTracks))
	}

	settings := videoTracks[0].GetSettings()
	if got := settings.FacingMode; got != "" {
		t.Fatalf("GetSettings().FacingMode = %q, want empty when fallback device direction is unknown", got)
	}
	if got := videoTracks[0].GetCapabilities().FacingMode; len(got) != 0 {
		t.Fatalf("GetCapabilities().FacingMode = %v, want empty when fallback device direction is unknown", got)
	}

	remoteTrack, packetCount := requireSyntheticMediaStreamInterop(t, stream)
	if got := remoteTrack.Kind(); got != webrtc.RTPCodecTypeVideo {
		t.Fatalf("remote track kind = %v, want %v", got, webrtc.RTPCodecTypeVideo)
	}
	if got := remoteTrack.StreamID(); got != stream.ID() {
		t.Fatalf("remote StreamID() = %q, want %q", got, stream.ID())
	}
	if packetCount.Load() == 0 {
		t.Fatal("remote RTP packet count = 0, want fallback capture to flow end to end")
	}
}

func requireSyntheticMediaStreamInterop(t *testing.T, stream *MediaStream) (*webrtc.TrackRemote, *atomic.Int64) {
	t.Helper()

	kind := singleSyntheticTrackKind(t, stream)
	remoteTracks, packetCounts := requireSyntheticMediaStreamInteropKinds(t, stream, []webrtc.RTPCodecType{kind})
	return remoteTracks[kind], packetCounts[kind]
}

func requireSyntheticMediaStreamInteropKinds(t *testing.T, stream *MediaStream, expectedKinds []webrtc.RTPCodecType) (map[webrtc.RTPCodecType]*webrtc.TrackRemote, map[webrtc.RTPCodecType]*atomic.Int64) {
	t.Helper()

	sender := newLoopbackPionPeerConnection(t)
	defer func() { _ = sender.Close() }()

	receiver := newLoopbackPionPeerConnection(t)
	defer func() { _ = receiver.Close() }()

	remoteTracks := make(map[webrtc.RTPCodecType]*webrtc.TrackRemote, len(expectedKinds))
	packetCounts := make(map[webrtc.RTPCodecType]*atomic.Int64, len(expectedKinds))
	packetCh := make(chan webrtc.RTPCodecType, len(expectedKinds))
	var mu sync.Mutex

	receiver.OnTrack(func(track *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) {
		kind := track.Kind()
		mu.Lock()
		packetCount, ok := packetCounts[kind]
		if !ok {
			packetCount = &atomic.Int64{}
			packetCounts[kind] = packetCount
		}
		if _, ok := remoteTracks[kind]; !ok {
			remoteTracks[kind] = track
		}
		mu.Unlock()

		go func() {
			for {
				_, _, err := track.ReadRTP()
				if err != nil {
					return
				}
				packetCount.Add(1)
				select {
				case packetCh <- kind:
				default:
				}
			}
		}()
	})

	senders, err := AddTracksToPionPeerConnection(sender, stream)
	if err != nil {
		t.Fatalf("AddTracksToPionPeerConnection() error = %v", err)
	}
	if len(senders) != len(expectedKinds) {
		t.Fatalf("AddTracksToPionPeerConnection() senders len = %d, want %d", len(senders), len(expectedKinds))
	}
	for _, rtpSender := range senders {
		go drainRemoteRegistryRTCP(rtpSender)
	}

	connectRemoteRegistryPionPeers(t, sender, receiver)

	for _, kind := range expectedKinds {
		deadline := time.After(10 * time.Second)
		for {
			mu.Lock()
			remoteTrack := remoteTracks[kind]
			packetCount := packetCounts[kind]
			mu.Unlock()
			if remoteTrack != nil && packetCount != nil && packetCount.Load() > 0 {
				break
			}
			select {
			case <-packetCh:
			case <-deadline:
				t.Fatalf("timed out waiting for remote %v track from GetUserMedia stream", kind)
			}
		}
	}

	return remoteTracks, packetCounts
}

func singleSyntheticTrackKind(t *testing.T, stream *MediaStream) webrtc.RTPCodecType {
	t.Helper()

	videoTracks := stream.GetVideoTracks()
	audioTracks := stream.GetAudioTracks()
	switch {
	case len(videoTracks) == 1 && len(audioTracks) == 0:
		return webrtc.RTPCodecTypeVideo
	case len(videoTracks) == 0 && len(audioTracks) == 1:
		return webrtc.RTPCodecTypeAudio
	default:
		t.Fatalf("stream has %d video and %d audio tracks, want exactly one total track", len(videoTracks), len(audioTracks))
		return webrtc.RTPCodecType(0)
	}
}

func stopSyntheticMediaStreamTracks(stream *MediaStream) {
	if stream == nil {
		return
	}
	for _, mediaTrack := range stream.GetTracks() {
		mediaTrack.Stop()
	}
}
