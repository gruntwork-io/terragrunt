package exec_test

import (
	"context"
	"io"
	"os"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signalAttempts is how many forwards a test here makes in a row. A forwarder that picks
// between two ready select cases leaks about half the time, so one pass proves nothing.
const signalAttempts = 32

// signalRecorder is both a [vexec.Exec] and the single [vexec.Cmd] it hands out. It
// records signals instead of delivering them, so a test can drive the forwarding logic
// without a subprocess whose own signal handling it would have to synchronize against.
type signalRecorder struct {
	sigs chan os.Signal
}

func newSignalRecorder() *signalRecorder {
	// Buffered for every signal a leaking forwarder could send, so a leak fails the test
	// with a count instead of deadlocking on a full channel.
	return &signalRecorder{sigs: make(chan os.Signal, signalAttempts)}
}

func (r *signalRecorder) Command(context.Context, string, ...string) vexec.Cmd { return r }
func (r *signalRecorder) LookPath(file string) (string, error)                 { return file, nil }

func (r *signalRecorder) Signal(sig os.Signal) error {
	r.sigs <- sig

	return nil
}

func (r *signalRecorder) SetStdin(io.Reader)         {}
func (r *signalRecorder) SetStdout(io.Writer)        {}
func (r *signalRecorder) SetStderr(io.Writer)        {}
func (r *signalRecorder) SetEnv([]string)            {}
func (r *signalRecorder) SetDir(string)              {}
func (r *signalRecorder) SetWaitDelay(time.Duration) {}
func (r *signalRecorder) SetCancel(func() error)     {}

func (r *signalRecorder) Run() error                      { return nil }
func (r *signalRecorder) Start() error                    { return nil }
func (r *signalRecorder) Wait() error                     { return nil }
func (r *signalRecorder) Output() ([]byte, error)         { return nil, nil }
func (r *signalRecorder) CombinedOutput() ([]byte, error) { return nil, nil }
func (r *signalRecorder) ProcessState() *os.ProcessState  { return nil }

// requireSignal returns the next signal the recorder saw, failing instead of
// hanging when nothing is forwarded.
func requireSignal(t *testing.T, sigs <-chan os.Signal) os.Signal {
	t.Helper()

	var sig os.Signal

	require.Eventually(t, func() bool {
		select {
		case sig = <-sigs:
			return true
		default:
			return false
		}
	}, 10*time.Second, 10*time.Millisecond, "nothing was forwarded to the command")

	return sig
}

// noopNotifier stands in for the OS in tests that never fire a repeat signal.
func noopNotifier(context.Context, signal.NotifyFunc, ...os.Signal) {}

// TestRegisterGracefullyShutdownForwardsCancelCauseSignalWithRacing pins the first half
// of the double-Ctrl+C path. When an OS signal is what cancelled the context, Terragrunt
// forwards that same signal rather than the command's own interrupt.
func TestRegisterGracefullyShutdownForwardsCancelCauseSignalWithRacing(t *testing.T) {
	t.Parallel()

	rec := newSignalRecorder()

	ctx, cancel := context.WithCancelCause(t.Context())

	cmd := exec.Command(ctx, venvtest.New().WithExec(rec), "tofu", "apply")

	stopShutdown := cmd.RegisterGracefullyShutdown(ctx, logger.CreateLogger())
	defer stopShutdown()

	cancel(signal.NewContextCanceledError(syscall.SIGTERM))

	assert.Equal(t, syscall.SIGTERM, requireSignal(t, rec.sigs))
}

// TestRegisterGracefullyShutdownInterruptsWithoutSignalCauseWithRacing pins the other
// branch. A failure elsewhere cancelled the context, so the command never saw a signal
// of its own and Terragrunt interrupts it at once. The hour-long delay would swallow the
// signal entirely if this went through [exec.Cmd.ForwardSignal].
func TestRegisterGracefullyShutdownInterruptsWithoutSignalCauseWithRacing(t *testing.T) {
	t.Parallel()

	rec := newSignalRecorder()

	ctx, cancel := context.WithCancel(t.Context())

	cmd := exec.Command(ctx, venvtest.New().WithExec(rec), "tofu", "apply")
	cmd.Configure(exec.WithForwardSignalDelay(time.Hour))

	stopShutdown := cmd.RegisterGracefullyShutdown(ctx, logger.CreateLogger())
	defer stopShutdown()

	cancel()

	assert.Equal(t, signal.InterruptSignal, requireSignal(t, rec.sigs))
}

// TestForwardSignalSkipsTheDelayOnRepeatWithRacing pins double Ctrl+C. The first signal
// starts the grace period and the second one ends it. The delay is an hour, so a
// synthetic clock that has not moved is what shows the wait was cut short.
func TestForwardSignalSkipsTheDelayOnRepeatWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rec := newSignalRecorder()
		notified := make(chan signal.NotifyFunc, 1)

		cmd := exec.Command(t.Context(), venvtest.New().WithExec(rec), "tofu", "apply")
		cmd.Configure(
			exec.WithForwardSignalDelay(time.Hour),
			exec.WithSignalNotifier(func(_ context.Context, notifyFn signal.NotifyFunc, _ ...os.Signal) {
				notified <- notifyFn
			}),
		)

		start := time.Now()

		go cmd.ForwardSignal(t.Context(), logger.CreateLogger(), os.Interrupt)

		(<-notified)(os.Interrupt)

		assert.Equal(t, os.Interrupt, <-rec.sigs)
		assert.Zero(t, time.Since(start))
	})
}

// TestForwardSignalWaitsOutTheDelayWithRacing pins the grace period itself. With no
// repeat signal, Terragrunt leaves the command alone until the whole delay has elapsed.
func TestForwardSignalWaitsOutTheDelayWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rec := newSignalRecorder()

		cmd := exec.Command(t.Context(), venvtest.New().WithExec(rec), "tofu", "apply")
		cmd.Configure(
			exec.WithForwardSignalDelay(time.Minute),
			exec.WithSignalNotifier(noopNotifier),
		)

		start := time.Now()

		go cmd.ForwardSignal(t.Context(), logger.CreateLogger(), os.Interrupt)

		assert.Equal(t, os.Interrupt, <-rec.sigs)
		assert.Equal(t, time.Minute, time.Since(start))
	})
}

// TestForwardSignalNeverSignalsAfterTheContextIsDoneWithRacing pins the ordering that
// Ctrl+C actually produces. The child dies on the signal the terminal already delivered,
// the caller cancels, and only then does the forwarder reach its wait. A signal sent here
// lands on a finished process, which Terragrunt logs as an error.
func TestForwardSignalNeverSignalsAfterTheContextIsDoneWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rec := newSignalRecorder()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		v := venvtest.New().WithExec(rec)

		for range signalAttempts {
			cmd := exec.Command(t.Context(), v, "tofu", "apply")
			cmd.Configure(
				exec.WithForwardSignalDelay(time.Minute),
				exec.WithSignalNotifier(noopNotifier),
			)

			cmd.ForwardSignal(ctx, logger.CreateLogger(), os.Interrupt)
		}

		assert.Empty(
			t,
			rec.sigs,
			"%d of %d forwards reached a command whose context was already done",
			len(rec.sigs),
			signalAttempts,
		)
	})
}

// TestForwardSignalStopsWhenTheCommandFinishesMidDelayWithRacing pins the case the
// grace period exists for. The command exits while the delay is still running and the
// caller cancels, leaving nothing to signal.
func TestForwardSignalStopsWhenTheCommandFinishesMidDelayWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rec := newSignalRecorder()

		ctx, cancel := context.WithCancel(t.Context())

		cmd := exec.Command(t.Context(), venvtest.New().WithExec(rec), "tofu", "apply")
		cmd.Configure(
			exec.WithForwardSignalDelay(time.Minute),
			exec.WithSignalNotifier(noopNotifier),
		)

		returned := make(chan struct{})

		go func() {
			cmd.ForwardSignal(ctx, logger.CreateLogger(), os.Interrupt)
			close(returned)
		}()

		synctest.Wait()
		cancel()
		<-returned

		assert.Empty(t, rec.sigs, "a signal was forwarded after the caller gave up on it")
	})
}
