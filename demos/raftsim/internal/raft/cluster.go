package raft

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net"
	"slices"
	"sync"
	"time"

	"demos/raftsim/internal/transport"
)

type ClusterConfig struct {
	NodeIDs []string
	Seed    int64
	Bugs    BugConfig

	ElectionBase   time.Duration
	ElectionJitter time.Duration
	Heartbeat      time.Duration

	DialFaults    transport.FaultConfig
	MessageFaults MessageFaultHook
}

type Cluster struct {
	network   *transport.BufNet
	nodes     map[string]*Node
	listeners map[string]net.Listener
	addresses map[string]string

	mu     sync.Mutex
	events []string
}

func NewCluster(cfg ClusterConfig) (*Cluster, error) {
	if len(cfg.NodeIDs) < 3 {
		return nil, fmt.Errorf("raft demo requires at least 3 nodes, got %d", len(cfg.NodeIDs))
	}
	if cfg.Seed == 0 {
		cfg.Seed = 1
	}
	ids := append([]string(nil), cfg.NodeIDs...)
	slices.Sort(ids)

	c := &Cluster{
		network:   transport.NewBufNet(1<<20, cfg.DialFaults),
		nodes:     make(map[string]*Node, len(ids)),
		listeners: make(map[string]net.Listener, len(ids)),
		addresses: make(map[string]string, len(ids)),
	}
	for _, id := range ids {
		addr := "node://" + id
		lis, err := c.network.Listen(addr)
		if err != nil {
			return nil, err
		}
		c.listeners[id] = lis
		c.addresses[id] = addr
	}

	for i, id := range ids {
		peers := make(map[string]string)
		for _, peer := range ids {
			if peer == id {
				continue
			}
			peers[peer] = c.addresses[peer]
		}
		r := rand.New(rand.NewSource(cfg.Seed + int64(i+1)))
		node, err := NewNode(NodeConfig{
			ID:             id,
			Address:        c.addresses[id],
			Peers:          peers,
			Listener:       c.listeners[id],
			Dialer:         c.network.Dialer(id),
			ElectionBase:   cfg.ElectionBase,
			ElectionJitter: cfg.ElectionJitter,
			Heartbeat:      cfg.Heartbeat,
			Rand:           r,
			Bugs:           cfg.Bugs,
			MessageFaults:  cfg.MessageFaults,
			RecordEvent: func(event string) {
				c.recordEvent(event)
			},
		})
		if err != nil {
			return nil, err
		}
		c.nodes[id] = node
	}
	return c, nil
}

func (c *Cluster) Start(ctx context.Context) {
	for _, node := range c.nodes {
		node.Start(ctx)
	}
}

func (c *Cluster) Close() error {
	return c.network.Close()
}

func (c *Cluster) LeaderID() string {
	leaders := make([]string, 0, len(c.nodes))
	for _, n := range c.nodes {
		s := n.Snapshot()
		if s.Role == RoleLeader {
			leaders = append(leaders, s.ID)
		}
	}
	if len(leaders) != 1 {
		return ""
	}
	return leaders[0]
}

func (c *Cluster) WaitForSingleLeader(ctx context.Context, interval time.Duration) (string, error) {
	if interval <= 0 {
		interval = 10 * time.Millisecond
	}
	timer := time.NewTicker(interval)
	defer timer.Stop()
	for {
		id := c.LeaderID()
		if id != "" {
			return id, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Cluster) Propose(ctx context.Context, leaderID, payload string) error {
	n, ok := c.nodes[leaderID]
	if !ok {
		return fmt.Errorf("unknown leader node %q", leaderID)
	}
	return n.Propose(ctx, payload)
}

func (c *Cluster) BumpNodeTerm(nodeID string, term int) error {
	n, ok := c.nodes[nodeID]
	if !ok {
		return fmt.Errorf("unknown node %q", nodeID)
	}
	n.BumpTerm(term)
	return nil
}

func (c *Cluster) MaxTerm() int {
	maxTerm := 0
	for _, n := range c.nodes {
		s := n.Snapshot()
		if s.Term > maxTerm {
			maxTerm = s.Term
		}
	}
	return maxTerm
}

func (c *Cluster) NodeSnapshots() []NodeSnapshot {
	snaps := make([]NodeSnapshot, 0, len(c.nodes))
	for _, n := range c.nodes {
		snaps = append(snaps, n.Snapshot())
	}
	slices.SortFunc(snaps, func(a, b NodeSnapshot) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return snaps
}

func (c *Cluster) NodeSnapshot(nodeID string) (NodeSnapshot, error) {
	n, ok := c.nodes[nodeID]
	if !ok {
		return NodeSnapshot{}, fmt.Errorf("unknown node %q", nodeID)
	}
	return n.Snapshot(), nil
}

func (c *Cluster) InjectAppendEntries(ctx context.Context, fromID, toID string, req Message) (Message, error) {
	req.Type = MsgAppendEntries
	return c.InjectMessage(ctx, fromID, toID, req)
}

func (c *Cluster) InjectMessage(ctx context.Context, fromID, toID string, req Message) (Message, error) {
	src, ok := c.nodes[fromID]
	if !ok {
		return Message{}, fmt.Errorf("unknown source node %q", fromID)
	}
	targetAddr, ok := c.addresses[toID]
	if !ok {
		return Message{}, fmt.Errorf("unknown target node %q", toID)
	}
	return src.sendMessage(ctx, toID, targetAddr, req)
}

func (c *Cluster) EventLog() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.events))
	copy(out, c.events)
	return out
}

func (c *Cluster) EventHash() string {
	h := fnv.New64a()
	lines := c.EventLog()
	slices.Sort(lines)
	for _, line := range lines {
		_, _ = h.Write([]byte(line))
		_, _ = h.Write([]byte{'\n'})
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

func (c *Cluster) recordEvent(event string) {
	c.mu.Lock()
	c.events = append(c.events, event)
	c.mu.Unlock()
}
