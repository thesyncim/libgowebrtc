package ffi

import (
	"errors"
	"strings"
	"testing"
)

func TestLinuxLoadFailureHintForGLIBC(t *testing.T) {
	err := errors.New("dlopen: /lib/x86_64-linux-gnu/libc.so.6: version `GLIBC_2.36' not found")

	hint := linuxLoadFailureHintFor("linux", err)
	if hint == "" {
		t.Fatal("expected glibc compatibility hint")
	}
	if !strings.Contains(hint, "newer glibc") {
		t.Fatalf("hint = %q, want newer glibc guidance", hint)
	}
}

func TestLinuxLoadFailureHintForNonLinux(t *testing.T) {
	err := errors.New("dlopen: /lib/x86_64-linux-gnu/libc.so.6: version `GLIBC_2.36' not found")

	if hint := linuxLoadFailureHintFor("darwin", err); hint != "" {
		t.Fatalf("hint = %q, want empty", hint)
	}
}
