package runnerpool

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
)

// RunnerPool executes tasks concurrently with dependency awareness.
type RunnerPool struct {
	q           *dagQueue
	runner      TaskRunner
	concurrency int
	failFast    bool
}

func New(units []*runbase.Unit, r TaskRunner, maxConc int, failFast bool) *RunnerPool {
	if maxConc <= 0 {
		maxConc = 1
	}
	return &RunnerPool{
		q:           buildQueue(units, failFast),
		runner:      r,
		concurrency: maxConc,
		failFast:    failFast,
	}
}

// Run blocks until all runnable tasks finish and returns their results.
func (p *RunnerPool) Run(ctx context.Context) []Result {
	sem := make(chan struct{}, p.concurrency)
	done := make(chan *entry)

	// fan‑in goroutine collects completions
	go func() {
		for e := range done {
			p.q.markDone(e, e.result)
			if p.failFast && e.state == statusFailed {
				// fast‑fail: mark remaining as skipped
				for _, ent := range p.q.entries {
					if ent.state == statusPending || ent.state == statusBlocked || ent.state == statusReady {
						ent.state = statusFailFast
					}
				}
			}
		}
	}()

	for {
		if p.q.empty() {
			break
		}
		ready := p.q.getReady(cap(sem) - len(sem))
		for _, e := range ready {
			sem <- struct{}{}
			go func(ent *entry) {
				res := p.runner(ctx, ent.task)
				ent.result = res
				done <- ent
				<-sem
			}(e)
		}
		// allow context cancellation
		select {
		case <-ctx.Done():
			break
		default:
		}
	}

	close(done)
	return p.q.results()
}
