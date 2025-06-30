package runnerpool

import (
	"context"
	"fmt"
	"sync"

	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"

	"github.com/puzpuzpuz/xsync/v3"
)

// UnitExecutor defines a function type that executes a Unit within a given context and returns an exit code and error.
type UnitExecutor func(ctx context.Context, u *common.Unit) (int, error)

// DAGRunner orchestrates concurrent execution over a DAG.
type DAGRunner struct {
	q           *queue.Queue
	runner      UnitExecutor
	readyCh     chan struct{}
	unitsMap    map[string]*common.Unit
	concurrency int
}

// DAGRunnerOption is a function that modifies a DAGRunner.
type DAGRunnerOption func(*DAGRunner)

// WithRunner sets the UnitExecutor for the DAGRunner.
func WithRunner(runner UnitExecutor) DAGRunnerOption {
	return func(dr *DAGRunner) {
		dr.runner = runner
	}
}

// WithMaxConcurrency sets the concurrency for the DAGRunner.
func WithMaxConcurrency(maxConc int) DAGRunnerOption {
	return func(dr *DAGRunner) {
		if maxConc <= 0 {
			maxConc = 1
		}

		dr.concurrency = maxConc
	}
}

// NewDAGRunner creates a new DAGRunner with the given options and a pre-built queue.
func NewDAGRunner(q *queue.Queue, units []*common.Unit, opts ...DAGRunnerOption) *DAGRunner {
	dr := &DAGRunner{
		q:       q,
		readyCh: make(chan struct{}, 1), // buffered to avoid blocking
	}
	// Build unitsMap from units slice
	unitsMap := make(map[string]*common.Unit)

	for _, u := range units {
		if u != nil && u.Path != "" {
			unitsMap[u.Path] = u
		}
	}

	dr.unitsMap = unitsMap
	for _, opt := range opts {
		opt(dr)
	}

	if dr.q == nil {
		// If the queue was not set, create an empty queue
		dr.q = &queue.Queue{Entries: []*queue.Entry{}}
	}

	return dr
}

// RunResult Define struct for results
type RunResult struct {
	Err      error
	ExitCode int
}

// Run executes the DAG and returns one result per queue entry
// (order preserved).  The function:
//
//   - never blocks forever – if progress is impossible it bails out;
//   - is data-race free (results map is mutex-protected);
//   - needs no special-casing for fail-fast.
func (dr *DAGRunner) Run(ctx context.Context, l log.Logger) []RunResult {
	var (
		wg      sync.WaitGroup
		sem     = make(chan struct{}, dr.concurrency)
		results = xsync.NewMapOf[string, RunResult]()
	)

	l.Debugf("DAGRunner: starting with %d tasks, concurrency %d",
		len(dr.q.Entries), dr.concurrency)

	// Initial signal to start scheduling
	select {
	case dr.readyCh <- struct{}{}:
	default:
	}

	for {
		ready := dr.q.GetReadyWithDependencies()
		l.Debugf("DAGRunner: found %d ready tasks", len(ready))

		for _, e := range ready {
			// log debug which entry is running
			l.Debugf("DAGRunner: running %s", e.Config.Path)
			dr.q.SetStatus(e, queue.StatusRunning)
			sem <- struct{}{}

			wg.Add(1)

			go func(ent *queue.Entry) {
				defer func() {
					<-sem
					wg.Done()
					select {
					case dr.readyCh <- struct{}{}:
					default:
					}
				}()

				unit := dr.unitsMap[ent.Config.Path]
				if unit == nil {
					err := fmt.Errorf("unit for path %s is nil", ent.Config.Path)
					l.Errorf("DAGRunner: %s unit is nil, skipping execution", ent.Config.Path)
					dr.q.SetStatus(ent, queue.StatusFailed)
					results.Store(ent.Config.Path, RunResult{ExitCode: 1, Err: err})

					return
				}

				exit, err := dr.runner(ctx, unit)
				results.Store(ent.Config.Path, RunResult{ExitCode: exit, Err: err})

				if err != nil {
					l.Debugf("DAGRunner: %s failed", ent.Config.Path)
					dr.q.SetStatus(ent, queue.StatusFailed)

					return
				}

				l.Debugf("DAGRunner: %s succeeded", ent.Config.Path)
				dr.q.SetStatus(ent, queue.StatusSucceeded)
			}(e)
		}

		if len(ready) == 0 {
			// If no goroutines are running, break
			if len(sem) == 0 {
				break
			}
		}

		select {
		case <-dr.readyCh:
		case <-ctx.Done():
			wg.Wait()
			return nil
		}
	}

	wg.Wait()

	// Preserve original order
	ordered := make([]RunResult, 0, len(dr.q.Entries))

	for _, e := range dr.q.Entries {
		if res, ok := results.Load(e.Config.Path); ok {
			ordered = append(ordered, res)
		}
	}

	return ordered
}
