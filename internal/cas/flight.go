package cas

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
)

// flights coalesces the remote probe and the tree ingest of every CAS
// instance in the process. It is package state rather than a CAS field
// because each call site constructs a CAS per unit, so a `run --all` over
// units sharing one source builds one CAS each; a group hanging off the
// instance would never see a second caller. Keys carry the store path,
// since what a flight produces (a probe-cache entry, a stored tree) lives
// in one store and instances over different stores must not share it.
var flights struct {
	probe  flightGroup[probeResult]
	ingest flightGroup[string]
}

// flightGroup coalesces concurrent calls that share a key into one
// execution, the way golang.org/x/sync/singleflight does, with one change
// to the context contract. singleflight runs the shared call under the
// first caller's context, so that caller giving up (its unit finished or
// its context expired) fails every waiter with the leader's cancellation.
// Here the call runs under a context detached from any single caller and
// canceled only once the last waiter has left, while each waiter still
// returns as soon as its own context ends.
type flightGroup[T any] struct {
	flights map[string]*flight[T]
	mu      sync.Mutex
}

// flight is one in-progress shared call.
type flight[T any] struct {
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	err      error
	panic    *flightPanic
	val      T
	waiters  int
	exited   bool
	finished bool
}

// flightPanic carries a panic out of the goroutine that ran the shared
// call and into every waiter. The stack is captured where the panic
// happened, since re-raising it on a waiter's goroutine would otherwise
// point at the flight machinery instead of the bug.
type flightPanic struct {
	value any
	stack []byte
}

func (p *flightPanic) Error() string {
	return fmt.Sprintf("%v\n\n%s", p.value, p.stack)
}

// Unwrap returns the panic value when it is itself an error, and nil
// otherwise.
func (p *flightPanic) Unwrap() error {
	err, ok := p.value.(error)
	if !ok {
		return nil
	}

	return err
}

// do runs fn once for key across concurrent callers and returns its
// result to each of them. A caller whose ctx ends before the call
// completes receives ctx.Err(); the call keeps running for the others.
// A panic in fn is re-raised on every caller still waiting, and fn
// calling runtime.Goexit exits every waiter the same way, so the shared
// call cannot escape the recover a caller's goroutine already has. A
// panic with no caller left to raise it crashes the process from the
// flight's own goroutine, the way singleflight does, rather than
// vanishing.
//
// fn receives the flight's context, which carries the values of the
// first caller's ctx (telemetry, logger) but not its cancellation.
func (g *flightGroup[T]) do(
	ctx context.Context,
	key string,
	fn func(context.Context) (T, error),
) (T, error) {
	f := g.join(ctx, key, fn)

	select {
	case <-f.done:
		f.raise()

		return f.val, f.err
	case <-ctx.Done():
		if g.leave(f) {
			f.raise()
		}

		var zero T

		return zero, ctx.Err()
	}
}

// join registers the caller on the flight for key, starting one when
// none is in progress.
func (g *flightGroup[T]) join(
	ctx context.Context,
	key string,
	fn func(context.Context) (T, error),
) *flight[T] {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.flights == nil {
		g.flights = make(map[string]*flight[T])
	}

	// A flight whose context is already done was abandoned by its last
	// waiter and is winding down with a cancellation nobody here asked
	// for, so it is replaced rather than joined.
	f, ok := g.flights[key]
	if !ok || f.ctx.Err() != nil {
		flightCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))

		f = &flight[T]{ctx: flightCtx, cancel: cancel, done: make(chan struct{})}
		g.flights[key] = f

		go g.run(key, f, fn)
	}

	f.waiters++

	return f
}

// run executes fn for f and publishes the result. The map entry goes
// before done closes so a caller arriving after completion starts a fresh
// flight instead of joining a finished one; an entry that already points
// at a replacement flight is left alone.
//
// fn returning normally, panicking, and calling runtime.Goexit are told
// apart the way singleflight does: recover reports the panic, and a
// deferred function that runs with neither a normal return nor a
// recovered value observed is unwinding from Goexit.
func (g *flightGroup[T]) run(key string, f *flight[T], fn func(context.Context) (T, error)) {
	returned := false
	recovered := false

	defer func() {
		if !returned && !recovered {
			f.exited = true
		}

		g.mu.Lock()

		if g.flights[key] == f {
			delete(g.flights, key)
		}

		// Waiters that receive through done never leave, so a zero count
		// here means every caller has already left and none will raise;
		// a caller leaving after this point raises instead (see leave).
		f.finished = true
		orphaned := f.waiters == 0

		g.mu.Unlock()

		f.cancel()
		close(f.done)

		if orphaned && f.panic != nil {
			panic(f.panic)
		}
	}()

	func() {
		defer func() {
			if returned {
				return
			}

			if r := recover(); r != nil {
				f.panic = &flightPanic{value: r, stack: debug.Stack()}
			}
		}()

		f.val, f.err = fn(f.ctx)
		returned = true
	}()

	if !returned {
		recovered = true
	}
}

// raise replays on the calling goroutine whatever ended fn abnormally.
func (f *flight[T]) raise() {
	if f.panic != nil {
		panic(f.panic)
	}

	if f.exited {
		runtime.Goexit()
	}
}

// leave drops a waiter from f and cancels the call once nobody is left
// to receive its result. It reports whether the leaving waiter must
// raise what ended the call: the call finished under the same lock the
// count is kept under, so exactly one of run and the last waiter sees
// the flight both finished and empty.
func (g *flightGroup[T]) leave(f *flight[T]) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	f.waiters--

	if f.waiters > 0 {
		return false
	}

	f.cancel()

	return f.finished
}
