package util

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// DoWithRetry runs the specified action. If it returns a value, return that value. If it returns an error, sleep for
// sleepBetweenRetries and try again, up to a maximum of maxRetries retries. If maxRetries is exceeded, return a
// MaxRetriesExceeded error.
func DoWithRetry(actionDescription string, maxRetries int, sleepBetweenRetries time.Duration, logger *logrus.Entry, logLevel logrus.Level, action func() error) error {
	for i := 0; i <= maxRetries; i++ {
		logger.Logf(logLevel, actionDescription)

		err := action()
		if err == nil {
			return nil
		}

		if _, isFatalErr := err.(FatalError); isFatalErr {
			return err
		}

		logger.Errorf("%s returned an error: %s. Sleeping for %s and will try again.", actionDescription, err.Error(), sleepBetweenRetries)
		time.Sleep(sleepBetweenRetries)
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
