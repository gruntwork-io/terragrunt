package configstack

import (
	"context"
	"sort"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/telemetry"
)

const (
	channelSize = 1000 // Use a huge buffer to ensure senders are never blocked
)

const (
	NormalOrder DependencyOrder = iota
	ReverseOrder
	IgnoreOrder
)

// DependencyOrder controls in what order dependencies should be enforced between units.
type DependencyOrder int

// DependencyController manages dependencies and dependency order, and contains a UnitRunner.
type DependencyController struct {
	Runner         *runbase.UnitRunner
	DependencyDone chan *DependencyController
	Dependencies   map[string]*DependencyController
	NotifyWhenDone []*DependencyController
}

// NewDependencyController Create a new dependency controller for the given unit.
func NewDependencyController(unit *runbase.Unit) *DependencyController {
	return &DependencyController{
		Runner:         runbase.NewUnitRunner(unit),
		DependencyDone: make(chan *DependencyController, channelSize),
		Dependencies:   map[string]*DependencyController{},
		NotifyWhenDone: []*DependencyController{},
	}
}

// runUnitWhenReady a unit once all of its dependencies have finished executing.
func (ctrl *DependencyController) runUnitWhenReady(ctx context.Context, opts *options.TerragruntOptions, r *report.Report, semaphore chan struct{}) {
	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "wait_for_unit_ready", map[string]any{
		"path":             ctrl.Runner.Unit.Path,
		"terraformCommand": ctrl.Runner.Unit.TerragruntOptions.TerraformCommand,
	}, func(_ context.Context) error {
		return ctrl.waitForDependencies(opts, r)
	})

	semaphore <- struct{}{} // Add one to the buffered channel. Will block if parallelism limit is met
	defer func() {
		<-semaphore // Remove one from the buffered channel
	}()

	if err == nil {
		err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "run_unit", map[string]any{
			"path":             ctrl.Runner.Unit.Path,
			"terraformCommand": ctrl.Runner.Unit.TerragruntOptions.TerraformCommand,
		}, func(ctx context.Context) error {
			return ctrl.Runner.Run(ctx, opts, r)
		})
	}

	ctrl.unitFinished(err, r, opts.Experiments.Evaluate(experiment.Report))
}

// waitForDependencies for all of this unit's dependencies to finish executing. Return an error if any of those dependencies complete
// with an error. Return immediately if this unit has no dependencies.
func (ctrl *DependencyController) waitForDependencies(opts *options.TerragruntOptions, r *report.Report) error {
	ctrl.Runner.Unit.Logger.Debugf("Unit %s must wait for %d dependencies to finish", ctrl.Runner.Unit.Path, len(ctrl.Dependencies))

	handleDependencyError := func(doneDependency *DependencyController) error {
		if ctrl.Runner.Unit.TerragruntOptions.IgnoreDependencyErrors {
			ctrl.Runner.Unit.Logger.Errorf("Dependency %s of unit %s just finished with an error. Normally, unit %s would exit early, however, because --queue-ignore-errors has been set, unit %s will run anyway.", doneDependency.Runner.Unit.Path, ctrl.Runner.Unit.Path, ctrl.Runner.Unit.Path, ctrl.Runner.Unit.Path)
			return nil
		}

		ctrl.Runner.Unit.Logger.Errorf("Dependency %s of unit %s just finished with an error. Unit %s will have to return an error too.", doneDependency.Runner.Unit.Path, ctrl.Runner.Unit.Path, ctrl.Runner.Unit.Path)

		if opts.Experiments.Evaluate(experiment.Report) {
			run, err := r.EnsureRun(ctrl.Runner.Unit.Path)

			if err != nil {
				ctrl.Runner.Unit.Logger.Errorf("Error ensuring run for unit %s: %v", ctrl.Runner.Unit.Path, err)
				return err
			}

			if err := r.EndRun(
				run.Path,
				report.WithResult(report.ResultEarlyExit),
				report.WithReason(report.ReasonAncestorError),
				report.WithCauseAncestorExit(doneDependency.Runner.Unit.Path),
			); err != nil {
				ctrl.Runner.Unit.Logger.Errorf("Error ending run for unit %s: %v", ctrl.Runner.Unit.Path, err)
			}
		}

		return runbase.ProcessingUnitDependencyError{Unit: ctrl.Runner.Unit, Dependency: doneDependency.Runner.Unit, Err: doneDependency.Runner.Err}
	}

	for len(ctrl.Dependencies) > 0 {
		doneDependency := <-ctrl.DependencyDone
		delete(ctrl.Dependencies, doneDependency.Runner.Unit.Path)

		if doneDependency.Runner.Err != nil {
			if err := handleDependencyError(doneDependency); err != nil {
				return err
			}
		} else {
			ctrl.Runner.Unit.Logger.Debugf("Dependency %s of unit %s just finished successfully. Unit %s must wait on %d more dependencies.", doneDependency.Runner.Unit.Path, ctrl.Runner.Unit.Path, ctrl.Runner.Unit.Path, len(ctrl.Dependencies))
		}
	}

	return nil
}

// unitFinished Record that a unit has finished executing and notify all of this unit's dependencies
func (ctrl *DependencyController) unitFinished(unitErr error, r *report.Report, reportExperiment bool) {
	if unitErr == nil {
		ctrl.Runner.Unit.Logger.Debugf("Unit %s has finished successfully!", ctrl.Runner.Unit.Path)

		if reportExperiment {
			if err := r.EndRun(ctrl.Runner.Unit.Path); err != nil {
				// If the run is not found in the report, it likely means this module was an external dependency
				// that was excluded from the queue (e.g., with --queue-exclude-external).
				if !errors.Is(err, report.ErrRunNotFound) {
					ctrl.Runner.Unit.Logger.Errorf("Error ending run for unit %s: %v", ctrl.Runner.Unit.Path, err)

					return
				}

				if ctrl.Runner.Unit.AssumeAlreadyApplied {
					run, err := report.NewRun(ctrl.Runner.Unit.Path)
					if err != nil {
						ctrl.Runner.Unit.Logger.Errorf("Error creating run for unit %s: %v", ctrl.Runner.Unit.Path, err)
						return
					}

					if err := r.AddRun(run); err != nil {
						ctrl.Runner.Unit.Logger.Errorf("Error adding run for unit %s: %v", ctrl.Runner.Unit.Path, err)
						return
					}

					if err := r.EndRun(
						run.Path,
						report.WithResult(report.ResultExcluded),
						report.WithReason(report.ReasonExcludeExternal),
					); err != nil {
						ctrl.Runner.Unit.Logger.Errorf("Error ending run for unit %s: %v", ctrl.Runner.Unit.Path, err)
					}
				}
			}
		}
	} else {
		ctrl.Runner.Unit.Logger.Errorf("Unit %s has finished with an error", ctrl.Runner.Unit.Path)

		if reportExperiment {
			if err := r.EndRun(
				ctrl.Runner.Unit.Path,
				report.WithResult(report.ResultFailed),
				report.WithReason(report.ReasonRunError),
				report.WithCauseRunError(unitErr.Error()),
			); err != nil {
				// If we can't find the run, then it never started,
				// So we should start it and then end it as a failed run.
				//
				// Early exit runs should already be ended at this point.
				if errors.Is(err, report.ErrRunNotFound) {
					run, err := report.NewRun(ctrl.Runner.Unit.Path)
					if err != nil {
						ctrl.Runner.Unit.Logger.Errorf("Error creating run for unit %s: %v", ctrl.Runner.Unit.Path, err)
						return
					}

					if err := r.AddRun(run); err != nil {
						ctrl.Runner.Unit.Logger.Errorf("Error adding run for unit %s: %v", ctrl.Runner.Unit.Path, err)
						return
					}

					if err := r.EndRun(
						run.Path,
						report.WithResult(report.ResultFailed),
						report.WithReason(report.ReasonRunError),
						report.WithCauseRunError(unitErr.Error()),
					); err != nil {
						ctrl.Runner.Unit.Logger.Errorf("Error ending run for unit %s: %v", ctrl.Runner.Unit.Path, err)
					}
				} else {
					ctrl.Runner.Unit.Logger.Errorf("Error ending run for unit %s: %v", ctrl.Runner.Unit.Path, err)
				}
			}
		}
	}

	ctrl.Runner.Status = runbase.Finished
	ctrl.Runner.Err = unitErr

	for _, toNotify := range ctrl.NotifyWhenDone {
		toNotify.DependencyDone <- ctrl
	}
}

// RunningUnits is a map of unit path to DependencyController, representing the units that are currently running or
type RunningUnits map[string]*DependencyController

// categorizeUnitsForIteration categorizes units into those to deploy, those to remove, and those to defer.
func categorizeUnitsForIteration(units RunningUnits) (currentIterationDeploy runbase.Units, removeDep []string, next RunningUnits) {
	currentIterationDeploy = runbase.Units{}
	next = RunningUnits{}
	removeDep = []string{}

	for path, unit := range units {
		switch {
		case unit.Runner.Unit.AssumeAlreadyApplied:
			removeDep = append(removeDep, path)
		case len(unit.Dependencies) == 0:
			currentIterationDeploy = append(currentIterationDeploy, unit.Runner.Unit)
			removeDep = append(removeDep, path)
		default:
			next[path] = unit
		}
	}

	return
}

// toTerraformUnitGroups organizes the RunningUnits into groups of units that can be executed in parallel based on their dependencies.
// Each group in the returned slice contains units that have no remaining dependencies and can be run concurrently in that iteration.
// The function iteratively removes units with no dependencies, updates the dependency graph, and continues until all units are grouped or maxDepth is reached.
func (units RunningUnits) toTerraformUnitGroups(maxDepth int) []runbase.Units {
	// Walk the graph in run order, capturing which groups will run at each iteration. In each iteration, this pops out
	// the units that have no dependencies and captures that as a run group.
	groups := []runbase.Units{}

	for len(units) > 0 && len(groups) < maxDepth {
		currentIterationDeploy, removeDep, next := categorizeUnitsForIteration(units)

		// Go through the remaining units and remove the dependencies that were selected to run in this current
		// iteration.
		for _, unit := range next {
			for _, path := range removeDep {
				_, hasDep := unit.Dependencies[path]
				if hasDep {
					delete(unit.Dependencies, path)
				}
			}
		}

		// Sort the group by path so that it is easier to read and test.
		sort.Slice(
			currentIterationDeploy,
			func(i, j int) bool {
				return currentIterationDeploy[i].Path < currentIterationDeploy[j].Path
			},
		)

		// Finally, update the trackers so that the next iteration runs.
		units = next

		if len(currentIterationDeploy) > 0 {
			groups = append(groups, currentIterationDeploy)
		}
	}

	return groups
}

// crossLinkDependencies Loop through the map of runningUnits and for each unit U:
//
//   - If dependencyOrder is NormalOrder, plug in all the units U depends on into the Dependencies field and all the
//     units that depend on U into the NotifyWhenDone field.
//   - If dependencyOrder is ReverseOrder, do the reverse.
//   - If dependencyOrder is IgnoreOrder, do nothing.
func (units RunningUnits) crossLinkDependencies(dependencyOrder DependencyOrder) (RunningUnits, error) {
	for _, unit := range units {
		for _, dependency := range unit.Runner.Unit.Dependencies {
			runningDependency, hasDependency := units[dependency.Path]
			if !hasDependency {
				return units, errors.New(runbase.DependencyNotFoundWhileCrossLinkingError{Unit: unit.Runner.Unit, Dependency: dependency})
			}

			// TODO: Remove lint suppression
			switch dependencyOrder { //nolint:exhaustive
			case NormalOrder:
				unit.Dependencies[runningDependency.Runner.Unit.Path] = runningDependency
				runningDependency.NotifyWhenDone = append(runningDependency.NotifyWhenDone, unit)
			case IgnoreOrder:
				// Nothing
			default:
				runningDependency.Dependencies[unit.Runner.Unit.Path] = unit
				unit.NotifyWhenDone = append(unit.NotifyWhenDone, runningDependency)
			}
		}
	}

	return units, nil
}

// RemoveFlagExcluded returns a cleaned-up map that only contains units and
// dependencies that should not be excluded
func (units RunningUnits) RemoveFlagExcluded(r *report.Report, reportExperiment bool) (RunningUnits, error) {
	var finalUnits = make(map[string]*DependencyController)

	var errs []error

	for key, unit := range units {
		// Only add units that should not be excluded
		if !unit.Runner.Unit.FlagExcluded {
			finalUnits[key] = &DependencyController{
				Runner:         unit.Runner,
				DependencyDone: unit.DependencyDone,
				Dependencies:   make(map[string]*DependencyController),
				NotifyWhenDone: unit.NotifyWhenDone,
			}

			// Only add dependencies that should not be excluded
			for path, dependency := range unit.Dependencies {
				if !dependency.Runner.Unit.FlagExcluded {
					finalUnits[key].Dependencies[path] = dependency
				}
			}
		} else if reportExperiment {
			run, err := r.EnsureRun(unit.Runner.Unit.Path)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if err := r.EndRun(
				run.Path,
				report.WithResult(report.ResultExcluded),
				report.WithReason(report.ReasonExcludeBlock),
			); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return finalUnits, errors.Join(errs...)
	}

	return finalUnits, nil
}

// runUnits Run the given map of unit path to runningUnit. To "run" a unit, execute the runTerragrunt command in its
// TerragruntOptions object. The units will be executed in an order determined by their inter-dependencies, using
// as much concurrency as possible.
func (units RunningUnits) runUnits(ctx context.Context, opts *options.TerragruntOptions, r *report.Report, parallelism int) error {
	var (
		waitGroup sync.WaitGroup
		semaphore = make(chan struct{}, parallelism) // Make a semaphore from a buffered channel
	)

	for _, unit := range units {
		waitGroup.Add(1)

		go func(unit *DependencyController) {
			defer waitGroup.Done()

			unit.runUnitWhenReady(ctx, opts, r, semaphore)
		}(unit)
	}

	waitGroup.Wait()

	return units.collectErrors()
}

// collectErrors Collect the errors from the given units and return a single error object to represent them, or nil if no errors
// occurred
func (units RunningUnits) collectErrors() error {
	var errs *errors.MultiError

	for _, unit := range units {
		if unit.Runner.Err != nil {
			errs = errs.Append(unit.Runner.Err)
		}
	}

	return errs.ErrorOrNil()
}
