package util

import (
	"context"
	"fmt"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// DoWithRetry runs the specified action. If it returns a value, return that value. If it returns an error, sleep for
// sleepBetweenRetries and try again, up to a maximum of maxRetries retries. If maxRetries is exceeded, return a
// MaxRetriesExceeded error.
func DoWithRetry(ctx context.Context, actionDescription string, maxRetries int, sleepBetweenRetries time.Duration, logger log.Logger, logLevel log.Level, action func(ctx context.Context) error) error {
	for i := 0; i <= maxRetries; i++ {
		logger.Logf(logLevel, actionDescription)

		err := action(ctx)
		if err == nil {
			return nil
		}

		var fatalErr FatalError
		if ok := errors.As(err, &fatalErr); ok {
			return err
		}

		if ctx.Err() != nil {
			logger.Debugf("%s returned an error: %s.", actionDescription, err.Error())

			return errors.New(ctx.Err())
		}

		logger.Errorf("%s returned an error: %s. Retry %d of %d. Sleeping for %s and will try again.", actionDescription, err.Error(), i, maxRetries, sleepBetweenRetries)

		select {
		case <-time.After(sleepBetweenRetries): // Try again
		case <-ctx.Done():
			return errors.New(ctx.Err())
		}
	}

	return MaxRetriesExceeded{Description: actionDescription, MaxRetries: maxRetries}
}

// MaxRetriesExceeded is an error that occurs when the maximum amount of retries is exceeded.
type MaxRetriesExceeded struct {
	Description string
	MaxRetries  int
}

func (err MaxRetriesExceeded) Error() string {
	return fmt.Sprintf("'%s' unsuccessful after %d retries", err.Description, err.MaxRetries)
}

// FatalError is error interface for cases that should not be retried.
type FatalError struct {
	Underlying error
}

func (err FatalError) Error() string {
	return err.Underlying.Error()
}
