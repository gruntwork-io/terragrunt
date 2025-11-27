package runnerpool

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/options"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"

	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/puzpuzpuz/xsync/v3"
)

// UnitRunner defines a function type that executes a Unit within a given context and returns an error.
type UnitRunner func(ctx context.Context, u *component.Unit) error

// Controller orchestrates concurrent execution over a DAG.
type Controller struct {
	q           *queue.Queue
	runner      UnitRunner
	readyCh     chan struct{}
	unitsMap    map[string]*component.Unit
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
func NewController(q *queue.Queue, units []*component.Unit, opts ...ControllerOption) *Controller {
	dr := &Controller{
		q:           q,
		readyCh:     make(chan struct{}, 1), // buffered to avoid blocking
		concurrency: options.DefaultParallelism,
	}
	// Map to link runner Units and Queue Entries
	unitsMap := make(map[string]*component.Unit)

	for _, u := range units {
		if u != nil && u.Path() != "" {
			unitsMap[u.Path()] = u
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
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "runner_pool_controller", map[string]any{
		"total_tasks":             len(dr.q.Entries),
		"concurrency":             dr.concurrency,
		"fail_fast":               dr.q.FailFast,
		"ignore_dependency_order": dr.q.IgnoreDependencyOrder,
	}, func(childCtx context.Context) error {
		var (
			wg      sync.WaitGroup
			sem     = make(chan struct{}, dr.concurrency)
			results = xsync.NewMapOf[string, error]()
		)

		if dr.runner == nil {
			return errors.Errorf("Runner Pool Controller: runner is not set, cannot run")
		}

		l.Debugf("Runner Pool Controller: starting with %d tasks, concurrency %d",
			len(dr.q.Entries), dr.concurrency)

		// Initial signal to start scheduling
		select {
		case dr.readyCh <- struct{}{}:
		default:
		}

		for {
			readyEntries := dr.q.GetReadyWithDependencies()
			l.Debugf("Runner Pool Controller: found %d readyEntries tasks", len(readyEntries))

			for _, e := range readyEntries {
				// log debug which entry is running
				l.Debugf("Runner Pool Controller: running %s", e.Component.Path())
				dr.q.SetEntryStatus(e, queue.StatusRunning)

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

					unit := dr.unitsMap[ent.Component.Path()]
					if unit == nil {
						err := errors.Errorf("unit for path %s not found in discovered units", ent.Component.Path())
						l.Errorf("Runner Pool Controller: unit for path %s not found in discovered units, skipping execution", ent.Component.Path())
						dr.q.FailEntry(ent)
						results.Store(ent.Component.Path(), err)

						return
					}

					err := dr.runner(childCtx, unit)
					results.Store(ent.Component.Path(), err)

					if err != nil {
						l.Debugf("Runner Pool Controller: %s failed", ent.Component.Path())
						dr.q.FailEntry(ent)

						return
					}

					l.Debugf("Runner Pool Controller: %s succeeded", ent.Component.Path())
					dr.q.SetEntryStatus(ent, queue.StatusSucceeded)
				}(e)
			}

			if len(readyEntries) == 0 && len(sem) == 0 {
				break
			}

			select {
			case <-dr.readyCh:
			case <-childCtx.Done():
				wg.Wait()
				return nil
			}
		}

		wg.Wait()

		// Collect errors from results map and check for errors
		errCollector := &errors.MultiError{}

		for _, entry := range dr.q.Entries {
			if err, ok := results.Load(entry.Component.Path()); ok {
				if err == nil {
					continue
				}

				errCollector = errCollector.Append(err)

				continue
			}

			if entry.Status == queue.StatusEarlyExit {
				errCollector = errCollector.Append(errors.Errorf("unit %s did not run due to early exit", entry.Component.Path()))
			}

			if entry.Status == queue.StatusFailed {
				errCollector = errCollector.Append(errors.Errorf("unit %s failed to run", entry.Component.Path()))
			}
		}

		return errCollector.ErrorOrNil()
	})
}
