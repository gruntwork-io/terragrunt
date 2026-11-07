// Package signal provides convenience methods for intercepting and handling OS signals.
package signal

import (
	"context"
	"os"
)

// NotifyFunc is a callback function for a [NotifierFunc].
type NotifyFunc func(sig os.Signal)

// NotifierFunc delivers each of trackSignals to notifyFn until ctx is done; code that waits on a signal takes one so its caller can drive the notifications instead of the process-global registry.
type NotifierFunc func(ctx context.Context, notifyFn NotifyFunc, trackSignals ...os.Signal)

// NewNotifier returns a [NotifierFunc] that registers its channel through subscribe and releases it through unsubscribe once ctx is done, with os/signal's Notify and Stop as the real pair.
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
