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

func TestNewNotifierSubscribesRequestedSignalsWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		src := newFakeSource()

		signal.NewNotifier(src.subscribe, src.unsubscribe)(t.Context(), func(os.Signal) {}, hangup, quit)

		require.Len(t, src.channels, 1, "the notifier should subscribe one channel")
		assert.Equal(t, [][]os.Signal{{hangup, quit}}, src.tracked, "the notifier should subscribe the requested signals")
	})
}

func TestNewNotifierDeliversEachSignalWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		src := newFakeSource()
		got := make(chan os.Signal, 1)

		signal.NewNotifier(src.subscribe, src.unsubscribe)(t.Context(), func(sig os.Signal) { got <- sig }, hangup, quit)

		require.Len(t, src.channels, 1)

		for _, sig := range []os.Signal{hangup, quit, hangup} {
			src.channels[0] <- sig

			assert.Equal(t, sig, <-got, "each signal should reach the callback")
		}
	})
}

func TestNewNotifierKeepsSubscriptionWhileContextIsLiveWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		src := newFakeSource()

		signal.NewNotifier(src.subscribe, src.unsubscribe)(t.Context(), func(os.Signal) {}, hangup)

		synctest.Wait()

		assert.Empty(t, src.stopped, "the channel should stay subscribed until ctx is done")
	})
}

func TestNewNotifierUnsubscribesWhenDoneWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		src := newFakeSource()

		ctx, cancel := context.WithCancel(t.Context())

		signal.NewNotifier(src.subscribe, src.unsubscribe)(ctx, func(os.Signal) {}, hangup)

		require.Len(t, src.channels, 1)

		cancel()

		assert.Equal(t, src.channels[0], <-src.stopped, "the notifier should unsubscribe its own channel")
	})
}

func TestNewNotifierUnsubscribesAtOnceForDoneContextWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		src := newFakeSource()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		signal.NewNotifier(src.subscribe, src.unsubscribe)(ctx, func(os.Signal) {}, hangup)

		require.Len(t, src.channels, 1)
		assert.Equal(t, src.channels[0], <-src.stopped, "a done ctx should unsubscribe the channel right away")
	})
}

func TestNewNotifierDeliversSignalSentBeforeUnsubscribeWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		src := newFakeSource()
		got := make(chan os.Signal, 1)

		ctx, cancel := context.WithCancel(t.Context())

		signal.NewNotifier(src.subscribe, src.unsubscribe)(ctx, func(sig os.Signal) { got <- sig }, hangup)

		require.Len(t, src.channels, 1)

		src.channels[0] <- hangup

		cancel()

		assert.Equal(t, hangup, <-got, "a signal sent before unsubscribe should still reach the callback")
		assert.Equal(t, src.channels[0], <-src.stopped, "the channel should then be unsubscribed")
	})
}

func TestNewNotifierCallsAreIndependentWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		src := newFakeSource()
		notify := signal.NewNotifier(src.subscribe, src.unsubscribe)
		got := make(chan os.Signal, 1)

		ctx, cancel := context.WithCancel(t.Context())

		notify(ctx, func(os.Signal) {}, hangup)
		notify(t.Context(), func(sig os.Signal) { got <- sig }, quit)

		require.Len(t, src.channels, 2)
		require.NotEqual(t, src.channels[0], src.channels[1], "each call should subscribe its own channel")

		cancel()

		assert.Equal(t, src.channels[0], <-src.stopped, "only the done call should be unsubscribed")

		src.channels[1] <- quit

		assert.Equal(t, quit, <-got, "the live call should keep delivering")
	})
}

type testSignal string

func (s testSignal) String() string { return string(s) }

func (testSignal) Signal() {}

type fakeSource struct {
	stopped  chan chan<- os.Signal
	channels []chan<- os.Signal
	tracked  [][]os.Signal
}

func newFakeSource() *fakeSource {
	return &fakeSource{stopped: make(chan chan<- os.Signal, 4)}
}

func (s *fakeSource) subscribe(ch chan<- os.Signal, sigs ...os.Signal) {
	s.channels = append(s.channels, ch)
	s.tracked = append(s.tracked, sigs)
}

func (s *fakeSource) unsubscribe(ch chan<- os.Signal) {
	s.stopped <- ch
}
