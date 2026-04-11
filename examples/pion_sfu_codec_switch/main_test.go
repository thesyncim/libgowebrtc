package main

import (
	"context"
	"io"
	"log"
	"sync/atomic"
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

func TestRecommendedSettleDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		total    time.Duration
		interval time.Duration
		want     time.Duration
	}{
		{name: "uses minimum settle window", total: 12 * time.Second, interval: 500 * time.Millisecond, want: 2 * time.Second},
		{name: "uses interval when longer", total: 9 * time.Second, interval: 3 * time.Second, want: 3 * time.Second},
		{name: "caps settle window when total is short", total: 1500 * time.Millisecond, interval: 2 * time.Second, want: 500 * time.Millisecond},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := recommendedSettleDuration(tt.total, tt.interval); got != tt.want {
				t.Fatalf("recommendedSettleDuration(%v, %v) = %v, want %v", tt.total, tt.interval, got, tt.want)
			}
		})
	}
}

func TestWaitForSwitchConvergenceWaitsForSwitchLoop(t *testing.T) {
	t.Parallel()

	publisher := &codecSwitchingPublisher{}
	publisher.switchCount.Store(2)

	switchLoopDone := make(chan struct{})
	var sfuSwitches atomic.Int64
	var subscriberSwitches atomic.Int64

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(switchLoopDone)
		time.Sleep(50 * time.Millisecond)
		subscriberSwitches.Store(2)
	}()

	if err := waitForSwitchConvergence(ctx, publisher, switchLoopDone, &sfuSwitches, &subscriberSwitches); err != nil {
		t.Fatalf("waitForSwitchConvergence: %v", err)
	}
}
