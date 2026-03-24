package ffi

import (
	"testing"
)

func TestPeerConnectionCreateDataChannelSelectsAvailableShimPath(t *testing.T) {
	release := withFFITestSerialExecution(t)
	defer release()

	prevCreate := shimPeerConnectionCreateDataChannel
	prevCreateEx := shimPeerConnectionCreateDataChannelEx
	prevLoaded := libLoaded.Load()
	t.Cleanup(func() {
		shimPeerConnectionCreateDataChannel = prevCreate
		shimPeerConnectionCreateDataChannelEx = prevCreateEx
		libLoaded.Store(prevLoaded)
	})

	libLoaded.Store(true)

	t.Run("extended symbol", func(t *testing.T) {
		shimPeerConnectionCreateDataChannel = func(params uintptr) uintptr {
			t.Fatal("legacy create path should not be used when extended symbol is available")
			return 0
		}

		var got shimPeerConnectionCreateDataChannelExParams
		var gotLabel string
		var gotProtocol string
		shimPeerConnectionCreateDataChannelEx = func(params uintptr) uintptr {
			var ok bool
			got, ok = CopyStructFromC[shimPeerConnectionCreateDataChannelExParams](params)
			if !ok {
				t.Fatal("expected params to be readable")
			}
			gotLabel = GoStringFromC(got.Label)
			gotProtocol = GoStringFromC(got.Protocol)
			return 123
		}

		handle := PeerConnectionCreateDataChannel(9, "dc-ex", false, 250, 7, "proto", true, 42)
		if handle != 123 {
			t.Fatalf("PeerConnectionCreateDataChannel() = %d, want 123", handle)
		}
		if got.PC != 9 {
			t.Fatalf("PC = %d, want 9", got.PC)
		}
		if got.Ordered != 0 {
			t.Fatalf("Ordered = %d, want 0", got.Ordered)
		}
		if got.MaxPacketLifeTime != 250 {
			t.Fatalf("MaxPacketLifeTime = %d, want 250", got.MaxPacketLifeTime)
		}
		if got.MaxRetransmits != 7 {
			t.Fatalf("MaxRetransmits = %d, want 7", got.MaxRetransmits)
		}
		if got.Negotiated != 1 {
			t.Fatalf("Negotiated = %d, want 1", got.Negotiated)
		}
		if got.ID != 42 {
			t.Fatalf("ID = %d, want 42", got.ID)
		}
		if gotLabel != "dc-ex" {
			t.Fatalf("Label = %q, want %q", gotLabel, "dc-ex")
		}
		if gotProtocol != "proto" {
			t.Fatalf("Protocol = %q, want %q", gotProtocol, "proto")
		}
		if got.ErrorOut == 0 {
			t.Fatal("ErrorOut should be set")
		}
	})

	t.Run("legacy fallback", func(t *testing.T) {
		var got shimPeerConnectionCreateDataChannelParams
		var gotLabel string
		var gotProtocol string
		shimPeerConnectionCreateDataChannel = func(params uintptr) uintptr {
			var ok bool
			got, ok = CopyStructFromC[shimPeerConnectionCreateDataChannelParams](params)
			if !ok {
				t.Fatal("expected params to be readable")
			}
			gotLabel = GoStringFromC(got.Label)
			gotProtocol = GoStringFromC(got.Protocol)
			return 456
		}
		shimPeerConnectionCreateDataChannelEx = nil

		handle := PeerConnectionCreateDataChannel(11, "dc-legacy", false, 250, 5, "chat", true, 17)
		if handle != 456 {
			t.Fatalf("PeerConnectionCreateDataChannel() = %d, want 456", handle)
		}
		if got.PC != 11 {
			t.Fatalf("PC = %d, want 11", got.PC)
		}
		if got.Ordered != 0 {
			t.Fatalf("Ordered = %d, want 0", got.Ordered)
		}
		if got.MaxRetransmits != 5 {
			t.Fatalf("MaxRetransmits = %d, want 5", got.MaxRetransmits)
		}
		if gotLabel != "dc-legacy" {
			t.Fatalf("Label = %q, want %q", gotLabel, "dc-legacy")
		}
		if gotProtocol != "chat" {
			t.Fatalf("Protocol = %q, want %q", gotProtocol, "chat")
		}
		if got.ErrorOut == 0 {
			t.Fatal("ErrorOut should be set")
		}
	})
}
