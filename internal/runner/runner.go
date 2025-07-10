// Package runner provides logic for applying Stacks and Units Terragrunt.
package runner

import (
	"maps"
	"context"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/shell"

	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/runner/configstack"

	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/options"
)

// FindStackInSubfolders finds all the Terraform modules in the subfolders of the working directory of the given TerragruntOptions and
// assemble them into a Stack object that can be applied or destroyed in a single command
func FindStackInSubfolders(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...common.Option) (common.StackRunner, error) {
	if terragruntOptions.Experiments.Evaluate(experiment.RunnerPool) {
		l.Infof("Using runner pool for stack %s", terragruntOptions.WorkingDir)

		return runnerpool.Build(ctx, l, terragruntOptions, opts...)
	}

	return configstack.Build(ctx, l, terragruntOptions, opts...)
}

// FindWhereWorkingDirIsIncluded - find where working directory is included, flow:
// 1. Find root git top level directory and build list of modules
// 2. Iterate over includes from opts if git top level directory detection failed
// 3. Filter found module only items which has in dependencies working directory
func FindWhereWorkingDirIsIncluded(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) common.Units {
	matchedModulesMap := make(common.UnitsMap)
	pathsToCheck := discoverPathsToCheck(ctx, l, opts, terragruntConfig)

	for _, dir := range pathsToCheck {
		maps.Copy(matchedModulesMap, findMatchingUnitsInPath(ctx, l, dir, opts, terragruntConfig))
	}

	var matchedModules = make(common.Units, 0, len(matchedModulesMap))
	for _, module := range matchedModulesMap {
		matchedModules = append(matchedModules, module)
	}

	return matchedModules
}

// discoverPathsToCheck finds root git top level directory and builds list of modules, or iterates over includes if git detection fails.
func discoverPathsToCheck(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) []string {
	var pathsToCheck []string

	if gitTopLevelDir, err := shell.GitTopLevelDir(ctx, l, opts, opts.WorkingDir); err == nil {
		pathsToCheck = append(pathsToCheck, gitTopLevelDir)
	} else {
		uniquePaths := make(map[string]bool)
		for _, includePath := range terragruntConfig.ProcessedIncludes {
			uniquePaths[filepath.Dir(includePath.Path)] = true
		}

		for path := range uniquePaths {
			pathsToCheck = append(pathsToCheck, path)
		}
	}

	return pathsToCheck
}

// findMatchingUnitsInPath builds the stack from the config directory and filters modules by working dir dependencies.
func findMatchingUnitsInPath(ctx context.Context, l log.Logger, dir string, opts *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) common.UnitsMap {
	matchedModulesMap := make(common.UnitsMap)

	dir += filepath.FromSlash("/")

	cfgOptions, err := options.NewTerragruntOptionsWithConfigPath(dir)
	if err != nil {
		l.Debugf("Failed to build terragrunt options from %s %v", dir, err)
		return matchedModulesMap
	}

	cfgOptions.Env = opts.Env
	cfgOptions.OriginalTerragruntConfigPath = opts.OriginalTerragruntConfigPath
	cfgOptions.TerraformCommand = opts.TerraformCommand
	cfgOptions.NonInteractive = true

	runner, err := FindStackInSubfolders(ctx, l, cfgOptions, common.WithChildTerragruntConfig(terragruntConfig))
	if err != nil {
		l.Debugf("Failed to build module stack %v", err)
		return matchedModulesMap
	}

	stack := runner.GetStack()
	dependentModules := runner.ListStackDependentUnits()

	deps, found := dependentModules[opts.WorkingDir]
	if found {
		for _, module := range stack.Units {
			if slices.Contains(deps, module.Path) {
				matchedModulesMap[module.Path] = module
			}
		}
	}

	return matchedModulesMap
}
