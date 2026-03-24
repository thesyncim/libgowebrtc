package ffi

import (
	"strings"
	"testing"
)

func TestCheckRuntimeDependenciesReportsVersionMismatch(t *testing.T) {
	release := withFFITestSerialExecution(t)
	defer release()

	prevShimVersion := shimVersion
	prevLibWebRTCVersion := shimLibwebrtcVersion
	prevLoaded := libLoaded.Load()
	t.Cleanup(func() {
		shimVersion = prevShimVersion
		shimLibwebrtcVersion = prevLibWebRTCVersion
		libLoaded.Store(prevLoaded)
	})

	badShimVersion := CString("0.0.0-test")
	badLibWebRTCVersion := CString("M000")

	libLoaded.Store(true)
	shimVersion = func() uintptr { return ByteSlicePtr(badShimVersion) }
	shimLibwebrtcVersion = func() uintptr { return ByteSlicePtr(badLibWebRTCVersion) }

	report, err := CheckRuntimeDependencies()
	if err != nil {
		t.Fatalf("CheckRuntimeDependencies(): %v", err)
	}

	if report.Shim.VersionStatus != DiagnosticStatusMismatch {
		t.Fatalf("shim version status = %q, want %q", report.Shim.VersionStatus, DiagnosticStatusMismatch)
	}
	if !containsIssue(report.Shim.BlockingIssues, "shim version mismatch") {
		t.Fatalf("expected shim version mismatch issue, got %v", report.Shim.BlockingIssues)
	}
	if !containsIssue(report.Shim.BlockingIssues, "libwebrtc version mismatch") {
		t.Fatalf("expected libwebrtc version mismatch issue, got %v", report.Shim.BlockingIssues)
	}
}

func containsIssue(issues []string, substring string) bool {
	for _, issue := range issues {
		if strings.Contains(issue, substring) {
			return true
		}
	}
	return false
}
