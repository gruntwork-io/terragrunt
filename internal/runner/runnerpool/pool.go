package runnerpool

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"

	"github.com/puzpuzpuz/xsync/v3"
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

// RunResult Define RunResult struct for results
type RunResult struct {
	ExitCode int
	Err      error
}

// Run executes the DAG and returns one result per queue entry
// (order preserved).  The function:
//
//   - never blocks forever – if progress is impossible it bails out;
//   - is data-race free (results map is mutex-protected);
//   - needs no special-casing for fail-fast.
func (p *RunnerPool) Run(ctx context.Context, l log.Logger) []RunResult {
	var (
		wg      sync.WaitGroup
		sem     = make(chan struct{}, p.concurrency)
		results = xsync.NewMapOf[string, RunResult]()
	)

	l.Debugf("RunnerPool: starting with %d tasks, concurrency %d, failFast=%t",
		len(p.q.Entries), p.concurrency, p.failFast)

	// Initial signal to start scheduling
	select {
	case p.readyCh <- struct{}{}:
	default:
	}

	for {
		ready := p.q.GetReadyWithDependencies()

		for _, e := range ready {
			p.q.SetStatus(e, queue.StatusRunning)
			sem <- struct{}{}
			wg.Add(1)

			go func(ent *queue.Entry) {
				defer func() {
					<-sem
					wg.Done()
					select {
					case p.readyCh <- struct{}{}:
					default:
					}
				}()
				exit, err := p.runner(ctx, p.unitsMap[ent.Config.Path])
				if err == nil {
					p.q.SetStatus(ent, queue.StatusSucceeded)
				} else {
					p.q.SetStatus(ent, queue.StatusFailed)
				}
				results.Store(ent.Config.Path, RunResult{ExitCode: exit, Err: err})
			}(e)
		}

		if len(ready) == 0 {
			// If no goroutines are running, break
			if len(sem) == 0 {
				break
			}
		}

		select {
		case <-p.readyCh:
		case <-ctx.Done():
			wg.Wait()
			return nil
		}
	}

	wg.Wait()

	// Preserve original order
	ordered := make([]RunResult, 0, len(p.q.Entries))
	for _, e := range p.q.Entries {
		if res, ok := results.Load(e.Config.Path); ok {
			ordered = append(ordered, res)
		}
	}
	return ordered
}
