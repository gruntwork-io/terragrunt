package runnerpool

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/errors"

	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"

	"github.com/puzpuzpuz/xsync/v3"
)

// UnitRunner defines a function type that executes a Unit within a given context and returns an error.
type UnitRunner func(ctx context.Context, u *common.Unit) error

// Controller orchestrates concurrent execution over a DAG.
type Controller struct {
	q           *queue.Queue
	runner      UnitRunner
	readyCh     chan struct{}
	unitsMap    map[string]*common.Unit
	concurrency int
}

// ControllerOption is a function that modifies a Controller.
type ControllerOption func(*Controller)

// WithRunner sets the UnitRunner for the Controller.
func WithRunner(runner UnitRunner) ControllerOption {
	return func(dr *Controller) {
		dr.runner = runner
	}
}

// WithMaxConcurrency sets the concurrency for the Controller.
func WithMaxConcurrency(concurrency int) ControllerOption {
	return func(dr *Controller) {
		if concurrency <= 0 {
			concurrency = 1
		}

		dr.concurrency = concurrency
	}
}

// NewController creates a new Controller with the given options and a pre-built queue.
func NewController(q *queue.Queue, units []*common.Unit, opts ...ControllerOption) *Controller {
	dr := &Controller{
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

// Run executes the Queue return error summarizing all entries that failed to run.
func (dr *Controller) Run(ctx context.Context, l log.Logger) error {
	var (
		wg      sync.WaitGroup
		sem     = make(chan struct{}, dr.concurrency)
		results = xsync.NewMapOf[string, error]()
	)

	l.Debugf("Controller: starting with %d tasks, concurrency %d",
		len(dr.q.Entries), dr.concurrency)

	// Initial signal to start scheduling
	select {
	case dr.readyCh <- struct{}{}:
	default:
	}

	for {
		ready := dr.q.GetReadyWithDependencies()
		l.Debugf("Controller: found %d ready tasks", len(ready))

		for _, e := range ready {
			// log debug which entry is running
			l.Debugf("Controller: running %s", e.Config.Path)
			e.Status = queue.StatusRunning
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
					err := errors.Errorf("unit for path %s is nil", ent.Config.Path)
					l.Errorf("Controller: %s unit is nil, skipping execution", ent.Config.Path)
					dr.q.FailEntry(ent)
					results.Store(ent.Config.Path, err)

					return
				}

				err := dr.runner(ctx, unit)
				results.Store(ent.Config.Path, err)

				if err != nil {
					l.Debugf("Controller: %s failed", ent.Config.Path)
					dr.q.FailEntry(ent)

					return
				}

				l.Debugf("Controller: %s succeeded", ent.Config.Path)
				ent.Status = queue.StatusSucceeded
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

	// Collect errors from results map and check for errors
	errCollector := &errors.MultiError{}

	for _, entry := range dr.q.Entries {
		if err, ok := results.Load(entry.Config.Path); ok {
			if err == nil {
				continue
			}

			errCollector = errCollector.Append(err)

			continue
		}

		if entry.Status == queue.StatusEarlyExit {
			errCollector = errCollector.Append(errors.Errorf("unit %s did not run due to early exit", entry.Config.Path))
		}

		if entry.Status == queue.StatusFailed {
			errCollector = errCollector.Append(errors.Errorf("unit %s failed to run", entry.Config.Path))
		}
	}

	return errCollector.ErrorOrNil()
}
