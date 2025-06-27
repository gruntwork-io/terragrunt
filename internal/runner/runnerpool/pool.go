package runnerpool

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// TaskRunner defines a function type that executes a Unit within a given context and returns an exit code and error.
type TaskRunner func(ctx context.Context, u *common.Unit) (int, error)

// RunnerPool orchestrates concurrent execution over a DAG.
type RunnerPool struct {
	q           *queue.Queue
	runner      TaskRunner
	readyCh     chan struct{}
	concurrency int
	failFast    bool
	// unitsMap maps unit paths to their corresponding *runbase.Unit for efficient lookup during task execution.
	unitsMap map[string]*common.Unit
}

// RunnerPoolOption is a function that modifies a RunnerPool.
type RunnerPoolOption func(*RunnerPool)

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

// NewRunnerPool creates a new RunnerPool with the given options and a pre-built queue.
func NewRunnerPool(q *queue.Queue, units []*common.Unit, opts ...RunnerPoolOption) *RunnerPool {
	rp := &RunnerPool{
		q:        q,
		failFast: false,
		readyCh:  make(chan struct{}, 1), // buffered to avoid blocking
	}
	// Build unitsMap from units slice
	unitsMap := make(map[string]*common.Unit)
	for _, u := range units {
		if u != nil && u.Path != "" {
			unitsMap[u.Path] = u
		}
	}
	rp.unitsMap = unitsMap
	for _, opt := range opts {
		opt(rp)
	}
	if rp.q == nil {
		// If queue was not set, create an empty queue
		rp.q = &queue.Queue{Entries: []*queue.Entry{}}
	}
	return rp
}

// Define RunResult struct for results
type RunResult struct {
	ExitCode int
	Err      error
}

// Run blocks until the DAG finishes and returns ordered Results.
func (p *RunnerPool) Run(ctx context.Context, l log.Logger) []RunResult {
	var (
		wg      sync.WaitGroup
		sem     = make(chan struct{}, p.concurrency)
		results = make(map[string]RunResult)
	)

	l.Debugf("RunnerPool: starting with %d tasks, concurrency %d, failFast=%t", len(p.q.Entries), p.concurrency, p.failFast)

	signalReady := func() {
		select {
		case p.readyCh <- struct{}{}:
		default:
		}
	}

	signalReady() // initial signal in case there are ready tasks at the start

	for {
		ready := p.q.GetReadyWithDependencies()
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
				return nil
			}
			continue
		}
		l.Debugf("RunnerPool: found %d ready tasks", len(ready))
		for _, e := range ready {
			// Set status to running explicitly
			e.Status = queue.StatusRunning
			sem <- struct{}{}
			wg.Add(1)
			go func(ent *queue.Entry) {
				defer func() {
					if r := recover(); r != nil {
						results[ent.Config.Path] = RunResult{ExitCode: 1, Err: errors.Errorf("panic: %v", r)}
						p.q.SetStatus(ent, queue.StatusFailed)
						signalReady()
					}
					<-sem
					wg.Done()
				}()
				unit := p.unitsMap[ent.Config.Path]
				exitCode, err := p.runner(ctx, unit)
				results[ent.Config.Path] = RunResult{ExitCode: exitCode, Err: err}
				if err == nil {
					p.q.SetStatus(ent, queue.StatusSucceeded)
				} else {
					p.q.SetStatus(ent, queue.StatusFailed)
				}
				signalReady() // notify that new tasks may be ready
			}(e)
		}
	}
	wg.Wait()

	// After all goroutines finish, set failed result for any entry that does not have a result (e.g., due to fail-fast)
	for _, e := range p.q.Entries {
		if _, ok := results[e.Config.Path]; !ok {
			// Mark as failed/skipped
			results[e.Config.Path] = RunResult{ExitCode: 1, Err: errors.New("skipped or failed due to fail-fast or ancestor failure")}
		}
	}

	// Collect results in order of queue entries
	ordered := make([]RunResult, 0, len(p.q.Entries))
	for _, e := range p.q.Entries {
		if res, ok := results[e.Config.Path]; ok {
			ordered = append(ordered, res)
		}
	}
	return ordered
}
