# libgowebrtc: Pion-Compatible libwebrtc Wrapper

## Goal
Create a Go library wrapping libwebrtc for encode/decode/packetize with a **Pion-compatible interface** - no networking, just media processing. Uses **purego** instead of CGO for cleaner builds.

## Design Principles

1. **Allocation-free hot paths** - All encode/decode operations write into caller-provided buffers
2. **Single way to do things** - No convenience methods, one clear API
3. **Pion interoperability** - Implements Pion interfaces for seamless integration
4. **Browser-compatible API** - API should be flexible enough to do everything browser WebRTC can do (offer/answer, ICE, tracks, data channels, transceivers, simulcast, SVC)
5. **Modular & maintainable** - Code organized into small, focused modules with clear responsibilities. Test helpers abstract common patterns.

## Code Organization Principles

- **Small focused files** - Each file has a single responsibility
- **Test helpers** - Common test patterns extracted to reusable helpers (see `test/interop/helpers.go`)
- **Minimal dependencies** - Each package imports only what it needs
- **Clear interfaces** - Public APIs are well-defined interfaces, implementations are internal
- **Error handling** - Typed errors with clear messages, no panics in library code

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Go Application                           │
├─────────────────────────────────────────────────────────────────┤
│  libgowebrtc (Go)                                               │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │ track.Local  │  │ encoder/     │  │ packetizer/          │  │
│  │ (implements  │  │ decoder      │  │ depacketizer         │  │
│  │ webrtc.      │  │              │  │                      │  │
│  │ TrackLocal)  │  └──────────────┘  └──────────────────────┘  │
│  └──────────────┘                                               │
│         │                    │                    │             │
│         └────────────────────┼────────────────────┘             │
│                              │                                  │
│                     ┌────────▼────────┐                         │
│                     │  internal/ffi   │  ← purego bindings      │
│                     └────────┬────────┘                         │
└──────────────────────────────┼──────────────────────────────────┘
                               │ dlopen/dlsym
                    ┌──────────▼──────────┐
                    │  libwebrtc_shim.so  │  ← C wrapper (pre-built)
                    │  (C API over C++)   │
                    └──────────┬──────────┘
                               │
                    ┌──────────▼──────────┐
                    │     libwebrtc       │  ← Google's WebRTC
                    └─────────────────────┘
```

## Project Structure

```
libgowebrtc/
├── go.mod
├── PLAN.md
├── .gitignore
│
├── pkg/
│   ├── codec/                    # Codec types and constants
│   │   └── codec.go
│   │
│   ├── frame/                    # Frame types
│   │   ├── video.go              # VideoFrame (I420, NV12)
│   │   └── audio.go              # AudioFrame (PCM)
│   │
│   ├── encoder/                  # Encoders (allocation-free)
│   │   ├── encoder.go            # VideoEncoder, AudioEncoder interfaces
│   │   ├── h264.go
│   │   ├── vp8.go
│   │   ├── vp9.go
│   │   ├── av1.go
│   │   └── opus.go
│   │
│   ├── decoder/                  # Decoders (allocation-free)
│   │   ├── decoder.go
│   │   ├── h264.go
│   │   ├── vp8.go
│   │   ├── vp9.go
│   │   ├── av1.go
│   │   └── opus.go
│   │
│   ├── packetizer/
│   │   └── packetizer.go
│   │
│   ├── depacketizer/
│   │   └── depacketizer.go
│   │
│   └── track/
│       └── local.go              # Implements webrtc.TrackLocal
│
├── internal/
│   ├── ffi/                      # purego FFI bindings
│   │   ├── lib.go
│   │   ├── types.go
│   │   ├── encoder.go
│   │   └── decoder.go
│   │
│   └── pool/
│       └── pool.go
│
├── shim/
│   ├── shim.h                    # C API (allocation-free design)
│   └── ...
│
├── lib/                          # Pre-built binaries (gitignored)
│
└── examples/
```

## Core Interfaces (Allocation-Free)

### VideoEncoder

```go
type VideoEncoder interface {
    // EncodeInto encodes a video frame into the destination buffer.
    // Caller must provide buffer of at least MaxEncodedSize() bytes.
    EncodeInto(src *frame.VideoFrame, dst []byte, forceKeyframe bool) (EncodeResult, error)

    // MaxEncodedSize returns max possible encoded size for configured resolution.
    MaxEncodedSize() int

    SetBitrate(bps uint32) error
    SetFramerate(fps float64) error
    RequestKeyFrame()
    Codec() codec.Type
    Close() error
}

type EncodeResult struct {
    N          int  // bytes written
    IsKeyframe bool
}
```

### VideoDecoder

```go
type VideoDecoder interface {
    // DecodeInto decodes into pre-allocated frame.
    // dst.Data must have sufficient space (use frame.NewI420Frame).
    DecodeInto(src []byte, dst *frame.VideoFrame, timestamp uint32, isKeyframe bool) error

    Codec() codec.Type
    Close() error
}
```

### AudioEncoder

```go
type AudioEncoder interface {
    // EncodeInto encodes audio samples into the destination buffer.
    EncodeInto(src *frame.AudioFrame, dst []byte) (n int, err error)

    // MaxEncodedSize returns max possible encoded size for a single frame.
    MaxEncodedSize() int

    SetBitrate(bps uint32) error
    Codec() codec.Type
    Close() error
}
```

### AudioDecoder

```go
type AudioDecoder interface {
    // DecodeInto decodes into pre-allocated frame.
    DecodeInto(src []byte, dst *frame.AudioFrame) (numSamples int, err error)

    // MaxSamplesPerFrame returns max samples per channel from one encoded frame.
    MaxSamplesPerFrame() int

    Codec() codec.Type
    Close() error
}
```

## Codec Configuration Options

### H.264 Config
```go
type H264Config struct {
    Width, Height int              // Resolution
    Bitrate       uint32           // Target bitrate (0 = auto)
    MaxBitrate    uint32           // VBR max (0 = 1.5x)
    RateControl   RateControlMode  // CBR, VBR, CQ
    FPS           float64          // Framerate (0 = 30)
    KeyInterval   int              // Keyframe interval
    Profile       H264Profile      // Baseline, Main, High
    CRF           int              // CQ quality (0-51)
    LowDelay      bool             // Low latency mode
    ZeroLatency   bool             // Ultra low latency
    PreferHW      bool             // Hardware encoder
}
```

### VP8/VP9/AV1 Config
```go
// Common options across VP8/VP9/AV1:
Width, Height   int              // Resolution
Bitrate         uint32           // Target bitrate
RateControl     RateControlMode  // CBR, VBR, CQ
FPS             float64          // Framerate
KeyInterval     int              // Keyframe interval
CQ              int              // Quality (0-63)
Speed           int              // Encode speed (0-10)
LowDelay        bool             // Low latency
PreferHW        bool             // Hardware encoder

// VP9/AV1 specific:
Profile         VP9Profile/AV1Profile
TileColumns     int              // Parallel tiles
TileRows        int
FrameParallel   bool
SvcEnabled      bool             // Scalable video coding
SvcNumLayers    int
```

### Opus Config
```go
type OpusConfig struct {
    SampleRate  int              // 8000-48000
    Channels    int              // 1 or 2
    Bitrate     uint32           // 6000-510000 bps
    VBR         bool             // Variable bitrate
    Application OpusApplication  // VoIP, Audio, LowDelay
    Bandwidth   OpusBandwidth    // Narrow to Full
    Complexity  int              // 0-10
    FrameSize   float64          // 2.5-60ms
    FEC         bool             // Forward error correction
    DTX         bool             // Silence suppression
    InBandFEC   bool             // Packet loss recovery
    PacketLoss  int              // Expected loss %
}
```

### Default Configurations
```go
// Sensible defaults with auto bitrate estimation:
codec.DefaultH264Config(1280, 720)  // H.264 720p
codec.DefaultVP8Config(1920, 1080)  // VP8 1080p
codec.DefaultVP9Config(1280, 720)   // VP9 720p
codec.DefaultAV1Config(1920, 1080)  // AV1 1080p
codec.DefaultOpusConfig()            // Opus 48kHz stereo
```

## Runtime Controls

Encoders support runtime adjustments without recreation. Use type assertion for advanced controls.

### Video Encoder Runtime API
```go
// Core runtime controls (all video encoders)
enc.SetBitrate(2_000_000)     // Change bitrate
enc.SetFramerate(60)           // Change framerate
enc.RequestKeyFrame()          // Force next frame to be keyframe

// Advanced controls (use type assertion)
if adv, ok := enc.(encoder.VideoEncoderAdvanced); ok {
    adv.SetQuality(23)                       // CQ mode quality
    adv.SetKeyInterval(120)                  // Keyframe every 4s at 30fps
    adv.SetRateControl(codec.RateControlVBR) // Switch rate control
    stats := adv.Stats()                     // Get encoder statistics
}
```

### Audio Encoder Runtime API
```go
// Core runtime controls (all audio encoders)
enc.SetBitrate(128_000)  // Change bitrate

// Advanced Opus controls (use type assertion)
if adv, ok := enc.(encoder.AudioEncoderAdvanced); ok {
    adv.SetComplexity(10)          // Max quality
    adv.SetFEC(true)               // Enable forward error correction
    adv.SetDTX(true)               // Enable discontinuous transmission
    adv.SetPacketLossPercentage(5) // Tune FEC for 5% packet loss
    adv.SetBandwidth(codec.OpusBandwidthFull)
}
```

### Encoder Statistics
```go
type EncoderStats struct {
    FramesEncoded   uint64  // Total frames encoded
    BytesEncoded    uint64  // Total bytes produced
    KeyframesForced uint32  // Keyframes from RequestKeyFrame
    AvgBitrate      uint32  // Average bitrate in bps
    AvgFrameSize    uint32  // Average encoded frame size
    AvgEncodeTimeUs uint32  // Average encode time in microseconds
}
```

## SVC & Simulcast

Scalable Video Coding allows a single encoder to produce multiple quality layers, enabling SFUs to forward appropriate layers without transcoding.

### SVC Modes
```go
// Standard SVC (inter-layer prediction)
SVCModeL1T1  // No SVC (baseline)
SVCModeL1T2  // 1 spatial, 2 temporal layers
SVCModeL1T3  // 1 spatial, 3 temporal layers
SVCModeL2T3  // 2 spatial, 3 temporal layers
SVCModeL3T3  // 3 spatial, 3 temporal layers (full SVC)

// K-SVC (Key-frame dependent - best for SFU)
SVCModeL2T3_KEY  // 2 spatial, 3 temporal (K-SVC)
SVCModeL3T3_KEY  // 3 spatial, 3 temporal (K-SVC, Chrome default)

// Simulcast (separate independent encoders)
SVCModeS2T1  // 2 simulcast streams
SVCModeS3T3  // 3 simulcast streams with temporal layers
```

### Browser-Like Presets
```go
// Use presets that match browser behavior:
codec.SVCPresetNone()        // No SVC
codec.SVCPresetScreenShare() // L1T3 - temporal only for screen sharing
codec.SVCPresetLowLatency()  // L1T2 - minimal overhead
codec.SVCPresetSFU()         // L3T3_KEY - best for SFU forwarding
codec.SVCPresetSFULite()     // L2T3_KEY - lighter alternative
codec.SVCPresetSimulcast()   // S3T3 - classic 3-stream simulcast
codec.SVCPresetChrome()      // L3T3_KEY - matches Chrome defaults
codec.SVCPresetFirefox()     // L2T3 - matches Firefox defaults
```

### SVC Configuration Example
```go
// VP9 with Chrome-like SVC for SFU usage
enc, _ := encoder.NewVP9Encoder(codec.VP9Config{
    Width:   1280,
    Height:  720,
    Bitrate: 2_000_000,
    FPS:     30,
    SVC:     codec.SVCPresetSFU(), // L3T3_KEY
})

// AV1 with custom SVC layers
enc, _ := encoder.NewAV1Encoder(codec.AV1Config{
    Width:   1920,
    Height:  1080,
    Bitrate: 4_000_000,
    SVC: &codec.SVCConfig{
        Mode: codec.SVCModeL3T3_KEY,
        Layers: []codec.SVCLayerConfig{
            {Width: 480, Height: 270, Bitrate: 300_000, Active: true},
            {Width: 960, Height: 540, Bitrate: 1_000_000, Active: true},
            {Width: 1920, Height: 1080, Bitrate: 2_700_000, Active: true},
        },
    },
})
```

### Runtime SVC Control
```go
// Check if encoder supports SVC controls
if svc, ok := enc.(encoder.VideoEncoderSVC); ok {
    // Change SVC mode at runtime
    svc.SetSVCMode(codec.SVCModeL2T3_KEY)

    // Adjust individual layer bitrates (bandwidth adaptation)
    svc.SetLayerBitrate(0, 200_000)  // Low quality layer
    svc.SetLayerBitrate(1, 800_000)  // Medium quality
    svc.SetLayerBitrate(2, 2_000_000) // High quality

    // Disable/enable layers dynamically
    svc.SetLayerActive(2, false) // Disable highest layer

    // Request keyframe for specific layer
    svc.RequestLayerKeyFrame(0)

    // Get current layer status
    spatial, temporal := svc.GetActiveLayerCount()
}
```

## Unified Codec Creation

```go
// Type-safe per-codec constructors
enc, _ := encoder.NewH264Encoder(codec.H264Config{...})
enc, _ := encoder.NewVP9Encoder(codec.VP9Config{...})

// Decoders (codec-only, no config needed)
dec, _ := decoder.NewVideoDecoder(codec.H264)
dec, _ := decoder.NewAudioDecoder(codec.Opus)
```

## Browser-Like API

The library provides a browser-like API that mirrors the Web APIs, making it easy for developers familiar with WebRTC in browsers.

### pkg/media - MediaStream & Tracks (like getUserMedia)
```go
import "github.com/thesyncim/libgowebrtc/pkg/media"

// Just like navigator.mediaDevices.getUserMedia()
stream, _ := media.GetUserMedia(media.Constraints{
    Video: &media.VideoConstraints{
        Width:     1280,
        Height:    720,
        FrameRate: 30,
        Codec:     codec.VP9,
        SVC:       codec.SVCPresetSFU(), // L3T3_KEY for SFU
    },
    Audio: &media.AudioConstraints{
        SampleRate:       48000,
        ChannelCount:     2,
        EchoCancellation: true,
        NoiseSuppression: true,
    },
})

// Screen sharing (like getDisplayMedia)
screenStream, _ := media.GetDisplayMedia(media.DisplayConstraints{
    Video: &media.DisplayVideoConstraints{
        Width:  1920,
        Height: 1080,
        Codec:  codec.AV1,
        SVC:    codec.SVCPresetScreenShare(),
    },
})

// Access tracks like in browser
videoTracks := stream.GetVideoTracks()
audioTracks := stream.GetAudioTracks()

// Control tracks
videoTracks[0].SetEnabled(false) // Disable video
audioTracks[0].Stop()            // Stop audio track
```

### pkg/pc - Native libwebrtc PeerConnection
```go
import "github.com/thesyncim/libgowebrtc/pkg/pc"

// Create PeerConnection (backed by libwebrtc, not Pion!)
peerConnection, _ := pc.NewPeerConnection(pc.Configuration{
    ICEServers: []pc.ICEServer{
        {URLs: []string{"stun:stun.l.google.com:19302"}},
        {URLs: []string{"turn:turn.example.com:3478"},
         Username: "user", Credential: "pass"},
    },
    SDPSemantics: "unified-plan",
})

// Event handlers - exactly like browser JavaScript
peerConnection.OnICECandidate = func(candidate *pc.ICECandidate) {
    // Send candidate to remote peer via signaling
    signalingChannel.Send(candidate)
}

peerConnection.OnTrack = func(track *pc.Track, receiver *pc.RTPReceiver, streams []string) {
    // Handle incoming track
    if track.Kind() == "video" {
        // Decode and display video
    }
}

peerConnection.OnConnectionStateChange = func(state pc.PeerConnectionState) {
    fmt.Printf("Connection state: %s\n", state)
}

// Add tracks (like browser's addTrack)
videoTrack, _ := peerConnection.CreateVideoTrack("video-0", codec.VP9)
audioTrack, _ := peerConnection.CreateAudioTrack("audio-0")

sender, _ := peerConnection.AddTrack(videoTrack, "stream-0")
peerConnection.AddTrack(audioTrack, "stream-0")

// Configure SVC/simulcast per-sender
sender.SetParameters(pc.RTPSendParameters{
    Encodings: []pc.RTPEncodingParameters{
        {RID: "low", MaxBitrate: 300_000, ScaleResolutionDownBy: 4},
        {RID: "mid", MaxBitrate: 1_000_000, ScaleResolutionDownBy: 2},
        {RID: "high", MaxBitrate: 2_500_000, ScalabilityMode: "L3T3_KEY"},
    },
})

// Create offer/answer - standard WebRTC flow
offer, _ := peerConnection.CreateOffer(nil)
peerConnection.SetLocalDescription(offer)
// ... send offer via signaling, receive answer ...
peerConnection.SetRemoteDescription(answer)

// Feed raw frames to tracks
for frame := range videoSource {
    videoTrack.WriteVideoFrame(frame)
}
```

### Comparison: Browser JS vs libgowebrtc Go

| Browser JavaScript | libgowebrtc Go |
|-------------------|----------------|
| `navigator.mediaDevices.getUserMedia(constraints)` | `media.GetUserMedia(constraints)` |
| `new RTCPeerConnection(config)` | `pc.NewPeerConnection(config)` |
| `pc.addTrack(track, stream)` | `pc.AddTrack(track, "stream")` |
| `pc.createOffer()` | `pc.CreateOffer(nil)` |
| `pc.setLocalDescription(offer)` | `pc.SetLocalDescription(offer)` |
| `pc.ontrack = (e) => {}` | `pc.OnTrack = func(track, receiver, streams) {}` |
| `pc.onicecandidate = (e) => {}` | `pc.OnICECandidate = func(candidate) {}` |
| `sender.setParameters(params)` | `sender.SetParameters(params)` |
| `track.enabled = false` | `track.SetEnabled(false)` |

## State-of-the-Art Examples

### Example 1: SFU-Ready Video Conference with K-SVC
```go
// Production-ready video conferencing with SFU-optimized encoding
package main

import (
    "github.com/thesyncim/libgowebrtc/pkg/codec"
    "github.com/thesyncim/libgowebrtc/pkg/media"
    "github.com/thesyncim/libgowebrtc/pkg/pc"
)

func main() {
    // Create peer connection with TURN fallback
    peerConnection, _ := pc.NewPeerConnection(pc.Configuration{
        ICEServers: []pc.ICEServer{
            {URLs: []string{"stun:stun.l.google.com:19302"}},
            {URLs: []string{"turns:turn.example.com:443?transport=tcp"},
             Username: "user", Credential: "pass"},
        },
    })

    // Get camera+mic with SFU-optimized SVC (Chrome-like)
    stream, _ := media.GetUserMedia(media.Constraints{
        Video: &media.VideoConstraints{
            Width:     1280,
            Height:    720,
            FrameRate: 30,
            Codec:     codec.VP9,              // Best SVC support
            SVC:       codec.SVCPresetChrome(), // L3T3_KEY for SFU
        },
        Audio: &media.AudioConstraints{
            SampleRate:       48000,
            EchoCancellation: true,
            NoiseSuppression: true,
        },
    })

    // Add all tracks to peer connection
    senders, _ := media.AddTracksToPC(peerConnection, stream)

    // Configure simulcast layers for SFU on video senders
    for _, sender := range senders {
        if sender.Track().Kind() == webrtc.RTPCodecTypeVideo {
            sender.SetParameters(webrtc.RTPSendParameters{
                Encodings: []webrtc.RTPEncodingParameters{
                    {RID: "q", MaxBitrate: 150_000, ScaleResolutionDownBy: 4},  // 320x180
                    {RID: "h", MaxBitrate: 500_000, ScaleResolutionDownBy: 2},  // 640x360
                    {RID: "f", MaxBitrate: 2_500_000},                          // 1280x720
                },
            })
        }
    }

    // Handle incoming tracks (from SFU)
    peerConnection.OnTrack = func(track *pc.Track, receiver *pc.RTPReceiver, streams []string) {
        go handleRemoteTrack(track)
    }

    // Signaling...
    offer, _ := peerConnection.CreateOffer(nil)
    peerConnection.SetLocalDescription(offer)
}
```

### Example 2: Screen Share with High-Quality AV1
```go
// 4K Screen sharing with AV1 for maximum quality
stream, _ := media.GetDisplayMedia(media.DisplayConstraints{
    Video: &media.DisplayVideoConstraints{
        Width:     3840,
        Height:    2160,
        FrameRate: 60,
        Codec:     codec.AV1,
        Bitrate:   8_000_000,
        SVC:       codec.SVCPresetScreenShare(), // L1T3 temporal only
    },
})

// Access video track for advanced control
videoTrack := stream.GetVideoTracks()[0]

// Dynamic quality adjustment based on network
go func() {
    for {
        stats := getNetworkStats()
        if stats.PacketLoss > 5 {
            // Reduce quality under packet loss
            videoTrack.ApplyConstraints(media.VideoConstraints{
                Bitrate: 4_000_000,
            })
        }
    }
}()
```

### Example 3: Low-Latency Game Streaming
```go
// Ultra low-latency game streaming pipeline
enc, _ := encoder.NewH264Encoder(codec.H264Config{
    Width:       1920,
    Height:      1080,
    Bitrate:     15_000_000,
    FPS:         120,
    Profile:     codec.H264ProfileHigh,
    RateControl: codec.RateControlCBR,  // Constant bitrate for predictable latency
    LowDelay:    true,
    ZeroLatency: true,  // No B-frames, no lookahead
    PreferHW:    true,  // Use GPU encoder
})

// Pre-allocate buffers for zero-allocation encoding
encBuf := make([]byte, enc.MaxEncodedSize())
framePool := frame.NewVideoFramePool(1920, 1080, frame.FormatI420, 4)

// Game capture loop - 8.3ms per frame at 120fps
for {
    f := framePool.Get()
    captureGameFrame(f)

    result, _ := enc.EncodeInto(f, encBuf, false)
    sendToNetwork(encBuf[:result.N])

    f.Release()
}
```

### Example 4: Media Server / SFU Integration
```go
// Production SFU that leverages libwebrtc for encode/decode
type SFURoom struct {
    participants map[string]*Participant
    // Use libwebrtc decoders for server-side processing
    decoders     map[string]decoder.VideoDecoder
}

func (r *SFURoom) ProcessIncomingTrack(participantID string, track *pc.Track) {
    // Selective forwarding - no decode needed for most cases
    // But decode when we need to:

    if needsRecording || needsTranscoding {
        dec, _ := decoder.NewVideoDecoder(codec.VP9)
        dstFrame := frame.NewI420Frame(1920, 1080)

        for packet := range track.Packets() {
            // Depacketize RTP
            depacketizer.Push(packet)
            if frame, ok := depacketizer.Pop(); ok {
                // Decode for server-side processing
                dec.DecodeInto(frame, dstFrame, timestamp, isKeyframe)

                // Record, transcode, or analyze
                recorder.Write(dstFrame)
            }
        }
    }
}

// Bandwidth estimation and layer selection
func (r *SFURoom) SelectLayerForSubscriber(sub *Subscriber, pub *Publisher) {
    bw := sub.EstimatedBandwidth()

    // Select appropriate SVC layer based on bandwidth
    switch {
    case bw > 2_000_000:
        sub.ForwardLayer(pub, 2, 2) // Spatial 2, Temporal 2 (highest)
    case bw > 500_000:
        sub.ForwardLayer(pub, 1, 2) // Medium resolution, full framerate
    default:
        sub.ForwardLayer(pub, 0, 1) // Lowest resolution, reduced framerate
    }
}
```

### Example 5: AI Video Processing Pipeline
```go
// Real-time AI processing with libwebrtc decode → process → encode
func AIVideoProcessor(input *pc.Track, output *pc.Track) {
    dec, _ := decoder.NewVideoDecoder(codec.VP9)
    enc, _ := encoder.NewVP9Encoder(codec.VP9Config{
        Width:   1280,
        Height:  720,
        Bitrate: 2_000_000,
        SVC:     codec.SVCPresetLowLatency(),
    })

    srcFrame := frame.NewI420Frame(1280, 720)
    dstFrame := frame.NewI420Frame(1280, 720)
    encBuf := make([]byte, enc.MaxEncodedSize())

    for packet := range input.Packets() {
        // Decode
        dec.DecodeInto(packet.Data, srcFrame, packet.Timestamp, packet.IsKeyframe)

        // AI Processing (blur background, add effects, etc.)
        aiModel.Process(srcFrame, dstFrame)

        // Re-encode and send
        result, _ := enc.EncodeInto(dstFrame, encBuf, false)
        output.WriteEncodedData(encBuf[:result.N], packet.Timestamp, result.IsKeyframe)
    }
}
```

## Usage Examples

### Low-Level Encoding (Allocation-Free)
```go
// Create encoder with defaults
enc, _ := encoder.NewH264Encoder(codec.DefaultH264Config(1280, 720))
defer enc.Close()

// Pre-allocate buffers once
encBuf := make([]byte, enc.MaxEncodedSize())
srcFrame := frame.NewI420Frame(1280, 720)

// Encode loop - zero allocations
for {
    // Fill srcFrame.Data with raw pixels...
    srcFrame.PTS = timestamp

    result, err := enc.EncodeInto(srcFrame, encBuf, false)
    if err != nil {
        // handle error
    }

    // Use encBuf[:result.N] - the encoded data
    // result.IsKeyframe tells if it's a keyframe
}
```

### Pion Integration (High-Level)
```go
import (
    "github.com/pion/webrtc/v4"
    "github.com/thesyncim/libgowebrtc/pkg/track"
    "github.com/thesyncim/libgowebrtc/pkg/codec"
    "github.com/thesyncim/libgowebrtc/pkg/frame"
)

// Create Pion PeerConnection
pc, _ := webrtc.NewPeerConnection(webrtc.Configuration{})

// Create libwebrtc-backed video track (implements webrtc.TrackLocal)
videoTrack, _ := track.NewVideoTrack(track.VideoTrackConfig{
    ID:      "video",
    Codec:   codec.H264,
    Width:   1280,
    Height:  720,
    Bitrate: 2_000_000,
})
defer videoTrack.Close()

// Add to Pion - seamless interop!
pc.AddTrack(videoTrack)

// Create audio track
audioTrack, _ := track.NewAudioTrack(track.AudioTrackConfig{
    ID:         "audio",
    SampleRate: 48000,
    Channels:   2,
    Bitrate:    64000,
})
defer audioTrack.Close()

pc.AddTrack(audioTrack)

// Feed raw frames - encoding + packetization handled internally
videoFrame := frame.NewI420Frame(1280, 720)
audioFrame := frame.NewS16Frame(48000, 2, 960) // 20ms at 48kHz

for {
    // Fill frames with raw data...
    videoTrack.WriteFrame(videoFrame, false)
    audioTrack.WriteFrame(audioFrame)
}
```

### Pre-Encoded Data
```go
// If you already have encoded H.264/VP8/etc data:
videoTrack.WriteEncodedData(h264NALUs, rtpTimestamp, isKeyframe)

// Or pre-encoded Opus:
audioTrack.WriteEncodedData(opusPacket, rtpTimestamp)
```

## Implementation Progress

### Phase 1: Foundation ✅
- [x] Set up go.mod with purego dependency
- [x] Create shim/shim.h C API definition (allocation-free)
- [x] Create internal/ffi/lib.go library loading
- [x] Create internal/ffi/types.go FFI type mappings
- [x] Create internal/ffi/encoder.go FFI encoder bindings
- [x] Create internal/ffi/decoder.go FFI decoder bindings
- [x] Create pkg/frame types (VideoFrame, AudioFrame)
- [x] Create pkg/codec constants

### Phase 2: Allocation-Free Encoders/Decoders ✅
- [x] pkg/encoder/encoder.go - allocation-free VideoEncoder interface
- [x] pkg/encoder/h264.go - H.264 encoder (FFI integrated)
- [x] pkg/encoder/vp8.go - VP8 encoder (FFI integrated)
- [x] pkg/encoder/vp9.go - VP9 encoder (FFI integrated)
- [x] pkg/encoder/av1.go - AV1 encoder (FFI integrated)
- [x] pkg/encoder/opus.go - Opus encoder (FFI integrated)
- [x] pkg/decoder/decoder.go - allocation-free VideoDecoder interface
- [x] pkg/decoder/h264.go - H.264 decoder (FFI integrated)
- [x] pkg/decoder/vp8.go - VP8 decoder (FFI integrated)
- [x] pkg/decoder/vp9.go - VP9 decoder (FFI integrated)
- [x] pkg/decoder/av1.go - AV1 decoder (FFI integrated)
- [x] pkg/decoder/opus.go - Opus decoder (FFI integrated)

### Phase 3: Packetization ✅
- [x] Implement pkg/packetizer (Pion-compatible, FFI integrated)
- [x] Implement pkg/depacketizer (FFI integrated)

### Phase 4: Pion TrackLocal ✅
- [x] Implement pkg/track/local.go (webrtc.TrackLocal)
- [x] VideoTrack with WriteFrame, WriteEncodedData, WriteRTP
- [x] AudioTrack with WriteFrame, WriteEncodedData
- [ ] Integration test with Pion PeerConnection

### Phase 5: SVC & Simulcast ✅
- [x] SVC mode definitions (L1T1 through L3T3, K-SVC variants)
- [x] Browser-like presets (SVCPresetChrome, SVCPresetFirefox, etc.)
- [x] SVCConfig with per-layer configuration
- [x] VideoEncoderSVC interface for runtime layer control
- [x] VP9/AV1 SVC config integration

### Phase 6: Runtime Controls ✅
- [x] VideoEncoderAdvanced interface (SetQuality, SetKeyInterval, SetRateControl)
- [x] AudioEncoderAdvanced interface (SetComplexity, SetFEC, SetDTX)
- [x] EncoderStats for monitoring
- [x] VideoEncoderSVC for layer control

### Phase 7: Browser-Like API ✅
- [x] pkg/media - API defined (getUserMedia pattern)
- [x] pkg/pc - API defined (PeerConnection, event handlers)
- [x] Browser-like event handlers (OnTrack, OnICECandidate, etc.)
- [x] RTPSender/RTPReceiver/RTPTransceiver API
- [x] DataChannel API
- [x] **pkg/pc FFI integration** (CreateOffer, CreateAnswer, SetLocalDescription, AddICECandidate, AddTrack, RemoveTrack, CreateDataChannel, Close)
- [x] internal/ffi/peerconnection.go - FFI bindings for PeerConnection
- [x] **pkg/media FFI integration** - Complete via track/encoder layer (library uses raw frame input, not device capture)

### Phase 8: Testing ✅

**Testing Principle:** Every feature must be tested. Test the high-level public API, not just FFI plumbing. Use idiomatic Go test patterns (table-driven tests, subtests, got/want assertions).

**High-Level Tests (test the real thing):**
- [x] pkg/encoder/e2e_test.go - H264, VP8, VP9, Opus encoding with real API
- [x] pkg/decoder/e2e_test.go - Full encode/decode roundtrip tests
- [x] pkg/track/e2e_test.go - VideoTrack, AudioTrack pipeline tests
- [x] pkg/pc/e2e_test.go - PeerConnection, CreateOffer, AddTrack, DataChannel

**Unit Tests (types, configs, enums):**
- [x] pkg/codec - Codec types, SVC modes, configs
- [x] pkg/frame - VideoFrame, AudioFrame types
- [x] pkg/encoder - Interface validation, error handling
- [x] pkg/decoder - Interface validation, error handling
- [x] pkg/packetizer - Packetizer interface tests
- [x] pkg/depacketizer - Depacketizer interface tests
- [x] pkg/media - MediaStream, Constraints
- [x] pkg/pc - Enums, states, type tests

**FFI Tests (internal layer):**
- [x] internal/ffi/ffi_test.go - FFI type mappings, helpers
- [x] internal/ffi/integration_test.go - Library loading
- [x] internal/ffi/e2e_test.go - Encode/decode pipeline via FFI
- [x] internal/ffi/peerconnection_test.go - PeerConnection FFI bindings

**Note:** Tests require the shim library to be available.

### Phase 9: Pion Interop Testing ✅

**Principle:** Everything must be tested against Pion to ensure real-world interoperability.

**Interop Tests (test against real Pion WebRTC):**
- [x] `test/interop/helpers.go` - Reusable test helpers (PeerPair, DataChannelPair, ICE exchange)
- [x] `test/interop/offer_answer_test.go` - Offer/answer exchange between libwebrtc and Pion
- [x] `test/interop/video_roundtrip_test.go` - Send video from libwebrtc → Pion and Pion → libwebrtc
- [x] `test/interop/audio_roundtrip_test.go` - Send audio from libwebrtc → Pion and Pion → libwebrtc
- [x] `test/interop/datachannel_test.go` - DataChannel message exchange (lib→pion, pion→lib, bidirectional)
- [x] ICE candidate exchange integrated into helpers (STUNPeerPairConfig)

**Test Setup (using helpers):**
```go
// Simple offer/answer test
func TestOfferAnswerExchange(t *testing.T) {
    pp, err := NewPeerPair(t, DefaultPeerPairConfig())
    if err != nil {
        t.Fatalf("Failed to create peer pair: %v", err)
    }
    defer pp.Close()

    // Perform offer/answer exchange with libwebrtc as offerer
    if err := pp.ExchangeOfferAnswer(); err != nil {
        t.Fatalf("Offer/answer exchange failed: %v", err)
    }
    t.Log("Success")
}

// With ICE connectivity
func TestWithICE(t *testing.T) {
    pp, err := NewPeerPair(t, STUNPeerPairConfig())
    if err != nil {
        t.Fatal(err)
    }
    defer pp.Close()

    pp.ExchangeOfferAnswer()

    // ICE exchange happens automatically via helpers
    if pp.WaitForConnection(10 * time.Second) {
        t.Log("Both peers connected")
    }
}

// Data channel test
func TestDataChannel(t *testing.T) {
    pp, _ := NewPeerPair(t, DefaultPeerPairConfig())
    defer pp.Close()

    dcp, _ := pp.CreateDataChannelPair("test-channel", true)
    pp.ExchangeOfferAnswer()

    if dcp.WaitForReceived(5 * time.Second) {
        t.Log("Data channel received by Pion")
    }
}
```

### Phase 10: Shim Build & Distribution
- [x] shim/CMakeLists.txt - CMake build configuration
- [x] shim/shim.cc - Full implementation (encode/decode/packetizer/PeerConnection)
- [ ] Build shim with actual libwebrtc for darwin-arm64
- [ ] Build shim with actual libwebrtc for darwin-amd64
- [ ] Build shim with actual libwebrtc for linux-amd64
- [ ] Build shim with actual libwebrtc for linux-arm64
- [ ] CI/CD for automated shim builds
- [ ] GitHub releases with pre-built binaries

### Phase 11: Device Capture (via libwebrtc)

**Goal:** Wrap libwebrtc's native device capture APIs so `pkg/media.GetUserMedia()` actually captures from camera/mic.

**libwebrtc APIs to wrap:**
- `webrtc::VideoCaptureModule` - Camera enumeration and capture
- `webrtc::AudioDeviceModule` - Microphone/speaker enumeration and capture
- `webrtc::DesktopCapturer` - Screen/window capture for `GetDisplayMedia()`

**Shim additions (shim/shim.h):**
```c
// Device enumeration
int device_enumerate_video(char** names, char** ids, int max_devices);
int device_enumerate_audio_input(char** names, char** ids, int max_devices);
int device_enumerate_audio_output(char** names, char** ids, int max_devices);

// Video capture
VideoCapture* video_capture_create(const char* device_id, int width, int height, int fps);
int video_capture_start(VideoCapture* cap, FrameCallback callback, void* ctx);
void video_capture_stop(VideoCapture* cap);
void video_capture_destroy(VideoCapture* cap);

// Audio capture
AudioCapture* audio_capture_create(const char* device_id, int sample_rate, int channels);
int audio_capture_start(AudioCapture* cap, AudioCallback callback, void* ctx);
void audio_capture_stop(AudioCapture* cap);
void audio_capture_destroy(AudioCapture* cap);

// Screen capture
ScreenCapture* screen_capture_create(int screen_id);
ScreenCapture* window_capture_create(int window_id);
int screen_capture_start(ScreenCapture* cap, FrameCallback callback, void* ctx);
void screen_capture_stop(ScreenCapture* cap);
void screen_capture_destroy(ScreenCapture* cap);
```

**Go API (pkg/media):**
```go
// Enumerate available devices
devices, _ := media.EnumerateDevices()
for _, d := range devices {
    fmt.Printf("%s: %s (%s)\n", d.Kind, d.Label, d.DeviceID)
}

// Capture from camera (like browser getUserMedia)
stream, _ := media.GetUserMedia(media.Constraints{
    Video: &media.VideoConstraints{
        DeviceID: "device-id",  // or omit for default
        Width:    1280,
        Height:   720,
        FPS:      30,
    },
    Audio: &media.AudioConstraints{
        DeviceID:   "mic-id",
        SampleRate: 48000,
    },
})

// Capture screen (like browser getDisplayMedia)
screenStream, _ := media.GetDisplayMedia(media.DisplayConstraints{
    Video: &media.DisplayVideoConstraints{
        ScreenID: 0,  // or WindowID for window capture
        Codec:    codec.VP9,
        SVC:      codec.SVCPresetScreenShare(),
    },
})

// Enumerate screens/windows
screens, _ := media.EnumerateScreens()
for _, s := range screens {
    fmt.Printf("Screen %d: %s (window=%v)\n", s.ID, s.Title, s.IsWindow)
}
```

**Implementation steps:**
- [x] Add device enumeration to shim (shim.h + shim.cc stubs)
- [x] Add video capture start/stop to shim (shim.h + shim.cc stubs)
- [x] Add audio capture to shim (shim.h + shim.cc stubs)
- [x] Add screen capture to shim (shim.h + shim.cc stubs)
- [x] Add FFI bindings for device APIs (internal/ffi/device.go)
- [x] Implement pkg/media.EnumerateDevices()
- [x] Implement pkg/media.EnumerateScreens() - extension API
- [x] Implement pkg/media.GetDisplayMedia() with DisplayConstraints
- [ ] Implement pkg/media.GetUserMedia() with real capture (requires built shim)
- [ ] Complete shim with libwebrtc device APIs (VideoCaptureModule, AudioDeviceModule, DesktopCapturer)
- [ ] Test on macOS (AVFoundation backend)
- [ ] Test on Linux (V4L2/PulseAudio backend)

### Phase 12: Browser Example [COMPLETE]

**Goal:** Minimal example: Go server captures camera via libwebrtc, streams to browser.

**Components:**
- [x] `examples/camera_to_browser/main.go` - Go server with:
  - HTTP server for signaling (WebSocket)
  - Animated test pattern generator (camera capture when shim is built)
  - libwebrtc PeerConnection to stream to browser
  - DataChannel for bidirectional messaging
  - Real-time statistics display
- [x] `examples/camera_to_browser/index.html` - Browser client (embedded):
  - WebSocket signaling
  - RTCPeerConnection to receive video
  - `<video>` element with status overlay
  - Chat via DataChannel
  - Connection stats display
  - Modern responsive UI

**Architecture:**
```
┌─────────────────┐         WebSocket          ┌─────────────────┐
│   Go Server     │◄─────────────────────────► │    Browser      │
│                 │       (signaling)          │                 │
│  Camera ──►     │                            │     ◄── video   │
│  libwebrtc      │◄═══════════════════════════│   WebRTC        │
│  encoder        │        WebRTC/SRTP         │   decoder       │
└─────────────────┘         (media)            └─────────────────┘
```

**Example code sketch:**
```go
// examples/camera_to_browser/main.go
func main() {
    // Capture camera
    stream, _ := media.GetUserMedia(media.Constraints{
        Video: &media.VideoConstraints{Width: 1280, Height: 720},
    })

    // Create PeerConnection
    pc, _ := pc.NewPeerConnection(pc.DefaultConfiguration())

    // Add video track
    for _, track := range stream.GetVideoTracks() {
        pc.AddTrack(track)
    }

    // Signaling server
    http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
        // WebSocket signaling: exchange offer/answer/ICE
    })

    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, "index.html")
    })

    http.ListenAndServe(":8080", nil)
}
```

### Phase 13: Polish [COMPLETE]
- [x] State-of-the-art examples in PLAN.md
- [x] examples/encode_decode/main.go - Basic encode/decode example
- [x] examples/pion_interop/main.go - Pion PeerConnection integration example
- [x] Makefile with dependency management
- [x] examples/camera_to_browser/ - Camera streaming to browser with WebSocket signaling, DataChannel chat, and live stats

## Dependencies

**Go:**
- `github.com/ebitengine/purego` - FFI without CGO
- `github.com/pion/webrtc/v4` - Pion WebRTC (for interface compatibility)
- `github.com/pion/rtp` - RTP types

**Build (for shim):**
- CMake
- libwebrtc source or pre-built binaries
- C++17 compiler

## Notes

- purego requires shared libraries (.so/.dylib/.dll), not static
- Memory management: Go allocates buffers, passes to C, C writes into them
- No allocations in hot paths - caller provides all buffers
- Pre-build shim for all platforms, distribute via GitHub releases

---

## Recent Session Changes (Phase 11 - Device Capture)

### Completed

**1. Shim Device Capture API (`shim/shim.h`, `shim/shim.cc`)**
- Added ShimDeviceInfo struct for device enumeration
- Added ShimVideoCapture with callback-based frame delivery
- Added ShimAudioCapture with callback-based sample delivery
- Added ShimScreenCapture for screen/window capture
- Added ShimScreenInfo for screen enumeration
- All implementations include thread-safe state management

**2. FFI Device Bindings (`internal/ffi/device.go`)**
- CapturedVideoFrame struct for I420 video frames
- CapturedAudioFrame struct for S16LE audio samples
- VideoCaptureCallback and AudioCaptureCallback types
- Full purego callback bridge implementation with global registry
- Thread-safe capture lifecycle (Start/Stop/Close/IsRunning)
- Comprehensive unit tests with concurrency testing

**3. pkg/media API Improvements**
- MediaDeviceInfo and MediaDeviceKind types (browser-compatible)
- EnumerateDevices() function
- EnumerateScreens() function (extension API)
- DisplayConstraints and DisplayVideoConstraints types
- Updated GetDisplayMedia() to support typed constraints
- **Removed all interface{} from public API**:
  - VideoStreamTrack interface with typed methods
  - AudioStreamTrack interface with typed methods
  - Type-safe GetConstraints(), ApplyConstraints(), GetSettings()

### SVC/Simulcast Support (Already Complete)
- SVCMode enum: L1-L3, T1-T3, K-SVC, S2/S3 simulcast
- SVCConfig with per-layer configuration
- Browser presets: Chrome, Firefox, SFU, ScreenShare, LowLatency
- Full integration in VideoConstraints and encoder configs

### API Design Principles Applied
1. **Type-safe interfaces** - No interface{} in public API
2. **Zero-copy frame delivery** - CapturedVideoFrame wraps C memory directly
3. **Thread-safe callbacks** - Registry pattern for C→Go callback dispatch
4. **Browser-compatible naming** - Matches navigator.mediaDevices API
5. **Comprehensive testing** - Lifecycle, concurrency, edge cases

### Code Quality Session Changes

**1. Shim.cc TODOs Fixed**
- Implemented DataChannelObserver with full callback support
- Added device capture with conditional compilation (`SHIM_ENABLE_DEVICE_CAPTURE`)
- Full libwebrtc API integration:
  - VideoCaptureModule for camera capture
  - AudioDeviceModule for microphone capture
  - DesktopCapturer for screen/window capture
- CMakeLists.txt updated with device capture option

**2. golangci-lint Configuration**
- Added `.golangci.yml` with comprehensive settings
- Enabled linters: errcheck, govet, staticcheck, gosimple, unused, revive, gosec, etc.
- Reasonable exclusions for:
  - Test files (errcheck, govet, funlen, etc.)
  - SVC mode naming (L1T2_KEY is industry standard)
  - FFI layer (unparam, gocritic for special requirements)
  - Browser-compatible type naming (MediaDeviceKind stutter intentional)

**3. All Linter Issues Resolved**
- Fixed errorlint issues (use errors.Is instead of ==)
- Removed unused struct fields
- Fixed variable shadowing (cap → capture)
- Formatted all Go files
- Preallocated slices where appropriate
- Fixed nil pointer dereference patterns in tests

### Next Steps

**Completed:**
- ✅ Cleaned up trivial tests across codebase
- ✅ Wired device capture into GetUserMedia/GetDisplayMedia
- ✅ Created comprehensive README.md

**Top 20 Tasks (First Batch):**

| # | Task | Status | Notes |
|---|------|--------|-------|
| 1 | Create README.md | ✅ Done | Full project documentation |
| 2 | Update PLAN.md | ✅ Done | This update |
| 3 | Add GetStats to shim.h | ✅ Done | Connection monitoring |
| 4 | Add SetParameters/GetParameters to shim.h | ✅ Done | Bitrate adaptation |
| 5 | Add Transceiver functions to shim.h | ✅ Done | Direction control |
| 6 | Add AddTransceiver to shim.h | ✅ Done | Media direction |
| 7 | Add FFI bindings for new shim functions | ✅ Done | purego bindings |
| 8 | Wire up pkg/pc GetStats | ✅ Done | Go API layer |
| 9 | Wire up pkg/pc SetParameters/GetParameters | ✅ Done | Go API layer |
| 10 | Wire up pkg/pc Transceiver methods | ✅ Done | Go API layer |
| 11 | Add GetSenders/GetReceivers/GetTransceivers to shim | ✅ Done | Accessor functions |
| 12 | Add ICE restart to shim | ✅ Done | ICE renegotiation |
| 13 | Add connection state callback to shim | ✅ Done | C++ impl complete |
| 14 | Wire up GetSenders/GetReceivers/GetTransceivers in pkg/pc | ✅ Done | Go API layer |
| 15 | Wire up ICE restart in pkg/pc | ✅ Done | Go API layer |
| 16 | Wire up connection state callbacks in pkg/pc | ✅ Done | Full event support |
| 17 | Add RTCP feedback handling to shim | ✅ Done | PLI/FIR/NACK |
| 18 | Add simulcast layer control to shim | ✅ Done | Layer on/off |
| 19 | Wire up RTCP feedback in pkg/pc | ✅ Done | Go API layer |
| 20 | Wire up simulcast layer control in pkg/pc | ✅ Done | Go API layer |

**Second Batch - Callbacks & Runtime Control:**

| # | Task | Status | Notes |
|---|------|--------|-------|
| 21 | Add OnSignalingStateChange callback | ✅ Done | Signaling state events |
| 22 | Add OnICEConnectionStateChange callback | ✅ Done | ICE state events |
| 23 | Add OnICEGatheringStateChange callback | ✅ Done | ICE gathering events |
| 24 | Add OnNegotiationNeeded callback | ✅ Done | Renegotiation trigger |
| 25 | Add runtime scalability mode API | ✅ Done | SetScalabilityMode/GetScalabilityMode |
| 26 | Wire all callbacks in pkg/pc | ✅ Done | Full NewPeerConnection callback setup |
| 27 | C++ shim implementations | ✅ Done | All callback setters + scalability mode |
| 28 | Add SCTP transport stats | ✅ Done | DataChannel statistics |
| 29 | Add codec capability queries | ✅ Done | Supported codecs/profiles |
| 30 | Add bandwidth estimation hooks | ✅ Done | BWE callbacks |

**Third Batch - Statistics & Codec APIs:**

| # | Task | Status | Notes |
|---|------|--------|-------|
| 31 | Add quality limitation reason to stats | ✅ Done | CPU/bandwidth/other limitation |
| 32 | Add remote RTP stats | ✅ Done | Remote jitter/RTT/packet loss |
| 33 | Add DataChannel message stats | ✅ Done | Messages sent/received |
| 34 | Codec capability C++ impl | ✅ Done | Video/audio codec enumeration |
| 35 | BWE callback implementation | ✅ Done | Bandwidth estimate polling |

**Summary:** 35/35 tasks complete. All WebRTC APIs implemented.

**New Features Added (Latest Session):**
- **SCTP/DataChannel Stats** - `DataChannelsOpened`, `DataChannelsClosed`, `MessagesSent`, `MessagesReceived`, `BytesSentDataChannel`, `BytesReceivedDataChannel`
- **Quality Limitation** - `QualityLimitationReason` (none/cpu/bandwidth/other), `QualityLimitationDurationMs`
- **Remote RTP Stats** - `RemotePacketsLost`, `RemoteJitterMs`, `RemoteRoundTripTimeMs`
- **Codec Capabilities** - `GetSupportedVideoCodecs()`, `GetSupportedAudioCodecs()`, `IsCodecSupported()`
- **Bandwidth Estimation** - `GetBandwidthEstimate()`, `SetOnBandwidthEstimate()` callback

**Priority:**
1. Build shim with actual libwebrtc for darwin/linux platforms
2. Test device capture on macOS/Linux

**Completed This Session:**
- Browser example (`examples/camera_to_browser/`) with:
  - WebSocket signaling for offer/answer/ICE
  - Animated test pattern video generator
  - DataChannel bidirectional messaging
  - Real-time connection statistics
  - Modern responsive browser UI

---

## libwebrtc Build & Pion Interop Session (January 2025)

### Fixed Issues

**1. purego Callback Type Mismatch (CRITICAL FIX)**
- **Problem:** Video and audio callbacks received garbage data, causing panics
- **Root cause:** C uses `int` (32-bit) for width/height/strides, but Go's purego callback was using `int` (64-bit on arm64)
- **Fix:** Changed callback parameter types from `int` to `int32` in `internal/ffi/peerconnection.go`:
  ```go
  // Before (broken):
  func(ctx uintptr, width, height int, yPlane, uPlane, vPlane uintptr, ...)

  // After (fixed):
  func(ctx uintptr, width, height int32, yPlane, uPlane, vPlane uintptr, yStride, uStride, vStride int32, timestampUs int64)
  ```
- This fix applies to both video and audio callbacks

**2. libwebrtc Build Configuration**
- Added `use_custom_libcxx = false` to use system libc++ for ABI compatibility
- Added creation of separate codec factory archives (thin archives don't copy correctly):
  - `libbuiltin_video_encoder_factory.a`
  - `libbuiltin_video_decoder_factory.a`
  - `librtc_internal_video_codecs.a`
  - `librtc_simulcast_encoder_adapter.a`
  - `librtc_software_fallback_wrappers.a`
- Fixed missing macOS frameworks: ScreenCaptureKit, AppKit, ApplicationServices
- Fixed AudioDeviceModule API change: `AudioDeviceModule::Create` → `CreateAudioDeviceModule`

### Test Status

**E2E Tests (all pass):**
- ✅ TestEnumerateDevices
- ✅ TestEnumerateScreens
- ✅ TestVideoCodecRoundtrip (H264, VP8, VP9, AV1 - all pass!)
- ✅ TestOpusRoundtrip
- ✅ TestEncoderBitrateControl
- ✅ TestEncoderFramerateControl
- ✅ TestKeyframeRequest
- ✅ TestVideoTrackCreation
- ✅ TestAudioTrackCreation
- ✅ TestVideoFrameWrite
- ✅ TestAudioFrameWrite
- ✅ TestOfferAnswerWithTracks
- ✅ TestTrackReception (1 video frame received)
- ✅ TestVideoFrameReceiving
- ✅ TestVideoAndAudioTrackReception
- ✅ TestMultipleCodecs
- ✅ TestTrackDisable
- ✅ TestPeerConnectionLifecycle
- ✅ TestConcurrentFrameWrites

**Pion Interop Tests (new):**
- ✅ TestPionToLibVideoInterop - Lib receives track from pion
- ✅ TestBidirectionalVideoInterop - Bidirectional media flow works
- ✅ TestDataChannelInterop - Data channel negotiation works
- ✅ TestCodecNegotiation - VP8/VP9 codec negotiation works
- ✅ TestMultipleTracksInterop - Multiple tracks can be added
- ✅ TestRenegotiationInterop - Track addition after initial connection
- ✅ TestConnectionStateInterop - Connection state callbacks work (connecting → connected)
- ✅ TestICECandidateExchange - ICE candidates exchange correctly
- ✅ TestFrameIntegrity - Frame transmission works
- ✅ TestLibToPionVideoInterop - Lib sends video to pion (fixed: proper ICE gathering)
- ✅ TestSDPParsing - Bidirectional SDP parsing (fixed: set local desc before remote)

### H264 and AV1 Support (Fixed - January 2025)

**H264 - Direct OpenH264 Integration (January 2025):**
- The shim now calls OpenH264 APIs **directly** for H.264 encoding/decoding
- Bypasses libwebrtc's codec factories (which require `rtc_use_h264=true` build)
- OpenH264 is loaded dynamically via `dlsym(RTLD_DEFAULT, ...)` at runtime
- Go downloads OpenH264 from Cisco with `RTLD_GLOBAL` so symbols are available
- No FFmpeg dependency - OpenH264 handles both encoding AND decoding
- Configuration matches libwebrtc exactly (Constrained Baseline, temporal layers=1, VBR)

**Platform Behavior:**
| Platform | Default | With `PreferHW: true` | With `PreferHW: false` |
|----------|---------|----------------------|----------------------|
| Linux | OpenH264 | OpenH264 | OpenH264 |
| macOS | VideoToolbox | VideoToolbox | OpenH264 |
| Windows | OpenH264 | OpenH264 | OpenH264 |

**Files Added:**
- `shim/openh264_types.h` - OpenH264 type definitions (copied from headers)
- `shim/openh264_codec.h` - Wrapper class declarations
- `shim/openh264_codec.cc` - Dynamic loader + encoder/decoder implementations

**AV1 Encoding/Decoding (Fixed):**
- AV1 requires `qpMax = 63` to be set in codec settings
- AV1 requires `SetScalabilityMode(kL1T1)` for single-layer encoding
- Added `rtc_include_dav1d_in_internal_decoder_factory = true` for AV1 decoder
- Uses libaom encoder (software), dav1d decoder

**All 4 Video Codecs Now Work:**
- ✅ H264 (Direct OpenH264 - both encoder AND decoder)
- ✅ VP8 (libvpx via libwebrtc)
- ✅ VP9 (libvpx via libwebrtc)
- ✅ AV1 (libaom encoder + dav1d decoder via libwebrtc)

### Known Issues

1. **Frame reception in stress tests** - Some frames not received due to ICE connectivity timing
   - Expected behavior in loopback without full ICE establishment

### Files Modified

- `internal/ffi/peerconnection.go` - Fixed callback types (int → int32)
- `scripts/build_libwebrtc.sh` - Updated library installation
- `shim/CMakeLists.txt` - Added codec factory libraries and frameworks
- `shim/shim_capture.cc` - Fixed AudioDeviceModule API
- `test/e2e/pion_interop_test.go` - New comprehensive pion interop test suite

---

## Phase 14: Real-Time Transcoding Example [COMPLETE]

**Goal:** Demonstrate the library's transcoding capabilities with live browser streaming.

**Example:** `examples/transcode_to_browser/main.go`

**Features:**
- Real-time codec transcoding (any → any: VP8→AV1, H264→VP9, etc.)
- Zero-allocation encode/decode pipeline
- Live statistics (source/destination frame sizes, compression ratio, transcode time)
- WebSocket signaling with browser
- DataChannel for status messages
- Configurable source/destination codecs via command line

**Architecture:**
```
┌────────────────────────────────────────────────────────────────────┐
│                      Go Transcoder Server                          │
│                                                                    │
│  Test Pattern ─► Source Encoder ─► Decoder ─► Track (dst codec)   │
│      (I420)        (VP8/H264)      (raw)       (AV1/VP9)          │
│                        │              │                            │
│                   Stats: 1321 B    Stats: →    ─► Browser WebRTC   │
│                   (source avg)                    (stream output)  │
└────────────────────────────────────────────────────────────────────┘
```

**Usage:**
```bash
# Default: VP8 → AV1 transcoding at 1280x720
LIBWEBRTC_SHIM_PATH=$PWD/lib/darwin_arm64/libwebrtc_shim.dylib go run ./examples/transcode_to_browser/

# Custom codecs: H264 → VP9
go run ./examples/transcode_to_browser/ -src h264 -dst vp9

# Higher resolution
go run ./examples/transcode_to_browser/ -width 1920 -height 1080 -bitrate 4000000
```

**Browser UI shows:**
- Source/destination codec badges
- Frames processed counter
- Average source/destination frame sizes
- Compression ratio (src/dst)
- Per-frame transcode time (microseconds)
- Live transcoded video stream

---

## Codec Encoding Benchmarks (Apple M2 Pro, 720p)

| Codec | Encode Time | Notes |
|-------|-------------|-------|
| H264 | ~1.14 ms/frame | OpenH264 software encoder |
| VP8 | ~3.08 ms/frame | libvpx |
| VP9 | ~3.21 ms/frame | libvpx |
| AV1 | ~1.88 ms/frame | libaom (surprisingly fast) |

Run benchmarks:
```bash
LIBWEBRTC_SHIM_PATH=$PWD/lib/darwin_arm64/libwebrtc_shim.dylib go test -bench=BenchmarkAllVideoCodecs -benchtime=1s ./test/e2e/
```

---

## Current Priority Tasks

| # | Task | Status | Notes |
|---|------|--------|-------|
| 1 | Add AV1 benchmark test | ✅ Done | All 4 codecs benchmarked |
| 2 | Update CI with codec tests | ✅ Done | Added H264/AV1 build flags + Pion interop tests |
| 3 | Browser video streaming | ✅ Done | Fixed wall-clock timestamp issue, video now streams to browser |
| 4 | Document codec transcoding pipeline | ⏳ Pending | API documentation for transcoding use cases |

---

### Build Requirements

**libwebrtc (M141.7390):**
```bash
# Full build with scripts/build_libwebrtc.sh
./scripts/build_libwebrtc.sh

# Or skip fetch for rebuilds
./scripts/build_libwebrtc.sh --skip-fetch
```

**Shim:**
```bash
LIBWEBRTC_DIR=~/libwebrtc make shim shim-install
```

**Run tests:**
```bash
LIBWEBRTC_SHIM_PATH=$PWD/lib/darwin_arm64/libwebrtc_shim.dylib go test ./test/e2e/ -v
```

---

## Implementation Notes

### Error Handling
- FFI error sentinels in `internal/ffi/lib.go` support `errors.Is()` pattern
- All decoders properly handle `ErrNeedMoreData` for incomplete frames
- `runtime.KeepAlive()` used in device capture to prevent GC issues

### Buffer Safety
- Encoder functions accept `dst_buffer_size` parameter for overflow protection
- Returns `SHIM_ERROR_BUFFER_TOO_SMALL` if encoded frame exceeds buffer

---

## Security Audit - Memory Safety Fixes (January 2025)

### Summary
Comprehensive audit revealed 29 issues: **8 CRITICAL**, **11 HIGH**, **10 MEDIUM**

### Critical Issues (Status: In Progress)

| # | Issue | File | Status |
|---|-------|------|--------|
| 1 | Encoder/Decoder callback memory leak | shim_video_codec.cc:137,363 | ✅ Fixed |
| 2 | DataChannel reference count leak | shim_peer_connection.cc:50 | ✅ Fixed |
| 3 | CStringPtr use-after-free | internal/ffi/types.go:154 | ✅ Fixed |
| 4 | Callbacks not unregistered on Close | pkg/pc/peerconnection.go:1540 | ⏳ |
| 5 | Missing Unregister*Callback functions | internal/ffi/peerconnection.go | ⏳ |
| 6 | Close() race with running callbacks | pkg/pc/peerconnection.go:1540 | ⏳ |
| 7 | Panic in callbacks unwinding through C | internal/ffi/peerconnection.go | ⏳ |
| 8 | Device capture zero-copy to C memory | internal/ffi/device.go:239 | ⏳ |

### High Severity Issues (Status: Pending)

| # | Issue | File | Status |
|---|-------|------|--------|
| 1 | SetLocalDescription potential UAF | shim_peer_connection.cc:469 | ⏳ |
| 2 | GetOrCreateWrapper dangling pointer | shim_data_channel.cc:83 | ⏳ |
| 3 | Screen capture race condition | shim_capture.cc:716 | ⏳ |
| 4 | PushFrame thread safety | shim_track_source.cc:95 | ⏳ |
| 5 | Dangling sender pointer | shim_peer_connection.cc:608 | ⏳ |
| 6 | Missing KeepAlive for CString | internal/ffi/peerconnection.go:267+ | ⏳ |
| 7 | Missing bounds validation | internal/ffi/device.go:233 | ⏳ |
| 8 | Slices passed to C unpinned | pkg/pc/peerconnection.go:855 | ⏳ |
| 9 | RTPSender RTCP callback leak | pkg/pc/peerconnection.go:388 | ⏳ |
| 10 | Unclear handle ownership | Multiple | ⏳ |
| 11 | Data race on callbackInitMu | internal/ffi/peerconnection.go:27 | ⏳ |

### Medium Severity Issues (Status: Pending)

| # | Issue | File |
|---|-------|------|
| 1 | Missing encoder null check | shim_video_codec.cc:176 |
| 2 | Encoder output race condition | shim_video_codec.cc:220 |
| 3 | AudioDeviceModule cleanup leak | shim_capture.cc:316 |
| 4 | Thread-local buffer lifetime | shim_data_channel.cc:156 |
| 5 | DataChannel reference leak | shim_peer_connection.cc:655 |
| 6 | Integer overflow in size calc | internal/ffi/peerconnection.go:65 |
| 7 | BandwidthEstimate pointer lifetime | internal/ffi/peerconnection.go:1590 |
| 8 | Device registry race | internal/ffi/device.go:255 |
| 9 | Stale transceiver handles | pkg/pc/peerconnection.go:1444 |
| 10 | DataChannel callbacks not wired | pkg/pc/peerconnection.go:930 |

### Implementation Phases

**Phase 1: Critical Memory Safety**
- [x] Fix CStringPtr use-after-free (types.go - removed unsafe CStringPtr function)
- [x] Add callback ownership to encoder/decoder (shim_video_codec.cc - store callback in struct, unregister before destroy)
- [x] Fix DataChannel reference counting (shim_peer_connection.cc - store in PC's data_channels vector, use .get() not .release(); shim_data_channel.cc - wrapper uses raw pointer to avoid double ref count)

**Phase 2: Callback Lifecycle**
- [ ] Add Unregister*Callback functions
- [ ] Add callback cleanup in Close()
- [ ] Add closed check in callbacks
- [ ] Add panic recovery

**Phase 3: Memory Pinning**
- [ ] Add runtime.KeepAlive() for CString
- [ ] Copy device capture data
- [ ] Add bounds validation

**Phase 4: Thread Safety**
- [ ] Fix callback init mutex
- [ ] Add screen capture synchronization
- [ ] Document thread requirements

**Phase 5: Documentation & Cleanup**
- [ ] Document handle ownership
- [ ] Wire DataChannel callbacks
- [ ] Add null/overflow checks

---

## API Surface Cleanup (January 2025)

Cleaned up public API to hide C implementation details and provide more browser/Pion-like patterns.

### Breaking Changes

| Change | Migration |
|--------|-----------|
| Removed `PionTrack()` from MediaStreamTrack interface | Use `media.PionTrackLocal(track)` helper |
| Removed `NewVideoEncoder(codec, interface{})` | Use type-safe constructors: `NewH264Encoder()`, `NewVP9Encoder()`, etc. |
| Removed `NewAudioEncoder(codec, interface{})` | Use `NewOpusEncoder()` directly |
| Removed `VideoTrack.Encoder()` | Use track methods: `SetBitrate()`, `SetFramerate()`, `RequestKeyFrame()` |
| Removed `AudioTrack.Encoder()` | Use track methods: `SetBitrate()` |
| Changed `GetDisplayMedia(interface{})` | Use `GetDisplayMedia(DisplayConstraints)` directly |

### New APIs

| API | Description |
|-----|-------------|
| `PeerConnection.IsValid()` | Check if handle is valid (replaces direct `handle != 0` checks) |
| `Track.IsValid()` | Validation helper for Track |
| `RTPSender.IsValid()` | Validation helper for RTPSender |
| `RTPReceiver.IsValid()` | Validation helper for RTPReceiver |
| `RTPTransceiver.IsValid()` | Validation helper for RTPTransceiver |
| `DataChannel.IsValid()` | Validation helper for DataChannel |
| `media.PionTrackLocal()` | Extract Pion TrackLocal from MediaStreamTrack |
| `media.AddTracksToPC()` | Add all tracks from MediaStream to Pion PeerConnection |
| `media.IntConstraint` | Browser-like exact/ideal/min/max constraint for integers |
| `media.FloatConstraint` | Browser-like exact/ideal/min/max constraint for floats |
| `media.FacingMode` | Camera facing mode enum (`user`, `environment`) |
| `media.DisplaySurface` | Screen capture surface enum (`monitor`, `window`) |
| `media.OverconstrainedError` | Error type for constraint validation failures |

### Deferred Changes

- **VideoConstraints migration**: Converting existing `int`/`float64` fields to `IntConstraint`/`FloatConstraint` is deferred due to invasive changes required across the codebase

---

## Browser-like RTCP/BWE Handling (January 2025)

Added browser-like automatic RTCP feedback and bandwidth estimation handling to `pkg/track` for Pion interop.

### Two Paths

| Path | Description | BWE Source |
|------|-------------|------------|
| `pkg/pc` | Full libwebrtc pipeline | Internal (automatic) |
| `pkg/track` | Pion interop with manual encoding | External via callback |

**pkg/pc path** already provides full browser-like behavior - libwebrtc handles encoding, BWE, RTCP internally via VideoSendStream.

**pkg/track path** now supports browser-like auto-adaptation when wired to a BWE source.

### New VideoTrackConfig Options

```go
type VideoTrackConfig struct {
    // ... existing fields ...

    // Auto adaptation (all default true for browser-like behavior)
    AutoKeyframe   bool  // PLI/FIR → RequestKeyFrame()
    AutoBitrate    bool  // BWE → adjust bitrate
    AutoFramerate  bool  // BWE → adjust framerate
    AutoResolution bool  // BWE → scale resolution

    // Constraints (like browser MediaTrackConstraints)
    MinBitrate   uint32  // Floor for bitrate adaptation
    MaxBitrate   uint32  // Ceiling for bitrate adaptation
    MinFramerate float64 // Floor for framerate
    MaxFramerate float64 // Ceiling for framerate
    MinWidth     int     // Don't scale below this
    MinHeight    int     // Don't scale below this
}
```

### New APIs

**pkg/track:**
```go
// Browser-like SetParameters (mirrors RTCRtpSender.setParameters)
track.SetParameters(track.TrackParameters{
    Active:                true,
    MaxBitrate:            1_000_000,
    MaxFramerate:          15,
    ScaleResolutionDownBy: 2.0,
    ScalabilityMode:       "L3T3_KEY",
    Priority:              "high",
})

// Wire up BWE from libwebrtc PeerConnection
track.SetBWESource(func() *track.BandwidthEstimate {
    return convertBWE(pc.GetCurrentBandwidthEstimate())
})

// Handle RTCP feedback (PLI/FIR/NACK)
track.HandleRTCPFeedback(feedbackType, ssrc)
```

**pkg/pc:**
```go
// Get libwebrtc's BWE (TWCC/GCC)
pc.SetOnBandwidthEstimate(func(bwe *pc.BandwidthEstimate) {
    log.Printf("BWE: target=%d", bwe.TargetBitrateBps)
})

bwe := pc.GetCurrentBandwidthEstimate()
```

### Frame Scaling

Added I420 frame scaling utilities in `pkg/track/scale.go`:
- `ScaleI420Frame()` - Box filter downsampling (quality)
- `ScaleI420FrameFast()` - Nearest neighbor (performance)

### Files Added/Modified

| File | Change |
|------|--------|
| `pkg/track/local.go` | Added adaptation config, TrackParameters, SetParameters(), adaptLoop(), BWE wiring |
| `pkg/track/scale.go` | NEW: I420 frame scaling utilities |
| `pkg/track/track_test.go` | Added tests for SetParameters, adaptation, scaling |
| `pkg/pc/peerconnection.go` | Added SetOnBandwidthEstimate(), GetCurrentBandwidthEstimate() |

### Comparison with Browser API

| Browser API | libgowebrtc |
|-------------|-------------|
| `sender.setParameters({encodings: [{maxBitrate}]})` | `track.SetParameters({MaxBitrate})` |
| `sender.setParameters({encodings: [{scaleResolutionDownBy}]})` | `track.SetParameters({ScaleResolutionDownBy})` |
| BWE auto-adjusts internally | `AutoBitrate: true` + adaptLoop() |
| PLI triggers keyframe internally | `AutoKeyframe: true` + HandleRTCPFeedback() |

---

## Phase 15: Advanced Browser Features (Planned)

Features that would differentiate libgowebrtc from any other Go WebRTC library.

### Priority Rankings

| # | Feature | Impact | Complexity | Status |
|---|---------|--------|------------|--------|
| 🥇 | Insertable Streams (Encoded Transform) | E2E encryption - killer feature | Medium | ⏳ Planned |
| 🥈 | Congestion Control Internals | Unique GCC visibility | Medium | ⏳ Planned |
| 🥉 | Jitter Buffer Control | Latency vs quality tradeoff | Low | ✅ Complete |
| 4 | FEC Control | Packet loss resilience | Low | ⏳ Planned |
| 5 | RTP Header Extension Access | SFU layer selection | Medium | ⏳ Planned |
| 6 | Pacer/Send Queue Visibility | Congestion early warning | Low | ⏳ Planned |

---

### 🥇 1. Insertable Streams (Encoded Transform)

**Why:** E2E encryption like Zoom/Google Meet. No Go library has this today.

**Browser API:** `RTCRtpScriptTransform`

**Proposed Go API:**
```go
// Sender side - transform before RTP packetization
sender.SetEncodedFrameTransform(func(frame *EncodedFrame) *EncodedFrame {
    // E2E encrypt, watermark, or ML processing
    encrypted := encryptFrame(frame.Data, e2eKey)
    return &EncodedFrame{
        Data:       encrypted,
        Timestamp:  frame.Timestamp,
        IsKeyframe: frame.IsKeyframe,
        Metadata:   frame.Metadata,
    }
})

// Receiver side - transform after RTP depacketization
receiver.SetEncodedFrameTransform(func(frame *EncodedFrame) *EncodedFrame {
    decrypted := decryptFrame(frame.Data, e2eKey)
    return &EncodedFrame{Data: decrypted, ...}
})

// EncodedFrame exposes codec-specific metadata
type EncodedFrame struct {
    Data       []byte
    Timestamp  uint32
    IsKeyframe bool
    Codec      codec.Type
    // H264: NAL units, VP8/VP9: frame header, AV1: OBUs
    Metadata   *FrameMetadata
}
```

**Shim additions:**
```c
// Set transform callback on sender
void rtp_sender_set_encoded_transform(RTPSender* sender,
    EncodedFrameCallback callback, void* ctx);

// Set transform callback on receiver
void rtp_receiver_set_encoded_transform(RTPReceiver* receiver,
    EncodedFrameCallback callback, void* ctx);
```

---

### 🥈 2. Congestion Control Internals

**Why:** Unique visibility into GCC (Google Congestion Control) for building adaptive streaming logic.

**Proposed Go API:**
```go
pc.SetOnCongestionState(func(state *CongestionState) {
    log.Printf("GCC: delay_gradient=%.2f loss_estimate=%d probe=%d",
        state.DelayGradient,
        state.LossBasedEstimateBps,
        state.ProbeResultBps)
})

type CongestionState struct {
    // Delay-based estimation (TWCC)
    DelayGradient        float64  // Positive = congestion building
    DelayBasedEstimateBps int64

    // Loss-based estimation
    LossBasedEstimateBps int64
    LossFraction         float64

    // Bandwidth probing
    ProbeResultBps       int64
    ProbeState           ProbeState  // NotProbing, Probing, Success, Failure

    // Controller state
    InSlowStart          bool  // Initial ramp-up
    InRecovery           bool  // After loss event
    InAlr                bool  // Application-limited region

    // Timing
    LastUpdateTime       time.Time
    RttMs                float64
}

type ProbeState int
const (
    ProbeStateNotProbing ProbeState = iota
    ProbeStateProbing
    ProbeStateSuccess
    ProbeStateFailure
)
```

**Use cases:**
- Custom ABR (Adaptive Bitrate) algorithms
- Debugging congestion issues
- Research/logging

---

### 🥉 3. Jitter Buffer Control

**Why:** Critical for latency-sensitive apps (gaming, live streaming, real-time collaboration).

**Proposed Go API:**
```go
// Set target jitter buffer delay
receiver.SetJitterBufferTarget(50 * time.Millisecond)   // Ultra-low latency
receiver.SetJitterBufferTarget(200 * time.Millisecond)  // Smooth playback

// Set min/max bounds
receiver.SetJitterBufferBounds(20*time.Millisecond, 500*time.Millisecond)

// Get real-time stats
receiver.OnJitterBufferStats(func(stats *JitterBufferStats) {
    if stats.LatePacketRatio > 0.05 {
        // 5% late packets - increase buffer
        receiver.SetJitterBufferTarget(stats.CurrentDelayMs * 1.2)
    }
})

type JitterBufferStats struct {
    CurrentDelayMs    int     // Current buffer delay
    TargetDelayMs     int     // Target delay
    MinDelayMs        int     // Minimum achievable
    BufferSizePackets int     // Packets in buffer
    BufferSizeMs      int     // Buffer duration
    LatePackets       int64   // Packets arrived too late
    LatePacketRatio   float64 // Late / total
    Underruns         int64   // Buffer ran empty
}
```

---

### 4. FEC Control (Forward Error Correction)

**Why:** Tune redundancy for lossy networks without re-encoding.

**Proposed Go API:**
```go
// Video FEC
sender.SetFECEnabled(true)
sender.SetFECRate(0.2)  // 20% redundancy
sender.SetFECMode(FECModeFlexFEC)  // or RED, ULPFEC

type FECMode int
const (
    FECModeNone FECMode = iota
    FECModeRED      // Redundant Encoding
    FECModeULPFEC   // Unequal Level Protection
    FECModeFlexFEC  // Flexible FEC (newer)
)

// Audio FEC (Opus)
audioTrack.SetInbandFEC(true)
audioTrack.SetExpectedPacketLoss(10)  // Tune for 10% loss
```

---

### 5. RTP Header Extension Access

**Why:** Essential for SFU layer selection, speaker detection, A/V sync.

**Proposed Go API:**
```go
receiver.OnRTPExtension(func(ext *RTPExtensions) {
    // Speaker detection
    if ext.AudioLevel != nil {
        if ext.AudioLevel.Level > -30 {
            markAsSpeaking(ext.SSRC)
        }
    }

    // SVC layer selection
    if ext.DependencyDescriptor != nil {
        dd := ext.DependencyDescriptor
        // Forward only base layer to constrained subscribers
        if dd.SpatialLayer == 0 && dd.TemporalLayer == 0 {
            forwardToLowBandwidthPeers(packet)
        }
    }
})

type RTPExtensions struct {
    SSRC                 uint32
    AudioLevel           *AudioLevelExtension
    VideoOrientation     *VideoOrientationExtension
    PlayoutDelay         *PlayoutDelayExtension
    DependencyDescriptor *DependencyDescriptorExtension
    AbsSendTime          *AbsSendTimeExtension
    TransportCC          *TransportCCExtension
}

type DependencyDescriptorExtension struct {
    SpatialLayer   int
    TemporalLayer  int
    IsKeyframe     bool
    Dependencies   []int  // Frame dependencies
    DecodeTargets  []int  // Which targets can decode this
}
```

---

### 6. Pacer/Send Queue Visibility

**Why:** Know when you're congested before frames start dropping.

**Proposed Go API:**
```go
pc.OnPacerStats(func(stats *PacerStats) {
    if stats.Congested {
        // Slow down frame production
        videoSource.SetFramerate(15)
    }

    if stats.QueueDelayMs > 500 {
        // Queue backing up - drop quality
        encoder.SetBitrate(encoder.GetBitrate() / 2)
    }
})

type PacerStats struct {
    QueuedPackets   int
    QueuedBytes     int64
    QueueDelayMs    int      // Estimated delay to send all queued
    PacingRateBps   int64    // Current pacing rate
    Congested       bool     // Pacer is behind
    MediaRateBps    int64    // Rate media is being added
    PaddingRateBps  int64    // Rate of padding being sent
}
```

---

### Implementation Order

**Phase 15a: Jitter Buffer Control** ✅ COMPLETE
- [x] Add shim functions for jitter buffer target/bounds (shim_peer_connection.cc)
- [x] Add FFI bindings (internal/ffi/peerconnection.go)
- [x] Add receiver.SetJitterBufferTarget() API (pkg/pc/peerconnection.go)
- [x] Add receiver.SetJitterBufferBounds() API
- [x] Add receiver.GetJitterBufferStats() API
- [x] Add receiver.OnJitterBufferStats() callback
- [x] Add receiver.SetAdaptiveJitterBuffer() API
- [x] Add tests (pkg/pc/e2e_test.go)

**Usage:**
```go
// Get a receiver from transceiver
receiver := transceiver.Receiver()

// Set target delay (low latency mode)
receiver.SetJitterBufferTarget(50)  // 50ms

// Or high buffering mode for unreliable networks
receiver.SetJitterBufferTarget(500)  // 500ms

// Set bounds
receiver.SetJitterBufferBounds(20, 500)  // min 20ms, max 500ms

// Disable adaptive mode for manual control
receiver.SetAdaptiveJitterBuffer(false)

// Get stats
stats, _ := receiver.GetJitterBufferStats()
log.Printf("Buffer: %dms current, %dms target, %d packets",
    stats.CurrentDelayMs, stats.TargetDelayMs, stats.BufferSizePackets)

// Periodic stats callback
receiver.OnJitterBufferStats(func(stats *JitterBufferStats) {
    log.Printf("Underruns: %d, Late: %d", stats.Underruns, stats.LatePackets)
}, 1000)  // Every 1000ms
```

**Phase 15b: Insertable Streams** (Medium complexity, killer feature)
- [ ] Add EncodedFrame type to shim
- [ ] Add transform callbacks to sender/receiver
- [ ] Add FFI bindings with proper buffer management
- [ ] Add Go API with zero-copy where possible
- [ ] Example: E2E encryption demo

**Phase 15c: Congestion Control Internals** (Medium complexity, unique feature)
- [ ] Expose GCC state from libwebrtc
- [ ] Add CongestionState struct
- [ ] Add pc.SetOnCongestionState() callback
- [ ] Example: Custom ABR algorithm

