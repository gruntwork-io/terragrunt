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

// DependencyOrder controls in what order dependencies should be enforced between modules.
type DependencyOrder int

// DependencyController manages dependencies and dependency order, and contains a UnitRunner.
type DependencyController struct {
	Runner         *runbase.UnitRunner
	DependencyDone chan *DependencyController
	Dependencies   map[string]*DependencyController
	NotifyWhenDone []*DependencyController
}

// Create a new NewDependencyController struct for the given module.
func NewDependencyController(module *runbase.Unit) *DependencyController {
	return &DependencyController{
		Runner:         runbase.NewUnitRunner(module),
		DependencyDone: make(chan *DependencyController, channelSize),
		Dependencies:   map[string]*DependencyController{},
		NotifyWhenDone: []*DependencyController{},
	}
}

// Run a module once all of its dependencies have finished executing.
func (ctrl *DependencyController) runModuleWhenReady(ctx context.Context, opts *options.TerragruntOptions, r *report.Report, semaphore chan struct{}) {
	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "wait_for_module_ready", map[string]any{
		"path":             ctrl.Runner.Module.Path,
		"terraformCommand": ctrl.Runner.Module.TerragruntOptions.TerraformCommand,
	}, func(_ context.Context) error {
		return ctrl.waitForDependencies(opts, r)
	})

	semaphore <- struct{}{} // Add one to the buffered channel. Will block if parallelism limit is met
	defer func() {
		<-semaphore // Remove one from the buffered channel
	}()

	if err == nil {
		err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "run_module", map[string]any{
			"path":             ctrl.Runner.Module.Path,
			"terraformCommand": ctrl.Runner.Module.TerragruntOptions.TerraformCommand,
		}, func(ctx context.Context) error {
			return ctrl.Runner.Run(ctx, opts, r)
		})
	}

	ctrl.moduleFinished(err, r, opts.Experiments.Evaluate(experiment.Report))
}

// Wait for all of this module's dependencies to finish executing. Return an error if any of those dependencies complete
// with an error. Return immediately if this module has no dependencies.
func (ctrl *DependencyController) waitForDependencies(opts *options.TerragruntOptions, r *report.Report) error {
	ctrl.Runner.Logger.Debugf("Module %s must wait for %d dependencies to finish", ctrl.Runner.Module.Path, len(ctrl.Dependencies))

	for len(ctrl.Dependencies) > 0 {
		doneDependency := <-ctrl.DependencyDone
		delete(ctrl.Dependencies, doneDependency.Runner.Module.Path)

		if doneDependency.Runner.Err != nil {
			if ctrl.Runner.Module.TerragruntOptions.IgnoreDependencyErrors {
				ctrl.Runner.Logger.Errorf("Dependency %s of module %s just finished with an error. Module %s will have to return an error too. However, because of --queue-ignore-errors, module %s will run anyway.", doneDependency.Runner.Module.Path, ctrl.Runner.Module.Path, ctrl.Runner.Module.Path, ctrl.Runner.Module.Path)
			} else {
				ctrl.Runner.Logger.Errorf("Dependency %s of module %s just finished with an error. Module %s will have to return an error too.", doneDependency.Runner.Module.Path, ctrl.Runner.Module.Path, ctrl.Runner.Module.Path)

				if opts.Experiments.Evaluate(experiment.Report) {
					run, err := r.GetRun(ctrl.Runner.Module.Path)
					if err != nil {
						if errors.Is(err, report.ErrRunNotFound) {
							run, err = report.NewRun(ctrl.Runner.Module.Path)
							if err != nil {
								ctrl.Runner.Logger.Errorf("Error creating run for unit %s: %v", ctrl.Runner.Module.Path, err)
								return err
							}

							if err := r.AddRun(run); err != nil {
								ctrl.Runner.Logger.Errorf("Error adding run for unit %s: %v", ctrl.Runner.Module.Path, err)
								return err
							}
						} else {
							ctrl.Runner.Logger.Errorf("Error getting run for unit %s: %v", ctrl.Runner.Module.Path, err)
							return err
						}
					}

					if err := r.EndRun(
						run.Path,
						report.WithResult(report.ResultEarlyExit),
						report.WithReason(report.ReasonAncestorError),
						report.WithCauseAncestorExit(doneDependency.Runner.Module.Path),
					); err != nil {
						ctrl.Runner.Logger.Errorf("Error ending run for unit %s: %v", ctrl.Runner.Module.Path, err)
					}
				}

				return runbase.ProcessingModuleDependencyError{Module: ctrl.Runner.Module, Dependency: doneDependency.Runner.Module, Err: doneDependency.Runner.Err}
			}
		} else {
			ctrl.Runner.Logger.Debugf("Dependency %s of module %s just finished successfully. Module %s must wait on %d more dependencies.", doneDependency.Runner.Module.Path, ctrl.Runner.Module.Path, ctrl.Runner.Module.Path, len(ctrl.Dependencies))
		}
	}

	return nil
}

// Record that a module has finished executing and notify all of this module's dependencies
func (ctrl *DependencyController) moduleFinished(moduleErr error, r *report.Report, reportExperiment bool) {
	if moduleErr == nil {
		ctrl.Runner.Logger.Debugf("Module %s has finished successfully!", ctrl.Runner.Module.Path)

		if reportExperiment {
			if err := r.EndRun(ctrl.Runner.Module.Path); err != nil {
				ctrl.Runner.Logger.Errorf("Error ending run for module %s: %v", ctrl.Runner.Module.Path, err)
			}
		}
	} else {
		ctrl.Runner.Logger.Errorf("Module %s has finished with an error", ctrl.Runner.Module.Path)

		if reportExperiment {
			if err := r.EndRun(
				ctrl.Runner.Module.Path,
				report.WithResult(report.ResultFailed),
				report.WithReason(report.ReasonRunError),
				report.WithCauseRunError(moduleErr.Error()),
			); err != nil {
				// If we can't find the run, then it never started,
				// So we should start it and then end it as a failed run.
				//
				// Early exit runs should already be ended at this point.
				if errors.Is(err, report.ErrRunNotFound) {
					run, err := report.NewRun(ctrl.Runner.Module.Path)
					if err != nil {
						ctrl.Runner.Logger.Errorf("Error creating run for unit %s: %v", ctrl.Runner.Module.Path, err)
						return
					}

					if err := r.AddRun(run); err != nil {
						ctrl.Runner.Logger.Errorf("Error adding run for unit %s: %v", ctrl.Runner.Module.Path, err)
						return
					}

					if err := r.EndRun(
						run.Path,
						report.WithResult(report.ResultFailed),
						report.WithReason(report.ReasonRunError),
						report.WithCauseRunError(moduleErr.Error()),
					); err != nil {
						ctrl.Runner.Logger.Errorf("Error ending run for unit %s: %v", ctrl.Runner.Module.Path, err)
					}
				} else {
					ctrl.Runner.Logger.Errorf("Error ending run for unit %s: %v", ctrl.Runner.Module.Path, err)
				}
			}
		}
	}

	ctrl.Runner.Status = runbase.Finished
	ctrl.Runner.Err = moduleErr

	for _, toNotify := range ctrl.NotifyWhenDone {
		toNotify.DependencyDone <- ctrl
	}
}

type RunningModules map[string]*DependencyController

func (modules RunningModules) toTerraformModuleGroups(maxDepth int) []runbase.Units {
	// Walk the graph in run order, capturing which groups will run at each iteration. In each iteration, this pops out
	// the modules that have no dependencies and captures that as a run group.
	groups := []runbase.Units{}

	for len(modules) > 0 && len(groups) < maxDepth {
		currentIterationDeploy := runbase.Units{}

		// next tracks which modules are being deferred to a later run.
		next := RunningModules{}
		// removeDep tracks which modules are run in the current iteration so that they need to be removed in the
		// dependency list for the next iteration. This is separately tracked from currentIterationDeploy for
		// convenience: this tracks the map key of the Dependencies attribute.
		var removeDep []string

		// Iterate the modules, looking for those that have no dependencies and select them for "running". In the
		// process, track those that still need to run in a separate map for further processing.
		for path, module := range modules {
			// Anything that is already applied is culled from the graph when running, so we ignore them here as well.
			switch {
			case module.Runner.Module.AssumeAlreadyApplied:
				removeDep = append(removeDep, path)
			case len(module.Dependencies) == 0:
				currentIterationDeploy = append(currentIterationDeploy, module.Runner.Module)
				removeDep = append(removeDep, path)
			default:
				next[path] = module
			}
		}

		// Go through the remaining module and remove the dependencies that were selected to run in this current
		// iteration.
		for _, module := range next {
			for _, path := range removeDep {
				_, hasDep := module.Dependencies[path]
				if hasDep {
					delete(module.Dependencies, path)
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
		modules = next

		if len(currentIterationDeploy) > 0 {
			groups = append(groups, currentIterationDeploy)
		}
	}

	return groups
}

// Loop through the map of runningModules and for each module M:
//
//   - If dependencyOrder is NormalOrder, plug in all the modules M depends on into the Dependencies field and all the
//     modules that depend on M into the NotifyWhenDone field.
//   - If dependencyOrder is ReverseOrder, do the reverse.
//   - If dependencyOrder is IgnoreOrder, do nothing.
func (modules RunningModules) crossLinkDependencies(dependencyOrder DependencyOrder) (RunningModules, error) {
	for _, module := range modules {
		for _, dependency := range module.Runner.Module.Dependencies {
			runningDependency, hasDependency := modules[dependency.Path]
			if !hasDependency {
				return modules, errors.New(runbase.DependencyNotFoundWhileCrossLinkingError{Module: module.Runner.Module, Dependency: dependency})
			}

			// TODO: Remove lint suppression
			switch dependencyOrder { //nolint:exhaustive
			case NormalOrder:
				module.Dependencies[runningDependency.Runner.Module.Path] = runningDependency
				runningDependency.NotifyWhenDone = append(runningDependency.NotifyWhenDone, module)
			case IgnoreOrder:
				// Nothing
			default:
				runningDependency.Dependencies[module.Runner.Module.Path] = module
				module.NotifyWhenDone = append(module.NotifyWhenDone, runningDependency)
			}
		}
	}

	return modules, nil
}

// RemoveFlagExcluded returns a cleaned-up map that only contains modules and
// dependencies that should not be excluded
func (modules RunningModules) RemoveFlagExcluded(r *report.Report, reportExperiment bool) (RunningModules, error) {
	var finalModules = make(map[string]*DependencyController)

	var errs []error

	for key, module := range modules {
		// Only add modules that should not be excluded
		if !module.Runner.FlagExcluded {
			finalModules[key] = &DependencyController{
				Runner:         module.Runner,
				DependencyDone: module.DependencyDone,
				Dependencies:   make(map[string]*DependencyController),
				NotifyWhenDone: module.NotifyWhenDone,
			}

			// Only add dependencies that should not be excluded
			for path, dependency := range module.Dependencies {
				if !dependency.Runner.FlagExcluded {
					finalModules[key].Dependencies[path] = dependency
				}
			}
		} else if reportExperiment {
			run, err := r.GetRun(module.Runner.Module.Path)
			if errors.Is(err, report.ErrRunNotFound) {
				run, err = report.NewRun(module.Runner.Module.Path)
				if err != nil {
					errs = append(errs, err)
					continue
				}

				if err := r.AddRun(run); err != nil {
					errs = append(errs, err)
					continue
				}
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
		return finalModules, errors.Join(errs...)
	}

	return finalModules, nil
}

// Run the given map of module path to runningModule. To "run" a module, execute the runTerragrunt command in its
// TerragruntOptions object. The modules will be executed in an order determined by their inter-dependencies, using
// as much concurrency as possible.
func (modules RunningModules) runModules(ctx context.Context, opts *options.TerragruntOptions, r *report.Report, parallelism int) error {
	var (
		waitGroup sync.WaitGroup
		semaphore = make(chan struct{}, parallelism) // Make a semaphore from a buffered channel
	)

	for _, module := range modules {
		waitGroup.Add(1)

		go func(module *DependencyController) {
			defer waitGroup.Done()

			module.runModuleWhenReady(ctx, opts, r, semaphore)
		}(module)
	}

	waitGroup.Wait()

	return modules.collectErrors()
}

// Collect the errors from the given modules and return a single error object to represent them, or nil if no errors
// occurred
func (modules RunningModules) collectErrors() error {
	var errs *errors.MultiError

	for _, module := range modules {
		if module.Runner.Err != nil {
			errs = errs.Append(module.Runner.Err)
		}
	}

	return errs.ErrorOrNil()
}
