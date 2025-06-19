package runnerpool

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
)

// RunnerPool executes Tasks using a bounded worker pool governed by the queue.
type RunnerPool struct {
	q           *queue
	runner      TaskRunner
	concurrency int
}

// New constructs a RunnerPool.
func New(units []*runbase.Unit, r TaskRunner, maxConc int, failFast bool) *RunnerPool {
	if maxConc <= 0 {
		maxConc = 1
	}
	return &RunnerPool{
		q:           buildQueue(units, failFast),
		runner:      r,
		concurrency: maxConc,
	}
}

// Run blocks until all runnable tasks complete and returns their Results.
func (p *RunnerPool) Run(ctx context.Context) []Result {
	sem := make(chan struct{}, p.concurrency)
	done := make(chan Result)
	var wg sync.WaitGroup

	for {
		ready := p.q.getReady(cap(sem) - len(sem))
		for _, e := range ready {
			p.q.markRunning(e)
			sem <- struct{}{}
			wg.Add(1)
			go func(ent *entry) {
				defer wg.Done()
				res := p.runner(ctx, ent.task)
				done <- res
				p.q.done(ent, res)
				<-sem
			}(e)
		}

		// Drain completions quickly (non‑blocking).
	drain:
		for {
			select {
			case <-done:
			default:
				break drain
			}
		}

		if p.q.empty() {
			break
		}
	}

	wg.Wait()

	var results []Result
	for _, e := range p.q.entries {
		results = append(results, e.result)
	}
	return results
}
