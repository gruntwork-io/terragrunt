package configstack

import (
	"context"
	"path/filepath"
	"sort"

	"github.com/gruntwork-io/terragrunt/internal/runner/common"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

const maxLevelsOfRecursion = 20
const existingModulesCacheName = "existingModules"

// Confirm with the user whether they want Terragrunt to assume the given dependency of the given module is already
// applied. If the user selects "yes", then Terragrunt will apply that module as well.
// Note that we skip the prompt for `run --all destroy` calls. Given the destructive and irreversible nature of destroy, we don't
// want to provide any risk to the user of accidentally destroying an external dependency unless explicitly included
// with the --queue-include-external or --queue-include-dir flags.
func confirmShouldApplyExternalDependency(ctx context.Context, u *config.Unit, l log.Logger, dependency *config.Unit, opts *options.TerragruntOptions) (bool, error) {
	if opts.IncludeExternalDependencies {
		l.Debugf("The --queue-include-external flag is set, so automatically including all external dependencies, and will run this command against module %s, which is a dependency of module %s.", dependency.Path, module.Path)
		return true, nil
	}

	if opts.NonInteractive {
		l.Debugf("The --non-interactive flag is set. To avoid accidentally affecting external dependencies with a run --all command, will not run this command against module %s, which is a dependency of module %s.", dependency.Path, module.Path)
		return false, nil
	}

	stackCmd := opts.TerraformCommand
	if stackCmd == "destroy" {
		l.Debugf("run --all command called with destroy. To avoid accidentally having destructive effects on external dependencies with run --all command, will not run this command against module %s, which is a dependency of module %s.", dependency.Path, module.Path)
		return false, nil
	}

	l.Infof("Module %s has external dependency %s", u.Path, dependency.Path)

	return shell.PromptUserForYesNo(ctx, l, "Should Terragrunt apply the external dependency?", opts)
}

// RunModules runs the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed in an order determined by their inter-dependencies, using
// as much concurrency as possible.
func RunModules(ctx context.Context, opts *options.TerragruntOptions, units common.Units, r *report.Report, parallelism int) error {
	runningModules, err := ToRunningModules(units, NormalOrder, r, opts)
	if err != nil {
		return err
	}

	return runningModules.runModules(ctx, opts, r, parallelism)
}

// RunModulesReverseOrder runs the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed in the reverse order of their inter-dependencies, using
// as much concurrency as possible.
func RunModulesReverseOrder(ctx context.Context, opts *options.TerragruntOptions, units common.Units, r *report.Report, parallelism int) error {
	runningModules, err := ToRunningModules(units, ReverseOrder, r, opts)
	if err != nil {
		return err
	}

	return runningModules.runModules(ctx, opts, r, parallelism)
}

// RunModulesIgnoreOrder runs the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed without caring for inter-dependencies.
func RunModulesIgnoreOrder(ctx context.Context, opts *options.TerragruntOptions, units common.Units, r *report.Report, parallelism int) error {
	runningModules, err := ToRunningModules(units, IgnoreOrder, r, opts)
	if err != nil {
		return err
	}

	return runningModules.runModules(ctx, opts, r, parallelism)
}

// ToRunningModules converts the list of modules to a map from module path to a runningModule struct. This struct contains information
// about executing the module, such as whether it has finished running or not and any errors that happened. Note that
// this does NOT actually run the module. For that, see the RunModules method.
func ToRunningModules(units common.Units, dependencyOrder DependencyOrder, r *report.Report, opts *options.TerragruntOptions) (RunningModules, error) {
	runningModules := RunningModules{}
	for _, module := range units {
		runningModules[module.Path] = newRunningModule(module)
	}

	crossLinkedModules, err := runningModules.crossLinkDependencies(dependencyOrder)
	if err != nil {
		return crossLinkedModules, err
	}

	return crossLinkedModules.RemoveFlagExcluded(r, opts.Experiments.Evaluate(experiment.Report))
}

// flagIncludedDirs includes all units by default.
//
// However, when anything that triggers ExcludeByDefault is set, the function will instead
// selectively include only the units that are in the list specified via the IncludeDirs option.
func (modules TerraformModules) flagIncludedDirs(opts *options.TerragruntOptions) TerraformModules {
	if !opts.ExcludeByDefault {
		return modules
	}

	for _, module := range modules {
		if module.findModuleInPath(opts.IncludeDirs) {
			module.FlagExcluded = false
		} else {
			module.FlagExcluded = true
		}
	}

	// Mark all affected dependencies as included before proceeding if not in strict include mode.
	if !opts.StrictInclude {
		for _, module := range modules {
			if !module.FlagExcluded {
				for _, dependency := range module.Dependencies {
					dependency.FlagExcluded = false
				}
			}
		}
	}

	return modules
}

// flagUnitsThatAreIncluded iterates over a module slice and flags all modules that include at least one file in
// the specified include list on the TerragruntOptions ModulesThatInclude attribute.
func (modules TerraformModules) flagUnitsThatAreIncluded(opts *options.TerragruntOptions) (TerraformModules, error) {
	// The two flags ModulesThatInclude and UnitsReading should both be considered when determining which
	// units to include in the run queue.
	unitsThatInclude := append(opts.ModulesThatInclude, opts.UnitsReading...) //nolint:gocritic

	// If no unitsThatInclude is specified return the modules list instantly
	if len(unitsThatInclude) == 0 {
		return modules, nil
	}

	modulesThatIncludeCanonicalPaths := []string{}

	for _, includePath := range unitsThatInclude {
		canonicalPath, err := util.CanonicalPath(includePath, opts.WorkingDir)
		if err != nil {
			return nil, err
		}

		modulesThatIncludeCanonicalPaths = append(modulesThatIncludeCanonicalPaths, canonicalPath)
	}

	for _, module := range modules {
		for _, includeConfig := range module.Config.ProcessedIncludes {
			// resolve include config to canonical path to compare with modulesThatIncludeCanonicalPath
			// https://github.com/gruntwork-io/terragrunt/issues/1944
			canonicalPath, err := util.CanonicalPath(includeConfig.Path, module.Path)
			if err != nil {
				return nil, err
			}

			if util.ListContainsElement(modulesThatIncludeCanonicalPaths, canonicalPath) {
				module.FlagExcluded = false
			}
		}

		// Also search module dependencies and exclude if the dependency path doesn't include any of the specified
		// paths, using a similar logic.
		for _, dependency := range module.Dependencies {
			if dependency.FlagExcluded {
				continue
			}

			for _, includeConfig := range dependency.Config.ProcessedIncludes {
				canonicalPath, err := util.CanonicalPath(includeConfig.Path, module.Path)
				if err != nil {
					return nil, err
				}

				if util.ListContainsElement(modulesThatIncludeCanonicalPaths, canonicalPath) {
					dependency.FlagExcluded = false
				}
			}
		}
	}

	return modules, nil
}

// flagExcludedUnits iterates over a module slice and flags all modules that are excluded based on the exclude block.
func (modules TerraformModules) flagExcludedUnits(l log.Logger, opts *options.TerragruntOptions) TerraformModules {
	for _, module := range modules {
		excludeConfig := module.Config.Exclude

		if excludeConfig == nil {
			continue
		}

		if !excludeConfig.IsActionListed(opts.TerraformCommand) {
			continue
		}

		if excludeConfig.If {
			l.Debugf("Module %s is excluded by exclude block", module.Path)
			module.FlagExcluded = true
		}

		if excludeConfig.ExcludeDependencies != nil && *excludeConfig.ExcludeDependencies {
			l.Debugf("Excluding dependencies for module %s by exclude block", module.Path)

			for _, dependency := range module.Dependencies {
				dependency.FlagExcluded = true
			}
		}
	}

	return modules
}

// flagUnitsThatRead iterates over a module slice and flags all modules that read at least one file in the specified
// file list in the TerragruntOptions UnitsReading attribute.
func (modules TerraformModules) flagUnitsThatRead(opts *options.TerragruntOptions) TerraformModules {
	// If no UnitsThatRead is specified return the modules list instantly
	if len(opts.UnitsReading) == 0 {
		return modules
	}

	for _, path := range opts.UnitsReading {
		if !filepath.IsAbs(path) {
			path = filepath.Join(opts.WorkingDir, path)
			path = filepath.Clean(path)
		}

		for _, module := range modules {
			if opts.DidReadFile(path, module.Path) {
				module.FlagExcluded = false
			}
		}
	}

	return modules
}

// flagExcludedDirs iterates over a module slice and flags all entries as excluded listed in the queue-exclude-dir CLI flag.
func (modules TerraformModules) flagExcludedDirs(l log.Logger, opts *options.TerragruntOptions, r *report.Report) TerraformModules {
	// If we don't have any excludes, we don't need to do anything.
	if len(opts.ExcludeDirs) == 0 {
		return modules
	}

	for _, module := range modules {
		if module.findModuleInPath(opts.ExcludeDirs) {
			// Mark module itself as excluded
			module.FlagExcluded = true

			if opts.Experiments.Evaluate(experiment.Report) {
				// TODO: Make an upsert option for ends,
				// so that I don't have to do this every time.
				run, err := r.GetRun(module.Path)
				if err != nil {
					run, err = report.NewRun(module.Path)
					if err != nil {
						l.Errorf("Error creating run for unit %s: %v", module.Path, err)

						continue
					}

					if err := r.AddRun(run); err != nil {
						l.Errorf("Error adding run for unit %s: %v", module.Path, err)

						continue
					}
				}

				if err := r.EndRun(
					run.Path,
					report.WithResult(report.ResultExcluded),
					report.WithReason(report.ReasonExcludeDir),
				); err != nil {
					l.Errorf("Error ending run for unit %s: %v", module.Path, err)

					continue
				}
			}
		}

		// Mark all affected dependencies as excluded
		for _, dependency := range module.Dependencies {
			if dependency.findModuleInPath(opts.ExcludeDirs) {
				dependency.FlagExcluded = true

				if opts.Experiments.Evaluate(experiment.Report) {
					run, err := r.GetRun(dependency.Path)
					if err != nil {
						return modules
					}

					if err := r.EndRun(
						run.Path,
						report.WithResult(report.ResultExcluded),
						report.WithReason(report.ReasonExcludeDir),
					); err != nil {
						return modules
					}
				}
			}
		}
	}

	return modules
}

var existingModules = cache.NewCache[*common.UnitsMap](existingModulesCacheName)

// Go through each module in the given map and cross-link its dependencies to the other modules in that same map. If
// a dependency is referenced that is not in the given map, return an error.
func (modulesMap TerraformModulesMap) crosslinkDependencies(canonicalTerragruntConfigPaths []string) (TerraformModules, error) {
	modules := TerraformModules{}

	keys := modulesMap.getSortedKeys()
	for _, key := range keys {
		module := modulesMap[key]

		dependencies, err := module.getDependenciesForModule(modulesMap, canonicalTerragruntConfigPaths)
		if err != nil {
			return modules, err
		}

		module.Dependencies = dependencies
		modules = append(modules, module)
	}

	return modules, nil
}

// Return the keys for the given map in sorted order. This is used to ensure we always iterate over maps of modules
// in a consistent order (Go does not guarantee iteration order for maps, and usually makes it random)
func (modulesMap TerraformModulesMap) getSortedKeys() []string {
	keys := []string{}
	for key := range modulesMap {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}
