// Package retry re-runs operations that fail.
//
// It also holds the attempt count, interval, and error patterns Terragrunt
// retries tofu invocations with by default.
package retry

import (
	"context"
	"errors"
	"time"
)

// ErrNoAttempts is the panic value [Do] raises when given fewer than one
// attempt.
var ErrNoAttempts = errors.New("retry: Do requires at least one attempt")

// Do runs fn until it succeeds, until next declines another attempt, or until
// attempts is spent, and returns fn's last error.
//
// After a failed attempt, next is given the attempt number, counting from one,
// and the error it returned. It reports how long to wait and whether to go
// again. A context that ends during the wait ends the retry with its own
// error.
func Do(
	ctx context.Context,
	attempts int,
	fn func(ctx context.Context) error,
	next func(attempt int, err error) (time.Duration, bool),
) error {
	if attempts < 1 {
		panic(ErrNoAttempts)
	}

	var err error

	for attempt := range attempts {
		if err = fn(ctx); err == nil {
			return nil
		}

		if attempt == attempts-1 {
			break
		}

		delay, again := next(attempt+1, err)
		if !again {
			break
		}

		timer := time.NewTimer(delay)

		select {
		case <-ctx.Done():
			timer.Stop()

			return ctx.Err()
		case <-timer.C:
		}
	}

	return err
}
