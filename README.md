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
- **Browser-like capture API** - `GetUserMedia()`, `GetDisplayMedia()`, `PeerConnection`
- **SVC/Simulcast** support with Chrome/Firefox-compatible presets
- **purego FFI** - no CGO required by default, optional CGO mode for 5x faster FFI
- **Device capture** - camera, microphone, screen/window capture
- **Runtime diagnostics** - `pkg/diagnostics.Check()` preflights shim/OpenH264 state without downloading

## Why libgowebrtc?

libgowebrtc brings native codec performance to Go WebRTC applications. It's designed to **complement** [Pion](https://github.com/pion/webrtc) - use Pion for networking and signaling, libgowebrtc for encoding/decoding.

**Key benefits:**
- **Native codec performance** - H.264, VP8, VP9, AV1 via Google's libwebrtc
- **Hardware acceleration** - VideoToolbox on macOS for H.264
- **SVC/Simulcast** - Full support with Chrome/Firefox-compatible presets
- **Browser-like capture API** - `GetUserMedia`, `GetDisplayMedia`, `PeerConnection`
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
OS/arch combinations (`darwin_arm64`, `darwin_amd64`, `linux_386`, `linux_amd64`, `windows_amd64`)
from GitHub Releases and cache it under `~/.libgowebrtc`. For other platforms, build the
shim locally and set `LIBWEBRTC_SHIM_PATH`.

Override behavior with:

- `LIBWEBRTC_SHIM_PATH=/path/to/libwebrtc_shim.{so|dylib|dll}` (use a local shim)
- `LIBWEBRTC_SHIM_DISABLE_DOWNLOAD=1` (disable auto-download)
- `LIBWEBRTC_SHIM_CACHE_DIR=/custom/cache/dir` (override cache location)
- `LIBWEBRTC_SHIM_FLAVOR=basic` (override shim flavor; default: basic)

If `LIBWEBRTC_SHIM_PATH` is set, it is treated as authoritative. A missing or
invalid path fails diagnostics and runtime loading instead of falling back to a
different shim on disk.

## Supported Platform Tiers

| Platform | Shim Auto-Download | Tier | Notes |
|----------|--------------------|------|-------|
| `darwin_arm64` | Yes | Primary | Most complete local development path |
| `darwin_amd64` | Yes | Primary | Validated via Rosetta in CI |
| `linux_amd64` | Yes | Primary | Main Linux runtime target |
| `linux_386` | Yes | Secondary | Release-validated in Docker |
| `windows_amd64` | Yes | Secondary | Shim release artifact supported |
| `linux_arm`, `linux_arm64` | No | Experimental | Build shim locally and set `LIBWEBRTC_SHIM_PATH` |

## Runtime Diagnostics

Use [`pkg/diagnostics`](./pkg/diagnostics) to inspect runtime readiness without
triggering downloads or mutating loader state:

```go
report, err := diagnostics.Check()
if err != nil {
    log.Fatal(err)
}

if !report.Ready {
    log.Printf("blocking issues: %v", report.Shim.BlockingIssues)
}
log.Printf("shim source=%s path=%s checksum=%s", report.Shim.Source, report.Shim.Path, report.Shim.ChecksumStatus)
```

`diagnostics.Check()` reports resolved shim/OpenH264 paths, source (`local` vs
`downloaded`), cache directories, version/checksum status, and blocking issues.

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
| libwebrtc (Linux source build) | branch-heads/7390 @ `d2eaa5570fc9959f8dbde32912a16366b8ee75f4` | [webrtc.googlesource.com/src](https://webrtc.googlesource.com/src) |
| libwebrtc_shim | shim-v0.4.5 | [thesyncim/libgowebrtc releases](https://github.com/thesyncim/libgowebrtc/releases) |
| OpenH264 | 2.5.1 | [Cisco OpenH264](https://github.com/cisco/openh264/releases) |

### Building the Shim

The shim is built using Bazel.

- macOS and Windows use pre-built libwebrtc archives from
  [crow-misia/libwebrtc-bin](https://github.com/crow-misia/libwebrtc-bin)
- Linux local builds use a pinned WebRTC source checkout by default because the
  upstream Linux prebuilt archive is built against Chromium's custom libc++
  runtime and does not link cleanly with the shim toolchain

```bash
# Build for current platform
./scripts/build.sh

# Build/validate the Linux shim in a compatibility Docker image
./scripts/validate_linux_docker.sh --target linux_amd64

# Validate published Linux release artifacts in Docker
./scripts/validate_linux_docker.sh --target linux_amd64 --download-only
./scripts/validate_linux_docker.sh --target linux_386 --download-only

# Publish a prepared local release directory
./scripts/release.sh 0.4.5 --release-dir release/shim-v0.4.5
```

Environment variables:
- `LIBWEBRTC_VERSION` - Pre-compiled version (default: 141.7390.2.0)
- `INSTALL_DIR` - Where to cache libwebrtc (default: ~/libwebrtc)
- `LIBWEBRTC_SOURCE_BUILD` - `auto` (default), `true`, or `false`

Local build targets: `darwin_arm64`, `darwin_amd64`, `linux_386`, `linux_arm`, `linux_amd64`, `linux_arm64`, `windows_amd64`

Release flow:
- Build or validate the platform artifacts locally
- Build Linux release artifacts in the compatibility Docker image or on a host
  that passes the `MAX_GLIBC_VERSION` release check
- Upload the prepared `release/shim-vX.Y.Z/` directory with `./scripts/release.sh`
- CI downloads the published artifacts and runs the smoke tests; it does not rebuild the shim

## Quick Start

### Browser-Like API

```go
import (
    "github.com/pion/webrtc/v4"

    "github.com/thesyncim/libgowebrtc/pkg/media"
    "github.com/thesyncim/libgowebrtc/pkg/codec"
)

// Get camera and microphone (like navigator.mediaDevices.getUserMedia)
stream, _ := media.GetUserMedia(media.Constraints{
    Video: &media.VideoConstraints{
        Width:     media.ExactInt(1280),
        Height:    media.ExactInt(720),
        FrameRate: media.ExactFloat(30),
        Codec:     codec.VP9,
    },
    Audio: &media.AudioConstraints{
        SampleRate: media.ExactInt(48000),
    },
})

// Create PeerConnection
peerConnection, _ := webrtc.NewPeerConnection(webrtc.Configuration{
    ICEServers: []webrtc.ICEServer{
        {URLs: []string{"stun:stun.l.google.com:19302"}},
    },
})

// Add capture-backed tracks to a Pion PeerConnection
senders, _ := media.AddTracksToPionPeerConnection(peerConnection, stream)

// Create offer
offer, _ := peerConnection.CreateOffer(nil)
peerConnection.SetLocalDescription(offer)
```

`pkg/media` is capture-only. For synthetic/manual frame production, use [`pkg/track`](./pkg/track).
For native libwebrtc-backed senders, create tracks through [`pkg/pc`](./pkg/pc).
Use `media.GetSupportedConstraints()` and typed `GetCapabilities()` calls to
inspect which browser-shaped knobs the library can actually honor for a given
capture-backed track.

### Pion Integration

```go
import (
    "github.com/pion/webrtc/v4"
    "github.com/thesyncim/libgowebrtc/pkg/codec"
    "github.com/thesyncim/libgowebrtc/pkg/track"
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

When you want browser-like `MediaStream` semantics with manual Pion control,
use `media.PionTrackLocalForStream(stream, track)` so the remote side sees the
stream's `msid`. `media.PionTrackLocal(track)` remains the raw escape hatch.

### Browser Codec Presets

```go
import (
    "github.com/pion/webrtc/v4"
    "github.com/thesyncim/libgowebrtc/pkg/pc"
    "github.com/thesyncim/libgowebrtc/pkg/pioncodec"
    "github.com/thesyncim/libgowebrtc/pkg/track"
)

// Chrome-shaped local encode preferences.
preset := pioncodec.BrowserPreset(
    pioncodec.BrowserChrome,
    pioncodec.DirectionEncode,
    pioncodec.PresetModeSupported,
)

videoTrack, _ := track.NewVideoTrackFromPreset(preset, track.VideoTrackConfig{
    ID:      "video",
    Width:   1280,
    Height:  720,
    Bitrate: 2_000_000,
    FPS:     30,
})

// Apply the same supported subset to libgowebrtc's native PeerConnection wrapper.
pc, _ := pc.NewPeerConnection(pc.DefaultConfiguration())
transceiver, _ := pc.AddTransceiver("video", &pc.TransceiverInit{Direction: pc.TransceiverDirectionSendOnly})
_ = transceiver.SetCodecSet(preset)

// For direct Pion use, keep the full browser-shaped RTP codec list.
pionPC, _ := webrtc.NewPeerConnection(webrtc.Configuration{})
recv, _ := pionPC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
_ = recv.SetCodecPreferences(
    pioncodec.BrowserPreset(
        pioncodec.BrowserChrome,
        pioncodec.DirectionDecode,
        pioncodec.PresetModeNegotiation,
    ).VideoCodecs(),
)
```

### Pion Receive Integration

`pionrecv.BindRemoteTrack(...)` is the preferred entrypoint inside a Pion `OnTrack` callback. Automatic PLI requests are best-effort; explicit `RequestKeyframe()` calls still return writer errors.

```go
import (
    "github.com/pion/webrtc/v4"
    "github.com/thesyncim/libgowebrtc/pkg/frame"
    "github.com/thesyncim/libgowebrtc/pkg/pionrecv"
)

pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
    decoded, err := pionrecv.BindRemoteTrack(
        track,
        receiver,
        pionrecv.WithRTCPWriter(receiver.Transport()),
    )
    if err != nil {
        return
    }

    decoded.SetOnCodecChange(func(change pionrecv.CodecChange) {
        println("codec switch", change.PreviousCodec.MimeType, "->", change.CurrentCodec.MimeType)
    })
    _ = decoded.SetOnVideoFrame(func(frame *frame.VideoFrame) {
        // Use decoded I420 frame
    })

    go decoded.Run()
})
```

### Callback-Based Receive Integration

```go
import (
    "github.com/pion/rtcp"
    "github.com/pion/webrtc/v4"
    "github.com/thesyncim/libgowebrtc/pkg/frame"
    "github.com/thesyncim/libgowebrtc/pkg/pionrecv"
)

type receivedTrack struct {
    Track       *webrtc.TrackRemote
    RTPReceiver *webrtc.RTPReceiver
    WriteRTCP   func([]rtcp.Packet) error
}

func (h *subscriber) OnTrack(track receivedTrack) {
    decoded, err := pionrecv.BindRemoteTrack(
        track.Track,
        track.RTPReceiver,
        pionrecv.WithWriteRTCP(track.WriteRTCP),
    )
    if err != nil {
        return
    }

    decoded.SetOnCodecChange(func(change pionrecv.CodecChange) {
        // React to receiver-side codec switches
    })
    _ = decoded.SetOnVideoFrame(func(frame *frame.VideoFrame) {
        // Render, forward, or transform decoded frames
    })

    go decoded.Run()
}
```

### Browser-Like Remote Streams

When you want browser-style `ontrack` behavior, use
`media.RemoteStreamRegistry`. It groups repeated track callbacks into stable
`MediaStream` objects keyed by remote stream ID.

The browser-like layer is backend-neutral:
- `media.RemoteTrack`, `media.RemoteVideoTrack`, and `media.RemoteAudioTrack`
  work across Pion and native `pkg/pc`
- `media.PionRemoteVideoTrack` / `media.PionRemoteAudioTrack` add decoded-track
  metadata and codec-switch controls
- `media.PCRemoteVideoTrack` / `media.PCRemoteAudioTrack` expose the underlying
  libwebrtc `pkg/pc` receiver objects when you need to drop lower

Pion-backed receive flow:

```go
import (
    "github.com/pion/webrtc/v4"
    "github.com/thesyncim/libgowebrtc/pkg/frame"
    "github.com/thesyncim/libgowebrtc/pkg/media"
)

registry := media.NewRemoteStreamRegistry()

pc.OnTrack(registry.PionOnTrack(func(event media.PionTrackEvent) {
    video, ok := event.Track.(media.PionRemoteVideoTrack)
    if !ok {
        return
    }

    _ = video.SetOnVideoFrame(func(f *frame.VideoFrame) {
        println("decoded frame", f.Width, f.Height, "from stream", event.Streams[0].ID())
    })
}, func(err error) {
    println("remote bind error:", err.Error())
}))
```

Native libwebrtc-backed receive flow:

```go
import (
    "github.com/thesyncim/libgowebrtc/pkg/frame"
    "github.com/thesyncim/libgowebrtc/pkg/media"
    "github.com/thesyncim/libgowebrtc/pkg/pc"
)

registry := media.NewRemoteStreamRegistry()

peer.SetOnTrack(registry.PCOnTrack(func(event media.PCTrackEvent) {
    video, ok := event.Track.(media.PCRemoteVideoTrack)
    if !ok {
        return
    }

    _ = video.SetOnVideoFrame(func(f *frame.VideoFrame) {
        println("native frame", f.Width, f.Height, "streams", len(event.Streams))
    })
}, func(err error) {
    println("remote bind error:", err.Error())
}))
```

If you already manage the Pion receive pipeline yourself with `pionrecv.New(...)`
or `pionrecv.BindRemoteTrack(...)`, use `registry.BindDecodedTrack(decoded)` to
project that decoded track into the same browser-like `MediaStream` model.

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
│   ├── diagnostics/    # Runtime preflight checks
│   ├── frame/          # VideoFrame, AudioFrame types
│   ├── packetizer/     # RTP packetization
│   ├── depacketizer/   # RTP depacketization
│   ├── track/          # Pion-compatible TrackLocal
│   ├── pionrecv/       # Pion TrackRemote -> decoded frame bridge
│   ├── pc/             # PeerConnection (libwebrtc-backed)
│   └── media/          # Browser-like capture/stream API
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
| **RTP Control** | ⚠️ Partial | Sender/receiver/transceiver, simulcast layers; RTCP feedback callback not yet supported |
| **Media Capture** | ✅ Complete | Camera, microphone, screen/window |
| **Statistics** | ⚠️ Partial | PeerConnection and RTPReceiver stats; sender stats and BWE are not yet supported |

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
- Browser-style `addTrack(track, streamA, streamB, ...)` stream identity preservation
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
| `SetOnRTCPFeedback()` | Returns `pc.ErrNotSupported` in the current shim |
| `SetScalabilityMode()` / `GetScalabilityMode()` | Runtime SVC mode control |
| `StreamIDs()` | MediaStream IDs associated with the sender |
| `GetStats()` | Returns `pc.ErrNotSupported` in the current shim |
</details>

<details>
<summary><strong>RTPReceiver</strong></summary>

| Method | Description |
|--------|-------------|
| `GetStats()` | Receiver statistics |
| `SetJitterBufferMinDelay()` | Set minimum jitter buffer delay |
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
| `SetOnConnectionStateChange(...)` | Connection state events |
| `SetOnSignalingStateChange(...)` | Signaling state events |
| `SetOnICEConnectionStateChange(...)` | ICE connection state events |
| `SetOnICEGatheringStateChange(...)` | ICE gathering progress events |
| `SetOnNegotiationNeeded(...)` | Renegotiation trigger events |
| `SetOnICECandidate(...)` | New ICE candidate events |
| `SetOnTrack(...)` | Remote track received events with browser-style stream IDs |
| `SetOnDataChannel(...)` | Data channel received events |
</details>

<details>
<summary><strong>Media Capture</strong></summary>

- Capture-backed device/screen streams via `GetUserMedia`/`GetDisplayMedia`
- Browser-style supported-constraint and per-track capability discovery
- Manual frame injection lives in `pkg/track`
- Pion interop, including MediaStream-aware `msid` preservation via `AddTracksToPC`
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
- `GetCurrentBandwidthEstimate()` currently returns `pc.ErrNotSupported`
- `SetOnBandwidthEstimate(callback)` currently returns `pc.ErrNotSupported`
</details>

### Jitter Buffer Control

Control libwebrtc's minimum jitter-buffer floor for latency vs quality tradeoffs:

```go
receiver := transceiver.Receiver()

// Low latency floor (gaming, live streaming)
_ = receiver.SetJitterBufferMinDelay(50)

stats, _ := receiver.GetStats()
log.Printf("buffer target=%.2fms minimum=%.2fms emitted=%d",
    stats.JitterBufferTargetDelayMs,
    stats.JitterBufferMinimumDelayMs,
    stats.JitterBufferEmittedCount,
)
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
| linux_386 (Linux x86) | Source build path |
| linux_amd64 (Linux x86_64) | Source build path |
| linux_arm64 (Linux ARM64) | Experimental local source build |
| linux_arm (Linux ARM32) | Source build path |
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
- curl
- macOS shim builds: full Xcode (Command Line Tools alone are not enough for Bazel's Apple toolchain)
- Linux only: enough disk space and time for a pinned WebRTC source checkout on
  the first build

### Build Commands

```bash
# Build shim for the current platform
./scripts/build.sh

# Cross-compile for Intel Mac (from ARM64 Mac)
./scripts/build.sh --target darwin_amd64

# Force the source-build path explicitly
./scripts/build.sh --source-libwebrtc

# Create release tarball
./scripts/build.sh --release

# Recommended Linux release path (portable glibc baseline)
./scripts/validate_linux_docker.sh --target linux_amd64

# Clean and rebuild
./scripts/build.sh --clean
```

Build behavior:

- Linux defaults to a pinned WebRTC source build and caches it under
  `~/libwebrtc_source_<target>`
- macOS and Windows default to the prebuilt libwebrtc download path
- `LIBWEBRTC_SOURCE_BUILD=false` forces the prebuilt path
- `LIBWEBRTC_SOURCE_BUILD=true` forces the source-build path
- Linux `--release` builds enforce a maximum GLIBC symbol version
  (`MAX_GLIBC_VERSION`, default `2.31`) to keep published shims portable

### Manual Bazel Build

```bash
# Ensure libwebrtc is available
export LIBWEBRTC_DIR=~/libwebrtc

# Build shim
bazel build //shim:webrtc_shim --config=darwin_arm64

# Output: bazel-bin/shim/libwebrtc_shim.{dylib,so}
```

### FFI Generation (when shim.h changes)

If you add or modify any `SHIM_EXPORT` or `typedef struct { ... }` in `shim/shim.h`:

```bash
# 1) Update function signatures in internal/ffi/gen/funcs.json
#    (params are passed as a single uintptr to the params struct)

# 2) Ensure matching Go structs exist in internal/ffi/ for every C struct
#    (layout tests are generated from these)

# 3) Regenerate bindings, layout tests, and types.json
go generate ./internal/ffi

# 4) Rebuild the shim for your platform
./scripts/build.sh
```

Notes:
- `internal/ffi/gen/types.json` is generated from `shim/shim.h` and Go structs; do not edit by hand.
- When you break the shim ABI, bump `kShimVersion` in `shim/shim_common.cc` and `ExpectedShimVersion` in `internal/ffi/lib.go`.
- For CGO layout tests, ensure `CGO_ENABLED=1` and run: `go test -tags ffigo_cgo ./internal/ffi -run TestShimStructLayoutCgo`.

## Troubleshooting

<details>
<summary><strong>Shim library not found</strong></summary>

**Error:** `failed to load libwebrtc_shim`

**Solutions:**
1. Let auto-download work (default behavior downloads from GitHub releases)
2. Set explicit path: `export LIBWEBRTC_SHIM_PATH=/path/to/libwebrtc_shim.dylib`
3. Confirm that explicit path exists; once set, it is authoritative and will not fall back to another shim
4. Check platform is supported for auto-download: `darwin_arm64`, `darwin_amd64`, `linux_386`, `linux_amd64`, `windows_amd64`
5. For unsupported platforms, build the shim locally and point `LIBWEBRTC_SHIM_PATH` at it

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
pc.SetOnConnectionStateChange(func(state pc.PeerConnectionState) {
    log.Printf("Connection state: %s", state)
})
pc.SetOnICEConnectionStateChange(func(state pc.ICEConnectionState) {
    log.Printf("ICE state: %s", state)
})
```

</details>

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](./CONTRIBUTING.md) for the
current development and quality-check flow.

Quick start:

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

# Run contract checks
bash ./scripts/check_docs_contract.sh
```

**Code style:**
- Run `golangci-lint run` before committing
- Follow existing patterns in the codebase
- Add tests for new functionality
- Keep allocation-free hot paths allocation-free

## License

MIT. See [LICENSE](./LICENSE).

## See Also

- [API Documentation (pkg.go.dev)](https://pkg.go.dev/github.com/thesyncim/libgowebrtc) - Full Go API reference
- [PLAN.md](PLAN.md) - Detailed design document and implementation progress
- [Pion WebRTC](https://github.com/pion/webrtc) - Pure Go WebRTC implementation
- [libwebrtc](https://webrtc.googlesource.com/src) - Google's WebRTC implementation
- [OpenH264](https://github.com/cisco/openh264) - Cisco's H.264 codec (BSD licensed)
