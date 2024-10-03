// Package signal provides convenience methods for intercepting and handling OS signals.
package signal

import (
	"context"
	"os"
	"os/signal"
)

// NotifyFunc is a callback function for Notifier.
type NotifyFunc func(sig os.Signal)

// Notifier registers a handler for receiving signals from the OS.
// When signal is receiving, it calls the given callback func `notifyFn`.
func Notifier(notifyFn NotifyFunc, trackSignals ...os.Signal) {
	NotifierWithContext(context.Background(), notifyFn, trackSignals...)
}

// NotifierWithContext does the same as `Notifier`, but if the given `ctx` becomes `Done`, the notification is stopped.
func NotifierWithContext(ctx context.Context, notifyFn NotifyFunc, trackSignals ...os.Signal) {
	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, trackSignals...)

	go func() {
		for {
			select {
			case <-ctx.Done():
				signal.Stop(sigCh)
				close(sigCh)

				return

			case sig, ok := <-sigCh:
				if !ok {
					return
				}

				notifyFn(sig)
			}
		}
	}()
}
