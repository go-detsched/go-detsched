package raft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"demos/raftsim/internal/transport"
)

type Role string

const (
	RoleFollower  Role = "follower"
	RoleCandidate Role = "candidate"
	RoleLeader    Role = "leader"
)

type MessageType string

const (
	MsgRequestVote   MessageType = "request_vote"
	MsgVoteResponse  MessageType = "vote_response"
	MsgAppendEntries MessageType = "append_entries"
	MsgAppendResult  MessageType = "append_result"
)

type LogEntry struct {
	Index int    `json:"index"`
	Term  int    `json:"term"`
	Data  string `json:"data"`
}

type Message struct {
	Type MessageType `json:"type"`
	From string      `json:"from"`
	Term int         `json:"term"`

	// RequestVote.
	CandidateID  string `json:"candidate_id,omitempty"`
	LastLogTerm  int    `json:"last_log_term,omitempty"`
	LastLogIndex int    `json:"last_log_index,omitempty"`

	// AppendEntries.
	LeaderID     string     `json:"leader_id,omitempty"`
	PrevLogIndex int        `json:"prev_log_index,omitempty"`
	PrevLogTerm  int        `json:"prev_log_term,omitempty"`
	Entries      []LogEntry `json:"entries,omitempty"`
	LeaderCommit int        `json:"leader_commit,omitempty"`

	// Responses.
	VoteGranted  bool   `json:"vote_granted,omitempty"`
	Success      bool   `json:"success,omitempty"`
	MatchIndex   int    `json:"match_index,omitempty"`
	RejectReason string `json:"reject_reason,omitempty"`
}

type BugConfig struct {
	FixedElectionTimeout bool
	AcceptStaleLeader    bool
	CommitOnSingleAck    bool
}

type MessageFaultHook func(from, to string, msgType MessageType, seq uint64) (drop bool, delay time.Duration)

type NodeConfig struct {
	ID      string
	Address string
	Peers   map[string]string

	Listener net.Listener
	Dialer   transport.ContextDialer

	ElectionBase   time.Duration
	ElectionJitter time.Duration
	Heartbeat      time.Duration
	Rand           *rand.Rand
	Bugs           BugConfig
	MessageFaults  MessageFaultHook
	RecordEvent    func(string)
}

type Node struct {
	id      string
	address string
	peers   map[string]string
	order   []string

	lis    net.Listener
	dialer transport.ContextDialer

	electionBase   time.Duration
	electionJitter time.Duration
	heartbeat      time.Duration
	rng            *rand.Rand

	bugs          BugConfig
	messageFaults MessageFaultHook
	recordEvent   func(string)

	mu            sync.Mutex
	role          Role
	term          int
	votedFor      string
	leaderID      string
	lastHeartbeat time.Time
	log           []LogEntry
	commitIndex   int

	rpcSeq atomic.Uint64
}

type NodeSnapshot struct {
	ID          string
	Term        int
	Role        Role
	LeaderID    string
	LogLength   int
	CommitIndex int
}

func NewNode(cfg NodeConfig) (*Node, error) {
	if cfg.Listener == nil {
		return nil, errors.New("listener is required")
	}
	if cfg.Dialer == nil {
		return nil, errors.New("dialer is required")
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.New(rand.NewSource(1))
	}
	n := &Node{
		id:             cfg.ID,
		address:        cfg.Address,
		peers:          cloneMap(cfg.Peers),
		lis:            cfg.Listener,
		dialer:         cfg.Dialer,
		electionBase:   maxDuration(cfg.ElectionBase, 120*time.Millisecond),
		electionJitter: cfg.ElectionJitter,
		heartbeat:      maxDuration(cfg.Heartbeat, 40*time.Millisecond),
		rng:            cfg.Rand,
		bugs:           cfg.Bugs,
		messageFaults:  cfg.MessageFaults,
		recordEvent:    cfg.RecordEvent,
		role:           RoleFollower,
		lastHeartbeat:  time.Now(),
	}
	for id := range n.peers {
		n.order = append(n.order, id)
	}
	return n, nil
}

func (n *Node) Start(ctx context.Context) {
	go n.serveLoop(ctx)
	go n.electionLoop(ctx)
}

func (n *Node) ID() string {
	return n.id
}

func (n *Node) Address() string {
	return n.address
}

func (n *Node) Snapshot() NodeSnapshot {
	n.mu.Lock()
	defer n.mu.Unlock()
	return NodeSnapshot{
		ID:          n.id,
		Term:        n.term,
		Role:        n.role,
		LeaderID:    n.leaderID,
		LogLength:   len(n.log),
		CommitIndex: n.commitIndex,
	}
}

func (n *Node) BumpTerm(term int) {
	n.mu.Lock()
	if term > n.term {
		n.term = term
		n.role = RoleFollower
		n.votedFor = ""
	}
	n.mu.Unlock()
	n.eventf("term_bump node=%s term=%d", n.id, term)
}

func (n *Node) Propose(ctx context.Context, payload string) error {
	n.mu.Lock()
	if n.role != RoleLeader {
		leader := n.leaderID
		n.mu.Unlock()
		return fmt.Errorf("node %s is not leader (leader=%s)", n.id, leader)
	}
	nextIndex := len(n.log) + 1
	entry := LogEntry{
		Index: nextIndex,
		Term:  n.term,
		Data:  payload,
	}
	prevIndex := nextIndex - 1
	prevTerm := 0
	if prevIndex > 0 {
		prevTerm = n.log[prevIndex-1].Term
	}
	n.log = append(n.log, entry)
	leaderTerm := n.term
	leaderCommit := n.commitIndex
	n.mu.Unlock()

	n.eventf("propose leader=%s index=%d payload=%q", n.id, entry.Index, payload)
	acks := 1 // self
	for _, peerID := range n.order {
		addr := n.peers[peerID]
		req := Message{
			Type:         MsgAppendEntries,
			From:         n.id,
			Term:         leaderTerm,
			LeaderID:     n.id,
			PrevLogIndex: prevIndex,
			PrevLogTerm:  prevTerm,
			Entries:      []LogEntry{entry},
			LeaderCommit: leaderCommit,
		}
		resp, err := n.sendMessage(ctx, peerID, addr, req)
		if err != nil {
			n.eventf("append_failed leader=%s target=%s err=%q", n.id, peerID, err.Error())
			continue
		}
		if resp.Success {
			acks++
		}
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	clusterSize := len(n.peers) + 1
	majority := clusterSize/2 + 1
	if n.bugs.CommitOnSingleAck {
		// Intentionally buggy commit rule for demo purposes.
		if acks >= 2 {
			n.commitIndex = entry.Index
			n.eventf("commit_buggy leader=%s index=%d acks=%d majority=%d", n.id, entry.Index, acks, majority)
			return nil
		}
		return fmt.Errorf("not enough acknowledgements for buggy commit: %d", acks)
	}

	if acks >= majority {
		n.commitIndex = entry.Index
		n.eventf("commit_majority leader=%s index=%d acks=%d", n.id, entry.Index, acks)
		return nil
	}
	return fmt.Errorf("not enough acknowledgements for majority commit: %d/%d", acks, majority)
}

func (n *Node) electionLoop(ctx context.Context) {
	timer := time.NewTimer(n.electionTimeout())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			n.mu.Lock()
			elapsed := time.Since(n.lastHeartbeat)
			timeout := n.electionTimeout()
			if n.role == RoleLeader || elapsed < timeout {
				n.mu.Unlock()
				timer.Reset(timeout)
				continue
			}
			n.mu.Unlock()

			n.runElectionRound(ctx)
			timer.Reset(n.electionTimeout())
		}
	}
}

func (n *Node) runElectionRound(ctx context.Context) {
	n.mu.Lock()
	n.role = RoleCandidate
	n.term++
	currentTerm := n.term
	n.votedFor = n.id
	n.leaderID = ""
	n.lastHeartbeat = time.Now()
	n.mu.Unlock()
	n.eventf("election_start node=%s term=%d", n.id, currentTerm)

	votes := 1
	total := len(n.peers) + 1
	majority := total/2 + 1

	for _, peerID := range n.order {
		addr := n.peers[peerID]
		req := Message{
			Type:        MsgRequestVote,
			From:        n.id,
			Term:        currentTerm,
			CandidateID: n.id,
		}
		resp, err := n.sendMessage(ctx, peerID, addr, req)
		if err != nil {
			n.eventf("vote_failed from=%s to=%s term=%d err=%q", n.id, peerID, currentTerm, err.Error())
			continue
		}
		if resp.Term > currentTerm {
			n.mu.Lock()
			if resp.Term > n.term {
				n.term = resp.Term
				n.role = RoleFollower
				n.votedFor = ""
			}
			n.mu.Unlock()
			n.eventf("election_step_down node=%s new_term=%d", n.id, resp.Term)
			return
		}
		if resp.VoteGranted {
			votes++
		}
	}

	n.mu.Lock()
	if votes >= majority {
		n.role = RoleLeader
		n.leaderID = n.id
		n.lastHeartbeat = time.Now()
		n.mu.Unlock()
		n.eventf("leader_elected node=%s term=%d votes=%d", n.id, currentTerm, votes)
		go n.heartbeatLoop(ctx, currentTerm)
		return
	}
	n.role = RoleCandidate
	n.mu.Unlock()
	n.eventf("election_split node=%s term=%d votes=%d", n.id, currentTerm, votes)
}

func (n *Node) heartbeatLoop(ctx context.Context, term int) {
	ticker := time.NewTicker(n.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.mu.Lock()
			if n.role != RoleLeader || n.term != term {
				n.mu.Unlock()
				return
			}
			leaderCommit := n.commitIndex
			n.mu.Unlock()
			for _, peerID := range n.order {
				addr := n.peers[peerID]
				req := Message{
					Type:         MsgAppendEntries,
					From:         n.id,
					Term:         term,
					LeaderID:     n.id,
					LeaderCommit: leaderCommit,
				}
				_, err := n.sendMessage(ctx, peerID, addr, req)
				if err != nil {
					n.eventf("heartbeat_failed leader=%s target=%s term=%d err=%q", n.id, peerID, term, err.Error())
				}
			}
		}
	}
}

func (n *Node) serveLoop(ctx context.Context) {
	for {
		conn, err := n.lis.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			n.eventf("accept_error node=%s err=%q", n.id, err.Error())
			continue
		}
		go n.handleConn(conn)
	}
}

func (n *Node) handleConn(conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)
	var req Message
	if err := dec.Decode(&req); err != nil {
		_ = enc.Encode(Message{Type: req.Type, Success: false, RejectReason: err.Error()})
		return
	}
	resp := n.dispatch(req)
	if err := enc.Encode(resp); err != nil && !errors.Is(err, io.EOF) {
		n.eventf("encode_error node=%s err=%q", n.id, err.Error())
	}
}

func (n *Node) dispatch(req Message) Message {
	switch req.Type {
	case MsgRequestVote:
		return n.handleRequestVote(req)
	case MsgAppendEntries:
		return n.handleAppendEntries(req)
	default:
		return Message{Type: req.Type, Term: n.currentTerm(), Success: false, RejectReason: "unknown message type"}
	}
}

func (n *Node) handleRequestVote(req Message) Message {
	n.mu.Lock()
	defer n.mu.Unlock()
	if req.Term < n.term {
		return Message{Type: MsgVoteResponse, Term: n.term, VoteGranted: false, Success: false, RejectReason: "stale term"}
	}
	if req.Term > n.term {
		n.term = req.Term
		n.role = RoleFollower
		n.votedFor = ""
	}
	grant := n.votedFor == "" || n.votedFor == req.CandidateID
	if grant {
		n.votedFor = req.CandidateID
		n.lastHeartbeat = time.Now()
	}
	n.eventf("vote node=%s candidate=%s term=%d grant=%v", n.id, req.CandidateID, req.Term, grant)
	return Message{
		Type:        MsgVoteResponse,
		Term:        n.term,
		VoteGranted: grant,
		Success:     grant,
	}
}

func (n *Node) handleAppendEntries(req Message) Message {
	n.mu.Lock()
	defer n.mu.Unlock()

	if req.Term < n.term && !n.bugs.AcceptStaleLeader {
		return Message{
			Type:         MsgAppendResult,
			Term:         n.term,
			Success:      false,
			RejectReason: "stale leader term",
			MatchIndex:   len(n.log),
		}
	}
	if req.Term < n.term && n.bugs.AcceptStaleLeader {
		n.eventf("bug_accept_stale node=%s leader=%s leader_term=%d local_term=%d", n.id, req.LeaderID, req.Term, n.term)
	}
	if req.Term >= n.term {
		n.term = req.Term
		n.role = RoleFollower
		n.votedFor = ""
	}
	n.leaderID = req.LeaderID
	n.lastHeartbeat = time.Now()

	for _, e := range req.Entries {
		if e.Index <= 0 {
			continue
		}
		if e.Index <= len(n.log) {
			n.log[e.Index-1] = e
			continue
		}
		n.log = append(n.log, e)
	}
	if req.LeaderCommit > n.commitIndex {
		n.commitIndex = min(req.LeaderCommit, len(n.log))
	}
	return Message{
		Type:       MsgAppendResult,
		Term:       n.term,
		Success:    true,
		MatchIndex: len(n.log),
	}
}

func (n *Node) sendMessage(ctx context.Context, peerID, address string, req Message) (Message, error) {
	seq := n.rpcSeq.Add(1)
	if n.messageFaults != nil {
		drop, delay := n.messageFaults(n.id, peerID, req.Type, seq)
		if drop {
			return Message{}, fmt.Errorf("message dropped from=%s to=%s type=%s seq=%d", n.id, peerID, req.Type, seq)
		}
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return Message{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	conn, err := n.dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return Message{}, err
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	}
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)
	if err := enc.Encode(req); err != nil {
		return Message{}, err
	}
	var resp Message
	if err := dec.Decode(&resp); err != nil {
		return Message{}, err
	}
	return resp, nil
}

func (n *Node) currentTerm() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.term
}

func (n *Node) electionTimeout() time.Duration {
	if n.bugs.FixedElectionTimeout {
		return n.electionBase
	}
	if n.electionJitter <= 0 {
		return n.electionBase
	}
	return n.electionBase + time.Duration(n.rng.Int63n(int64(n.electionJitter)))
}

func (n *Node) eventf(format string, args ...any) {
	if n.recordEvent == nil {
		return
	}
	n.recordEvent(fmt.Sprintf(format, args...))
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
