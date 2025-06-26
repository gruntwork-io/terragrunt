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
	q           *dagQueue
	runner      TaskRunner
	concurrency int
	failFast    bool
}

// New creates a pool; if maxConc ≤0 uses GOMAXPROCS.
func New(units []*runbase.Unit, r TaskRunner, maxConc int, failFast bool) *RunnerPool {
	if maxConc <= 0 {
		maxConc = runtime.GOMAXPROCS(0)
	}
	return &RunnerPool{
		q:           buildQueue(units, failFast),
		runner:      r,
		concurrency: maxConc,
		failFast:    failFast,
	}
}

// Run blocks until the DAG finishes and returns ordered results.
func (p *RunnerPool) Run(ctx context.Context, l log.Logger) []Result {
	var (
		wg  sync.WaitGroup
		sem = make(chan struct{}, p.concurrency)
	)

	// debug print the queue state
	l.Debugf("RunnerPool: starting with %d tasks, concurrency %d, failFast=%t", len(p.q.ordered), p.concurrency, p.failFast)

	for {
		ready := p.q.getReady(cap(sem) - len(sem))
		if len(ready) == 0 {
			if p.q.empty() {
				break
			}
			runtime.Gosched()
			continue
		}
		for _, e := range ready {
			l.Debugf("Running task %s with %d remaining dependencies", e.task.ID(), e.remainingDeps)
			sem <- struct{}{}
			wg.Add(1)

			go func(ent *entry) {
				defer func() {
					<-sem
					wg.Done()
				}()
				ent.result = p.runner(ctx, ent.task)
				p.q.markDone(ent, p.failFast)
			}(e)
		}
	}
	wg.Wait()

	return p.q.results()
}
