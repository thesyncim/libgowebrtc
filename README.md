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
- **purego FFI** - no CGO required by default, optional CGO mode for 5x faster FFI
- **Device capture** - camera, microphone, screen/window capture

## Installation

```bash
go get github.com/thesyncim/libgowebrtc
```

By default, the runtime will auto-download the prebuilt `libwebrtc_shim` for supported
OS/arch combinations (currently `darwin_arm64`, `linux_amd64`, `linux_arm64`) from
GitHub Releases and cache it under `~/.libgowebrtc`. For other platforms, build the
shim locally and set `LIBWEBRTC_SHIM_PATH`.

Override behavior with:

- `LIBWEBRTC_SHIM_PATH=/path/to/libwebrtc_shim.{so|dylib|dll}` (use a local shim)
- `LIBWEBRTC_SHIM_DISABLE_DOWNLOAD=1` (disable auto-download)
- `LIBWEBRTC_SHIM_CACHE_DIR=/custom/cache/dir` (override cache location)
- `LIBWEBRTC_SHIM_FLAVOR=basic` (override shim flavor; default: basic)

### FFI Variants

By default, libgowebrtc uses **purego** for FFI calls, requiring no CGO. For performance-critical applications, an optional CGO mode provides ~5x faster FFI calls:

```bash
# Default (purego) - no CGO required
go build ./...

# CGO mode - faster FFI, requires C compiler
go build -tags ffigo_cgo ./...
```

| Mode | FFI Overhead | Requirements |
|------|--------------|--------------|
| purego (default) | ~200 ns/call | None (pure Go) |
| CGO (`-tags ffigo_cgo`) | ~44 ns/call | C compiler |

Both modes use the same pre-built shim library - no recompilation needed.

### H.264 Support

H.264 encoding and decoding uses **direct OpenH264 integration** - the shim calls
OpenH264 APIs directly rather than going through libwebrtc's codec factories. This
means:

- **Zero configuration required** - works out of the box
- **No FFmpeg dependency** - OpenH264 handles both encoding AND decoding
- **Clean licensing** - Cisco's BSD-licensed OpenH264 binaries are royalty-free
- **Cross-platform** - works on Linux, macOS, and Windows

#### Platform Behavior

| Platform | Default | With `PreferHW: true` | With `PreferHW: false` |
|----------|---------|----------------------|----------------------|
| **Linux** | OpenH264 | OpenH264 | OpenH264 |
| **macOS** | VideoToolbox | VideoToolbox | OpenH264 |
| **Windows** | OpenH264 | OpenH264 | OpenH264 |

#### OpenH264 Runtime Download

OpenH264 is downloaded automatically from Cisco on first use and cached under
`~/.libgowebrtc/openh264/<version>/<platform>`.

Defaults:

- `codec.DefaultH264Config` prefers hardware on macOS (VideoToolbox) and software
  (OpenH264) elsewhere.
- Set `PreferHW: true` or `PreferHW: false` explicitly to override.

Environment knobs:

- `LIBWEBRTC_OPENH264_PATH=/path/to/openh264` (use a local OpenH264 binary)
- `LIBWEBRTC_OPENH264_DISABLE_DOWNLOAD=1` (disable auto-download)
- `LIBWEBRTC_OPENH264_URL=https://...` (override download URL)
- `LIBWEBRTC_OPENH264_BASE_URL=https://...` (override base URL)
- `LIBWEBRTC_OPENH264_VERSION=2.x.y` (override version)
- `LIBWEBRTC_OPENH264_SOVERSION=7` (override Linux SO version)
- `LIBWEBRTC_OPENH264_SHA256=...` (verify download)
- `LIBWEBRTC_PREFER_SOFTWARE_CODECS=1` (force software codecs in PeerConnection)

Note: Cisco provides OpenH264 binaries under their own terms. Downloading from
Cisco keeps libgowebrtc MIT/BSD, but users must accept Cisco's license.

### Pinned Versions

| Dependency | Version | Source |
|------------|---------|--------|
| libwebrtc (pre-compiled) | 141.7390.2.0 | [crow-misia/libwebrtc-bin](https://github.com/crow-misia/libwebrtc-bin) |
| libwebrtc_shim | shim-v0.3.0 | [thesyncim/libgowebrtc releases](https://github.com/thesyncim/libgowebrtc/releases) |
| OpenH264 | 2.5.1 | [Cisco OpenH264](https://github.com/cisco/openh264/releases) |

### Building the Shim

The shim is built using Bazel with pre-compiled libwebrtc from
[crow-misia/libwebrtc-bin](https://github.com/crow-misia/libwebrtc-bin) (~165MB).

```bash
# Build for current platform
./scripts/build.sh

# Cross-compile all platforms via Docker and create release
./scripts/release.sh

# Build all and upload to GitHub release
./scripts/release.sh --upload shim-v0.4.0
```

Environment variables:
- `LIBWEBRTC_VERSION` - Pre-compiled version (default: 141.7390.2.0)
- `INSTALL_DIR` - Where to cache libwebrtc (default: ~/libwebrtc)

Supported platforms: `darwin_arm64`, `linux_amd64`, `linux_arm64`

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
├── internal/ffi/       # FFI bindings (purego default, CGO optional)
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
- `SetJitterBufferTarget()` - set target buffer delay
- `SetJitterBufferBounds()` - set min/max delay bounds
- `GetJitterBufferStats()` - get buffer statistics
- `OnJitterBufferStats()` - periodic stats callback
- `SetAdaptiveJitterBuffer()` - enable/disable adaptive mode

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

### Jitter Buffer Control
Control libwebrtc's internal jitter buffer for latency vs quality tradeoffs:

```go
receiver := transceiver.Receiver()

// Low latency mode (gaming, live streaming)
receiver.SetJitterBufferTarget(50)  // 50ms target delay

// High buffering mode (unreliable networks)
receiver.SetJitterBufferTarget(500)  // 500ms buffer

// Set bounds
receiver.SetJitterBufferBounds(20, 500)

// Get stats
stats, _ := receiver.GetJitterBufferStats()
log.Printf("Buffer: %dms, Late packets: %d", stats.CurrentDelayMs, stats.LatePackets)

// Disable adaptive mode for manual control
receiver.SetAdaptiveJitterBuffer(false)
```

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

## Build Status

The Go layer and FFI bindings are complete for all WebRTC functionality. Bazel builds the shim:

| Platform | Status |
|----------|--------|
| darwin_arm64 | ✅ Working |
| linux_amd64 | ✅ Working |
| linux_arm64 | ✅ Working |

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

# Test with CGO FFI variant
go test -tags ffigo_cgo ./...

# Verbose
go test -v ./...
```

## Building from Source

### Prerequisites

- Bazel 7.4.1+ (via Bazelisk recommended)
- curl (for downloading pre-compiled libwebrtc)

### Build Commands

```bash
# Build shim (downloads pre-compiled libwebrtc automatically)
./scripts/build.sh

# Create release tarball
./scripts/build.sh --release

# Clean and rebuild
./scripts/build.sh --clean
```

The build script automatically downloads pre-compiled libwebrtc from
[crow-misia/libwebrtc-bin](https://github.com/crow-misia/libwebrtc-bin) and
caches it under `~/libwebrtc`.

### Manual Bazel Build

```bash
# Ensure libwebrtc is available
export LIBWEBRTC_DIR=~/libwebrtc

# Build shim
bazel build //shim:webrtc_shim --config=darwin_arm64

# Output: bazel-bin/shim/libwebrtc_shim.{dylib,so}
```

## License

MIT

## See Also

- [PLAN.md](PLAN.md) - Detailed design document and implementation progress
- [Pion WebRTC](https://github.com/pion/webrtc) - Pure Go WebRTC implementation
- [libwebrtc](https://webrtc.googlesource.com/src) - Google's WebRTC implementation
