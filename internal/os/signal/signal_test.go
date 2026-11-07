package signal_test

import (
	"context"
	"os"
	"testing"
	"testing/synctest"

	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	hangup os.Signal = testSignal("hangup")
	quit   os.Signal = testSignal("quit")
)

func TestNewNotifierDeliversUntilDone(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		src := &fakeSource{stopped: make(chan chan<- os.Signal, 1)}
		got := make(chan os.Signal, 1)

		ctx, cancel := context.WithCancel(t.Context())

		signal.NewNotifier(src.subscribe, src.unsubscribe)(ctx, func(sig os.Signal) { got <- sig }, hangup, quit)

		require.Equal(t, []os.Signal{hangup, quit}, src.tracked, "the notifier should subscribe to the requested signals")

		for _, sig := range []os.Signal{hangup, quit} {
			src.ch <- sig

			assert.Equal(t, sig, <-got, "each subscribed signal should reach the callback")
		}

		cancel()

		assert.Equal(t, src.ch, <-src.stopped, "the notifier should release its channel once ctx is done")
	})
}

type testSignal string

func (s testSignal) String() string { return string(s) }

func (testSignal) Signal() {}

type fakeSource struct {
	ch      chan<- os.Signal
	stopped chan chan<- os.Signal
	tracked []os.Signal
}

func (s *fakeSource) subscribe(ch chan<- os.Signal, sigs ...os.Signal) {
	s.ch = ch
	s.tracked = sigs
}

func (s *fakeSource) unsubscribe(ch chan<- os.Signal) {
	s.stopped <- ch
}
