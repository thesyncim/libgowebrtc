package validate

import (
	"net"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestICEEdgeRelayLifecycleAndPause(t *testing.T) {
	target, err := net.ListenUDP("udp", mustResolveUDPAddr("127.0.0.1:0"))
	if err != nil {
		t.Fatalf("ListenUDP(target): %v", err)
	}
	defer target.Close()

	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := target.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = target.WriteToUDP(buf[:n], addr)
		}
	}()

	relay, err := NewICEEdgeRelay(RelayConfig{
		ListenAddr: "127.0.0.1:0",
		TargetAddr: target.LocalAddr().String(),
		RandomSeed: 1,
	})
	if err != nil {
		t.Fatalf("NewICEEdgeRelay: %v", err)
	}
	defer relay.Close()

	client, err := net.ListenUDP("udp", nil)
	if err != nil {
		t.Fatalf("ListenUDP(client): %v", err)
	}
	defer client.Close()

	expectEcho := func(message string, timeout time.Duration) bool {
		if _, err := client.WriteToUDP([]byte(message), mustResolveUDPAddr(relay.LocalAddr())); err != nil {
			t.Fatalf("WriteToUDP(%q): %v", message, err)
		}
		_ = client.SetReadDeadline(time.Now().Add(timeout))
		buf := make([]byte, 2048)
		n, _, err := client.ReadFromUDP(buf)
		if err != nil {
			return false
		}
		return string(buf[:n]) == message
	}

	if !expectEcho("hello", 200*time.Millisecond) {
		t.Fatal("baseline echo through relay failed")
	}

	relay.SetImpairment(ImpairmentProfile{
		Rules: []NetworkImpairment{{Direction: DirectionBoth, Pause: true}},
	})
	if expectEcho("paused", 50*time.Millisecond) {
		t.Fatal("echo succeeded while relay was paused")
	}

	relay.ClearImpairment()
	if !expectEcho("resumed", 200*time.Millisecond) {
		t.Fatal("echo after ClearImpairment failed")
	}
}

func TestNewICEEdgeRelayRequiresExplicitAddresses(t *testing.T) {
	if _, err := NewICEEdgeRelay(RelayConfig{TargetAddr: "127.0.0.1:9"}); err == nil || err.Error() != "validate: relay listen address is required" {
		t.Fatalf("missing listen addr error = %v, want explicit listen address requirement", err)
	}
	if _, err := NewICEEdgeRelay(RelayConfig{ListenAddr: "127.0.0.1:0"}); err == nil || err.Error() != "validate: relay target address is required" {
		t.Fatalf("missing target addr error = %v, want explicit target address requirement", err)
	}
}

func TestICEEdgeRelayDelayAndDirectionalRules(t *testing.T) {
	relay := &ICEEdgeRelay{rng: randSourceForTest(1)}
	relay.profile = ImpairmentProfile{
		Rules: []NetworkImpairment{
			{Direction: DirectionUpstream, Pause: true},
			{Direction: DirectionDownstream, Delay: 10 * time.Millisecond},
		},
	}

	up := relay.impairmentForDirection(DirectionUpstream)
	if !up.Pause {
		t.Fatalf("upstream impairment = %+v, want Pause=true", up)
	}
	down := relay.impairmentForDirection(DirectionDownstream)
	if down.Delay != 10*time.Millisecond || down.Pause {
		t.Fatalf("downstream impairment = %+v, want delay only", down)
	}
}

func TestICEEdgeRelayApplyImpairmentLossDuplicateAndReorder(t *testing.T) {
	relay := &ICEEdgeRelay{rng: randSourceForTest(1)}

	var mu sync.Mutex
	var sent [][]byte
	send := func(packet []byte) {
		mu.Lock()
		sent = append(sent, append([]byte(nil), packet...))
		mu.Unlock()
	}

	relay.profile = ImpairmentProfile{
		Rules: []NetworkImpairment{{Direction: DirectionUpstream, Loss: 1}},
	}
	relay.applyImpairment(DirectionUpstream, []byte("drop"), send)
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	if len(sent) != 0 {
		mu.Unlock()
		t.Fatalf("loss profile sent %d packets, want 0", len(sent))
	}
	mu.Unlock()

	relay.profile = ImpairmentProfile{
		Rules: []NetworkImpairment{{Direction: DirectionUpstream, DuplicateProbability: 1}},
	}
	relay.applyImpairment(DirectionUpstream, []byte("dup"), send)
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	if len(sent) != 2 {
		mu.Unlock()
		t.Fatalf("duplicate profile sent %d packets, want 2", len(sent))
	}
	mu.Unlock()

	mu.Lock()
	sent = nil
	mu.Unlock()
	relay.profile = ImpairmentProfile{
		Rules: []NetworkImpairment{{Direction: DirectionUpstream, ReorderProbability: 1}},
	}
	relay.applyImpairment(DirectionUpstream, []byte("a"), send)
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	if len(sent) != 0 {
		mu.Unlock()
		t.Fatalf("first reordered packet sent immediately, got %d packets", len(sent))
	}
	mu.Unlock()
	relay.applyImpairment(DirectionUpstream, []byte("b"), send)
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	if len(sent) != 2 {
		mu.Unlock()
		t.Fatalf("reorder profile sent %d packets after second input, want 2", len(sent))
	}
	payloads := []string{string(sent[0]), string(sent[1])}
	mu.Unlock()
	slices.Sort(payloads)
	if !slices.Equal(payloads, []string{"a", "b"}) {
		t.Fatalf("reordered payloads = %v, want [a b]", payloads)
	}
}

func randSourceForTest(seed int64) relayRandomSource {
	return newRelayRandomSource(seed)
}

func TestRelayRandomHelpersAndFallbacks(t *testing.T) {
	relay := &ICEEdgeRelay{rng: randSourceForTest(2)}
	if got := relay.randomInt63n(0); got != 0 {
		t.Fatalf("randomInt63n(0) = %d, want 0", got)
	}
	if got := relay.randomInt63n(5); got < 0 || got >= 5 {
		t.Fatalf("randomInt63n(5) = %d, want [0,5)", got)
	}
	if _, ok := newRelayRandomSource(0).(cryptoRelayRandom); !ok {
		t.Fatal("newRelayRandomSource(0) did not return cryptoRelayRandom")
	}
	if _, ok := newRelayRandomSource(1).(*seededRelayRandom); !ok {
		t.Fatal("newRelayRandomSource(1) did not return *seededRelayRandom")
	}

	cryptoSource := cryptoRelayRandom{}
	if got := cryptoSource.Int63n(0); got != 0 {
		t.Fatalf("crypto Int63n(0) = %d, want 0", got)
	}
	if got := cryptoSource.Int63n(9); got < 0 || got >= 9 {
		t.Fatalf("crypto Int63n(9) = %d, want [0,9)", got)
	}
	if got := cryptoSource.Float64(); got < 0 || got >= 1 {
		t.Fatalf("crypto Float64() = %v, want [0,1)", got)
	}

	seeded := &seededRelayRandom{state: 1}
	if got := seeded.Int63n(7); got < 0 || got >= 7 {
		t.Fatalf("seeded Int63n(7) = %d, want [0,7)", got)
	}
	if got := seeded.Float64(); got < 0 || got >= 1 {
		t.Fatalf("seeded Float64() = %v, want [0,1)", got)
	}

	_ = cryptoUint64()

	addr := mustResolveUDPAddr("definitely-not-an-address")
	if addr == nil || !addr.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("mustResolveUDPAddr fallback = %+v, want 127.0.0.1", addr)
	}
}

func TestICEEdgeRelayScheduleSendBitrateCap(t *testing.T) {
	relay := &ICEEdgeRelay{rng: randSourceForTest(3)}
	sentAt := make(chan time.Time, 1)
	start := time.Now()
	relay.scheduleSend(NetworkImpairment{
		Delay:         5 * time.Millisecond,
		BitrateCapBps: 8000,
	}, []byte("1234567890"), func([]byte) {
		sentAt <- time.Now()
	})

	select {
	case at := <-sentAt:
		if elapsed := at.Sub(start); elapsed < 10*time.Millisecond {
			t.Fatalf("scheduleSend elapsed = %v, want bitrate/delay scheduling", elapsed)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("scheduleSend timed out")
	}
}
