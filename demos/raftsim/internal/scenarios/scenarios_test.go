package scenarios

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestSynctestDeterministicRepro(t *testing.T) {
	seedStart := envInt("RAFTSIM_SEED_START", 1)
	seedCount := envInt("RAFTSIM_SEED_COUNT", 25)
	nodes := envInt("RAFTSIM_NODES", 5)
	rounds := envInt("RAFTSIM_ROUNDS", 4)

	if seedCount <= 0 {
		t.Fatalf("RAFTSIM_SEED_COUNT must be > 0, got %d", seedCount)
	}

	for _, scenario := range ScenarioNames() {
		for i := 0; i < seedCount; i++ {
			seed := int64(seedStart + i)
			name := fmt.Sprintf("%s_seed_%d", scenario, seed)
			t.Run(name, func(t *testing.T) {
				cfg := RunConfig{
					Scenario: scenario,
					Seed:     seed,
					Nodes:    nodes,
					Rounds:   rounds,
					Verbose:  false,
					Synctest: true,
				}

				seedStartTime := time.Now()

				run1Start := time.Now()
				r1, err := RunWithSynctest(t, cfg)
				if err != nil {
					t.Fatalf("first run error: %v", err)
				}
				run1Dur := time.Since(run1Start)

				run2Start := time.Now()
				r2, err := RunWithSynctest(t, cfg)
				if err != nil {
					t.Fatalf("second run error: %v", err)
				}
				run2Dur := time.Since(run2Start)
				seedDur := time.Since(seedStartTime)

				if r1.IssueCode != r2.IssueCode || r1.EventHash != r2.EventHash || r1.Reason != r2.Reason || r1.Evidence != r2.Evidence || r1.BugObserved != r2.BugObserved || r1.Passed != r2.Passed {
					t.Fatalf(
						"non-deterministic replay scenario=%s seed=%d run1(status=%v bug=%v issue=%s hash=%s reason=%q evidence=%q) run2(status=%v bug=%v issue=%s hash=%s reason=%q evidence=%q)",
						scenario,
						seed,
						r1.Passed,
						r1.BugObserved,
						r1.IssueCode,
						r1.EventHash,
						r1.Reason,
						r1.Evidence,
						r2.Passed,
						r2.BugObserved,
						r2.IssueCode,
						r2.EventHash,
						r2.Reason,
						r2.Evidence,
					)
				}
				t.Logf(
					"scenario=%s seed=%d status=%s bug_observed=%t issue=%s hash=%s reason=%q evidence=%q run1_us=%d run2_us=%d total_us=%d",
					r1.Scenario,
					r1.Seed,
					statusString(r1.Passed),
					r1.BugObserved,
					r1.IssueCode,
					r1.EventHash,
					r1.Reason,
					r1.Evidence,
					run1Dur.Microseconds(),
					run2Dur.Microseconds(),
					seedDur.Microseconds(),
				)
			})
		}
	}

}

func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func statusString(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}
