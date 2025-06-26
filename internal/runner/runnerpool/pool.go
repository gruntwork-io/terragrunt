package runnerpool

import (
	"context"
	"sync"

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

// New creates a new RunnerPool with the given units, runner function.
func New(units []*runbase.Unit, r TaskRunner, maxConc int, failFast bool) *RunnerPool {
	return &RunnerPool{
		q:           BuildQueue(units, failFast),
		runner:      r,
		concurrency: maxConc,
		failFast:    failFast,
		readyCh:     make(chan struct{}, 1), // buffered to avoid blocking
	}
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
			sem <- struct{}{}

			wg.Add(1)

			go func(ent *Entry) {
				defer func() {
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
