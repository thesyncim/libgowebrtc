# Agent Guidance

This repository is still pre-release. Optimize for a thinner, more explicit API and for fixing real defects at the source.

## Core Rules

- Fix root causes. Do not mask bugs with sleeps, retries, skips, looser assertions, or "best effort" fallbacks when the underlying behavior can be repaired.
- Keep tests honest. If CI finds a real failure, repair the implementation or the test harness so it reflects the true contract. Do not weaken checks just to get green.
- Prefer explicit APIs. Remove invented defaults, hidden normalization, browser-style compatibility shims, and helper magic from runtime-facing packages unless they are clearly isolated as optional helpers or testkit code.
- Stay close to libwebrtc and Pion. Prefer libwebrtc-backed capability discovery, negotiation, packetization, payloading, and sender/receiver behavior over custom emulation in core paths.
- No backward-compatibility theater. Since the project is unreleased, rename, delete, or reshape APIs when that makes the surface simpler and more Go-like.

## When Fixing CI

- Reproduce the failure locally when feasible.
- Identify the failing layer precisely: test bug, shim bug, FFI bug, negotiation bug, media pipeline bug, or workflow bug.
- Land the smallest change that truly fixes that layer.
- Verify with the narrowest relevant test first, then rerun the broader suite that exercises the changed path.
