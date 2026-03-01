package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc/test/bufconn"
)

// FaultConfig controls deterministic dial failures and delays.
type FaultConfig struct {
	DropDial  func(source, target string, seq uint64) bool
	DialDelay func(source, target string, seq uint64) time.Duration
}

// ContextDialer mirrors net.Dialer-style dialing for swappable transports.
type ContextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type entry struct {
	addr string
	lis  *bufconn.Listener
}

// BufNet is an in-memory endpoint registry backed by gRPC bufconn listeners.
type BufNet struct {
	bufferSize int
	faults     FaultConfig

	mu        sync.RWMutex
	entries   map[string]entry
	nextDial  uint64
	isClosing bool
}

func NewBufNet(bufferSize int, faults FaultConfig) *BufNet {
	if bufferSize <= 0 {
		bufferSize = 1 << 20
	}
	return &BufNet{
		bufferSize: bufferSize,
		faults:     faults,
		entries:    make(map[string]entry),
	}
}

func (n *BufNet) Listen(address string) (net.Listener, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.isClosing {
		return nil, errors.New("bufnet is closed")
	}
	if _, ok := n.entries[address]; ok {
		return nil, fmt.Errorf("address already registered: %s", address)
	}
	lis := bufconn.Listen(n.bufferSize)
	n.entries[address] = entry{addr: address, lis: lis}
	return lis, nil
}

func (n *BufNet) Close() error {
	n.mu.Lock()
	if n.isClosing {
		n.mu.Unlock()
		return nil
	}
	n.isClosing = true
	items := make([]entry, 0, len(n.entries))
	for _, e := range n.entries {
		items = append(items, e)
	}
	n.entries = map[string]entry{}
	n.mu.Unlock()

	var firstErr error
	for _, e := range items {
		if err := e.lis.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (n *BufNet) Dialer(source string) ContextDialer {
	return &dialer{network: n, source: source}
}

type dialer struct {
	network *BufNet
	source  string
}

func (d *dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	_ = network // accepted for net.Dialer API compatibility.
	seq, lis, err := d.network.resolveDial(address)
	if err != nil {
		return nil, err
	}

	if d.network.faults.DropDial != nil && d.network.faults.DropDial(d.source, address, seq) {
		return nil, fmt.Errorf("dial dropped source=%s target=%s seq=%d", d.source, address, seq)
	}
	if d.network.faults.DialDelay != nil {
		delay := d.network.faults.DialDelay(d.source, address, seq)
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return lis.DialContext(ctx)
}

func (n *BufNet) resolveDial(address string) (uint64, *bufconn.Listener, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.isClosing {
		return 0, nil, errors.New("bufnet is closed")
	}
	e, ok := n.entries[address]
	if !ok {
		return 0, nil, fmt.Errorf("unknown address: %s", address)
	}
	n.nextDial++
	return n.nextDial, e.lis, nil
}
