package configstack

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/hashicorp/go-multierror"
)

const (
	Waiting ModuleStatus = iota
	Running
	Finished
	channelSize = 1000 // Use a huge buffer to ensure senders are never blocked
)

const (
	NormalOrder DependencyOrder = iota
	ReverseOrder
	IgnoreOrder
)

// Represents the status of a module that we are trying to apply as part of the apply-all or destroy-all command
type ModuleStatus int

// This controls in what order dependencies should be enforced between modules
type DependencyOrder int

// Represents a module we are trying to "run" (i.e. apply or destroy) as part of the apply-all or destroy-all command
type RunningModule struct {
	Module         *TerraformModule
	Status         ModuleStatus
	Err            error
	DependencyDone chan *RunningModule
	Dependencies   map[string]*RunningModule
	NotifyWhenDone []*RunningModule
	FlagExcluded   bool
}

// Create a new RunningModule struct for the given module. This will initialize all fields to reasonable defaults,
// except for the Dependencies and NotifyWhenDone, both of which will be empty. You should fill these using a
// function such as crossLinkDependencies.
func newRunningModule(module *TerraformModule) *RunningModule {
	return &RunningModule{
		Module:         module,
		Status:         Waiting,
		DependencyDone: make(chan *RunningModule, channelSize),
		Dependencies:   map[string]*RunningModule{},
		NotifyWhenDone: []*RunningModule{},
		FlagExcluded:   module.FlagExcluded,
	}
}

// Run a module once all of its dependencies have finished executing.
func (module *RunningModule) runModuleWhenReady(ctx context.Context, opts *options.TerragruntOptions, semaphore chan struct{}) {

	err := telemetry.Telemetry(ctx, opts, "wait_for_module_ready", map[string]interface{}{
		"path":             module.Module.Path,
		"terraformCommand": module.Module.TerragruntOptions.TerraformCommand,
	}, func(childCtx context.Context) error {
		return module.waitForDependencies()
	})

	semaphore <- struct{}{} // Add one to the buffered channel. Will block if parallelism limit is met
	defer func() {
		<-semaphore // Remove one from the buffered channel
	}()
	if err == nil {
		err = telemetry.Telemetry(ctx, opts, "run_module", map[string]interface{}{
			"path":             module.Module.Path,
			"terraformCommand": module.Module.TerragruntOptions.TerraformCommand,
		}, func(childCtx context.Context) error {
			return module.runNow(ctx, opts)
		})
	}
	module.moduleFinished(err)
}

// Wait for all of this modules dependencies to finish executing. Return an error if any of those dependencies complete
// with an error. Return immediately if this module has no dependencies.
func (module *RunningModule) waitForDependencies() error {
	module.Module.TerragruntOptions.Logger.Debugf("Module %s must wait for %d dependencies to finish", module.Module.Path, len(module.Dependencies))
	for len(module.Dependencies) > 0 {
		doneDependency := <-module.DependencyDone
		delete(module.Dependencies, doneDependency.Module.Path)

		if doneDependency.Err != nil {
			if module.Module.TerragruntOptions.IgnoreDependencyErrors {
				module.Module.TerragruntOptions.Logger.Errorf("Dependency %s of module %s just finished with an error. Module %s will have to return an error too. However, because of --terragrunt-ignore-dependency-errors, module %s will run anyway.", doneDependency.Module.Path, module.Module.Path, module.Module.Path, module.Module.Path)
			} else {
				module.Module.TerragruntOptions.Logger.Errorf("Dependency %s of module %s just finished with an error. Module %s will have to return an error too.", doneDependency.Module.Path, module.Module.Path, module.Module.Path)
				return ProcessingModuleDependencyError{module.Module, doneDependency.Module, doneDependency.Err}
			}
		} else {
			module.Module.TerragruntOptions.Logger.Debugf("Dependency %s of module %s just finished successfully. Module %s must wait on %d more dependencies.", doneDependency.Module.Path, module.Module.Path, module.Module.Path, len(module.Dependencies))
		}
	}

	return nil
}

// Run a module right now by executing the RunTerragrunt command of its TerragruntOptions field.
func (module *RunningModule) runNow(ctx context.Context, rootOptions *options.TerragruntOptions) error {
	module.Status = Running

	if module.Module.AssumeAlreadyApplied {
		module.Module.TerragruntOptions.Logger.Debugf("Assuming module %s has already been applied and skipping it", module.Module.Path)
		return nil
	} else {
		module.Module.TerragruntOptions.Logger.Debugf("Running module %s now", module.Module.Path)
		if err := module.Module.TerragruntOptions.RunTerragrunt(ctx, module.Module.TerragruntOptions); err != nil {
			return err
		}
		// convert terragrunt output to json
		if module.Module.outputJsonFile(module.Module.TerragruntOptions) != "" {
			jsonOptions := module.Module.TerragruntOptions.Clone(module.Module.TerragruntOptions.TerragruntConfigPath)
			stdout := bytes.Buffer{}
			jsonOptions.IncludeModulePrefix = false
			jsonOptions.TerraformLogsToJson = false
			jsonOptions.OutputPrefix = ""
			jsonOptions.Writer = &stdout
			jsonOptions.TerraformCommand = terraform.CommandNameShow
			jsonOptions.TerraformCliArgs = []string{terraform.CommandNameShow, "-json", module.Module.planFile(rootOptions)}
			if err := jsonOptions.RunTerragrunt(ctx, jsonOptions); err != nil {
				return err
			}
			// save the json output to the file plan file
			outputFile := module.Module.outputJsonFile(rootOptions)
			jsonDir := filepath.Dir(outputFile)
			if err := os.MkdirAll(jsonDir, os.ModePerm); err != nil {
				return err
			}
			if err := os.WriteFile(outputFile, stdout.Bytes(), os.ModePerm); err != nil {
				return err
			}
		}
		return nil
	}
}

// Record that a module has finished executing and notify all of this module's dependencies
func (module *RunningModule) moduleFinished(moduleErr error) {
	if moduleErr == nil {
		module.Module.TerragruntOptions.Logger.Debugf("Module %s has finished successfully!", module.Module.Path)
	} else {
		module.Module.TerragruntOptions.Logger.Errorf("Module %s has finished with an error: %v", module.Module.Path, moduleErr)
	}

	module.Status = Finished
	module.Err = moduleErr

	for _, toNotify := range module.NotifyWhenDone {
		toNotify.DependencyDone <- module
	}
}

type RunningModules map[string]*RunningModule

func (modules RunningModules) toTerraformModuleGroups(maxDepth int) []TerraformModules {
	// Walk the graph in run order, capturing which groups will run at each iteration. In each iteration, this pops out
	// the modules that have no dependencies and captures that as a run group.
	groups := []TerraformModules{}

	for len(modules) > 0 && len(groups) < maxDepth {
		currentIterationDeploy := TerraformModules{}

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
			case module.Module.AssumeAlreadyApplied:
				removeDep = append(removeDep, path)
			case len(module.Dependencies) == 0:
				currentIterationDeploy = append(currentIterationDeploy, module.Module)
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
		for _, dependency := range module.Module.Dependencies {
			runningDependency, hasDependency := modules[dependency.Path]
			if !hasDependency {
				return modules, errors.WithStackTrace(DependencyNotFoundWhileCrossLinkingError{module, dependency})
			}

			// TODO: Remove lint suppression
			switch dependencyOrder { //nolint:exhaustive
			case NormalOrder:
				module.Dependencies[runningDependency.Module.Path] = runningDependency
				runningDependency.NotifyWhenDone = append(runningDependency.NotifyWhenDone, module)
			case IgnoreOrder:
				// Nothing
			default:
				runningDependency.Dependencies[module.Module.Path] = module
				module.NotifyWhenDone = append(module.NotifyWhenDone, runningDependency)
			}
		}
	}

	return modules, nil
}

// Return a cleaned-up map that only contains modules and dependencies that should not be excluded
func (modules RunningModules) RemoveFlagExcluded() map[string]*RunningModule {
	var finalModules = make(map[string]*RunningModule)

	for key, module := range modules {

		// Only add modules that should not be excluded
		if !module.FlagExcluded {
			finalModules[key] = &RunningModule{
				Module:         module.Module,
				Dependencies:   make(map[string]*RunningModule),
				DependencyDone: module.DependencyDone,
				Err:            module.Err,
				NotifyWhenDone: module.NotifyWhenDone,
				Status:         module.Status,
			}

			// Only add dependencies that should not be excluded
			for path, dependency := range module.Dependencies {
				if !dependency.FlagExcluded {
					finalModules[key].Dependencies[path] = dependency
				}
			}
		}
	}

	return finalModules
}

// Run the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed in an order determined by their inter-dependencies, using
// as much concurrency as possible.
func (modules RunningModules) runModules(ctx context.Context, opts *options.TerragruntOptions, parallelism int) error {
	var waitGroup sync.WaitGroup
	var semaphore = make(chan struct{}, parallelism) // Make a semaphore from a buffered channel

	for _, module := range modules {
		waitGroup.Add(1)
		go func(module *RunningModule) {
			defer waitGroup.Done()
			module.runModuleWhenReady(ctx, opts, semaphore)
		}(module)
	}

	waitGroup.Wait()

	return modules.collectErrors()
}

// Collect the errors from the given modules and return a single error object to represent them, or nil if no errors
// occurred
func (modules RunningModules) collectErrors() error {
	var result *multierror.Error
	for _, module := range modules {
		if module.Err != nil {
			result = multierror.Append(result, module.Err)
		}
	}

	return result.ErrorOrNil()
}
