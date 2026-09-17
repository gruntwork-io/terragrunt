package cas_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
)

// errProbeBug stands in for a bug inside the shared probe.
var errProbeBug = errors.New("probe bug")

// TestGitResolver_ProbePanicReachesEveryCallerWithRacing pins that a panic
// inside the shared probe is raised again on each caller's own goroutine,
// where the command-level recover lives, instead of unwinding the flight's
// goroutine and aborting the process without a panic report. The value
// is an error so the assertion can match it by identity through the
// wrapper the flight adds.
func TestGitResolver_ProbePanicReachesEveryCallerWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &lsRemoteStub{hash: stubHash, gate: make(chan struct{}), panicValue: errProbeBug}
		v := stub.venv()
		url := "https://example.com/flight-panic.git"

		const callers = 4

		recovered := make([]any, callers)
		errs := make([]error, callers)

		var wg sync.WaitGroup

		for i := range callers {
			wg.Go(func() {
				defer func() { recovered[i] = recover() }()

				r := &cas.GitResolver{Venv: v, Branch: "main"}
				_, errs[i] = r.Probe(t.Context(), url)
			})
		}

		synctest.Wait()
		require.Equal(t, int32(1), stub.calls.Load(), "every caller must be waiting on one ls-remote")

		close(stub.gate)
		wg.Wait()

		for i := range callers {
			require.NoError(t, errs[i], "caller %d must not see the panic as a returned error", i)
			require.NotNil(t, recovered[i], "caller %d must see the panic on its own goroutine", i)

			err, ok := recovered[i].(error)
			require.True(t, ok, "caller %d: the panic must carry the original value as an error", i)
			assert.ErrorIs(t, err, errProbeBug, "caller %d", i)
		}
	})
}

// TestGitResolver_ProbeConcurrentCallersShareOneLsRemoteWithRacing pins the
// in-process probe flight: 32 resolvers built independently (the shape
// `run --all` produces, one CAS per unit) probing the same URL and ref at
// once cost one ls-remote between them. synctest parks every caller before
// the held ls-remote is released, so the coalescing is observed rather than
// assumed.
func TestGitResolver_ProbeConcurrentCallersShareOneLsRemoteWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &lsRemoteStub{hash: stubHash, gate: make(chan struct{})}
		v := stub.venv()

		const callers = 32

		url := "https://example.com/flight-share.git"
		hashes := make([]string, callers)
		errs := make([]error, callers)

		var wg sync.WaitGroup

		for i := range callers {
			wg.Go(func() {
				r := &cas.GitResolver{Venv: v, Branch: "main"}
				hashes[i], errs[i] = r.Probe(t.Context(), url)
			})
		}

		synctest.Wait()
		require.Equal(t, int32(1), stub.calls.Load(), "one ls-remote must be in flight for every caller")

		close(stub.gate)
		wg.Wait()

		for i := range callers {
			require.NoError(t, errs[i], "caller %d", i)
			assert.Equal(t, stubHash, hashes[i], "caller %d", i)
		}

		assert.Equal(t, int32(1), stub.calls.Load())
	})
}

// TestGitResolver_ProbeLeaderCancellationLeavesFollowersIntactWithRacing pins
// the context contract of the flight: the caller that started the ls-remote
// cancelling returns its own cancellation, while the flight carries on and
// still answers everyone who joined it with the single network call.
func TestGitResolver_ProbeLeaderCancellationLeavesFollowersIntactWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &lsRemoteStub{hash: stubHash, gate: make(chan struct{})}
		v := stub.venv()
		url := "https://example.com/flight-leader-cancel.git"

		leaderCtx, cancelLeader := context.WithCancel(t.Context())
		defer cancelLeader()

		var leaderErr error

		leaderDone := make(chan struct{})

		go func() {
			defer close(leaderDone)

			r := &cas.GitResolver{Venv: v, Branch: "main"}
			_, leaderErr = r.Probe(leaderCtx, url)
		}()

		synctest.Wait()
		require.Equal(t, int32(1), stub.calls.Load(), "the leader must be parked inside ls-remote")

		const followers = 4

		hashes := make([]string, followers)
		errs := make([]error, followers)

		var wg sync.WaitGroup

		for i := range followers {
			wg.Go(func() {
				r := &cas.GitResolver{Venv: v, Branch: "main"}
				hashes[i], errs[i] = r.Probe(t.Context(), url)
			})
		}

		synctest.Wait()

		cancelLeader()
		<-leaderDone
		require.ErrorIs(t, leaderErr, context.Canceled)

		synctest.Wait()
		require.Equal(t, int32(1), stub.calls.Load(), "the leader leaving must not restart the ls-remote")

		close(stub.gate)
		wg.Wait()

		for i := range followers {
			require.NoError(t, errs[i], "follower %d", i)
			assert.Equal(t, stubHash, hashes[i], "follower %d", i)
		}

		assert.Equal(t, int32(1), stub.calls.Load())
	})
}

// TestGitResolver_ProbeAbandonedFlightIsReplacedForLateCallerWithRacing
// pins the other half of the context contract: once every waiter has left,
// the flight is cancelled, and a caller arriving while that cancelled call
// is still unwinding must get a fresh ls-remote rather than the abandoned
// call's cancellation.
func TestGitResolver_ProbeAbandonedFlightIsReplacedForLateCallerWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &lsRemoteStub{hash: stubHash, gate: make(chan struct{}), ignoreCancel: true}
		v := stub.venv()
		url := "https://example.com/flight-abandoned.git"

		leaderCtx, cancelLeader := context.WithCancel(t.Context())
		defer cancelLeader()

		leaderDone := make(chan error, 1)

		go func() {
			_, err := (&cas.GitResolver{Venv: v, Branch: "main"}).Probe(leaderCtx, url)
			leaderDone <- err
		}()

		synctest.Wait()
		require.Equal(t, int32(1), stub.calls.Load())

		cancelLeader()
		require.ErrorIs(t, <-leaderDone, context.Canceled)

		// The abandoned call is still parked on the gate, ignoring its
		// cancellation, when the late caller arrives.
		synctest.Wait()

		lateDone := make(chan struct{})

		var (
			lateHash string
			lateErr  error
		)

		go func() {
			defer close(lateDone)

			lateHash, lateErr = (&cas.GitResolver{Venv: v, Branch: "main"}).Probe(t.Context(), url)
		}()

		synctest.Wait()
		require.Equal(t, int32(2), stub.calls.Load(), "the late caller must start its own ls-remote")

		close(stub.gate)
		<-lateDone

		require.NoError(t, lateErr)
		assert.Equal(t, stubHash, lateHash)
	})
}
