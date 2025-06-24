package runnerpool

import (
	"context"
	"runtime"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
)

// RunnerPool drives a bounded worker pool that executes tasks respecting
// dependency ordering held in a dagQueue.
type RunnerPool struct {
	q           *dagQueue
	runner      TaskRunner
	concurrency int
	failFast    bool
}

// New constructs a RunnerPool. If maxConc <= 0 it defaults to runtime.NumCPU().
func New(units []*runbase.Unit, r TaskRunner, maxConc int, failFast bool) *RunnerPool {
	if maxConc <= 0 {
		maxConc = runtime.NumCPU()
	}
	return &RunnerPool{
		q:           buildQueue(units, failFast),
		runner:      r,
		concurrency: maxConc,
		failFast:    failFast,
	}
}

// Run executes all tasks and returns a slice of Result once the DAG is fully
// processed. It blocks until every runnable task has finished.
func (p *RunnerPool) Run(ctx context.Context) []Result {
	sem := make(chan struct{}, p.concurrency) // acts as worker slots
	wg := sync.WaitGroup{}

	for {
		// Drain ctx cancelation early.
		select {
		case <-ctx.Done():
			wg.Wait()
			return p.q.results()
		default:
		}

		// fetch ready tasks respecting remaining slots
		ready := p.q.getReady(p.concurrency - len(sem))
		if len(ready) == 0 {
			// no ready entries; if queue empty we're done, else wait for a task to finish
			if p.q.empty() {
				break
			}
			// tiny sleep or continue after wg.Wait?
			select {
			case <-ctx.Done():
				wg.Wait()
				return p.q.results()
			default:
			}
			continue
		}

		for _, e := range ready {
			sem <- struct{}{}
			wg.Add(1)
			go func(ent *entry) {
				defer func() { <-sem; wg.Done() }()
				res := p.runner(ctx, ent.task)
				p.q.markDone(ent, res)
			}(e)
		}
	}

	// wait for running goroutines
	wg.Wait()
	return p.q.results()
}
