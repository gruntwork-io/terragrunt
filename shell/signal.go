package shell

import (
	"fmt"
	"os"
	"os/signal"
)

// RegisterSignalHandler registers a handler of interrupt signal from the OS.
// When signal is receiving, it calls the given callback func `notifyFn`.
func RegisterSignalHandler(notifyFn func(os.Signal), sigs ...os.Signal) {
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, sigs...)

		sig := <-sigCh
		// Carriage return helps prevent "^C" from being printed
		fmt.Print("\r")
		notifyFn(sig)
	}()
}
