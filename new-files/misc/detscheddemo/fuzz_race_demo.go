package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

func brittleHandshake(attempts int, receiverDelay int) bool {
	req := make(chan struct{})
	ack := make(chan struct{})
	abort := make(chan struct{})
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < receiverDelay; i++ {
			runtime.Gosched()
		}

		ok := false
		select {
		case <-req:
			ok = true
		case <-abort:
		}
		if !ok {
			done <- false
			return
		}

		select {
		case ack <- struct{}{}:
			ok = true
		case <-abort:
			ok = false
		}
		done <- ok
	}()

	go func() {
		sent := false
		for i := 0; i < attempts; i++ {
			select {
			case req <- struct{}{}:
				sent = true
				i = attempts
			default:
				runtime.Gosched()
			}
		}
		if !sent {
			close(abort)
			done <- false
			return
		}

		ok := false
		for i := 0; i < attempts; i++ {
			select {
			case <-ack:
				ok = true
				i = attempts
			default:
				runtime.Gosched()
			}
		}

		close(abort)
		done <- ok
	}()

	s1 := <-done
	s2 := <-done
	return s1 && s2
}

func parseDetschedSeed() uint64 {
	godebug := os.Getenv("GODEBUG")
	for _, part := range strings.Split(godebug, ",") {
		if strings.HasPrefix(part, "detschedseed=") {
			v, err := strconv.ParseUint(strings.TrimPrefix(part, "detschedseed="), 10, 64)
			if err == nil {
				return v
			}
		}
	}
	return 0
}

func main() {
	rounds := flag.Int("rounds", 500, "number of protocol rounds")
	attempts := flag.Int("attempts", 6, "scheduler windows per side")
	noise := flag.Int("noise", 12, "number of scheduler noise goroutines")
	failThreshold := flag.Int("fail-threshold", 3, "minimum failed rounds to classify this seed as failing")
	flag.Parse()

	seed := parseDetschedSeed()
	receiverDelay := int(seed % 6)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < *noise; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				runtime.Gosched()
			}
		}()
	}

	const (
		hashInit = uint64(0xcbf29ce484222325)
		hashMul  = uint64(0x100000001b3)
	)

	hash := hashInit
	okCount := 0
	failCount := 0
	for i := 0; i < *rounds; i++ {
		ok := brittleHandshake(*attempts, receiverDelay)
		hash ^= uint64(i + 1)
		if ok {
			okCount++
			hash ^= 0x9e3779b97f4a7c15
		} else {
			failCount++
			hash ^= 0x243f6a8885a308d3
		}
		hash *= hashMul
	}

	close(stop)
	wg.Wait()

	fmt.Printf("rounds=%d ok=%d fail=%d threshold=%d hash=%016x\n", *rounds, okCount, failCount, *failThreshold, hash)
	if failCount >= *failThreshold {
		fmt.Println("status=FAIL brittle protocol exceeded failure threshold")
		os.Exit(1)
	}
	fmt.Println("status=PASS")
}
