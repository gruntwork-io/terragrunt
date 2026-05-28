package runnerpool

import (
	"context"
	"fmt"
	"sync"

	"github.com/gruntwork-io/terragrunt/pkg/options"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/multierror"

	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"

	"github.com/puzpuzpuz/xsync/v4"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// UnitRunner defines a function type that executes a Unit within a given context and returns an error.
type UnitRunner func(ctx context.Context, u *component.Unit) error

// Controller orchestrates concurrent execution over a DAG.
type Controller struct {
	q           *queue.Queue
	runner      UnitRunner
	readyCh     chan struct{}
	unitsMap    map[string]*component.Unit
	experiments experiment.Experiments
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

// WithExperiments sets the experiments for the Controller.
func WithExperiments(experiments experiment.Experiments) ControllerOption {
	return func(dr *Controller) {
		dr.experiments = experiments
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

// weightedSemaphore tracks in-flight weight against a budget (--parallelism).
// When all units have weight 1, it behaves identically to a channel-based semaphore.
type weightedSemaphore struct {
	releaseCh chan struct{}
	budget    int
	inflight  int
	mu        sync.Mutex
}

func newWeightedSemaphore(budget int) *weightedSemaphore {
	return &weightedSemaphore{
		budget:    budget,
		releaseCh: make(chan struct{}, 1),
	}
}

// tryAcquire attempts to reserve weight without blocking. Returns true if
// the unit was admitted. Handles two cases:
//   - Normal: inflight + weight <= budget
//   - Oversized: weight > budget but inflight == 0 (run solo to avoid deadlock)
func (ws *weightedSemaphore) tryAcquire(weight int) bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.inflight+weight <= ws.budget {
		ws.inflight += weight

		return true
	}

	// Oversized unit: weight exceeds budget, run solo when pool is empty
	if weight > ws.budget && ws.inflight == 0 {
		ws.inflight += weight

		return true
	}

	return false
}

// release returns weight to the budget and signals that capacity is available.
func (ws *weightedSemaphore) release(weight int) {
	ws.mu.Lock()
	ws.inflight -= weight
	ws.mu.Unlock()

	// Non-blocking signal so the scheduling loop re-evaluates deferred entries.
	select {
	case ws.releaseCh <- struct{}{}:
	default:
	}
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
			sem     = newWeightedSemaphore(dr.concurrency)
			results = xsync.NewMap[string, error]()
		)

		if dr.runner == nil {
			return errors.New("runner Pool Controller: runner is not set, cannot run")
		}

		l.Debugf("Runner Pool Controller: starting with %d tasks, concurrency %d",
			len(dr.q.Entries), dr.concurrency)

		// Initial signal to start scheduling
		select {
		case dr.readyCh <- struct{}{}:
		default:
		}

		for {
			readyEntries := dr.q.GetReadyWithDependencies(l)
			l.Debugf("Runner Pool Controller: found %d readyEntries tasks", len(readyEntries))

			admitted := 0

			for _, e := range readyEntries {
				if !dr.q.ClaimForRunning(e) {
					l.Debugf("Runner Pool Controller: skipping %s; fail-fast cancelled before dispatch", e.Component.Path())
					continue
				}

				// Look up unit weight for budget-based admission.
				// Only used when the run-weight experiment is enabled;
				// otherwise all units have weight 1 (preserving existing behavior).
				weight := 1

				if dr.experiments.Evaluate(experiment.RunWeight) {
					if unit, ok := dr.unitsMap[e.Component.Path()]; ok {
						weight = unit.RunWeight()
					}
				}

				if !sem.tryAcquire(weight) {
					// Doesn't fit in remaining budget. Unclaim so it returns to
					// Ready status and can be retried once capacity frees up.
					dr.q.SetEntryStatus(e, queue.StatusReady)
					l.Debugf("Runner Pool Controller: deferring %s (weight %d); insufficient budget", e.Component.Path(), weight)

					continue
				}

				admitted++

				l.Debugf("Runner Pool Controller: running %s (weight %d)", e.Component.Path(), weight)

				wg.Add(1)

				go func(ent *queue.Entry, w int) {
					defer func() {
						sem.release(w)
						wg.Done()

						select {
						case dr.readyCh <- struct{}{}:
						default:
						}
					}()

					unit := dr.unitsMap[ent.Component.Path()]
					if unit == nil {
						err := fmt.Errorf("unit for path %s not found in discovered units", ent.Component.Path())
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
				}(e, weight)
			}

			if dr.q.Finished() {
				break
			}

			// Wait for either: new entries becoming ready (dependency resolved),
			// budget being freed (a running unit finished), or context cancellation.
			select {
			case <-dr.readyCh:
			case <-sem.releaseCh:
			case <-childCtx.Done():
				wg.Wait()
				return nil
			}
		}

		wg.Wait()

		var errCollector []error

		var succeeded, failed, earlyExit int

		for _, entry := range dr.q.Entries {
			switch entry.Status {
			case queue.StatusSucceeded:
				succeeded++
			case queue.StatusFailed:
				failed++
			case queue.StatusEarlyExit:
				earlyExit++
			case queue.StatusPending, queue.StatusBlocked, queue.StatusUnsorted, queue.StatusReady, queue.StatusRunning:
				// Non-terminal states are not counted in the summary.
			}

			if err, ok := results.Load(entry.Component.Path()); ok {
				if err == nil {
					continue
				}

				errCollector = append(errCollector, err)

				continue
			}

			if entry.Status == queue.StatusEarlyExit {
				failedDep := findFailedDependency(entry, dr.q)
				errCollector = append(errCollector, NewUnitEarlyExitError(entry.Component.Path(), failedDep))
			}

			if entry.Status == queue.StatusFailed {
				errCollector = append(errCollector, NewUnitFailedError(entry.Component.Path()))
			}
		}

		if span := trace.SpanFromContext(childCtx); span.IsRecording() {
			span.SetAttributes(
				attribute.Int("tasks_succeeded", succeeded),
				attribute.Int("tasks_failed", failed),
				attribute.Int("tasks_early_exit", earlyExit),
			)
		}

		return multierror.Join(errCollector...)
	})
}
