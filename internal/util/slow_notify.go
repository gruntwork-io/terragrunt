package util

import (
	"context"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// NotifyIfSlow runs fn and logs msg at Info level if fn takes longer than timeout.
// The notification fires at most once. The goroutine is cleaned up when fn returns or ctx is cancelled.
func NotifyIfSlow(ctx context.Context, l log.Logger, timeout time.Duration, msg string, fn func() error) error {
	done := make(chan struct{})

	go func() {
		select {
		case <-time.After(timeout):
			l.Info(msg)
		case <-done:
		case <-ctx.Done():
		}
	}()

	err := fn()

	close(done)

	return err
}

// NotifyIfSlowV is the generic variant of NotifyIfSlow that returns a value alongside the error.
func NotifyIfSlowV[T any](ctx context.Context, l log.Logger, timeout time.Duration, msg string, fn func() (T, error)) (T, error) {
	done := make(chan struct{})

	go func() {
		select {
		case <-time.After(timeout):
			l.Info(msg)
		case <-done:
		case <-ctx.Done():
		}
	}()

	val, err := fn()

	close(done)

	return val, err
}
