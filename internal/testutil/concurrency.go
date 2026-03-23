package testutil

import "testing"

var serialTestLock = make(chan struct{}, 1)

// WithSerialExecution ensures tests that touch global callback state or shared
// libwebrtc resources do not run concurrently.
func WithSerialExecution(tb testing.TB) func() {
	tb.Helper()
	serialTestLock <- struct{}{}
	return func() {
		<-serialTestLock
	}
}
