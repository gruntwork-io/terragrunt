package runner

import (
	"context"
	"maps"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// FindDependentUnits returns the units that depend on the unit in the working directory.
func FindDependentUnits(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
) []*component.Unit {
	matchedUnitsMap := make(map[string]*component.Unit)
	pathsToCheck := discoverPathsToCheck(ctx, l, v, opts, cfg)

	for _, dir := range pathsToCheck {
		maps.Copy(
			matchedUnitsMap,
			findMatchingUnitsInPath(
				ctx,
				l,
				v,
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

// discoverPathsToCheck returns the directories to search: the git top level directory,
// or the parents of the processed includes when git detection fails.
func discoverPathsToCheck(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	terragruntConfig *config.TerragruntConfig,
) []string {
	var pathsToCheck []string

	if gitTopLevelDir, err := shell.GitTopLevelDir(ctx, l, v, opts.WorkingDir); err == nil {
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
func findMatchingUnitsInPath(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	dir string,
	opts *options.TerragruntOptions,
) map[string]*component.Unit {
	matchedUnitsMap := make(map[string]*component.Unit)

	configPath := filepath.Join(dir, filepath.Base(opts.TerragruntConfigPath))

	cfgOpts, err := options.NewTerragruntOptionsWithConfigPath(v.Exec, configPath)
	if err != nil {
		l.Debugf("Failed to build terragrunt options from %s %v", configPath, err)
		return matchedUnitsMap
	}

	cfgOpts.OriginalTerragruntConfigPath = opts.OriginalTerragruntConfigPath
	cfgOpts.TerraformCommand = opts.TerraformCommand
	cfgOpts.TerraformCliArgs = opts.TerraformCliArgs
	cfgOpts.CheckDependentUnits = opts.CheckDependentUnits
	cfgOpts.NonInteractive = true

	l.Infof("Discovering dependent units for %s", opts.TerragruntConfigPath)

	rnr, err := New(ctx, l, v, cfgOpts)
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
