package validate

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"time"
)

// NetworkDirection identifies which path an impairment should affect.
type NetworkDirection string

// Impairment directions.
const (
	DirectionUpstream   NetworkDirection = "upstream"
	DirectionDownstream NetworkDirection = "downstream"
	DirectionBoth       NetworkDirection = "both"
)

// NetworkImpairment describes network impairment for one or both directions.
type NetworkImpairment struct {
	Direction            NetworkDirection
	Loss                 float64
	Delay                time.Duration
	Jitter               time.Duration
	ReorderProbability   float64
	DuplicateProbability float64
	BitrateCapBps        uint64
	Pause                bool
}

// ImpairmentProfile groups one or more impairment rules.
type ImpairmentProfile struct {
	Rules []NetworkImpairment
}

// RelayConfig configures the ICE-edge relay.
type RelayConfig struct {
	ListenAddr string
	TargetAddr string
	RandomSeed int64
}

type relayRandomSource interface {
	Float64() float64
	Int63n(int64) int64
}

// ICEEdgeRelay is a UDP proxy suitable for ICE-edge impairment injection.
type ICEEdgeRelay struct {
	inbound  *net.UDPConn
	outbound *net.UDPConn
	target   *net.UDPAddr

	mu        sync.Mutex
	client    *net.UDPAddr
	profile   ImpairmentProfile
	rng       relayRandomSource
	closed    bool
	pendingUp []byte
	pendingDn []byte
}

// NewICEEdgeRelay creates and starts a UDP relay that forwards traffic to the
// configured target endpoint.
func NewICEEdgeRelay(cfg RelayConfig) (*ICEEdgeRelay, error) {
	if cfg.TargetAddr == "" {
		return nil, errors.New("validate: relay target address is required")
	}
	target, err := net.ResolveUDPAddr("udp", cfg.TargetAddr)
	if err != nil {
		return nil, err
	}
	listenAddr := cfg.ListenAddr
	if listenAddr == "" {
		listenAddr = "127.0.0.1:0"
	}
	inbound, err := net.ListenUDP("udp", mustResolveUDPAddr(listenAddr))
	if err != nil {
		return nil, err
	}
	outbound, err := net.ListenUDP("udp", nil)
	if err != nil {
		_ = inbound.Close()
		return nil, err
	}

	relay := &ICEEdgeRelay{
		inbound:  inbound,
		outbound: outbound,
		target:   target,
		rng:      newRelayRandomSource(cfg.RandomSeed),
	}
	go relay.clientLoop()
	go relay.targetLoop()
	return relay, nil
}

// LocalAddr returns the relay's listen address.
func (r *ICEEdgeRelay) LocalAddr() string {
	if r == nil || r.inbound == nil {
		return ""
	}
	return r.inbound.LocalAddr().String()
}

// SetImpairment replaces the active impairment profile.
func (r *ICEEdgeRelay) SetImpairment(profile ImpairmentProfile) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.profile = profile
}

// ClearImpairment removes all impairments.
func (r *ICEEdgeRelay) ClearImpairment() {
	r.SetImpairment(ImpairmentProfile{})
}

// Close stops the relay.
func (r *ICEEdgeRelay) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	errIn := r.inbound.Close()
	errOut := r.outbound.Close()
	if errIn != nil {
		return errIn
	}
	return errOut
}

func (r *ICEEdgeRelay) clientLoop() {
	buffer := make([]byte, 2048)
	for {
		n, addr, err := r.inbound.ReadFromUDP(buffer)
		if err != nil {
			return
		}
		payload := append([]byte(nil), buffer[:n]...)

		r.mu.Lock()
		r.client = addr
		r.mu.Unlock()

		r.applyImpairment(DirectionUpstream, payload, func(packet []byte) {
			_, _ = r.outbound.WriteToUDP(packet, r.target)
		})
	}
}

func (r *ICEEdgeRelay) targetLoop() {
	buffer := make([]byte, 2048)
	for {
		n, _, err := r.outbound.ReadFromUDP(buffer)
		if err != nil {
			return
		}
		payload := append([]byte(nil), buffer[:n]...)

		r.mu.Lock()
		client := r.client
		r.mu.Unlock()
		if client == nil {
			continue
		}

		r.applyImpairment(DirectionDownstream, payload, func(packet []byte) {
			_, _ = r.inbound.WriteToUDP(packet, client)
		})
	}
}

func (r *ICEEdgeRelay) applyImpairment(direction NetworkDirection, payload []byte, send func([]byte)) {
	impairment := r.impairmentForDirection(direction)
	if impairment.Pause || len(payload) == 0 {
		return
	}
	if impairment.Loss > 0 && r.randomFloat() < impairment.Loss {
		return
	}

	if pending := r.consumePending(direction); pending != nil {
		r.scheduleSend(impairment, payload, send)
		r.scheduleSend(impairment, pending, send)
		return
	}
	if impairment.ReorderProbability > 0 && r.randomFloat() < impairment.ReorderProbability {
		r.storePending(direction, payload)
		return
	}

	r.scheduleSend(impairment, payload, send)
	if impairment.DuplicateProbability > 0 && r.randomFloat() < impairment.DuplicateProbability {
		r.scheduleSend(impairment, append([]byte(nil), payload...), send)
	}
}

func (r *ICEEdgeRelay) impairmentForDirection(direction NetworkDirection) NetworkImpairment {
	r.mu.Lock()
	defer r.mu.Unlock()

	var merged NetworkImpairment
	for _, rule := range r.profile.Rules {
		if rule.Direction != direction && rule.Direction != DirectionBoth {
			continue
		}
		merged.Direction = direction
		if rule.Loss > merged.Loss {
			merged.Loss = rule.Loss
		}
		if rule.Delay > merged.Delay {
			merged.Delay = rule.Delay
		}
		if rule.Jitter > merged.Jitter {
			merged.Jitter = rule.Jitter
		}
		if rule.ReorderProbability > merged.ReorderProbability {
			merged.ReorderProbability = rule.ReorderProbability
		}
		if rule.DuplicateProbability > merged.DuplicateProbability {
			merged.DuplicateProbability = rule.DuplicateProbability
		}
		if rule.BitrateCapBps > 0 {
			merged.BitrateCapBps = rule.BitrateCapBps
		}
		merged.Pause = merged.Pause || rule.Pause
	}
	return merged
}

func (r *ICEEdgeRelay) scheduleSend(impairment NetworkImpairment, payload []byte, send func([]byte)) {
	packet := append([]byte(nil), payload...)
	delay := impairment.Delay
	if impairment.Jitter > 0 {
		span := int64(impairment.Jitter) * 2
		offset := time.Duration(r.randomInt63n(span+1)) - impairment.Jitter
		delay += offset
		if delay < 0 {
			delay = 0
		}
	}
	if impairment.BitrateCapBps > 0 {
		delay += time.Duration((uint64(len(packet))*8)*uint64(time.Second)) / time.Duration(impairment.BitrateCapBps)
	}

	go func() {
		if delay > 0 {
			time.Sleep(delay)
		}
		send(packet)
	}()
}

func (r *ICEEdgeRelay) consumePending(direction NetworkDirection) []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	var pending []byte
	switch direction {
	case DirectionUpstream:
		pending = r.pendingUp
		r.pendingUp = nil
	case DirectionDownstream:
		pending = r.pendingDn
		r.pendingDn = nil
	}
	if pending == nil {
		return nil
	}
	return append([]byte(nil), pending...)
}

func (r *ICEEdgeRelay) storePending(direction NetworkDirection, payload []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch direction {
	case DirectionUpstream:
		r.pendingUp = append([]byte(nil), payload...)
	case DirectionDownstream:
		r.pendingDn = append([]byte(nil), payload...)
	}
}

func (r *ICEEdgeRelay) randomFloat() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rng == nil {
		r.rng = newRelayRandomSource(0)
	}
	return r.rng.Float64()
}

func (r *ICEEdgeRelay) randomInt63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rng == nil {
		r.rng = newRelayRandomSource(0)
	}
	return r.rng.Int63n(n)
}

func newRelayRandomSource(seed int64) relayRandomSource {
	if seed != 0 {
		return &seededRelayRandom{state: uint64(seed)}
	}
	return cryptoRelayRandom{}
}

type cryptoRelayRandom struct{}

func (cryptoRelayRandom) Float64() float64 {
	const denominator = float64(uint64(1) << 53)
	return float64(cryptoUint64()>>11) / denominator
}

func (cryptoRelayRandom) Int63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	limit := uint64(n)
	maxValue := ^uint64(0) - (^uint64(0) % limit)
	for {
		value := cryptoUint64()
		if value < maxValue {
			return int64(value % limit)
		}
	}
}

type seededRelayRandom struct {
	state uint64
}

func (r *seededRelayRandom) next() uint64 {
	if r.state == 0 {
		r.state = 0x9e3779b97f4a7c15
	}
	r.state ^= r.state >> 12
	r.state ^= r.state << 25
	r.state ^= r.state >> 27
	return r.state * 2685821657736338717
}

func (r *seededRelayRandom) Float64() float64 {
	const denominator = float64(uint64(1) << 53)
	return float64(r.next()>>11) / denominator
}

func (r *seededRelayRandom) Int63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	limit := uint64(n)
	maxValue := ^uint64(0) - (^uint64(0) % limit)
	for {
		value := r.next()
		if value < maxValue {
			return int64(value % limit)
		}
	}
}

func cryptoUint64() uint64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return uint64(time.Now().UnixNano())
	}
	return binary.LittleEndian.Uint64(buf[:])
}

func mustResolveUDPAddr(addr string) *net.UDPAddr {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}
	}
	return udpAddr
}
