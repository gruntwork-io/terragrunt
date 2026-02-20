// Package runner provides logic for applying Stacks and Units Terragrunt.
package runner

import (
	"context"
	"maps"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// NewStackRunner discovers all Terragrunt units under the working directory and
// assembles them into a StackRunner that can apply or destroy them.
func NewStackRunner(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	runnerOpts ...common.Option,
) (common.StackRunner, error) {
	return runnerpool.Build(ctx, l, opts, runnerOpts...)
}

// BuildUnitOpts is a facade for runnerpool.BuildUnitOpts.
func BuildUnitOpts(l log.Logger, stackOpts *options.TerragruntOptions, unit *component.Unit) (*options.TerragruntOptions, log.Logger, error) {
	return runnerpool.BuildUnitOpts(l, stackOpts, unit)
}

// FindDependentUnits - find dependent units for a given unit.
// 1. Find root git top level directory and build list of units
// 2. Iterate over includes from opts if git top level directory detection failed
// 3. Filter found units for those that have dependencies on the unit in the working directory
func FindDependentUnits(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
) []*component.Unit {
	matchedUnitsMap := make(map[string]*component.Unit)
	pathsToCheck := discoverPathsToCheck(ctx, l, opts, cfg)

	for _, dir := range pathsToCheck {
		maps.Copy(
			matchedUnitsMap,
			findMatchingUnitsInPath(
				ctx,
				l,
				dir,
				opts,
			),
		)
	}

	matchedUnits := make([]*component.Unit, 0, len(matchedUnitsMap))
	for _, unit := range matchedUnitsMap {
		matchedUnits = append(matchedUnits, unit)
	}

	return matchedUnits
}

// discoverPathsToCheck finds root git top level directory and builds list of units, or iterates over includes if git detection fails.
func discoverPathsToCheck(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) []string {
	var pathsToCheck []string

	if gitTopLevelDir, err := shell.GitTopLevelDir(ctx, l, opts.Env, opts.WorkingDir); err == nil {
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

// findMatchingUnitsInPath builds the stack from the config directory and filters units by working dir dependencies.
func findMatchingUnitsInPath(ctx context.Context, l log.Logger, dir string, opts *options.TerragruntOptions) map[string]*component.Unit {
	matchedUnitsMap := make(map[string]*component.Unit)

	// Construct the full path to terragrunt.hcl in the directory
	configPath := filepath.Join(dir, filepath.Base(opts.TerragruntConfigPath))

	cfgOpts, err := options.NewTerragruntOptionsWithConfigPath(configPath)
	if err != nil {
		l.Debugf("Failed to build terragrunt options from %s %v", configPath, err)
		return matchedUnitsMap
	}

	cfgOpts.Env = opts.Env
	cfgOpts.OriginalTerragruntConfigPath = opts.OriginalTerragruntConfigPath
	cfgOpts.TerraformCommand = opts.TerraformCommand
	cfgOpts.TerraformCliArgs = opts.TerraformCliArgs
	cfgOpts.CheckDependentUnits = opts.CheckDependentUnits
	cfgOpts.NonInteractive = true

	l.Infof("Discovering dependent units for %s", opts.TerragruntConfigPath)

	rnr, err := NewStackRunner(ctx, l, cfgOpts)
	if err != nil {
		l.Debugf("Failed to build unit stack %v", err)
		return matchedUnitsMap
	}

	stack := rnr.GetStack()
	dependentUnits := rnr.ListStackDependentUnits()

	deps, found := dependentUnits[opts.WorkingDir]
	if found {
		for _, unit := range stack.Units {
			if slices.Contains(deps, unit.Path()) {
				matchedUnitsMap[unit.Path()] = unit
			}
		}
	}

	return matchedUnitsMap
}
