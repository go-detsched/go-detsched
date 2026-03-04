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

func main() {
	rounds := flag.Int("rounds", 300, "protocol rounds")
	attempts := flag.Int("attempts", 5, "scheduler windows per side")
	noise := flag.Int("noise", 16, "noise goroutines")
	failThreshold := flag.Int("fail-threshold", 2, "minimum failures to classify as failing")
	flag.Parse()

	seed := parseSeed()
	receiverDelay := int(seed % 6)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < *noise; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					runtime.Gosched()
				}
			}
		}()
	}

	failCount := 0
	hash := uint64(0xcbf29ce484222325)
	for i := 0; i < *rounds; i++ {
		ok := brittleHandshake(*attempts, receiverDelay)
		hash ^= uint64(i + 1)
		if ok {
			hash ^= 0x9e3779b97f4a7c15
		} else {
			failCount++
			hash ^= 0x243f6a8885a308d3
		}
		hash *= 0x100000001b3
	}

	close(stop)
	wg.Wait()

	fmt.Printf("seed=%d fail=%d hash=%016x\n", seed, failCount, hash)
	if failCount >= *failThreshold {
		os.Exit(1)
	}
}

func brittleHandshake(attempts, receiverDelay int) bool {
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
			done <- true
		case <-abort:
			done <- false
		}
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

	return <-done && <-done
}

func parseSeed() uint64 {
	for _, part := range strings.Split(os.Getenv("GODEBUG"), ",") {
		if strings.HasPrefix(part, "detschedseed=") {
			v, err := strconv.ParseUint(strings.TrimPrefix(part, "detschedseed="), 10, 64)
			if err == nil {
				return v
			}
		}
	}
	return 0
}
