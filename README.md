# libgowebrtc

**Pion-compatible Go wrapper for libwebrtc** - high-performance video/audio encoding, decoding, and WebRTC connectivity without CGO.

[![Go Reference](https://pkg.go.dev/badge/github.com/thesyncim/libgowebrtc.svg)](https://pkg.go.dev/github.com/thesyncim/libgowebrtc)

## Features

- **H.264, VP8, VP9, AV1** video encoding/decoding via libwebrtc
- **Opus** audio encoding/decoding
- **Allocation-free** hot paths - caller provides all buffers
- **Pion-compatible** - implements `webrtc.TrackLocal` for seamless integration
- **Browser-like API** - `GetUserMedia()`, `GetDisplayMedia()`, `PeerConnection`
- **SVC/Simulcast** support with Chrome/Firefox-compatible presets
- **purego FFI** - no CGO required, just a pre-built shim library
- **Device capture** - camera, microphone, screen/window capture

## Installation

```bash
go get github.com/thesyncim/libgowebrtc
```

**Note:** Requires the pre-built `libwebrtc_shim` library. Set `LIBWEBRTC_SHIM_PATH` to point to it.

## Quick Start

### Browser-Like API

```go
import (
    "github.com/thesyncim/libgowebrtc/pkg/media"
    "github.com/thesyncim/libgowebrtc/pkg/pc"
    "github.com/thesyncim/libgowebrtc/pkg/codec"
)

// Get camera and microphone (like navigator.mediaDevices.getUserMedia)
stream, _ := media.GetUserMedia(media.Constraints{
    Video: &media.VideoConstraints{
        Width:     1280,
        Height:    720,
        FrameRate: 30,
        Codec:     codec.VP9,
    },
    Audio: &media.AudioConstraints{
        SampleRate: 48000,
    },
})

// Create PeerConnection
peerConnection, _ := pc.NewPeerConnection(pc.Configuration{
    ICEServers: []pc.ICEServer{
        {URLs: []string{"stun:stun.l.google.com:19302"}},
    },
})

// Add tracks using helper
senders, _ := media.AddTracksToPC(peerConnection, stream)

// Create offer
offer, _ := peerConnection.CreateOffer(nil)
peerConnection.SetLocalDescription(offer)
```

### Pion Integration

```go
import (
    "github.com/pion/webrtc/v4"
    "github.com/thesyncim/libgowebrtc/pkg/track"
    "github.com/thesyncim/libgowebrtc/pkg/codec"
)

// Create Pion PeerConnection
pionPC, _ := webrtc.NewPeerConnection(webrtc.Configuration{})

// Create libwebrtc-backed video track (implements webrtc.TrackLocal)
videoTrack, _ := track.NewVideoTrack(track.VideoTrackConfig{
    ID:      "video",
    Codec:   codec.H264,
    Width:   1280,
    Height:  720,
    Bitrate: 2_000_000,
})

// Add to Pion - seamless interop!
pionPC.AddTrack(videoTrack)

// Feed raw frames
frame := frame.NewI420Frame(1280, 720)
videoTrack.WriteFrame(frame, false)
```

### Low-Level Encoding (Allocation-Free)

```go
import (
    "github.com/thesyncim/libgowebrtc/pkg/encoder"
    "github.com/thesyncim/libgowebrtc/pkg/codec"
    "github.com/thesyncim/libgowebrtc/pkg/frame"
)

// Create encoder
enc, _ := encoder.NewH264Encoder(codec.DefaultH264Config(1280, 720))
defer enc.Close()

// Pre-allocate buffers once
encBuf := make([]byte, enc.MaxEncodedSize())
srcFrame := frame.NewI420Frame(1280, 720)

// Encode loop - zero allocations
for {
    result, _ := enc.EncodeInto(srcFrame, encBuf, false)
    // Use encBuf[:result.N]
}
```

## Project Structure

```
libgowebrtc/
├── pkg/
│   ├── codec/          # Codec types, configs, SVC presets
│   ├── encoder/        # Video/audio encoders
│   ├── decoder/        # Video/audio decoders
│   ├── frame/          # VideoFrame, AudioFrame types
│   ├── packetizer/     # RTP packetization
│   ├── depacketizer/   # RTP depacketization
│   ├── track/          # Pion-compatible TrackLocal
│   ├── pc/             # PeerConnection (libwebrtc-backed)
│   └── media/          # Browser-like API (GetUserMedia, etc.)
├── internal/ffi/       # purego FFI bindings
├── shim/               # C++ shim library
├── test/
│   ├── e2e/            # End-to-end tests
│   └── interop/        # Pion interop tests
└── examples/
```

## What's Working

### Core Encoding/Decoding
- H.264/VP8/VP9/AV1 video encoding/decoding via FFI
- Opus audio encoding/decoding via FFI
- Allocation-free encode/decode with reusable buffers
- Runtime bitrate/framerate control
- Keyframe request

### PeerConnection
- Full offer/answer/ICE support
- Track writing with frame push to native source
- Frame receiving from remote tracks (SetOnVideoFrame/SetOnAudioFrame)
- DataChannel communication
- `GetStats()` - connection statistics
- `RestartICE()` - ICE restart trigger
- `AddTransceiver()` - add transceivers with direction control

### RTPSender
- `ReplaceTrack()` - replace sender track
- `SetParameters()` / `GetParameters()` - encoding parameters
- `SetLayerActive()` / `SetLayerBitrate()` - simulcast layer control
- `GetActiveLayers()` - get active layer count
- `SetOnRTCPFeedback()` - RTCP feedback events (PLI/FIR/NACK)
- `SetScalabilityMode()` / `GetScalabilityMode()` - runtime SVC mode control
- `GetStats()` - sender statistics

### RTPReceiver
- `GetStats()` - receiver statistics
- `RequestKeyframe()` - send PLI

### RTPTransceiver
- `SetDirection()` / `Direction()` / `CurrentDirection()` - direction control
- `Stop()` - stop transceiver
- `Mid()` - get media ID
- `Sender()` / `Receiver()` - get sender/receiver

### Event Callbacks
- `OnConnectionStateChange` - connection state events
- `OnSignalingStateChange` - signaling state events
- `OnICEConnectionStateChange` - ICE connection state events
- `OnICEGatheringStateChange` - ICE gathering progress events
- `OnNegotiationNeeded` - renegotiation trigger events
- `OnICECandidate` - new ICE candidate events
- `OnTrack` - remote track received events
- `OnDataChannel` - data channel received events

### Media Capture
- Device/screen capture wired into GetUserMedia/GetDisplayMedia
- Pion interop (libwebrtc tracks work with Pion PC)

### Statistics (RTCStats)
- Transport stats (bytes/packets sent/received)
- Quality metrics (RTT, jitter, packet loss)
- Video stats (frames encoded/decoded, keyframes, NACK/PLI/FIR)
- Audio stats (audio level, energy, concealment)
- **SCTP/DataChannel stats** - channels opened/closed, messages sent/received
- **Quality limitation** - reason (none/cpu/bandwidth/other) and duration
- **Remote RTP stats** - remote jitter, RTT, packet loss

### Codec Capabilities
- `GetSupportedVideoCodecs()` - enumerate video codecs (VP8, VP9, H264, AV1)
- `GetSupportedAudioCodecs()` - enumerate audio codecs (Opus, PCMU, PCMA)
- `IsCodecSupported(mimeType)` - check codec support

### Bandwidth Estimation
- `GetBandwidthEstimate()` - get current BWE (target bitrate, available bandwidth)
- `SetOnBandwidthEstimate(callback)` - receive BWE updates

## Browser Example

A complete browser example is included that demonstrates video streaming from Go to browser:

```bash
# Run the example
LIBWEBRTC_SHIM_PATH=/path/to/libwebrtc_shim.dylib go run ./examples/camera_to_browser

# Then open http://localhost:8080 in your browser
```

The example showcases:
- WebSocket signaling for offer/answer/ICE exchange
- Video streaming with animated test pattern
- DataChannel for bidirectional messaging
- Real-time connection statistics
- Modern responsive UI

## What's Pending

The Go layer and FFI bindings are complete for all WebRTC functionality. The shim needs to be built with actual libwebrtc:

- Build shim for darwin-arm64/amd64
- Build shim for linux-amd64/arm64

## SVC & Simulcast

```go
// Chrome-like SVC for SFU
enc, _ := encoder.NewVP9Encoder(codec.VP9Config{
    Width:   1280,
    Height:  720,
    Bitrate: 2_000_000,
    SVC:     codec.SVCPresetChrome(), // L3T3_KEY
})

// Screen sharing preset
codec.SVCPresetScreenShare() // L1T3 temporal only

// SFU-optimized
codec.SVCPresetSFU() // L3T3_KEY
```

## Running Tests

```bash
# Unit tests (no shim required)
go test ./...

# With shim library (real encoding/decoding)
LIBWEBRTC_SHIM_PATH=./lib/darwin_arm64/libwebrtc_shim.dylib go test ./...

# Verbose
go test -v ./...
```

## Building the Shim

The shim library wraps libwebrtc's C++ API with a C interface. See `shim/CMakeLists.txt`.

```bash
cd shim
mkdir build && cd build
cmake .. -DLIBWEBRTC_ROOT=/path/to/libwebrtc
make
```

## License

MIT

## See Also

- [PLAN.md](PLAN.md) - Detailed design document and implementation progress
- [Pion WebRTC](https://github.com/pion/webrtc) - Pure Go WebRTC implementation
- [libwebrtc](https://webrtc.googlesource.com/src) - Google's WebRTC implementation
