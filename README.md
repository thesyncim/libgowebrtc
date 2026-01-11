# libgowebrtc

**Pion-compatible Go wrapper for libwebrtc** - high-performance video/audio encoding, decoding, and WebRTC connectivity without CGO.

[![Test](https://github.com/thesyncim/libgowebrtc/actions/workflows/test.yml/badge.svg)](https://github.com/thesyncim/libgowebrtc/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/thesyncim/libgowebrtc.svg)](https://pkg.go.dev/github.com/thesyncim/libgowebrtc)
[![Go Report Card](https://goreportcard.com/badge/github.com/thesyncim/libgowebrtc)](https://goreportcard.com/report/github.com/thesyncim/libgowebrtc)

## Features

- **H.264, VP8, VP9, AV1** video encoding/decoding via libwebrtc
- **Opus** audio encoding/decoding
- **Allocation-free** hot paths - caller provides all buffers
- **Pion-compatible** - implements `webrtc.TrackLocal` for seamless integration
- **Browser-like API** - `GetUserMedia()`, `GetDisplayMedia()`, `PeerConnection`
- **SVC/Simulcast** support with Chrome/Firefox-compatible presets
- **purego FFI** - no CGO required by default, optional CGO mode for 5x faster FFI
- **Device capture** - camera, microphone, screen/window capture

## Why libgowebrtc?

libgowebrtc brings native codec performance to Go WebRTC applications. It's designed to **complement** [Pion](https://github.com/pion/webrtc) - use Pion for networking and signaling, libgowebrtc for encoding/decoding.

**Key benefits:**
- **Native codec performance** - H.264, VP8, VP9, AV1 via Google's libwebrtc
- **Hardware acceleration** - VideoToolbox on macOS for H.264
- **SVC/Simulcast** - Full support with Chrome/Firefox-compatible presets
- **Browser-like API** - `GetUserMedia`, `GetDisplayMedia`, `PeerConnection`
- **No CGO required** - Uses purego by default (optional CGO mode for 5x faster FFI)
- **Pion integration** - Implements `webrtc.TrackLocal` for seamless interop

**Use cases:**
- Add native codecs to your Pion-based SFU/MCU
- Build browser-like WebRTC apps in Go
- High-throughput media processing pipelines
- Hardware-accelerated transcoding

## Architecture

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
        ┌──────────────────────┼──────────────────────┐
        │ (linked)             │ (dlopen)             │ (framework)
        │                      │                      │
┌───────▼───────┐    ┌─────────▼─────────┐    ┌──────▼───────┐
│   libwebrtc   │    │     OpenH264      │    │ VideoToolbox │
│ VP8/VP9/AV1   │    │   (H.264 codec)   │    │(macOS H.264) │
│    Opus       │    │ auto-downloaded   │    │  hardware    │
│ Google WebRTC │    │   from Cisco      │    │  accelerated │
└───────────────┘    └───────────────────┘    └──────────────┘
```

**Runtime loading:**
- **libwebrtc_shim**: Auto-downloaded from GitHub releases on first use, cached in `~/.libgowebrtc/`
- **OpenH264**: Auto-downloaded from Cisco on first H.264 use, loaded via `dlopen` at runtime
- **VideoToolbox**: macOS system framework (no download needed)

## Installation

```bash
go get github.com/thesyncim/libgowebrtc
```

By default, the runtime will auto-download the prebuilt `libwebrtc_shim` for supported
OS/arch combinations (`darwin_arm64`, `darwin_amd64`, `linux_amd64`, `linux_arm64`, `windows_amd64`)
from GitHub Releases and cache it under `~/.libgowebrtc`. For other platforms, build the
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
| libwebrtc_shim | shim-v0.4.0 | [thesyncim/libgowebrtc releases](https://github.com/thesyncim/libgowebrtc/releases) |
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

Supported platforms: `darwin_arm64`, `darwin_amd64`, `linux_amd64`, `linux_arm64`, `windows_amd64`

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

### At a Glance

| Category | Status | Key Features |
|----------|--------|--------------|
| **Encoding/Decoding** | ✅ Complete | H.264, VP8, VP9, AV1, Opus - allocation-free |
| **PeerConnection** | ✅ Complete | Offer/answer, ICE, tracks, data channels |
| **RTP Control** | ✅ Complete | Sender/receiver/transceiver, simulcast layers |
| **Media Capture** | ✅ Complete | Camera, microphone, screen/window |
| **Statistics** | ✅ Complete | Full RTCStats, BWE, quality metrics |

<details>
<summary><strong>Core Encoding/Decoding</strong></summary>

- H.264/VP8/VP9/AV1 video encoding/decoding via FFI
- Opus audio encoding/decoding via FFI
- Allocation-free encode/decode with reusable buffers
- Runtime bitrate/framerate control
- Keyframe request
</details>

<details>
<summary><strong>PeerConnection</strong></summary>

- Full offer/answer/ICE support
- Track writing with frame push to native source
- Frame receiving from remote tracks (`SetOnVideoFrame`/`SetOnAudioFrame`)
- DataChannel communication
- `GetStats()` - connection statistics
- `RestartICE()` - ICE restart trigger
- `AddTransceiver()` - add transceivers with direction control
</details>

<details>
<summary><strong>RTPSender</strong></summary>

| Method | Description |
|--------|-------------|
| `ReplaceTrack()` | Replace sender track |
| `SetParameters()` / `GetParameters()` | Encoding parameters |
| `SetLayerActive()` / `SetLayerBitrate()` | Simulcast layer control |
| `GetActiveLayers()` | Get active layer count |
| `SetOnRTCPFeedback()` | RTCP feedback events (PLI/FIR/NACK) |
| `SetScalabilityMode()` / `GetScalabilityMode()` | Runtime SVC mode control |
| `GetStats()` | Sender statistics |
</details>

<details>
<summary><strong>RTPReceiver</strong></summary>

| Method | Description |
|--------|-------------|
| `GetStats()` | Receiver statistics |
| `RequestKeyframe()` | Send PLI |
| `SetJitterBufferTarget()` | Set target buffer delay |
| `SetJitterBufferBounds()` | Set min/max delay bounds |
| `GetJitterBufferStats()` | Get buffer statistics |
| `OnJitterBufferStats()` | Periodic stats callback |
| `SetAdaptiveJitterBuffer()` | Enable/disable adaptive mode |
</details>

<details>
<summary><strong>RTPTransceiver</strong></summary>

- `SetDirection()` / `Direction()` / `CurrentDirection()` - direction control
- `Stop()` - stop transceiver
- `Mid()` - get media ID
- `Sender()` / `Receiver()` - get sender/receiver
</details>

<details>
<summary><strong>Event Callbacks</strong></summary>

| Callback | Description |
|----------|-------------|
| `OnConnectionStateChange` | Connection state events |
| `OnSignalingStateChange` | Signaling state events |
| `OnICEConnectionStateChange` | ICE connection state events |
| `OnICEGatheringStateChange` | ICE gathering progress events |
| `OnNegotiationNeeded` | Renegotiation trigger events |
| `OnICECandidate` | New ICE candidate events |
| `OnTrack` | Remote track received events |
| `OnDataChannel` | Data channel received events |
</details>

<details>
<summary><strong>Media Capture</strong></summary>

- Device/screen capture via `GetUserMedia`/`GetDisplayMedia`
- Pion interop (libwebrtc tracks work with Pion PC)
</details>

<details>
<summary><strong>Statistics (RTCStats)</strong></summary>

- Transport stats (bytes/packets sent/received)
- Quality metrics (RTT, jitter, packet loss)
- Video stats (frames encoded/decoded, keyframes, NACK/PLI/FIR)
- Audio stats (audio level, energy, concealment)
- SCTP/DataChannel stats - channels opened/closed, messages sent/received
- Quality limitation - reason (none/cpu/bandwidth/other) and duration
- Remote RTP stats - remote jitter, RTT, packet loss
</details>

<details>
<summary><strong>Codec & Bandwidth APIs</strong></summary>

**Codec Capabilities:**
- `GetSupportedVideoCodecs()` - enumerate video codecs (VP8, VP9, H264, AV1)
- `GetSupportedAudioCodecs()` - enumerate audio codecs (Opus, PCMU, PCMA)
- `IsCodecSupported(mimeType)` - check codec support

**Bandwidth Estimation:**
- `GetBandwidthEstimate()` - get current BWE (target bitrate, available bandwidth)
- `SetOnBandwidthEstimate(callback)` - receive BWE updates
</details>

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

## Performance Benchmarks

Tested on Apple M2 Pro at 1280x720:

| Codec | Encode Time | Notes |
|-------|-------------|-------|
| H.264 | ~1.14 ms/frame | OpenH264 software encoder |
| VP8 | ~3.08 ms/frame | libvpx |
| VP9 | ~3.21 ms/frame | libvpx |
| AV1 | ~1.88 ms/frame | libaom |

**FFI Overhead:**

| Mode | Overhead | Requirements |
|------|----------|--------------|
| purego (default) | ~200 ns/call | None (pure Go) |
| CGO (`-tags ffigo_cgo`) | ~44 ns/call | C compiler |

Run benchmarks locally:
```bash
go test -bench=BenchmarkAllVideoCodecs -benchtime=1s ./test/e2e/
```

## Build Status

The Go layer and FFI bindings are complete for all WebRTC functionality. Bazel builds the shim:

| Platform | Status |
|----------|--------|
| darwin_arm64 (macOS Apple Silicon) | ✅ Working |
| darwin_amd64 (macOS Intel) | ✅ Working |
| linux_amd64 (Linux x86_64) | ✅ Working |
| linux_arm64 (Linux ARM64) | ✅ Working |
| windows_amd64 (Windows x64) | ✅ Working |

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

# Cross-compile for Intel Mac (from ARM64 Mac)
./scripts/build.sh --target darwin_amd64

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

## Troubleshooting

<details>
<summary><strong>Shim library not found</strong></summary>

**Error:** `failed to load libwebrtc_shim`

**Solutions:**
1. Let auto-download work (default behavior downloads from GitHub releases)
2. Set explicit path: `export LIBWEBRTC_SHIM_PATH=/path/to/libwebrtc_shim.dylib`
3. Check platform is supported: `darwin_arm64`, `darwin_amd64`, `linux_amd64`, `linux_arm64`, `windows_amd64`

</details>

<details>
<summary><strong>H.264 encoding fails</strong></summary>

**Error:** `failed to create H264 encoder` or codec not found

**Solutions:**
1. OpenH264 should auto-download from Cisco on first use
2. Check cache: `ls ~/.libgowebrtc/openh264/`
3. Set explicit path: `export LIBWEBRTC_OPENH264_PATH=/path/to/libopenh264.dylib`
4. On macOS, VideoToolbox is used by default - try `PreferHW: false` to use OpenH264

</details>

<details>
<summary><strong>CGO mode issues</strong></summary>

**Error:** `undefined: ...` when building with `-tags ffigo_cgo`

**Solutions:**
1. Ensure C compiler is installed (`gcc`, `clang`, or MSVC)
2. On macOS: `xcode-select --install`
3. On Linux: `apt install build-essential`
4. On Windows: Install Visual Studio Build Tools

</details>

<details>
<summary><strong>Video not appearing in browser</strong></summary>

**Potential causes:**
1. ICE connectivity failed - check STUN/TURN servers
2. Codec mismatch - browser may not support chosen codec
3. Firewall blocking UDP - try TURN with TCP

**Debug steps:**
```go
pc.OnConnectionStateChange = func(state pc.PeerConnectionState) {
    log.Printf("Connection state: %s", state)
}
pc.OnICEConnectionStateChange = func(state pc.ICEConnectionState) {
    log.Printf("ICE state: %s", state)
}
```

</details>

## Contributing

Contributions are welcome! Here's how to get started:

1. **Report bugs** - Open an issue with reproduction steps
2. **Request features** - Describe the use case in an issue
3. **Submit PRs** - Fork, create a branch, make changes, open PR

**Development setup:**
```bash
# Clone and build
git clone https://github.com/thesyncim/libgowebrtc
cd libgowebrtc

# Run tests (downloads shim automatically)
go test ./...

# Run with verbose output
go test -v ./pkg/encoder/...

# Run linter
golangci-lint run
```

**Code style:**
- Run `golangci-lint run` before committing
- Follow existing patterns in the codebase
- Add tests for new functionality
- Keep allocation-free hot paths allocation-free

## License

MIT

## See Also

- [API Documentation (pkg.go.dev)](https://pkg.go.dev/github.com/thesyncim/libgowebrtc) - Full Go API reference
- [PLAN.md](PLAN.md) - Detailed design document and implementation progress
- [Pion WebRTC](https://github.com/pion/webrtc) - Pure Go WebRTC implementation
- [libwebrtc](https://webrtc.googlesource.com/src) - Google's WebRTC implementation
- [OpenH264](https://github.com/cisco/openh264) - Cisco's H.264 codec (BSD licensed)
