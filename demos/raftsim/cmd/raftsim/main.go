package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"demos/raftsim/internal/scenarios"
)

func main() {
	scenario := flag.String("scenario", scenarios.ScenarioSplitVote, "scenario to run (split_vote|stale_leader|reorder_commit|all)")
	seed := flag.Int64("seed", 7, "deterministic scenario seed")
	nodes := flag.Int("nodes", 5, "number of raft nodes")
	rounds := flag.Int("rounds", 3, "proposal rounds for append scenarios")
	expectBug := flag.Bool("expect-bug", true, "expect vulnerable behavior (true) or fixed behavior (false)")
	verbose := flag.Bool("verbose", false, "print detailed event log")
	useSynctest := flag.Bool("synctest", true, "run scenarios inside testing/synctest bubble")
	flag.Parse()

	names, err := resolveScenarios(*scenario)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	exitCode := 0
	for _, name := range names {
		result, err := scenarios.Run(scenarios.RunConfig{
			Scenario: name,
			Seed:     *seed,
			Nodes:    *nodes,
			Rounds:   *rounds,
			ExpectBug: *expectBug,
			Verbose:  *verbose,
			Synctest: *useSynctest,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "scenario=%s seed=%d status=error err=%v\n", name, *seed, err)
			exitCode = 1
			continue
		}
		status := "FAIL"
		if result.Passed {
			status = "PASS"
		}
		fmt.Printf(
			"scenario=%s seed=%d status=%s bug_observed=%t issue=%s hash=%s reason=%q evidence=%q\n",
			result.Scenario,
			result.Seed,
			status,
			result.BugObserved,
			result.IssueCode,
			result.EventHash,
			result.Reason,
			result.Evidence,
		)
		if *verbose {
			for i, e := range result.Events {
				fmt.Printf("  [%03d] %s\n", i+1, e)
			}
		}
		if !result.Passed {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func resolveScenarios(input string) ([]string, error) {
	if input == "all" {
		return scenarios.ScenarioNames(), nil
	}
	for _, name := range scenarios.ScenarioNames() {
		if input == name {
			return []string{name}, nil
		}
	}
	return nil, fmt.Errorf("unknown scenario %q (valid: %s, all)", input, strings.Join(scenarios.ScenarioNames(), ", "))
}
