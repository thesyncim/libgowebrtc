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
| P0 | `pkg/pc` configuration and signaling helpers | `pkg/pc/peerconnection.go`, `pkg/pc/webrtc_helpers.go` | `DefaultConfiguration`, `normalizeConfiguration`, `validateConfiguration`, custom unsupported-field gating, enum/string conversion glue, MSID parsing/rewriting, stream-ID normalization/fallbacks | The wrapper is doing policy work in addition to transport work. Pion callers expect explicit `webrtc.Configuration` handling and fewer hidden rewrites. | Keep `pkg/pc` as a thin libwebrtc wrapper. Move normalization/policy helpers behind explicit opts or internal utilities. Prefer direct `webrtc`-typed inputs and explicit stream IDs over SDP rewriting. |
| P0 | `pkg/track` automatic adaptation defaults | `pkg/track/local.go` | `VideoTrackConfig` zero values implicitly enable auto keyframe/bitrate/framerate/resolution behavior; RTCP handling and BWE adaptation loops are built into the base track type | This makes the base track more opinionated than a normal Go constructor. Zero config currently means "turn on behavior" instead of "do nothing unless asked." | Split adaptive behavior from the base `TrackLocal` implementation or gate it behind explicit options/constructors. Make zero values unsurprising. |
| P1 | Browser capture/media model | `pkg/media/media.go`, `pkg/media/constraints.go`, `pkg/media/capabilities.go` | Browser-style constraints, `GetUserMedia`, `GetDisplayMedia`, `MediaStream`, `MediaStreamTrack`, browser constraint matching and error semantics | Useful ergonomics, but it is intentionally not a Pion-like or idiomatic Go capture API. It also brings a lot of helper logic into a top-level package. | Decide whether browser emulation is a first-class compatibility layer or a sidecar package. If sidecar, move it behind an opt-in package boundary and expose a more explicit device/source API in the core path. |
| P1 | Browser-style remote track registry | `pkg/media/remote.go`, `pkg/media/pion.go` | `RemoteStreamRegistry`, event adaptation, browser-like stream grouping, `AddTracksToPionPeerConnection`, `PionTrackLocalForStream` | This is a convenience layer on top of raw `OnTrack`/`AddTrack`, not the core primitive itself. It is helpful, but it hides where MSID and stream grouping are coming from. | Keep it as an adapter layer, but not as the canonical way to use remote/local tracks. Prefer direct Pion/native surfaces first, registry helpers second. |
| P1 | Browser codec preset logic | `pkg/pioncodec/presets.go` | Browser profiles, preset modes, canonical ordering, codec-family heuristics, negotiated/support-filtered presets | Presets are policy. They are valuable, but they are not a thin wrapper over libwebrtc or Pion. | Keep presets opt-in and separate from exact codec factory paths. The main encoder/decoder constructors should work from exact codec params without preset dependency. |
| P1 | Browser-shaped publish helpers | `pkg/pionsend/publish.go`, `pkg/pionsend/audio_publish.go` | `PublishVideo`, `PublishAudio`, default browser codec choice, layered/simulcast derivation, header-extension helper logic | This combines track construction, codec policy, and sender wiring into one helper path. Good ergonomics, but not minimal API surface. | Preserve as optional publishing helpers, but keep the lower-level "construct tracks, then add/send" path front and center. |
| P2 | Validation DSL and impairment tooling | `pkg/media/validate/doc.go`, `pkg/media/validate/session.go`, `pkg/media/validate/scenario.go`, `pkg/media/validate/relay.go` | Browser-style validation session, waiters, policy gating, scenario scripting, impairment relay | Powerful testkit, but very far from core media/transport API. It reads more like a testing framework than a foundational library package. | Treat it as `testkit`/validation tooling rather than as part of the core runtime story. Keep it available, but quarantine it conceptually and eventually by package path if possible. |
| P3 | Example-only summarization helpers | `internal/examplesupport/stats.go` | Collapses full WebRTC stats into a demo-friendly summary | Fine for examples, but it should stay obviously non-core. | Keep internal. Do not promote outward. |
| P3 | Test-only helpers | `internal/testutil/testutil.go`, `internal/testutil/concurrency.go` | Shim bootstrap helper, serial test lock | Fine as internal test plumbing. | Keep internal. No public API work needed. |

## Concrete Custom Behaviors To Remove Or Demote

### 1. Stop silent defaulting in core constructors

Current hotspots:

- `pkg/pc.DefaultConfiguration()`
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
- `pkg/media.RemoteStreamRegistry`

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

- `pkg/media/validate` session/waiter/scenario APIs

Desired end state:

- Validation utilities are clearly "for testing and observability."
- Runtime users should not have to mentally sort through scenario DSLs when discovering the core API.

## Suggested Refactor Order

- [ ] Freeze the thin-core target for `pkg/pc`, `pkg/track`, and exact codec factory APIs.
- [ ] Decide which browser-emulation features remain first-class and which move behind helper/testkit package boundaries.
- [ ] Remove implicit auto-behavior from `pkg/track.VideoTrackConfig`; replace with explicit opts or separate adaptive track helpers.
- [ ] Remove or demote `pkg/pc.DefaultConfiguration` and `normalizeConfiguration` from the primary constructor path.
- [ ] Isolate SDP/MSID rewriting to the narrowest possible internal compatibility layer.
- [ ] Make `pkg/media` clearly optional browser-compatibility API, or split it into a more explicit capture core plus browser adapter layer.
- [ ] Keep `pkg/pioncodec` presets opt-in; ensure the exact-params path stays primary in docs and examples.
- [ ] Reposition `pkg/media/validate` as validation/testkit functionality, not a general runtime helper package.

## Likely First Fixes

These feel like the highest-leverage cleanup steps:

1. Make `pkg/track.NewVideoTrack` explicit about adaptation.
2. Remove hidden configuration normalization from `pkg/pc.NewPeerConnection`.
3. Audit all exported helpers in `pkg/media` and `pkg/pionsend` for "silent defaults vs explicit constructor."
4. Decide whether `pkg/media/validate` should stay public under its current path or move to a clearer testkit boundary later.

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
