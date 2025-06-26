package runnerpool

import (
	"context"
	"runtime"
	"sync"

	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
)

// RunnerPool orchestrates concurrent execution over a DAG.
type RunnerPool struct {
	q           *DagQueue
	runner      TaskRunner
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
	}
}

// Run blocks until the DAG finishes and returns ordered Results.
func (p *RunnerPool) Run(ctx context.Context, l log.Logger) []Result {
	var (
		wg  sync.WaitGroup
		sem = make(chan struct{}, p.concurrency)
	)

	l.Debugf("RunnerPool: starting with %d tasks, concurrency %d, failFast=%t", len(p.q.Ordered), p.concurrency, p.failFast)

	for {
		ready := p.q.GetReady()
		if len(ready) == 0 {
			if p.q.Empty() {
				l.Debugf("RunnerPool: queue is Empty, breaking loop")
				break
			}

			l.Tracef("RunnerPool: no ready tasks, yielding (queue not Empty)")
			runtime.Gosched()

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
			}(e)
		}
	}

	wg.Wait()

	return p.q.Results()
}
