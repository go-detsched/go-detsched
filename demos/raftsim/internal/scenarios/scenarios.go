package scenarios

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"demos/raftsim/internal/raft"
)

const (
	ScenarioSplitVote        = "split_vote"
	ScenarioStaleLeader      = "stale_leader"
	ScenarioReorderCommitBug = "reorder_commit"
)

type RunConfig struct {
	Scenario string
	Seed     int64
	Nodes    int
	Rounds   int
	Verbose  bool
	Synctest bool
}

type Result struct {
	Scenario    string
	Seed        int64
	Passed      bool
	BugObserved bool
	IssueCode   string
	Reason      string
	Evidence    string
	EventHash   string
	Events      []string
}

func Run(cfg RunConfig) (Result, error) {
	if cfg.Nodes < 3 {
		cfg.Nodes = 3
	}
	if cfg.Rounds <= 0 {
		cfg.Rounds = 1
	}
	if cfg.Seed == 0 {
		cfg.Seed = 1
	}

	if cfg.Synctest {
		var (
			result Result
			runErr error
		)
		err := runInSynctest(func() {
			result, runErr = runOne(cfg)
			synctest.Wait()
		})
		if err != nil {
			return Result{}, err
		}
		return result, runErr
	}
	return runOne(cfg)
}

// RunWithSynctest executes one scenario in a proper testing/synctest bubble.
// This is the preferred entrypoint for deterministic test suites.
func RunWithSynctest(t *testing.T, cfg RunConfig) (Result, error) {
	t.Helper()
	if cfg.Nodes < 3 {
		cfg.Nodes = 3
	}
	if cfg.Rounds <= 0 {
		cfg.Rounds = 1
	}
	if cfg.Seed == 0 {
		cfg.Seed = 1
	}
	cfg.Synctest = true

	var (
		result Result
		runErr error
	)
	synctest.Test(t, func(t *testing.T) {
		result, runErr = runOne(cfg)
		synctest.Wait()
	})
	return result, runErr
}

func ScenarioNames() []string {
	return []string{
		ScenarioSplitVote,
		ScenarioStaleLeader,
		ScenarioReorderCommitBug,
	}
}

func runOne(cfg RunConfig) (Result, error) {
	switch cfg.Scenario {
	case ScenarioSplitVote:
		return runSplitVote(cfg)
	case ScenarioStaleLeader:
		return runStaleLeader(cfg)
	case ScenarioReorderCommitBug:
		return runReorderCommit(cfg)
	default:
		return Result{}, fmt.Errorf("unknown scenario %q", cfg.Scenario)
	}
}

func runSplitVote(cfg RunConfig) (Result, error) {
	nodeIDs := makeNodeIDs(max(cfg.Nodes, 3))
	cluster, cancel, err := startClusterWithMessageFaults(nodeIDs, cfg.Seed, raft.BugConfig{
		FixedElectionTimeout: true,
	}, func(from, to string, msgType raft.MessageType, seq uint64) (bool, time.Duration) {
		if msgType == raft.MsgRequestVote {
			return true, 0
		}
		return false, 0
	})
	if err != nil {
		return Result{}, err
	}
	defer cancel()
	defer cluster.Close()

	ctx, done := context.WithTimeout(context.Background(), scenarioTimeout(cfg, 4*time.Second, 500*time.Millisecond))
	defer done()
	_, err = cluster.WaitForSingleLeader(ctx, 100*time.Millisecond)
	result := Result{
		Scenario: ScenarioSplitVote,
		Seed:     cfg.Seed,
		Events:   cluster.EventLog(),
	}
	if err != nil {
		result.BugObserved = true
		result.Passed = true
		result.IssueCode = "RAFT_SPLIT_VOTE_LIVELOCK"
		result.Reason = "reproducible split-vote livelock (fixed timeout + dropped vote requests)"
		result.Evidence = "leader=none vote_requests=dropped timeout=4s"
		result.EventHash = stableHash(result.Scenario, result.Reason, strconv.FormatInt(result.Seed, 10))
		return result, nil
	}
	result.Passed = false
	result.BugObserved = false
	result.IssueCode = "RAFT_SPLIT_VOTE_NOT_REPRODUCED"
	result.Reason = "leader elected unexpectedly; adjust seed or scenario parameters"
	result.Evidence = "leader=present vote_requests=dropped"
	result.EventHash = stableHash(result.Scenario, result.Reason, strconv.FormatInt(result.Seed, 10))
	return result, nil
}

func runStaleLeader(cfg RunConfig) (Result, error) {
	nodeIDs := makeNodeIDs(max(cfg.Nodes, 3))
	cluster, cancel, err := startCluster(nodeIDs, cfg.Seed, raft.BugConfig{
		AcceptStaleLeader: true,
	})
	if err != nil {
		return Result{}, err
	}
	defer cancel()
	defer cluster.Close()

	ctx, done := context.WithTimeout(context.Background(), scenarioTimeout(cfg, 6*time.Second, 700*time.Millisecond))
	defer done()
	leaderID, err := cluster.WaitForSingleLeader(ctx, 30*time.Millisecond)
	if err != nil {
		return Result{}, err
	}

	var bumpTarget string
	for _, id := range nodeIDs {
		if id != leaderID {
			bumpTarget = id
			break
		}
	}
	if bumpTarget == "" {
		return Result{}, fmt.Errorf("unable to select follower for term bump")
	}
	_ = cluster.BumpNodeTerm(bumpTarget, cluster.MaxTerm()+1)

	// Allow stale heartbeats from the old leader to flow.
	if cfg.Synctest {
		synctest.Wait()
		time.Sleep(scenarioTimeout(cfg, 400*time.Millisecond, 50*time.Millisecond))
		synctest.Wait()
	} else {
		time.Sleep(scenarioTimeout(cfg, 400*time.Millisecond, 50*time.Millisecond))
	}

	result := Result{
		Scenario: ScenarioStaleLeader,
		Seed:     cfg.Seed,
		Events:   cluster.EventLog(),
	}
	for _, e := range result.Events {
		if strings.Contains(e, "bug_accept_stale") {
			result.BugObserved = true
			result.Passed = true
			result.IssueCode = "RAFT_STALE_LEADER_ACCEPTED"
			result.Reason = "stale leader append accepted after follower moved to higher term"
			result.Evidence = "event=bug_accept_stale follower_accepted_lower_term_append=true"
			result.EventHash = stableHash(result.Scenario, result.Reason, strconv.FormatInt(result.Seed, 10))
			return result, nil
		}
	}
	result.Passed = false
	result.IssueCode = "RAFT_STALE_LEADER_NOT_REPRODUCED"
	result.Reason = "no stale-leader acceptance event observed"
	result.Evidence = "event=bug_accept_stale missing"
	result.EventHash = stableHash(result.Scenario, result.Reason, strconv.FormatInt(result.Seed, 10))
	return result, nil
}

func runReorderCommit(cfg RunConfig) (Result, error) {
	nodeCount := max(cfg.Nodes, 5)
	nodeIDs := makeNodeIDs(nodeCount)

	cluster, cancel, err := startClusterWithMessageFaults(nodeIDs, cfg.Seed, raft.BugConfig{
		CommitOnSingleAck: true,
	}, func(from, to string, msgType raft.MessageType, seq uint64) (bool, time.Duration) {
		if msgType != raft.MsgAppendEntries {
			return false, 0
		}
		// Keep only one fast append path to trigger buggy single-ack commits.
		if to != "n2" {
			return true, 0
		}
		if seq%2 == 0 {
			return false, 20 * time.Millisecond
		}
		return false, 0
	})
	if err != nil {
		return Result{}, err
	}
	defer cancel()
	defer cluster.Close()

	ctx, done := context.WithTimeout(context.Background(), scenarioTimeout(cfg, 7*time.Second, 900*time.Millisecond))
	defer done()
	leaderID, err := cluster.WaitForSingleLeader(ctx, 40*time.Millisecond)
	if err != nil {
		return Result{}, err
	}

	for i := 0; i < cfg.Rounds; i++ {
		if err := cluster.Propose(ctx, leaderID, fmt.Sprintf("cmd-%d", i)); err != nil {
			// Keep trying rounds; occasional failures are expected in this fault mode.
		}
	}
	if cfg.Synctest {
		synctest.Wait()
		time.Sleep(scenarioTimeout(cfg, 500*time.Millisecond, 60*time.Millisecond))
		synctest.Wait()
	} else {
		time.Sleep(scenarioTimeout(cfg, 500*time.Millisecond, 60*time.Millisecond))
	}

	snaps := cluster.NodeSnapshots()
	highestCommit := 0
	replicatedAtOrAbove := 0
	for _, s := range snaps {
		if s.CommitIndex > highestCommit {
			highestCommit = s.CommitIndex
		}
	}
	for _, s := range snaps {
		if s.LogLength >= highestCommit {
			replicatedAtOrAbove++
		}
	}
	majority := len(snaps)/2 + 1

	result := Result{
		Scenario: ScenarioReorderCommitBug,
		Seed:     cfg.Seed,
		Events:   cluster.EventLog(),
	}
	if highestCommit > 0 && replicatedAtOrAbove < majority {
		result.BugObserved = true
		result.Passed = true
		result.IssueCode = "RAFT_COMMIT_WITHOUT_MAJORITY"
		result.Reason = fmt.Sprintf("commit advanced without majority replication commit=%d replicated=%d majority=%d", highestCommit, replicatedAtOrAbove, majority)
		result.Evidence = fmt.Sprintf("commit=%d replicated=%d majority=%d", highestCommit, replicatedAtOrAbove, majority)
		result.EventHash = stableHash(result.Scenario, result.Reason, strconv.FormatInt(result.Seed, 10))
		return result, nil
	}
	result.Passed = false
	result.IssueCode = "RAFT_COMMIT_BUG_NOT_REPRODUCED"
	result.Reason = "did not observe buggy commit advancement"
	result.Evidence = fmt.Sprintf("commit=%d replicated=%d majority=%d", highestCommit, replicatedAtOrAbove, majority)
	result.EventHash = stableHash(result.Scenario, result.Reason, strconv.FormatInt(result.Seed, 10))
	return result, nil
}

func startCluster(nodeIDs []string, seed int64, bugs raft.BugConfig) (*raft.Cluster, context.CancelFunc, error) {
	return startClusterWithMessageFaults(nodeIDs, seed, bugs, nil)
}

func startClusterWithMessageFaults(
	nodeIDs []string,
	seed int64,
	bugs raft.BugConfig,
	hook raft.MessageFaultHook,
) (*raft.Cluster, context.CancelFunc, error) {
	cluster, err := raft.NewCluster(raft.ClusterConfig{
		NodeIDs:        nodeIDs,
		Seed:           seed,
		Bugs:           bugs,
		ElectionBase:   180 * time.Millisecond,
		ElectionJitter: 120 * time.Millisecond,
		Heartbeat:      50 * time.Millisecond,
		MessageFaults:  hook,
	})
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	cluster.Start(ctx)
	return cluster, cancel, nil
}

func makeNodeIDs(n int) []string {
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = fmt.Sprintf("n%d", i+1)
	}
	return ids
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func runInSynctest(fn func()) error {
	var panicErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				panicErr = fmt.Errorf("synctest panic: %v", r)
			}
		}()
		synctest.Test(&testing.T{}, func(*testing.T) {
			fn()
			synctest.Wait()
		})
	}()
	<-done
	return panicErr
}

func stableHash(parts ...string) string {
	h := fnv.New64a()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{'\n'})
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

func scenarioTimeout(cfg RunConfig, normal, fast time.Duration) time.Duration {
	if cfg.Synctest {
		return fast
	}
	return normal
}
