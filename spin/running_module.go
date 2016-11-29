package spin

import (
	"github.com/gruntwork-io/terragrunt/util"
	"sync"
	"fmt"
	"strings"
)

// Represents the status of a module that we are trying to apply as part of the spin-up or tear-down command
type ModuleStatus int
const (
	Waiting ModuleStatus = iota
	Running
	Finished
)

// Represents a module we are trying to "run" (i.e. apply or destroy) as part of the spin-up or tear-down command
type runningModule struct {
	Module         TerraformModule
	Status         ModuleStatus
	Err            error
	DependencyDone chan runningModule
	Dependencies   []runningModule
	NotifyWhenDone []chan runningModule
}

// This controls in what order dependencies should be enforced between modules
type DependencyOrder int
const (
	NormalOrder DependencyOrder = iota
	ReverseOrder
)

// Create a new RunningModule struct for the given module. This will initialize all fields to reasonable defaults,
// except for the Dependencies and NotifyWhenDone lists, both of which will be empty. You should fill these using a
// function such as crossLinkDependencies.
func newRunningModule(module TerraformModule) runningModule {
	return runningModule{
		Module: module,
		Status: Waiting,
		DependencyDone: make(chan runningModule),
		Dependencies: []runningModule{},
		NotifyWhenDone: []chan runningModule{},
	}
}

// Run the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed in an order determined by their inter-dependencies, using
// as much concurrency as possible.
func RunModules(modules []TerraformModule) error {
	return runModules(toRunningModules(modules, NormalOrder))
}

// Run the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed in the reverse order of their inter-dependencies, using
// as much concurrency as possible.
func RunModulesReverseOrder(modules []TerraformModule) error {
	return runModules(toRunningModules(modules, ReverseOrder))
}

// Convert the list of modules to a map from module path to a runningModule struct. This struct contains information
// about executing the module, such as whether it has finished running or not and any errors that happened.
func toRunningModules(modules []TerraformModule, dependencyOrder DependencyOrder) map[string]runningModule {
	runningModules := map[string]runningModule{}
	for _, module := range modules {
		runningModules[module.Path] = newRunningModule(module)
	}

	return crossLinkDependencies(runningModules, dependencyOrder)
}

// Loop through the map of runningModules and for each module M:
//
// * If dependencyOrder is NormalOrder, plug in all the modules M depends on into the Dependencies field and all the
//   modules that depend on M into the NotifyWhenDone field.
// * If dependencyOrder is ReverseOrder, do the reverse.
func crossLinkDependencies(modules map[string]runningModule, dependencyOrder DependencyOrder) map[string]runningModule {
	for _, module := range modules {
		for _, dependency := range module.Module.DependsOn {
			runningDependency := modules[dependency.Path]
			if dependencyOrder == NormalOrder {
				module.Dependencies = append(module.Dependencies, runningDependency)
				runningDependency.NotifyWhenDone = append(runningDependency.NotifyWhenDone, module.DependencyDone)
			} else {
				runningDependency.Dependencies = append(runningDependency.Dependencies, module)
				module.NotifyWhenDone = append(module.NotifyWhenDone, runningDependency.DependencyDone)
			}
		}
	}

	return modules
}

// Run the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed in an order determined by their inter-dependencies, using
// as much concurrency as possible.
func runModules(modules map[string]runningModule) error {
	var waitGroup sync.WaitGroup

	for _, module := range modules {
		waitGroup.Add(1)
		go func(module runningModule) {
			defer waitGroup.Done()
			module.runModuleWhenReady()
		}(module)
	}

	waitGroup.Wait()

	return collectErrors(modules)
}

// Collect the errors from the given modules and return a single error object to represent them, or nil if no errors
// occurred
func collectErrors(modules map[string]runningModule) error {
	errs := []error{}
	for _, module := range modules {
		if module.Err != nil {
			errs = append(errs, module.Err)
		}
	}

	if len(errs) == 0 {
		return nil
	} else {
		return MultiError{Errors: errs}
	}
}

// Run a module once all of its dependencies have finished executing.
func (module runningModule) runModuleWhenReady() {
	err := module.waitForDependencies()
	if err == nil {
		err = module.runNow()
	}
	module.moduleFinished(err)
}

// Wait for all of this modules dependencies to finish executing. Return an error if any of those dependencies complete
// with an error. Return immediately if this module has no dependencies.
func (module runningModule) waitForDependencies() error {
	for len(module.Dependencies) > 0 {
		doneDependency := <- module.DependencyDone
		module.Dependencies = util.RemoveElementFromList(module.Dependencies, doneDependency)

		if doneDependency.Err != nil {
			return DependencyFinishedWithError{module.Module, doneDependency.Module, doneDependency.Err}
		}
	}

	return nil
}

// Run a module right now by executing the RunTerragrunt command of its TerragruntOptions field.
func (module runningModule) runNow() error {
	module.Status = Running
	return module.Module.TerragruntOptions.RunTerragrunt(module.Module.TerragruntOptions)
}

// Record that a module has finished executing and notify all of this module's dependencies
func (module runningModule) moduleFinished(moduleErr error) {
	module.Status = Finished
	module.Err = moduleErr

	for _, channel := range module.NotifyWhenDone {
		channel <- module
	}
}

// Custom error types

type DependencyFinishedWithError struct {
	Module     TerraformModule
	Dependency TerraformModule
	Err        error
}
func (err DependencyFinishedWithError) Error() string {
	return fmt.Sprintf("Cannot process module %s because one of its dependencies, %s, finished with an error: %s", err.Module, err.Dependency, err.Err)
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

type MissingDependencyInternalError struct {
	Module     TerraformModule
	Dependency TerraformModule
}
func (err MissingDependencyInternalError) Error() string {
	return fmt.Sprintf("Error: could not find dependency %s for module %s. This is an internal error in Terragrunt and should be filed as a bug.", err.Module, err.Dependency)
}