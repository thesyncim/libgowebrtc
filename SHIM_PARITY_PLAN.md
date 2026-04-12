# Shim Parity Plan

## Goal

Make the thin public layer feel complete and trustworthy on top of the shim.
That means:

- if libwebrtc has a real capability we intend to expose, the shim should carry it
- the Go thin layer should not stop at `ErrNotSupported` or Go-side validation
  when the native stack can support the feature
- unsupported surfaces should be explicit, tracked, and ranked rather than left
  as scattered TODOs or stubs

This document is the working backlog for that parity effort.

## Scope

This plan is about the thin runtime/core path:

- `pkg/pc`
- `pkg/track`
- direct `internal/ffi` bindings
- `shim/` coverage

It is not primarily about helper-layer cleanup, docs polish, or testkit DSLs.

## Current Baseline

The shim-backed thin layer already covers a lot of the transport/media surface:

- peer connection creation, offer/answer, local/remote descriptions, ICE
- data channels
- transceivers, senders, receivers, and track add/remove
- full peer connection stats via `PeerConnection.GetStats()`
- sender encoding controls for layer active/bitrate
- scalability-mode get/set
- receiver jitter-buffer minimum delay
- local audio/video tracks and direct frame push

The remaining parity work is concentrated in telemetry, feedback, and a few
control surfaces that are either stubbed in the shim or blocked in Go before
they reach libwebrtc.

## Parity Inventory

| Priority | Area | User-facing goal | Current state | Layers involved | Key files |
| --- | --- | --- | --- | --- | --- |
| P0 | Peer connection bandwidth estimate | Observe current BWE and receive live BWE updates from libwebrtc | FFI types/callback registries exist, but the shim returns `SHIM_ERROR_NOT_SUPPORTED` and there is no thin `pkg/pc` surface on `main` | `pkg/pc` + `internal/ffi` + `shim` | `pkg/pc/peerconnection.go`, `internal/ffi/peerconnection.go`, `internal/ffi/params_peerconnection.go`, `internal/ffi/params_stats.go`, `shim/shim.h`, `shim/shim_peer_connection.cc` |
| P0 | Sender statistics | Retrieve sender-local stats without forcing callers to scrape the full connection report | `internal/ffi.RTPSenderGetStats` returns `ErrNotSupported`; shim stub returns `SHIM_ERROR_NOT_SUPPORTED`; no public `RTPSender.GetStats()` surface | `pkg/pc` + `internal/ffi` + `shim` | `pkg/pc/peerconnection.go`, `internal/ffi/peerconnection.go`, `shim/shim.h`, `shim/shim_rtp_sender.cc` |
| P0 | Sender RTCP feedback callback | Observe PLI/FIR/NACK on the sender path so adaptation can react to real transport feedback | FFI callback plumbing exists, but it is stubbed; shim no-ops; no public `RTPSender.SetOnRTCPFeedback(...)` surface | `pkg/pc` + `internal/ffi` + `shim` | `pkg/pc/peerconnection.go`, `internal/ffi/peerconnection.go`, `internal/ffi/params_rtp.go`, `shim/shim.h`, `shim/shim_rtp_sender.cc` |
| P1 | Receiver-local stats | Provide `RTPReceiver.GetStats()` parity instead of returning an empty placeholder | Shim currently returns zeroed stats "for API consistency"; `PeerConnection.GetStats()` works, but receiver-local parity is incomplete | `pkg/pc` + `internal/ffi` + `shim` | `pkg/pc/peerconnection.go`, `internal/ffi/peerconnection.go`, `shim/shim.h`, `shim/shim_rtp_receiver.cc` |
| P1 | Fuller RTP sender parameter mutation | Let `RTPSender.SetParameters(...)` express more of libwebrtc's supported surface before we reject it in Go | Go validation currently blocks codec mutation, header-extension mutation, and most non-RID encoding fields before the shim gets a chance | `pkg/pc` first, then possibly `internal/ffi` + `shim` | `pkg/pc/webrtc_helpers.go`, `pkg/pc/peerconnection.go`, `internal/ffi/peerconnection.go`, `shim/shim.h` |
| P1 | Simulcast/SVC encoder parity | Make multi-layer and scalability behavior predictable across the thin path | Sender layer/scalability controls exist, but encoder init can still report `"simulcast parameters not supported"` and the exact supported matrix is not mapped cleanly | likely `shim` + `internal/ffi`, maybe some `pkg/encoder` / `pkg/pc` follow-up | `shim/shim_video_codec.cc`, `internal/ffi/*encoder*`, `pkg/encoder`, `pkg/pc/peerconnection.go` |
| P2 | Receiver feedback/telemetry completeness | Decide which receiver-side hooks should be first-class versus intentionally PC-stats-only | Jitter buffer min delay is present, but receiver-local observability remains thinner than sender/PC parity | mostly `pkg/pc` + `internal/ffi` + `shim` | `pkg/pc/peerconnection.go`, `internal/ffi/peerconnection.go`, `shim/shim_rtp_receiver.cc` |

## Detailed Gap Notes

### 1. Peer connection bandwidth estimate

What we want:

- `pc.SetOnBandwidthEstimate(func(*BandwidthEstimate))`
- `pc.GetCurrentBandwidthEstimate()`

Why it matters:

- unlocks congestion-aware adaptation on the thin path
- closes the loop for sender/track adaptation without forcing a helper layer
- already appears in the long-range product direction in `PLAN.md`

Current blockers:

- `internal/ffi.PeerConnectionSetOnBandwidthEstimate(...)` is stubbed
- `internal/ffi.PeerConnectionGetBandwidthEstimate(...)` is stubbed
- shim declarations exist in `shim/shim.h`
- shim implementation in `shim/shim_peer_connection.cc` is currently a no-op / not-supported return

What "done" looks like:

- callback fires from real libwebrtc bandwidth updates
- current estimate getter returns a non-zero structured snapshot when available
- `pkg/pc` exposes stable typed methods
- tests cover callback registration, callback delivery, getter behavior, and teardown

### 2. Sender statistics

What we want:

- `sender.GetStats()` on the thin API

Why it matters:

- parity with the rest of the RTP object model
- simplifies sender-local observability
- avoids forcing every caller through whole-connection stats parsing

Current blockers:

- `internal/ffi.RTPSenderGetStats(...)` always returns `ErrNotSupported`
- `shim_rtp_sender_get_stats(...)` always returns `SHIM_ERROR_NOT_SUPPORTED`

What "done" looks like:

- sender stats are retrieved through the shim using real libwebrtc data
- public Go API returns typed stats or `webrtc.StatsReport` consistently with the rest of `pkg/pc`
- tests verify non-empty sender stats after negotiated media is flowing

### 3. Sender RTCP feedback callbacks

What we want:

- `sender.SetOnRTCPFeedback(...)` or an equivalent typed sender callback

Why it matters:

- exposes real transport feedback instead of relying on helper-only heuristics
- directly supports adaptive track behavior
- important for keyframe, nack, and congestion-driven reactions

Current blockers:

- FFI registry/callback scaffolding exists
- shim currently no-ops `shim_rtp_sender_set_on_rtcp_feedback(...)`
- no public thin API method exists yet

What "done" looks like:

- shim subscribes to RTCP feedback at the sender path
- FFI callback passes event type and SSRC safely into Go
- `pkg/pc` exposes the callback without helper-layer wrapping
- tests validate registration, event delivery, and cleanup

### 4. Receiver-local stats

What we want:

- `receiver.GetStats()`

Why it matters:

- fills an obvious parity hole in the RTP object graph
- gives callers a direct receiver inspection path
- reduces whole-report parsing for common receive-side metrics

Current blockers:

- shim deliberately returns an empty stats struct today
- FFI therefore succeeds but returns no useful data

What "done" looks like:

- receiver stats are populated from real libwebrtc receiver data where possible
- gaps that still need `PeerConnection.GetStats()` are documented precisely
- tests prove the returned stats are not just zero placeholders

### 5. Full RTP sender parameter parity

What we want:

- let the thin layer attempt more of libwebrtc's actual `RtpParameters` surface
- only reject fields once the native stack genuinely cannot represent them

Current blockers:

- `validateSendParameters(...)` rejects:
  - codec mutation
  - header-extension mutation
  - most encoding fields other than RID

Why it matters:

- the Go layer is currently narrower than the native surface
- this is a true thin-layer parity gap even before touching the shim

What "done" looks like:

- every currently rejected field is classified as:
  - safe to support now
  - needs FFI/shim expansion
  - genuinely not representable and therefore should remain rejected
- the rejection policy matches actual runtime limits instead of legacy caution

### 6. Simulcast / scalability parity

What we want:

- predictable multi-layer behavior across encoder and sender paths
- clear supported matrix for simulcast and SVC, not just best-effort knobs

Current blockers:

- encoder init can still surface `"simulcast parameters not supported"`
- sender layer controls exist, but the full end-to-end parity story is not yet mapped

What "done" looks like:

- supported codec/layer combinations are explicit
- thin-layer APIs fail only for real native limitations
- tests cover at least one real negotiated multi-layer path end to end

## Implementation Order

### Phase 1: Close the core telemetry loop

1. Peer connection bandwidth estimate callback + getter
2. Sender stats
3. Sender RTCP feedback callback

Why this first:

- it delivers the biggest parity win for thin transport/runtime observability
- it unlocks smarter adaptation without dragging helper layers back in
- the missing pieces are already clearly stubbed in `internal/ffi` and `shim/`

### Phase 2: Finish object-level stats parity

1. Receiver-local stats
2. Public API cleanup around stats surfaces

### Phase 3: Remove Go-side artificial limits

1. Audit `RTPSender.SetParameters(...)` rejections
2. Expand support where libwebrtc already allows it
3. Only keep explicit rejections for truly unsupported native cases

### Phase 4: Stabilize multilayer parity

1. Map exact simulcast/SVC support matrix
2. Fix native/shim gaps needed for stable multilayer behavior
3. Add end-to-end parity tests for negotiated multilayer send paths

## Test Matrix We Need

For each parity feature we add, we should cover:

- unit tests in `internal/ffi`
- public API tests in `pkg/pc`
- at least one end-to-end test that proves the feature works with real media/session state
- callback teardown/unregister coverage
- purego and CGO layout/binding safety where relevant
- platform-sensitive CI coverage, especially `darwin_arm64`, `linux_amd64`, and `windows_amd64`

## Non-Goals For This Document

These are important, but they are not the first shim parity backlog:

- helper-layer cleanup in `pkg/media`, `pkg/pionsend`, and `pkg/testkit`
- docs-only naming polish
- example ergonomics

## Stretch Features After Core Parity

These are still desirable and should remain visible, but they come after the
core parity gaps above:

- insertable streams / encoded transforms
- deeper congestion-control internals beyond simple BWE exposure
- FEC control
- RTP header extension access/control
- pacer / send-queue visibility

## Working Rule

When we add or expose a thin surface, we should classify it immediately:

- fully supported now
- shim/ffi gap with a concrete file-level owner
- intentionally out of scope

No more silent `ErrNotSupported` surfaces without an entry here.
