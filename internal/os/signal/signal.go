// Package signal provides convenience methods for intercepting and handling OS signals.
package signal

import (
	"context"
	"os"
)

// NotifyFunc is a callback function for a [NotifierFunc].
type NotifyFunc func(sig os.Signal)

// NotifierFunc delivers each of trackSignals to notifyFn until ctx is done; code that waits on a signal takes one as a parameter, so a test can pass a fake.
type NotifierFunc func(ctx context.Context, notifyFn NotifyFunc, trackSignals ...os.Signal)

// NewNotifier returns a [NotifierFunc] that subscribes its channel with subscribe and closes it after unsubscribe once ctx is done; unsubscribe must stop all sends before returning, as os/signal.Stop does.
func NewNotifier(subscribe func(chan<- os.Signal, ...os.Signal), unsubscribe func(chan<- os.Signal)) NotifierFunc {
	return func(ctx context.Context, notifyFn NotifyFunc, trackSignals ...os.Signal) {
		sigCh := make(chan os.Signal, 1)

		subscribe(sigCh, trackSignals...)

		context.AfterFunc(ctx, func() {
			unsubscribe(sigCh)
			close(sigCh)
		})

		go func() {
			for sig := range sigCh {
				notifyFn(sig)
			}
		}()
	}
}
