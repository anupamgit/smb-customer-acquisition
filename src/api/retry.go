package main

import (
	"time"
)

// Retry bounds decided in Step 4: at most 3 attempts per external call
// (1 initial attempt + 2 retries), retries only for transient failures,
// backoff 1 second then 2 seconds. The orchestrator's overall 90-second
// run deadline bounds the total time; this helper never retries past the
// caller's context deadline on its own (see runDeadline in orchestrator.go).
var retryDelays = []time.Duration{time.Second, 2 * time.Second}

// doWithRetry runs fn until it succeeds or the retry budget is spent.
// Non-transient errors return immediately without retrying. fn receives
// the 1-based attempt number.
func doWithRetry(delays []time.Duration, fn func(attempt int) error) error {
	var err error
	for attempt := 1; ; attempt++ {
		err = fn(attempt)
		if err == nil {
			return nil
		}
		if !isTransient(err) {
			return err
		}
		if attempt-1 >= len(delays) {
			return err
		}
		time.Sleep(delays[attempt-1])
	}
}
