package ts

import (
	"iter"
	"time"
)

func Backoff(retries int, initial time.Duration, max time.Duration) iter.Seq2[int, time.Duration] {
	current := initial
	return func(yield func(int, time.Duration) bool) {
		for i := range retries {
			if !yield(i, current) { // first one is instant
				return
			}
			time.Sleep(current)
			current = min(current*2, max)
		}
	}
}
