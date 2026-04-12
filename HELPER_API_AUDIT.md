# Helper/API Audit

## Goal

This repo already has a strong thin-core story in places, but several public
packages also embed browser emulation, presets, policy engines, or convenience
logic. If we want the main API to feel more Pion-like and Go-like, the rule of
thumb should be:

- Core packages should be explicit, unsurprising wrappers over libwebrtc.
- Helpers should be opt-in and layered on top of the core, not baked into it.
- Zero values should not silently enable behavior.
- Test/example/browser ergonomics should live outside the primary API path.

## Progress

- `pkg/track` no longer auto-enables adaptation from zero values.
- `pkg/pc.NewPeerConnection` no longer silently normalizes zero-value config.
- `pkg/pc` no longer rewrites SDP/MSID lines to fake multi-stream sender semantics; callers now provide one explicit stream ID per native track.
- `pkg/media` remote-track helpers no longer pretend tracks belong to multiple stream IDs; the helper layer now mirrors the native single-stream contract.
- `pkg/media` no longer exposes stream-scoping or batch-add Pion convenience helpers; the package now offers only a raw `PionTrackLocal(...)` escape hatch.
- `pkg/media` no longer exposes browser-style supported-constraint or per-track capability discovery surfaces.
- `pkg/media` no longer exposes `RemoteStreamRegistry` or browser-style `ontrack` event wrappers; remote-track binding is explicit.
- `pkg/media` capture no longer exposes browser-style constraint structs or `GetUserMedia`/`GetDisplayMedia`; callers now use explicit `CaptureConfig`, `DisplayCaptureConfig`, `ListDevices`, `ListDisplays`, `OpenCapture`, and `OpenDisplay`.
- `pkg/testkit/validate` now binds remote tracks directly instead of depending on `pkg/media.RemoteStreamRegistry`.
- Validation DSL and waiter tooling now lives under `pkg/testkit/validate` instead of `pkg/media/validate`.
- `pkg/testkit/validate` now uses direct session assertion/config policy instead of browser-style validation profiles.
- `pkg/pionsend` no longer picks browser defaults for codec preferences, bitrates, PTIME, or stream IDs; publish helpers now require explicit caller intent.

## Core vs Helper Layers

The intended split is:

- Core: `pkg/codec`, `pkg/frame`, `pkg/encoder`, `pkg/decoder`,
  `pkg/packetizer`, `pkg/depacketizer`, and a thin `pkg/track`
- Helpers: browser/media compatibility packages, publishing conveniences,
  validation/testkit code, and example-only support

That split matters because the core path should read like normal Go or Pion:
construct, configure, add, send, observe. Helper layers can still exist, but
they should be obviously optional and never the only way to use the library.

## What Counts As "Custom Helper Logic"

For this audit, "custom helper logic" means any exported behavior that:

- silently rewrites or normalizes caller input
- invents browser-shaped abstractions over native/Pion concepts
- embeds policy or heuristics instead of requiring explicit caller intent
- adds a DSL or orchestration layer on top of the transport/media primitives
- is primarily useful for tests/examples but currently sits in normal package paths

## Inventory

| Priority | Area | Files | Current custom logic | Why it diverges from Pion/Go style | Target direction |
| --- | --- | --- | --- | --- | --- |
| P0 | `pkg/pc` configuration and signaling helpers | `pkg/pc/peerconnection.go`, `pkg/pc/webrtc_helpers.go` | zero-value config rejection, custom unsupported-field gating, enum/string conversion glue, MSID parsing/rewriting, stream-ID normalization/fallbacks | The wrapper is still doing policy work in addition to transport work. Pion callers expect zero-value `webrtc.Configuration` to pass through cleanly and fewer hidden rewrites. | Keep `pkg/pc` as a thin libwebrtc wrapper. Only reject fields the shim truly cannot represent, and prefer direct `webrtc`-typed inputs plus explicit stream IDs over helper policy. |
| P0 | `pkg/track` automatic adaptation defaults | `pkg/track/local.go` | `VideoTrackConfig` zero values implicitly enable auto keyframe/bitrate/framerate/resolution behavior; RTCP handling and BWE adaptation loops are built into the base track type | This makes the base track more opinionated than a normal Go constructor. Zero config currently means "turn on behavior" instead of "do nothing unless asked." | Split adaptive behavior from the base `TrackLocal` implementation or gate it behind explicit options/constructors. Make zero values unsurprising. |
| P1 | Capture/media model | `pkg/media/media.go`, `pkg/media/constraints.go` | `MediaStream`/`MediaStreamTrack` wrappers, capture device selection, and capture-backed track lifecycle/reconfigure helpers | The capture entrypoints are now explicit and much closer to Go/libwebrtc expectations, but `pkg/media` still owns a compatibility-oriented stream/track wrapper layer on top of the raw track constructors. | Keep the explicit `List...`/`Open...` capture path, and continue shrinking `pkg/media` toward capture/runtime helpers instead of browser emulation. |
| P1 | Browser-style remote track adapters | `pkg/media/remote.go` | `RemoteTrack` wrapper fanout, backend adaptation, browser-like remote track interfaces, codec/keyframe helper surfaces | This is still an adapter layer on top of raw `OnTrack`/`AddTrack`, not the core primitive itself. The registry/event wrapper layer is gone, but the runtime package still owns a browser-shaped projection of remote receive paths. | Keep shrinking direct runtime exposure. Prefer direct backend surfaces first, and move any remaining browser-like remote helpers behind explicit compatibility boundaries if they stay at all. |
| P1 | Browser codec preset logic | `pkg/pioncodec/presets.go` | Browser profiles, preset modes, canonical ordering, codec-family heuristics, negotiated/support-filtered presets | Presets are policy. They are valuable, but they are not a thin wrapper over libwebrtc or Pion. | Keep presets opt-in and separate from exact codec factory paths. The main encoder/decoder constructors should work from exact codec params without preset dependency. |
| P1 | Publish helpers | `pkg/pionsend/publish.go`, `pkg/pionsend/audio_publish.go` | `PublishVideo`, `PublishAudio`, layered/simulcast derivation, header-extension helper logic | This still combines track construction, encoding layout policy, and sender wiring into one helper path. It is much more explicit now, but it remains higher-level than the thin core API. | Preserve as optional publishing helpers, keep caller intent explicit, and consider whether encoding/layout derivation should eventually move to narrower helper constructors. |
| P2 | Validation DSL and impairment tooling | `pkg/testkit/validate/doc.go`, `pkg/testkit/validate/session.go`, `pkg/testkit/validate/scenario.go`, `pkg/testkit/validate/relay.go` | Assertion-gated validation session, waiters, scenario scripting, impairment relay | Powerful testkit, but very far from core media/transport API. It reads more like a testing framework than a foundational library package. | Keep it under an explicit testkit boundary rather than a runtime media package, and keep scenario/assertion policy direct and local to the test session config. |
| P3 | Example-only summarization helpers | `internal/examplesupport/stats.go` | Collapses full WebRTC stats into a demo-friendly summary | Fine for examples, but it should stay obviously non-core. | Keep internal. Do not promote outward. |
| P3 | Test-only helpers | `internal/testutil/testutil.go`, `internal/testutil/concurrency.go` | Shim bootstrap helper, serial test lock | Fine as internal test plumbing. | Keep internal. No public API work needed. |

## Concrete Custom Behaviors To Remove Or Demote

### 1. Stop silent defaulting in core constructors

Current hotspots:

- `pkg/pc.normalizeConfiguration(...)`
- `pkg/track.NewVideoTrack(...)` auto-enabling adaptation features

Desired end state:

- Constructors should accept explicit config and reject unsupported values clearly.
- Convenience defaults should live in explicit helper constructors, not in the base constructor.
- "Browser-like defaults" should never be the only visible path.

### 2. Stop rewriting transport/media semantics in helper code

Current hotspots:

- `pkg/pc.expandLocalTrackStreamIDs(...)`
- `pkg/pc.streamIDsForTrackID(...)`
- `pkg/pc.normalizeTrackStreamIDs(...)`
- `pkg/media.BindPionTrack(...)`
- `pkg/media.BindPCTrack(...)`
- `pkg/media.BindDecodedTrack(...)`

Desired end state:

- Prefer explicit stream identifiers and native track metadata.
- Avoid SDP surgery unless the shim absolutely requires it.
- If SDP rewriting must remain, hide it in a tightly-scoped internal compatibility layer.

### 3. Pull browser emulation away from the main API path

Current hotspots:

- `pkg/media` browser constraints and stream model
- `pkg/pioncodec` browser presets
- `pkg/pionsend` browser-shaped publishing

Desired end state:

- The core path should look like normal Go/Pion: construct, configure, add, send, observe.
- Browser-emulation features should be an explicit compatibility layer, not the default mental model.

### 4. Keep testkit logic out of runtime-facing packages

Current hotspots:

- `pkg/testkit/validate` session/waiter/scenario APIs

Desired end state:

- Validation utilities are clearly "for testing and observability."
- Runtime users should not have to mentally sort through scenario DSLs when discovering the core API.

## Suggested Refactor Order

- [ ] Freeze the thin-core target for `pkg/pc`, `pkg/track`, and exact codec factory APIs.
- [ ] Decide which browser-emulation features remain first-class and which move behind helper/testkit package boundaries.
- [ ] Remove implicit auto-behavior from `pkg/track.VideoTrackConfig`; replace with explicit opts or separate adaptive track helpers.
- [x] Remove `pkg/pc.DefaultConfiguration` and let zero-value `webrtc.Configuration` pass through the primary constructor path.
- [ ] Isolate SDP/MSID rewriting to the narrowest possible internal compatibility layer.
- [ ] Make `pkg/media` clearly optional browser-compatibility API, or split it into a more explicit capture core plus browser adapter layer.
- [ ] Keep `pkg/pioncodec` presets opt-in; ensure the exact-params path stays primary in docs and examples.
- [x] Remove silent browser/default inference from `pkg/pionsend.PublishVideo` and `pkg/pionsend.PublishAudio`.
- [x] Reposition validation/session tooling under `pkg/testkit/validate` instead of `pkg/media/validate`.

## Likely First Fixes

These feel like the highest-leverage cleanup steps:

1. Make `pkg/track.NewVideoTrack` explicit about adaptation.
2. Remove hidden configuration normalization from `pkg/pc.NewPeerConnection`.
3. Audit all exported helpers in `pkg/media` and `pkg/pionsend` for "silent defaults vs explicit constructor."
4. Continue trimming runtime-facing helper surfaces now that validation already lives under the clearer `pkg/testkit/validate` boundary.

## Open Questions

- Do we want browser emulation to stay a flagship feature, or should it become a clearly optional layer?
- Should `pkg/pc` aim to mirror Pion naming and semantics as closely as possible, even when the shim backend has gaps?
- Is SDP/MSID rewriting a temporary compatibility hack or a long-term API commitment?
- Should layered publishing defaults live in `pkg/pionsend`, or should callers build encodings explicitly and treat the helpers as example-level sugar?

## Working Rule For Future Changes

Until this is cleaned up, new features should prefer:

- exact codec parameters over preset inference
- explicit options over silent default mutation
- thin wrappers over scenario/helper DSLs
- helper packages layered on top of core packages, not inside them
