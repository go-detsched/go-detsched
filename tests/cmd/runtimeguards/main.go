package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	rttrace "runtime/trace"
	"strings"
)

func main() {
	procs := runtime.GOMAXPROCS(0)
	traceErr := rttrace.Start(io.Discard)
	if traceErr == nil {
		rttrace.Stop()
		fmt.Fprintln(os.Stderr, "expected trace start to fail in detsched mode")
		os.Exit(1)
	}
	if !strings.Contains(traceErr.Error(), "disabled in deterministic scheduler mode") {
		fmt.Fprintf(os.Stderr, "unexpected trace error: %v\n", traceErr)
		os.Exit(1)
	}

	fmt.Printf("gomaxprocs=%d trace_guard=ok\n", procs)
}
