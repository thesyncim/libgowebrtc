package main

import (
	"io"
	"log"
	"testing"
	"time"

	"github.com/thesyncim/libgowebrtc/internal/testutil"
)

func TestRunExampleSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping codec-switch smoke test in short mode")
	}

	testutil.RequireShim(t)

	previousFlags := log.Flags()
	previousWriter := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() {
		log.SetFlags(previousFlags)
		log.SetOutput(previousWriter)
	})

	stats, err := runExample(exampleConfig{
		Duration:       12 * time.Second,
		SwitchInterval: 2 * time.Second,
		Width:          160,
		Height:         120,
		FPS:            8,
		Bitrate:        150_000,
	})
	if err != nil {
		t.Fatalf("runExample: %v", err)
	}

	if stats.DecodedFrames == 0 {
		t.Fatal("expected decoded frames from subscriber")
	}
	if stats.PublisherSwitches < 2 {
		t.Fatalf("publisher switches = %d, want at least 2", stats.PublisherSwitches)
	}
	if stats.SFUSwitches == 0 {
		t.Fatal("expected at least one SFU-observed switch")
	}
	if stats.SubscriberSwitches != stats.PublisherSwitches {
		t.Fatalf("subscriber switches = %d, want %d", stats.SubscriberSwitches, stats.PublisherSwitches)
	}
	if stats.PostConnectRenegotes != 0 {
		t.Fatalf("post-connect renegotiations = %d, want 0", stats.PostConnectRenegotes)
	}
}
