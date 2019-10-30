package configstack

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/shell"
)

// Represents the status of a module that we are trying to apply as part of the apply-all or destroy-all command
type ModuleStatus int

const (
	Waiting ModuleStatus = iota
	Running
	Finished
)

// Represents a module we are trying to "run" (i.e. apply or destroy) as part of the apply-all or destroy-all command
type runningModule struct {
	Module         *TerraformModule
	Status         ModuleStatus
	Err            error
	DependencyDone chan *runningModule
	Dependencies   map[string]*runningModule
	NotifyWhenDone []*runningModule
	FlagExcluded   bool
}

// This controls in what order dependencies should be enforced between modules
type DependencyOrder int

const (
	NormalOrder DependencyOrder = iota
	ReverseOrder
	IgnoreOrder
)

// Create a new RunningModule struct for the given module. This will initialize all fields to reasonable defaults,
// except for the Dependencies and NotifyWhenDone, both of which will be empty. You should fill these using a
// function such as crossLinkDependencies.
func newRunningModule(module *TerraformModule) *runningModule {
	return &runningModule{
		Module:         module,
		Status:         Waiting,
		DependencyDone: make(chan *runningModule, 1000), // Use a huge buffer to ensure senders are never blocked
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   module.FlagExcluded,
	}
}

// Run the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed in an order determined by their inter-dependencies, using
// as much concurrency as possible.
func RunModules(modules []*TerraformModule) error {
	runningModules, err := toRunningModules(modules, NormalOrder)
	if err != nil {
		return err
	}
	return runModules(runningModules)
}

// Run the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed in the reverse order of their inter-dependencies, using
// as much concurrency as possible.
func RunModulesReverseOrder(modules []*TerraformModule) error {
	runningModules, err := toRunningModules(modules, ReverseOrder)
	if err != nil {
		return err
	}
	return runModules(runningModules)
}

// Run the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed without caring for inter-dependencies.
func RunModulesIgnoreOrder(modules []*TerraformModule) error {
	runningModules, err := toRunningModules(modules, IgnoreOrder)
	if err != nil {
		return err
	}
	return runModules(runningModules)
}

// Convert the list of modules to a map from module path to a runningModule struct. This struct contains information
// about executing the module, such as whether it has finished running or not and any errors that happened. Note that
// this does NOT actually run the module. For that, see the RunModules method.
func toRunningModules(modules []*TerraformModule, dependencyOrder DependencyOrder) (map[string]*runningModule, error) {
	runningModules := map[string]*runningModule{}
	for _, module := range modules {
		runningModules[module.Path] = newRunningModule(module)
	}

	crossLinkedModules, err := crossLinkDependencies(runningModules, dependencyOrder)
	if err != nil {
		return crossLinkedModules, err
	}

	return removeFlagExcluded(crossLinkedModules), nil
}

// Loop through the map of runningModules and for each module M:
//
// * If dependencyOrder is NormalOrder, plug in all the modules M depends on into the Dependencies field and all the
//   modules that depend on M into the NotifyWhenDone field.
// * If dependencyOrder is ReverseOrder, do the reverse.
// * If dependencyOrder is IgnoreOrder, do nothing.
func crossLinkDependencies(modules map[string]*runningModule, dependencyOrder DependencyOrder) (map[string]*runningModule, error) {
	for _, module := range modules {
		for _, dependency := range module.Module.Dependencies {
			runningDependency, hasDependency := modules[dependency.Path]
			if !hasDependency {
				return modules, errors.WithStackTrace(DependencyNotFoundWhileCrossLinking{module, dependency})
			}
			if dependencyOrder == NormalOrder {
				module.Dependencies[runningDependency.Module.Path] = runningDependency
				runningDependency.NotifyWhenDone = append(runningDependency.NotifyWhenDone, module)
			} else if dependencyOrder == IgnoreOrder {
				// Nothing
			} else {
				runningDependency.Dependencies[module.Module.Path] = module
				module.NotifyWhenDone = append(module.NotifyWhenDone, runningDependency)
			}
		}
	}

	return modules, nil
}

// Return a cleaned-up map that only contains modules and dependencies that should not be excluded
func removeFlagExcluded(modules map[string]*runningModule) map[string]*runningModule {
	var finalModules = make(map[string]*runningModule)

	for key, module := range modules {

		// Only add modules that should not be excluded
		if !module.FlagExcluded {
			finalModules[key] = &runningModule{
				Module:         module.Module,
				Dependencies:   make(map[string]*runningModule),
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
func runModules(modules map[string]*runningModule) error {
	var waitGroup sync.WaitGroup

	for _, module := range modules {
		waitGroup.Add(1)
		go func(module *runningModule) {
			defer waitGroup.Done()
			module.runModuleWhenReady()
		}(module)
	}

	waitGroup.Wait()

	return collectErrors(modules)
}

// Collect the errors from the given modules and return a single error object to represent them, or nil if no errors
// occurred
func collectErrors(modules map[string]*runningModule) error {
	errs := []error{}
	for _, module := range modules {
		if module.Err != nil {
			errs = append(errs, module.Err)
		}
	}

	if len(errs) == 0 {
		return nil
	} else {
		return errors.WithStackTrace(MultiError{Errors: errs})
	}
}

// Run a module once all of its dependencies have finished executing.
func (module *runningModule) runModuleWhenReady() {
	err := module.waitForDependencies()
	if err == nil {
		err = module.runNow()
	}
	module.moduleFinished(err)
}

// Wait for all of this modules dependencies to finish executing. Return an error if any of those dependencies complete
// with an error. Return immediately if this module has no dependencies.
func (module *runningModule) waitForDependencies() error {
	module.Module.TerragruntOptions.Logger.Printf("Module %s must wait for %d dependencies to finish", module.Module.Path, len(module.Dependencies))
	for len(module.Dependencies) > 0 {
		doneDependency := <-module.DependencyDone
		delete(module.Dependencies, doneDependency.Module.Path)

		if doneDependency.Err != nil {
			if module.Module.TerragruntOptions.IgnoreDependencyErrors {
				module.Module.TerragruntOptions.Logger.Printf("Dependency %s of module %s just finished with an error. Module %s will have to return an error too. However, because of --terragrunt-ignore-dependency-errors, module %s will run anyway.", doneDependency.Module.Path, module.Module.Path, module.Module.Path, module.Module.Path)
			} else {
				module.Module.TerragruntOptions.Logger.Printf("Dependency %s of module %s just finished with an error. Module %s will have to return an error too.", doneDependency.Module.Path, module.Module.Path, module.Module.Path)
				return DependencyFinishedWithError{module.Module, doneDependency.Module, doneDependency.Err}
			}
		} else {
			module.Module.TerragruntOptions.Logger.Printf("Dependency %s of module %s just finished successfully. Module %s must wait on %d more dependencies.", doneDependency.Module.Path, module.Module.Path, module.Module.Path, len(module.Dependencies))
		}
	}

	return nil
}

// Run a module right now by executing the RunTerragrunt command of its TerragruntOptions field.
func (module *runningModule) runNow() error {
	module.Status = Running

	if module.Module.AssumeAlreadyApplied {
		module.Module.TerragruntOptions.Logger.Printf("Assuming module %s has already been applied and skipping it", module.Module.Path)
		return nil
	} else {
		module.Module.TerragruntOptions.Logger.Printf("Running module %s now", module.Module.Path)
		return module.Module.TerragruntOptions.RunTerragrunt(module.Module.TerragruntOptions)
	}
}

// Record that a module has finished executing and notify all of this module's dependencies
func (module *runningModule) moduleFinished(moduleErr error) {
	if moduleErr == nil {
		module.Module.TerragruntOptions.Logger.Printf("Module %s has finished successfully!", module.Module.Path)
	} else {
		module.Module.TerragruntOptions.Logger.Printf("Module %s has finished with an error: %v", module.Module.Path, moduleErr)
	}

	module.Status = Finished
	module.Err = moduleErr

	for _, toNotify := range module.NotifyWhenDone {
		toNotify.DependencyDone <- module
	}
}

// Custom error types

type DependencyFinishedWithError struct {
	Module     *TerraformModule
	Dependency *TerraformModule
	Err        error
}

func (err DependencyFinishedWithError) Error() string {
	return fmt.Sprintf("Cannot process module %s because one of its dependencies, %s, finished with an error: %s", err.Module, err.Dependency, err.Err)
}

func (this DependencyFinishedWithError) ExitStatus() (int, error) {
	if exitCode, err := shell.GetExitCode(this.Err); err == nil {
		return exitCode, nil
	}
	return -1, this
}

type MultiError struct {
	Errors []error
}

func (err MultiError) Error() string {
	errorStrings := []string{}
	for _, err := range err.Errors {
		errorStrings = append(errorStrings, err.Error())
	}
	return fmt.Sprintf("Encountered the following errors:\n%s", strings.Join(errorStrings, "\n"))
}

func (this MultiError) ExitStatus() (int, error) {
	exitCode := 0
	for i := range this.Errors {
		if code, err := shell.GetExitCode(this.Errors[i]); err != nil {
			return -1, this
		} else if code > exitCode {
			exitCode = code
		}
	}
	return exitCode, nil
}

type DependencyNotFoundWhileCrossLinking struct {
	Module     *runningModule
	Dependency *TerraformModule
}

func (err DependencyNotFoundWhileCrossLinking) Error() string {
	return fmt.Sprintf("Module %v specifies a dependency on module %v, but could not find that module while cross-linking dependencies. This is most likely a bug in Terragrunt. Please report it.", err.Module, err.Dependency)
}
