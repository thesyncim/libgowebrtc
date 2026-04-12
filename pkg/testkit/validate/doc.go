// Package validate provides explicit validation helpers for Go
// publisher/subscriber topologies, including:
//   - session snapshots and waiters for connection, media, and data-channel state
//   - subscriber-visible audio/video continuity monitoring
//   - validation-profile policy gating for Chrome-, Firefox-, and Safari-like tests
//   - scenario scripting for layer switches, renegotiation hooks, and impairments
//   - an ICE-edge UDP relay for black-box SFU network impairment testing
//
// The package is designed for validation and observability rather than media
// transport itself. It lives under pkg/testkit to keep that boundary explicit,
// and layers on top of pkg/media, pkg/pionrecv, pkg/pionsend, and pkg/pc.
package validate
