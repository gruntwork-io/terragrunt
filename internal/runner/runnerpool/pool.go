package runnerpool

import (
	"context"
	"runtime"
	"sync"

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
func (p *RunnerPool) Run(ctx context.Context) []Result {
	var (
		wg   sync.WaitGroup
		sem  = make(chan struct{}, p.concurrency)
		done = make(chan *entry)
	)

	// collector goroutine
	go func() {
		for e := range done {
			p.q.markDone(e, p.failFast)
			<-sem
			wg.Done()
		}
	}()

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
			sem <- struct{}{}
			wg.Add(1)

			go func(ent *entry) {
				ent.result = p.runner(ctx, ent.task)
				done <- ent
			}(e)
		}
	}
	wg.Wait()
	close(done)

	return p.q.results()
}
