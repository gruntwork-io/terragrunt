package runnerpool

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/errors"

	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
)

// RunnerPool orchestrates concurrent execution over a DAG.
type RunnerPool struct {
	q           *DagQueue
	runner      TaskRunner
	readyCh     chan struct{}
	concurrency int
	failFast    bool
}

// RunnerPoolOption is a function that modifies a RunnerPool.
type RunnerPoolOption func(*RunnerPool)

// WithUnits sets the units for the RunnerPool.
func WithUnits(units []*runbase.Unit) RunnerPoolOption {
	return func(rp *RunnerPool) {
		rp.q = BuildQueue(units, rp.failFast)
	}
}

// WithRunner sets the TaskRunner for the RunnerPool.
func WithRunner(runner TaskRunner) RunnerPoolOption {
	return func(rp *RunnerPool) {
		rp.runner = runner
	}
}

// WithMaxConcurrency sets the concurrency for the RunnerPool.
func WithMaxConcurrency(maxConc int) RunnerPoolOption {
	return func(rp *RunnerPool) {
		if maxConc <= 0 {
			maxConc = 1
		}
		rp.concurrency = maxConc
	}
}

// WithFailFast sets the failFast flag for the RunnerPool.
func WithFailFast(failFast bool) RunnerPoolOption {
	return func(rp *RunnerPool) {
		rp.failFast = failFast
	}
}

// NewRunnerPool creates a new RunnerPool with the given options.
func NewRunnerPool(opts ...RunnerPoolOption) *RunnerPool {
	rp := &RunnerPool{
		concurrency: 1, // default
		failFast:    false,
		readyCh:     make(chan struct{}, 1), // buffered to avoid blocking
	}
	for _, opt := range opts {
		opt(rp)
	}
	if rp.q == nil {
		// If units were not set, create an empty queue
		rp.q = BuildQueue(nil, rp.failFast)
	}
	return rp
}

// Run blocks until the DAG finishes and returns ordered Results.
func (p *RunnerPool) Run(ctx context.Context, l log.Logger) []Result {
	var (
		wg  sync.WaitGroup
		sem = make(chan struct{}, p.concurrency)
	)

	l.Debugf("RunnerPool: starting with %d tasks, concurrency %d, failFast=%t", len(p.q.Ordered), p.concurrency, p.failFast)

	signalReady := func() {
		select {
		case p.readyCh <- struct{}{}:
		default:
		}
	}

	signalReady() // initial signal in case there are ready tasks at the start

	for {
		ready := p.q.GetReady()
		if len(ready) == 0 {
			if p.q.Empty() {
				l.Debugf("RunnerPool: queue is Empty, breaking loop")
				break
			}

			l.Tracef("RunnerPool: no ready tasks, waiting (queue not Empty)")
			select {
			case <-p.readyCh:
			case <-ctx.Done():
				l.Debugf("RunnerPool: context cancelled, breaking loop")
				return p.q.Results()
			}

			continue
		}

		l.Debugf("RunnerPool: found %d ready tasks", len(ready))

		for _, e := range ready {
			l.Debugf("Running Task %s with %d remaining dependencies", e.Task.ID(), e.RemainingDeps)
			p.q.MarkRunning(e)
			sem <- struct{}{}

			wg.Add(1)

			go func(ent *Entry) {
				defer func() {
					if r := recover(); r != nil {
						l.Errorf("Panic in task %s: %v", ent.Task.ID(), r)
						// Mark the task as failed due to panic
						ent.Result = Result{
							TaskID:   ent.Task.ID(),
							ExitCode: 1,
							Err:      errors.Errorf("panic: %v", r),
						}
						p.q.MarkDone(ent, p.failFast)
						signalReady()
					}

					<-sem
					wg.Done()
				}()

				ent.Result = p.runner(ctx, ent.Task)
				p.q.MarkDone(ent, p.failFast)
				signalReady() // notify that new tasks may be ready
			}(e)
		}
	}

	wg.Wait()

	return p.q.Results()
}
