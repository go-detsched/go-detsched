package scenarios

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"
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
	ScenarioLogTruncation    = "log_truncation"
)

type RunConfig struct {
	Scenario  string
	Seed      int64
	Nodes     int
	Rounds    int
	ExpectBug bool
	Verbose   bool
	Synctest  bool
}

type Result struct {
	Scenario             string
	Seed                 int64
	Passed               bool
	BugObserved          bool
	IssueCode            string
	Reason               string
	Evidence             string
	EventHash            string
	Events               []string
	OraclePassed         bool
	OracleViolationCount int
	OracleFirstViolation string
	OracleViolations     []string
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
		ScenarioLogTruncation,
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
	case ScenarioLogTruncation:
		return runLogTruncation(cfg)
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
		result.Passed = cfg.ExpectBug
		if cfg.ExpectBug {
			result.IssueCode = "RAFT_SPLIT_VOTE_LIVELOCK"
			result.Reason = "reproducible split-vote livelock (fixed timeout + dropped vote requests)"
			result.Evidence = "leader=none vote_requests=dropped timeout=4s"
		} else {
			result.IssueCode = "RAFT_SPLIT_VOTE_BUG_STILL_PRESENT"
			result.Reason = "split-vote livelock still reproduced after fix patch"
			result.Evidence = "leader=none vote_requests=dropped timeout=4s"
		}
		result.EventHash = stableHash(result.Scenario, result.Reason, strconv.FormatInt(result.Seed, 10))
		return withOracle(cluster, result), nil
	}
	result.BugObserved = false
	if cfg.ExpectBug {
		result.Passed = false
		result.IssueCode = "RAFT_SPLIT_VOTE_NOT_REPRODUCED"
		result.Reason = "leader elected unexpectedly; adjust seed or scenario parameters"
		result.Evidence = "leader=present vote_requests=dropped"
	} else {
		result.Passed = true
		result.IssueCode = "RAFT_SPLIT_VOTE_FIXED"
		result.Reason = "leader election survived vote-loss fault pattern without livelock"
		result.Evidence = "leader=present vote_requests=dropped"
	}
	result.EventHash = stableHash(result.Scenario, result.Reason, strconv.FormatInt(result.Seed, 10))
	return withOracle(cluster, result), nil
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
			result.Passed = cfg.ExpectBug
			if cfg.ExpectBug {
				result.IssueCode = "RAFT_STALE_LEADER_ACCEPTED"
				result.Reason = "stale leader append accepted after follower moved to higher term"
				result.Evidence = "event=bug_accept_stale follower_accepted_lower_term_append=true"
			} else {
				result.IssueCode = "RAFT_STALE_LEADER_BUG_STILL_PRESENT"
				result.Reason = "stale-leader acceptance still observed after fix patch"
				result.Evidence = "event=bug_accept_stale follower_accepted_lower_term_append=true"
			}
			result.EventHash = stableHash(result.Scenario, result.Reason, strconv.FormatInt(result.Seed, 10))
			return withOracle(cluster, result), nil
		}
	}
	if cfg.ExpectBug {
		result.Passed = false
		result.IssueCode = "RAFT_STALE_LEADER_NOT_REPRODUCED"
		result.Reason = "no stale-leader acceptance event observed"
		result.Evidence = "event=bug_accept_stale missing"
	} else {
		result.Passed = true
		result.IssueCode = "RAFT_STALE_LEADER_FIXED"
		result.Reason = "follower rejected stale leader append after term bump"
		result.Evidence = "event=bug_accept_stale missing"
	}
	result.EventHash = stableHash(result.Scenario, result.Reason, strconv.FormatInt(result.Seed, 10))
	return withOracle(cluster, result), nil
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
		result.Passed = cfg.ExpectBug
		if cfg.ExpectBug {
			result.IssueCode = "RAFT_COMMIT_WITHOUT_MAJORITY"
			result.Reason = fmt.Sprintf("commit advanced without majority replication commit=%d replicated=%d majority=%d", highestCommit, replicatedAtOrAbove, majority)
			result.Evidence = fmt.Sprintf("commit=%d replicated=%d majority=%d", highestCommit, replicatedAtOrAbove, majority)
		} else {
			result.IssueCode = "RAFT_COMMIT_BUG_STILL_PRESENT"
			result.Reason = fmt.Sprintf("commit still advances without majority after fix patch commit=%d replicated=%d majority=%d", highestCommit, replicatedAtOrAbove, majority)
			result.Evidence = fmt.Sprintf("commit=%d replicated=%d majority=%d", highestCommit, replicatedAtOrAbove, majority)
		}
		result.EventHash = stableHash(result.Scenario, result.Reason, strconv.FormatInt(result.Seed, 10))
		return withOracle(cluster, result), nil
	}
	if cfg.ExpectBug {
		result.Passed = false
		result.IssueCode = "RAFT_COMMIT_BUG_NOT_REPRODUCED"
		result.Reason = "did not observe buggy commit advancement"
		result.Evidence = fmt.Sprintf("commit=%d replicated=%d majority=%d", highestCommit, replicatedAtOrAbove, majority)
	} else {
		result.Passed = true
		result.IssueCode = "RAFT_COMMIT_WITH_MAJORITY_FIXED"
		result.Reason = "commit index did not advance without majority replication"
		result.Evidence = fmt.Sprintf("commit=%d replicated=%d majority=%d", highestCommit, replicatedAtOrAbove, majority)
	}
	result.EventHash = stableHash(result.Scenario, result.Reason, strconv.FormatInt(result.Seed, 10))
	return withOracle(cluster, result), nil
}

func runLogTruncation(cfg RunConfig) (Result, error) {
	nodeIDs := makeNodeIDs(max(cfg.Nodes, 3))
	cluster, cancel, err := startCluster(nodeIDs, cfg.Seed, raft.BugConfig{
		UnsafeLogTruncation: true,
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
	for i := 0; i < max(cfg.Rounds, 2); i++ {
		_ = cluster.Propose(ctx, leaderID, fmt.Sprintf("seed-cmd-%d", i))
	}
	if cfg.Synctest {
		synctest.Wait()
		time.Sleep(scenarioTimeout(cfg, 500*time.Millisecond, 60*time.Millisecond))
		synctest.Wait()
	} else {
		time.Sleep(scenarioTimeout(cfg, 500*time.Millisecond, 60*time.Millisecond))
	}

	var followerID string
	for _, id := range nodeIDs {
		if id != leaderID {
			followerID = id
			break
		}
	}
	if followerID == "" {
		return Result{}, fmt.Errorf("unable to choose follower for log truncation scenario")
	}

	leaderSnap, err := cluster.NodeSnapshot(leaderID)
	if err != nil {
		return Result{}, err
	}
	beforeSnap, err := cluster.NodeSnapshot(followerID)
	if err != nil {
		return Result{}, err
	}

	req := raft.Message{
		Type:         raft.MsgAppendEntries,
		From:         leaderID,
		Term:         leaderSnap.Term,
		LeaderID:     leaderID,
		PrevLogIndex: beforeSnap.LogLength + 4, // intentionally inconsistent
		PrevLogTerm:  leaderSnap.Term,
		Entries: []raft.LogEntry{
			{
				Index: beforeSnap.LogLength + 5,
				Term:  leaderSnap.Term,
				Data:  "forged-log-entry",
			},
		},
		LeaderCommit: beforeSnap.CommitIndex,
	}
	resp, err := cluster.InjectAppendEntries(ctx, leaderID, followerID, req)
	if err != nil {
		return Result{}, err
	}
	afterSnap, err := cluster.NodeSnapshot(followerID)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Scenario: ScenarioLogTruncation,
		Seed:     cfg.Seed,
		Events:   cluster.EventLog(),
	}
	bugObserved := resp.Success && afterSnap.LogLength > beforeSnap.LogLength
	if bugObserved {
		result.BugObserved = true
		result.Passed = cfg.ExpectBug
		if cfg.ExpectBug {
			result.IssueCode = "RAFT_LOG_TRUNCATION_ACCEPTED"
			result.Reason = "follower accepted append entries with inconsistent previous-log reference"
		} else {
			result.IssueCode = "RAFT_LOG_TRUNCATION_BUG_STILL_PRESENT"
			result.Reason = "inconsistent previous-log append is still accepted after fix patch"
		}
		result.Evidence = fmt.Sprintf("target=%s before_len=%d after_len=%d resp_success=%t", followerID, beforeSnap.LogLength, afterSnap.LogLength, resp.Success)
		result.EventHash = stableHash(result.Scenario, result.Reason, strconv.FormatInt(result.Seed, 10))
		return withOracle(cluster, result), nil
	}

	if cfg.ExpectBug {
		result.Passed = false
		result.IssueCode = "RAFT_LOG_TRUNCATION_NOT_REPRODUCED"
		result.Reason = "inconsistent previous-log append was rejected unexpectedly"
	} else {
		result.Passed = true
		result.IssueCode = "RAFT_LOG_TRUNCATION_FIXED"
		result.Reason = "follower rejected inconsistent previous-log append"
	}
	result.Evidence = fmt.Sprintf("target=%s before_len=%d after_len=%d resp_success=%t", followerID, beforeSnap.LogLength, afterSnap.LogLength, resp.Success)
	result.EventHash = stableHash(result.Scenario, result.Reason, strconv.FormatInt(result.Seed, 10))
	return withOracle(cluster, result), nil
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

func min(a, b int) int {
	if a < b {
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

func withOracle(cluster *raft.Cluster, result Result) Result {
	violations := raftSafetyViolations(cluster.NodeSnapshots(), result.Events)
	result.OraclePassed = len(violations) == 0
	result.OracleViolationCount = len(violations)
	result.OracleViolations = violations
	result.OracleFirstViolation = "none"
	if len(violations) > 0 {
		result.OracleFirstViolation = violations[0]
	}
	return result
}

func raftSafetyViolations(snaps []raft.NodeSnapshot, events []string) []string {
	violations := []string{}
	if len(snaps) == 0 {
		return []string{"ORACLE_EMPTY_CLUSTER"}
	}

	leadersByTerm := map[int]map[string]struct{}{}
	for _, e := range events {
		if !strings.Contains(e, "leader_elected") {
			continue
		}
		term := parseIntKV(e, "term")
		node := parseStringKV(e, "node")
		if term <= 0 || node == "" {
			continue
		}
		if _, ok := leadersByTerm[term]; !ok {
			leadersByTerm[term] = map[string]struct{}{}
		}
		leadersByTerm[term][node] = struct{}{}
	}
	for term, leaders := range leadersByTerm {
		if len(leaders) > 1 {
			violations = append(violations, fmt.Sprintf("RAFT_SAFETY_MULTI_LEADER_TERM_%d", term))
		}
	}

	for _, s := range snaps {
		if s.CommitIndex < 0 || s.CommitIndex > len(s.Log) {
			violations = append(violations, fmt.Sprintf("RAFT_SAFETY_INVALID_COMMIT_INDEX_%s", s.ID))
		}
		for pos, e := range s.Log {
			expectedIndex := pos + 1
			if e.Index != expectedIndex {
				violations = append(violations, fmt.Sprintf("RAFT_SAFETY_NON_CONTIGUOUS_LOG_%s", s.ID))
				break
			}
		}
	}

	majority := len(snaps)/2 + 1
	for _, s := range snaps {
		if s.CommitIndex <= 0 {
			continue
		}
		replicated := 0
		for _, other := range snaps {
			if len(other.Log) >= s.CommitIndex {
				replicated++
			}
		}
		if replicated < majority {
			violations = append(violations, fmt.Sprintf("RAFT_SAFETY_COMMIT_WITHOUT_MAJORITY_%s", s.ID))
		}
	}

	for i := 0; i < len(snaps); i++ {
		for j := i + 1; j < len(snaps); j++ {
			a := snaps[i]
			b := snaps[j]
			limit := min(a.CommitIndex, b.CommitIndex)
			for idx := 1; idx <= limit; idx++ {
				ae := a.Log[idx-1]
				be := b.Log[idx-1]
				if ae.Term != be.Term || ae.Data != be.Data {
					violations = append(violations, fmt.Sprintf("RAFT_SAFETY_COMMITTED_DIVERGENCE_IDX_%d", idx))
					break
				}
			}
		}
	}

	sort.Strings(violations)
	return dedupeStrings(violations)
}

func parseIntKV(event, key string) int {
	raw := parseStringKV(event, key)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}

func parseStringKV(event, key string) string {
	prefix := key + "="
	for _, token := range strings.Fields(event) {
		if strings.HasPrefix(token, prefix) {
			return strings.TrimPrefix(token, prefix)
		}
	}
	return ""
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, 0, len(in))
	last := ""
	for i, s := range in {
		if i == 0 || s != last {
			out = append(out, s)
			last = s
		}
	}
	return out
}
