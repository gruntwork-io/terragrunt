// Package runner provides logic for applying Stacks and Units Terragrunt.
package runner

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// FindStackInSubfolders finds all the Terraform modules in the subfolders of the working directory of the given TerragruntOptions and
// assemble them into a Stack object that can be applied or destroyed in a single command
func FindStackInSubfolders(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...common.Option) (common.StackRunner, error) {
	return runnerpool.Build(ctx, l, terragruntOptions, opts...)
}

// FindWhereWorkingDirIsIncluded - find where working directory is included, flow:
// 1. Find root git top level directory and build list of modules
// 2. Iterate over includes from opts if git top level directory detection failed
// 3. Filter found module only items which has in dependencies working directory
func FindWhereWorkingDirIsIncluded(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) []*component.Unit {
	matchedModulesMap := make(map[string]*component.Unit)
	pathsToCheck := discoverPathsToCheck(ctx, l, opts, terragruntConfig)

	for _, dir := range pathsToCheck {
		for k, v := range findMatchingUnitsInPath(ctx, l, dir, opts, terragruntConfig) {
			matchedModulesMap[k] = v
		}
	}

	matchedModules := make([]*component.Unit, 0, len(matchedModulesMap))
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
func findMatchingUnitsInPath(ctx context.Context, l log.Logger, dir string, opts *options.TerragruntOptions, _ *config.TerragruntConfig) map[string]*component.Unit {
	matchedModulesMap := make(map[string]*component.Unit)

	// Construct the full path to terragrunt.hcl in the directory
	configPath := filepath.Join(dir, filepath.Base(opts.TerragruntConfigPath))

	cfgOptions, err := options.NewTerragruntOptionsWithConfigPath(configPath)
	if err != nil {
		l.Debugf("Failed to build terragrunt options from %s %v", configPath, err)
		return matchedModulesMap
	}

	cfgOptions.Env = opts.Env
	cfgOptions.OriginalTerragruntConfigPath = opts.OriginalTerragruntConfigPath
	cfgOptions.TerraformCommand = opts.TerraformCommand
	cfgOptions.TerraformCliArgs = opts.TerraformCliArgs
	cfgOptions.CheckDependentUnits = opts.CheckDependentUnits
	cfgOptions.NonInteractive = true

	l.Infof("Discovering dependent units for %s", opts.TerragruntConfigPath)

	runner, err := FindStackInSubfolders(ctx, l, cfgOptions)
	if err != nil {
		l.Debugf("Failed to build module stack %v", err)
		return matchedModulesMap
	}

	stack := runner.GetStack()
	dependentModules := runner.ListStackDependentUnits()

	deps, found := dependentModules[opts.WorkingDir]
	if found {
		for _, module := range stack.Units {
			if slices.Contains(deps, module.Path()) {
				matchedModulesMap[module.Path()] = module
			}
		}
	}

	return matchedModulesMap
}

// DependentModulesFinder implements the runcfg.DependentModulesFinder interface.
// It wraps the FindWhereWorkingDirIsIncluded function to provide dependency information.
type DependentModulesFinder struct{}

// NewDependentModulesFinder creates a new DependentModulesFinder.
func NewDependentModulesFinder() *DependentModulesFinder {
	return &DependentModulesFinder{}
}

// FindDependentModules implements runcfg.DependentModulesFinder.
func (f *DependentModulesFinder) FindDependentModules(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, cfg *runcfg.RunConfig) []runcfg.DependentModule {
	// Convert RunConfig back to TerragruntConfig for the existing function
	// Note: This is a temporary shim until the underlying function can be refactored
	terragruntConfig := convertRunConfigToTerragruntConfig(cfg)

	units := FindWhereWorkingDirIsIncluded(ctx, l, opts, terragruntConfig)

	modules := make([]runcfg.DependentModule, len(units))
	for i, unit := range units {
		modules[i] = unit
	}

	return modules
}

// convertRunConfigToTerragruntConfig creates a minimal TerragruntConfig from RunConfig
// containing only the fields needed by FindWhereWorkingDirIsIncluded.
func convertRunConfigToTerragruntConfig(cfg *runcfg.RunConfig) *config.TerragruntConfig {
	if cfg == nil {
		return &config.TerragruntConfig{}
	}

	// Convert ProcessedIncludes from runcfg to config format
	processedIncludes := make(map[string]config.IncludeConfig)
	for name, include := range cfg.ProcessedIncludes {
		processedIncludes[name] = config.IncludeConfig{
			Name:          include.Name,
			Path:          include.Path,
			Expose:        include.Expose,
			MergeStrategy: include.MergeStrategy,
		}
	}

	return &config.TerragruntConfig{
		ProcessedIncludes: processedIncludes,
	}
}

// Ensure DependentModulesFinder implements the interface
var _ runcfg.DependentUnitsFinder = (*DependentModulesFinder)(nil)
