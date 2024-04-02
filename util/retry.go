package util

import (
	"context"
	"fmt"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/sirupsen/logrus"
)

// DoWithRetry runs the specified action. If it returns a value, return that value. If it returns an error, sleep for
// sleepBetweenRetries and try again, up to a maximum of maxRetries retries. If maxRetries is exceeded, return a
// MaxRetriesExceeded error.
func DoWithRetry(ctx context.Context, actionDescription string, maxRetries int, sleepBetweenRetries time.Duration, logLevel logrus.Level, action func() error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	for i := 0; i <= maxRetries; i++ {
		log.Logf(logLevel, actionDescription)

		err := action()
		if err == nil {
			return nil
		}

		if _, isFatalErr := err.(FatalError); isFatalErr {
			return err
		}

		log.Errorf("%s returned an error: %s. Retry %d of %d. Sleeping for %s and will try again.", actionDescription, err.Error(), i, maxRetries, sleepBetweenRetries)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleepBetweenRetries):
			// try again
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
