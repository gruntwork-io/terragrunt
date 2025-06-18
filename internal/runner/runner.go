// Package runner provides logic for applying Stacks and Units Terragrunt.
package runner

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/shell"

	configstack2 "github.com/gruntwork-io/terragrunt/internal/runner/configstack"

	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/options"
)

// FindStackInSubfolders finds all the Terraform modules in the subfolders of the working directory of the given TerragruntOptions and
// assemble them into a Stack object that can be applied or destroyed in a single command
func FindStackInSubfolders(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...config.Option) (config.Stack, error) {
	if terragruntOptions.Experiments.Evaluate(experiment.RunnerPool) {
		l.Infof("Using RunnerPoolStackBuilder to build stack for %s", terragruntOptions.WorkingDir)

		builder := runnerpool.NewRunnerPoolStackBuilder()

		return builder.BuildStack(ctx, l, terragruntOptions, opts...)
	}

	builder := &configstack2.DefaultStackBuilder{}

	return builder.BuildStack(ctx, l, terragruntOptions, opts...)
}

// FindWhereWorkingDirIsIncluded - find where working directory is included, flow:
// 1. Find root git top level directory and build list of modules
// 2. Iterate over includes from opts if git top level directory detection failed
// 3. Filter found module only items which has in dependencies working directory
func FindWhereWorkingDirIsIncluded(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) TerraformModules {
	var (
		pathsToCheck      []string
		matchedModulesMap = make(common.UnitsMap)
	)

	if gitTopLevelDir, err := shell.GitTopLevelDir(ctx, l, opts, opts.WorkingDir); err == nil {
		pathsToCheck = append(pathsToCheck, gitTopLevelDir)
	} else {
		// detection failed, trying to use include directories as source for stacks
		uniquePaths := make(map[string]bool)
		for _, includePath := range terragruntConfig.ProcessedIncludes {
			uniquePaths[filepath.Dir(includePath.Path)] = true
		}

		for path := range uniquePaths {
			pathsToCheck = append(pathsToCheck, path)
		}
	}

	for _, dir := range pathsToCheck { // iterate over detected paths, build stacks and filter modules by working dir
		dir += filepath.FromSlash("/")

		cfgOptions, err := options.NewTerragruntOptionsWithConfigPath(dir)
		if err != nil {
			l.Debugf("Failed to build terragrunt options from %s %v", dir, err)
			continue
		}

		cfgOptions.Env = opts.Env
		cfgOptions.OriginalTerragruntConfigPath = opts.OriginalTerragruntConfigPath
		cfgOptions.TerraformCommand = opts.TerraformCommand
		cfgOptions.NonInteractive = true

		// build stack from config directory
		stack, err := config.FindStackInSubfolders(ctx, l, cfgOptions, stack2.WithChildTerragruntConfig(terragruntConfig))
		if err != nil {
			// log error as debug since in some cases stack building may fail because parent files can be designed
			// to work with relative paths from downstream modules
			l.Debugf("Failed to build module stack %v", err)
			continue
		}

		depdendentModules := stack.ListStackDependentModules()

		deps, found := depdendentModules[opts.WorkingDir]
		if found {
			for _, module := range stack.Modules() {
				if slices.Contains(deps, module.Path) {
					matchedModulesMap[module.Path] = module
				}
			}
		}
	}

	// extract modules as list
	var matchedModules = make(TerraformModules, 0, len(matchedModulesMap))
	for _, module := range matchedModulesMap {
		matchedModules = append(matchedModules, module)
	}

	return matchedModules
}
