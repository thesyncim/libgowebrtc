// Package diagnostics provides preflight checks for libgowebrtc runtime dependencies.
package diagnostics

import "github.com/thesyncim/libgowebrtc/internal/ffi"

// Status describes the outcome of a runtime diagnostic check.
type Status = ffi.DiagnosticStatus

// Status values summarize how healthy a runtime dependency check is.
const (
	StatusUnknown    Status = ffi.DiagnosticStatusUnknown
	StatusOK         Status = ffi.DiagnosticStatusOK
	StatusMissing    Status = ffi.DiagnosticStatusMissing
	StatusMismatch   Status = ffi.DiagnosticStatusMismatch
	StatusUnverified Status = ffi.DiagnosticStatusUnverified
)

// DependencyReport summarizes the runtime state for one dependency.
type DependencyReport = ffi.DependencyReport

// Report summarizes libgowebrtc runtime dependency state.
type Report = ffi.RuntimeDiagnosticsReport

// Check inspects the current shim/OpenH264 configuration, cache, and loaded-library
// state without downloading new artifacts or mutating loader state.
func Check() (*Report, error) {
	return ffi.CheckRuntimeDependencies()
}
