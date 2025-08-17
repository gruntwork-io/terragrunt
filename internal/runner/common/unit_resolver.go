package common

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gobwas/glob"
	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/cli/commands/run/creds"
	"github.com/gruntwork-io/terragrunt/cli/commands/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/util"
)

// UnitResolver provides common functionality for resolving Terraform units from Terragrunt configuration files.
type UnitResolver struct {
	Stack        *Stack
	includeGlobs map[string]glob.Glob
	excludeGlobs map[string]glob.Glob
}

// NewUnitResolver creates a new UnitResolver with the given stack.
func NewUnitResolver(stack *Stack) (*UnitResolver, error) {
	includeGlobs, err := util.CompileGlobs(stack.TerragruntOptions.WorkingDir, stack.TerragruntOptions.IncludeDirs...)
	if err != nil {
		return nil, err
	}

	excludeGlobs, err := util.CompileGlobs(stack.TerragruntOptions.WorkingDir, stack.TerragruntOptions.ExcludeDirs...)
	if err != nil {
		return nil, err
	}

	return &UnitResolver{
		Stack:        stack,
		includeGlobs: includeGlobs,
		excludeGlobs: excludeGlobs,
	}, nil
}

// ResolveTerraformModules goes through each of the given Terragrunt configuration files
// and resolve the unit that configuration file represents into a Unit struct.
// Return the list of these Unit structs.
func (r *UnitResolver) ResolveTerraformModules(ctx context.Context, l log.Logger, terragruntConfigPaths []string) (Units, error) {
	canonicalTerragruntConfigPaths, err := util.CanonicalPaths(terragruntConfigPaths, ".")
	if err != nil {
		return nil, err
	}

	unitsMap, err := r.telemetryResolveUnits(ctx, l, canonicalTerragruntConfigPaths)
	if err != nil {
		return nil, err
	}

	externalDependencies, err := r.telemetryResolveExternalDependencies(ctx, l, unitsMap)
	if err != nil {
		return nil, err
	}

	crossLinkedUnits, err := r.telemetryCrossLinkDependencies(ctx, unitsMap, externalDependencies, canonicalTerragruntConfigPaths)
	if err != nil {
		return nil, err
	}

	withUnitsIncluded, err := r.telemetryFlagIncludedDirs(ctx, l, crossLinkedUnits)
	if err != nil {
		return nil, err
	}

	withUnitsThatAreIncludedByOthers, err := r.telemetryFlagUnitsThatAreIncluded(ctx, withUnitsIncluded)
	if err != nil {
		return nil, err
	}

	withExcludedUnits, err := r.telemetryFlagExcludedUnits(ctx, l, withUnitsThatAreIncludedByOthers)
	if err != nil {
		return nil, err
	}

	withUnitsRead, err := r.telemetryFlagUnitsThatRead(ctx, withExcludedUnits)
	if err != nil {
		return nil, err
	}

	withUnitsExcluded, err := r.telemetryFlagExcludedDirs(ctx, l, withUnitsRead)
	if err != nil {
		return nil, err
	}

	return withUnitsExcluded, nil
}

// telemetryResolveUnits resolves Terraform units from the given Terragrunt configuration paths
func (r *UnitResolver) telemetryResolveUnits(ctx context.Context, l log.Logger, canonicalTerragruntConfigPaths []string) (UnitsMap, error) {
	var unitsMap UnitsMap

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "resolve_units", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(ctx context.Context) error {
		howThesePathsWereFound := "Terragrunt config file found in a subdirectory of " + r.Stack.TerragruntOptions.WorkingDir

		result, err := r.resolveUnits(ctx, l, canonicalTerragruntConfigPaths, howThesePathsWereFound)
		if err != nil {
			return err
		}

		unitsMap = result

		return nil
	})

	return unitsMap, err
}

// telemetryResolveExternalDependencies resolves external dependencies for the given units
func (r *UnitResolver) telemetryResolveExternalDependencies(ctx context.Context, l log.Logger, unitsMap UnitsMap) (UnitsMap, error) {
	var externalDependencies UnitsMap

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "resolve_external_dependencies_for_units", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(ctx context.Context) error {
		result, err := r.resolveExternalDependenciesForUnits(ctx, l, unitsMap, UnitsMap{}, 0)
		if err != nil {
			return err
		}

		externalDependencies = result

		return nil
	})

	return externalDependencies, err
}

// telemetryCrossLinkDependencies cross-links dependencies between units
func (r *UnitResolver) telemetryCrossLinkDependencies(ctx context.Context, unitsMap, externalDependencies UnitsMap, canonicalTerragruntConfigPaths []string) (Units, error) {
	var crossLinkedUnits Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "crosslink_dependencies", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		result, err := unitsMap.MergeMaps(externalDependencies).CrossLinkDependencies(canonicalTerragruntConfigPaths)
		if err != nil {
			return err
		}

		crossLinkedUnits = result

		return nil
	})

	return crossLinkedUnits, err
}

// telemetryFlagIncludedDirs flags directories that are included in the Terragrunt configuration
func (r *UnitResolver) telemetryFlagIncludedDirs(ctx context.Context, l log.Logger, crossLinkedUnits Units) (Units, error) {
	var withUnitsIncluded Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_included_dirs", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		withUnitsIncluded = r.flagIncludedDirs(r.Stack.TerragruntOptions, l, crossLinkedUnits)
		return nil
	})

	return withUnitsIncluded, err
}

// telemetryFlagUnitsThatAreIncluded flags units that are included in the Terragrunt configuration
func (r *UnitResolver) telemetryFlagUnitsThatAreIncluded(ctx context.Context, withUnitsIncluded Units) (Units, error) {
	var withUnitsThatAreIncludedByOthers Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_units_that_are_included", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		result, err := r.flagUnitsThatAreIncluded(r.Stack.TerragruntOptions, withUnitsIncluded)
		if err != nil {
			return err
		}

		withUnitsThatAreIncludedByOthers = result

		return nil
	})

	return withUnitsThatAreIncludedByOthers, err
}

// telemetryFlagExcludedUnits flags units that are excluded in the Terragrunt configuration
func (r *UnitResolver) telemetryFlagExcludedUnits(ctx context.Context, l log.Logger, withUnitsThatAreIncludedByOthers Units) (Units, error) {
	var withExcludedUnits Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_excluded_units", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		result := r.flagExcludedUnits(l, r.Stack.TerragruntOptions, withUnitsThatAreIncludedByOthers)
		withExcludedUnits = result

		return nil
	})

	return withExcludedUnits, err
}

// telemetryFlagUnitsThatRead flags units that read files in the Terragrunt configuration
func (r *UnitResolver) telemetryFlagUnitsThatRead(ctx context.Context, withExcludedUnits Units) (Units, error) {
	var withUnitsRead Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_units_that_read", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		withUnitsRead = r.flagUnitsThatRead(r.Stack.TerragruntOptions, withExcludedUnits)
		return nil
	})

	return withUnitsRead, err
}

// telemetryFlagExcludedDirs flags directories that are excluded in the Terragrunt configuration
func (r *UnitResolver) telemetryFlagExcludedDirs(ctx context.Context, l log.Logger, withUnitsRead Units) (Units, error) {
	var withUnitsExcluded Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_excluded_dirs", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		withUnitsExcluded = r.flagExcludedDirs(l, r.Stack.TerragruntOptions, r.Stack.Report, withUnitsRead)
		return nil
	})

	return withUnitsExcluded, err
}

// Go through each of the given Terragrunt configuration files and resolve the unit that configuration file represents
// into a Unit struct. Note that this method will NOT fill in the Dependencies field of the Unit
// struct (see the crosslinkDependencies method for that). Return a map from unit path to Unit struct.
func (r *UnitResolver) resolveUnits(ctx context.Context, l log.Logger, canonicalTerragruntConfigPaths []string, howTheseUnitsWereFound string) (UnitsMap, error) {
	unitsMap := UnitsMap{}

	for _, terragruntConfigPath := range canonicalTerragruntConfigPaths {
		if !util.FileExists(terragruntConfigPath) {
			return nil, ProcessingUnitError{UnderlyingError: os.ErrNotExist, UnitPath: terragruntConfigPath, HowThisUnitWasFound: howTheseUnitsWereFound}
		}

		var unit *Unit

		err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "resolve_terraform_unit", map[string]any{
			"config_path": terragruntConfigPath,
			"working_dir": r.Stack.TerragruntOptions.WorkingDir,
		}, func(ctx context.Context) error {
			m, err := r.resolveTerraformUnit(ctx, l, terragruntConfigPath, unitsMap, howTheseUnitsWereFound)
			if err != nil {
				return err
			}

			unit = m

			return nil
		})

		if err != nil {
			return unitsMap, err
		}

		if unit != nil {
			unitsMap[unit.Path] = unit

			var dependencies UnitsMap

			err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "resolve_dependencies_for_unit", map[string]any{
				"config_path": terragruntConfigPath,
				"working_dir": r.Stack.TerragruntOptions.WorkingDir,
				"unit_path":   unit.Path,
			}, func(ctx context.Context) error {
				deps, err := r.resolveDependenciesForUnit(ctx, l, unit, unitsMap, true)
				if err != nil {
					return err
				}

				dependencies = deps

				return nil
			})
			if err != nil {
				return unitsMap, err
			}

			unitsMap = collections.MergeMaps(unitsMap, dependencies)
		}
	}

	return unitsMap, nil
}

// Create a Unit struct for the Terraform unit specified by the given Terragrunt configuration file path.
// Note that this method will NOT fill in the Dependencies field of the Unit struct (see the
// crosslinkDependencies method for that).
func (r *UnitResolver) resolveTerraformUnit(ctx context.Context, l log.Logger, terragruntConfigPath string, unitsMap UnitsMap, howThisUnitWasFound string) (*Unit, error) {
	unitPath, err := r.resolveUnitPath(terragruntConfigPath)
	if err != nil {
		return nil, err
	}

	if _, ok := unitsMap[unitPath]; ok {
		return nil, nil
	}

	l, opts, err := r.cloneOptionsWithConfigPath(l, terragruntConfigPath)
	if err != nil {
		return nil, err
	}

	includeConfig := r.setupIncludeConfig(terragruntConfigPath, opts)

	for globPath, glob := range r.excludeGlobs {
		if glob.Match(unitPath) {
			l.Debugf("Unit %s is excluded by glob %s", unitPath, globPath)
			return &Unit{Path: unitPath, Logger: l, TerragruntOptions: opts, FlagExcluded: true}, nil
		}
	}

	parseCtx := r.createParsingContext(ctx, l, opts)

	if err := r.acquireCredentials(ctx, l, opts); err != nil {
		return nil, err
	}

	//nolint:contextcheck
	terragruntConfig, err := r.partialParseConfig(parseCtx, l, terragruntConfigPath, includeConfig, howThisUnitWasFound)
	if err != nil {
		return nil, err
	}

	r.Stack.TerragruntOptions.CloneReadFiles(opts.ReadFiles)

	terragruntSource, err := config.GetTerragruntSourceForModule(r.Stack.TerragruntOptions.Source, unitPath, terragruntConfig)
	if err != nil {
		return nil, err
	}

	opts.Source = terragruntSource

	if err := r.setupDownloadDir(terragruntConfigPath, opts, l); err != nil {
		return nil, err
	}

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(terragruntConfigPath), "*.tf"))
	if err != nil {
		return nil, err
	}

	if (terragruntConfig.Terraform == nil || terragruntConfig.Terraform.Source == nil || *terragruntConfig.Terraform.Source == "") && len(matches) == 0 {
		l.Debugf("Unit %s does not have an associated terraform configuration and will be skipped.", filepath.Dir(terragruntConfigPath))
		return nil, nil
	}

	return &Unit{Path: unitPath, Logger: l, Config: *terragruntConfig, TerragruntOptions: opts}, nil
}

// resolveUnitPath converts a Terragrunt configuration file path to its corresponding unit path.
// Returns the canonical path of the directory containing the config file.
func (r *UnitResolver) resolveUnitPath(terragruntConfigPath string) (string, error) {
	return util.CanonicalPath(filepath.Dir(terragruntConfigPath), ".")
}

// cloneOptionsWithConfigPath creates a copy of the Terragrunt options with a new config path.
// Returns the cloned logger, options, and any error that occurred during cloning.
func (r *UnitResolver) cloneOptionsWithConfigPath(l log.Logger, terragruntConfigPath string) (log.Logger, *options.TerragruntOptions, error) {
	l, opts, err := r.Stack.TerragruntOptions.CloneWithConfigPath(l, terragruntConfigPath)
	if err != nil {
		return l, nil, err
	}

	opts.OriginalTerragruntConfigPath = terragruntConfigPath

	return l, opts, nil
}

// setupIncludeConfig creates an include configuration for Terragrunt config inheritance.
// Returns the include config if the path is processed, otherwise returns nil.
func (r *UnitResolver) setupIncludeConfig(terragruntConfigPath string, opts *options.TerragruntOptions) *config.IncludeConfig {
	var includeConfig *config.IncludeConfig
	if r.Stack.ChildTerragruntConfig != nil && r.Stack.ChildTerragruntConfig.ProcessedIncludes.ContainsPath(terragruntConfigPath) {
		includeConfig = &config.IncludeConfig{
			Path: terragruntConfigPath,
		}
		opts.TerragruntConfigPath = r.Stack.TerragruntOptions.OriginalTerragruntConfigPath
	}

	return includeConfig
}

// createParsingContext initializes a parsing context for Terragrunt configuration files.
// Returns a configured parsing context with specific decode options for Terraform blocks.
func (r *UnitResolver) createParsingContext(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) *config.ParsingContext {
	parseOpts := opts.Clone()
	parseOpts.SkipOutput = false

	return config.NewParsingContext(ctx, l, parseOpts).
		WithParseOption(r.Stack.ParserOptions).
		WithDecodeList(
			config.TerraformSource,
			config.DependenciesBlock,
			config.DependencyBlock,
			config.FeatureFlagsBlock,
			config.ErrorsBlock,
		)
}

// acquireCredentials obtains and updates environment credentials for Terraform providers.
// Returns an error if credential acquisition fails.
func (r *UnitResolver) acquireCredentials(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	credsGetter := creds.NewGetter()
	return credsGetter.ObtainAndUpdateEnvIfNecessary(ctx, l, opts, externalcmd.NewProvider(l, opts))
}

// partialParseConfig parses a Terragrunt configuration file with limited block decoding.
// Returns the parsed configuration or an error if parsing fails.
func (r *UnitResolver) partialParseConfig(parseCtx *config.ParsingContext, l log.Logger, terragruntConfigPath string, includeConfig *config.IncludeConfig, howThisUnitWasFound string) (*config.TerragruntConfig, error) {
	terragruntConfig, err := config.PartialParseConfigFile(
		parseCtx,
		l,
		terragruntConfigPath,
		includeConfig,
	)
	if err != nil {
		return nil, errors.New(ProcessingUnitError{
			UnderlyingError:     err,
			HowThisUnitWasFound: howThisUnitWasFound,
			UnitPath:            terragruntConfigPath,
		})
	}

	return terragruntConfig, nil
}

// setupDownloadDir configures the download directory for a Terragrunt unit.
// Returns an error if the download directory setup fails.
func (r *UnitResolver) setupDownloadDir(terragruntConfigPath string, opts *options.TerragruntOptions, l log.Logger) error {
	_, defaultDownloadDir, err := options.DefaultWorkingAndDownloadDirs(r.Stack.TerragruntOptions.TerragruntConfigPath)
	if err != nil {
		return err
	}

	if r.Stack.TerragruntOptions.DownloadDir == defaultDownloadDir {
		_, downloadDir, err := options.DefaultWorkingAndDownloadDirs(terragruntConfigPath)
		if err != nil {
			return err
		}

		l.Debugf("Setting download directory for unit %s to %s", filepath.Dir(opts.TerragruntConfigPath), downloadDir)
		opts.DownloadDir = downloadDir
	}

	return nil
}

// resolveDependenciesForUnit looks through the dependencies of the given unit and resolve the dependency paths listed in the unit's config.
// If `skipExternal` is true, the func returns only dependencies that are inside of the current working directory, which means they are part of the environment the
// user is trying to run --all apply or run --all destroy. Note that this method will NOT fill in the Dependencies field of the Unit struct (see the crosslinkDependencies method for that).
func (r *UnitResolver) resolveDependenciesForUnit(ctx context.Context, l log.Logger, unit *Unit, unitsMap UnitsMap, skipExternal bool) (UnitsMap, error) {
	if unit.Config.Dependencies == nil || len(unit.Config.Dependencies.Paths) == 0 {
		return UnitsMap{}, nil
	}

	externalTerragruntConfigPaths := []string{}

	for _, dependency := range unit.Config.Dependencies.Paths {
		dependencyPath, err := util.CanonicalPath(dependency, unit.Path)
		if err != nil {
			return UnitsMap{}, err
		}

		if skipExternal && !util.HasPathPrefix(dependencyPath, r.Stack.TerragruntOptions.WorkingDir) {
			continue
		}

		terragruntConfigPath := config.GetDefaultConfigPath(dependencyPath)

		if _, alreadyContainsUnit := unitsMap[dependencyPath]; !alreadyContainsUnit {
			externalTerragruntConfigPaths = append(externalTerragruntConfigPaths, terragruntConfigPath)
		}
	}

	howThesePathsWereFound := fmt.Sprintf("dependency of unit at '%s'", unit.Path)

	result, err := r.resolveUnits(ctx, l, externalTerragruntConfigPaths, howThesePathsWereFound)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Look through the dependencies of the units in the given map and resolve the "external" dependency paths listed in
// each units config (i.e. those dependencies not in the given list of Terragrunt config canonical file paths).
// These external dependencies are outside of the current working directory, which means they may not be part of the
// environment the user is trying to run --all apply or run --all destroy. Therefore, this method also confirms whether the user wants
// to actually apply those dependencies or just assume they are already applied. Note that this method will NOT fill in
// the Dependencies field of the Unit struct (see the crosslinkDependencies method for that).
func (r *UnitResolver) resolveExternalDependenciesForUnits(ctx context.Context, l log.Logger, unitsMap, unitsAlreadyProcessed UnitsMap, recursionLevel int) (UnitsMap, error) {
	allExternalDependencies := UnitsMap{}
	unitsToSkip := unitsMap.MergeMaps(unitsAlreadyProcessed)

	// Simple protection from circular dependencies causing a Stack Overflow due to infinite recursion
	const maxLevelsOfRecursion = 20
	if recursionLevel > maxLevelsOfRecursion {
		return allExternalDependencies, errors.New(InfiniteRecursionError{RecursionLevel: maxLevelsOfRecursion, Units: unitsToSkip})
	}

	sortedKeys := unitsMap.SortedKeys()
	for _, key := range sortedKeys {
		unit := unitsMap[key]

		externalDependencies, err := r.resolveDependenciesForUnit(ctx, l, unit, unitsToSkip, false)
		if err != nil {
			return externalDependencies, err
		}

		l, unitOpts, err := r.Stack.TerragruntOptions.CloneWithConfigPath(l, config.GetDefaultConfigPath(unit.Path))
		if err != nil {
			return nil, err
		}

		for _, externalDependency := range externalDependencies {
			if _, alreadyFound := unitsToSkip[externalDependency.Path]; alreadyFound {
				continue
			}

			shouldApply := false
			if !r.Stack.TerragruntOptions.IgnoreExternalDependencies {
				shouldApply, err = r.confirmShouldApplyExternalDependency(ctx, unit, l, externalDependency, unitOpts)
				if err != nil {
					return externalDependencies, err
				}
			}

			externalDependency.AssumeAlreadyApplied = !shouldApply
			allExternalDependencies[externalDependency.Path] = externalDependency
		}
	}

	if len(allExternalDependencies) > 0 {
		recursiveDependencies, err := r.resolveExternalDependenciesForUnits(ctx, l, allExternalDependencies, unitsMap, recursionLevel+1)
		if err != nil {
			return allExternalDependencies, err
		}

		return allExternalDependencies.MergeMaps(recursiveDependencies), nil
	}

	return allExternalDependencies, nil
}

// Confirm with the user whether they want Terragrunt to assume the given dependency of the given unit is already
// applied. If the user selects "yes", then Terragrunt will apply that unit as well.
// Note that we skip the prompt for `run --all destroy` calls. Given the destructive and irreversible nature of destroy, we don't
// want to provide any risk to the user of accidentally destroying an external dependency unless explicitly included
// with the --queue-include-external or --queue-include-dir flags.
func (r *UnitResolver) confirmShouldApplyExternalDependency(ctx context.Context, unit *Unit, l log.Logger, dependency *Unit, opts *options.TerragruntOptions) (bool, error) {
	if opts.IncludeExternalDependencies {
		l.Debugf("The --queue-include-external flag is set, so automatically including all external dependencies, and will run this command against unit %s, which is a dependency of unit %s.", dependency.Path, unit.Path)
		return true, nil
	}

	if opts.NonInteractive {
		l.Debugf("The --non-interactive flag is set. To avoid accidentally affecting external dependencies with a run --all command, will not run this command against unit %s, which is a dependency of unit %s.", dependency.Path, unit.Path)
		return false, nil
	}

	stackCmd := opts.TerraformCommand
	if stackCmd == "destroy" {
		l.Debugf("run --all command called with destroy. To avoid accidentally having destructive effects on external dependencies with run --all command, will not run this command against unit %s, which is a dependency of unit %s.", dependency.Path, unit.Path)
		return false, nil
	}

	l.Infof("Unit %s has external dependency %s", unit.Path, dependency.Path)

	return shell.PromptUserForYesNo(ctx, l, "Should Terragrunt apply the external dependency?", opts)
}

// flagIncludedDirs includes all units by default.
//
// However, when anything that triggers ExcludeByDefault is set, the function will instead
// selectively include only the units that are in the list specified via the IncludeDirs option.
func (r *UnitResolver) flagIncludedDirs(opts *options.TerragruntOptions, l log.Logger, units Units) Units {
	if !opts.ExcludeByDefault {
		return units
	}

	for _, unit := range units {
		unit.FlagExcluded = true
		for globPath, glob := range r.includeGlobs {
			if glob.Match(unit.Path) {
				unit.FlagExcluded = false
				l.Debugf("Unit %s is included by glob %s", unit.Path, globPath)
			}
		}
	}

	// Mark all affected dependencies as included before proceeding if not in strict include mode.
	if !opts.StrictInclude {
		for _, unit := range units {
			if !unit.FlagExcluded {
				for _, dependency := range unit.Dependencies {
					dependency.FlagExcluded = false
				}
			}
		}
	}

	return units
}

// flagUnitsThatAreIncluded iterates over a unit slice and flags all units that include at least one file in
// the specified include list on the TerragruntOptions ModulesThatInclude attribute.
func (r *UnitResolver) flagUnitsThatAreIncluded(opts *options.TerragruntOptions, units Units) (Units, error) {
	unitsThatInclude := append(opts.ModulesThatInclude, opts.UnitsReading...) //nolint:gocritic

	if len(unitsThatInclude) == 0 {
		return units, nil
	}

	unitsThatIncludeCanonicalPaths := []string{}

	for _, includePath := range unitsThatInclude {
		canonicalPath, err := util.CanonicalPath(includePath, opts.WorkingDir)
		if err != nil {
			return nil, err
		}

		unitsThatIncludeCanonicalPaths = append(unitsThatIncludeCanonicalPaths, canonicalPath)
	}

	for _, unit := range units {
		if err := r.flagUnitIncludes(unit, unitsThatIncludeCanonicalPaths); err != nil {
			return nil, err
		}

		if err := r.flagUnitDependencies(unit, unitsThatIncludeCanonicalPaths); err != nil {
			return nil, err
		}
	}

	return units, nil
}

// flagUnitIncludes marks a unit as included if any of its include paths match the canonical paths.
// Returns an error if path resolution fails during the comparison.
func (r *UnitResolver) flagUnitIncludes(unit *Unit, canonicalPaths []string) error {
	for _, includeConfig := range unit.Config.ProcessedIncludes {
		canonicalPath, err := util.CanonicalPath(includeConfig.Path, unit.Path)
		if err != nil {
			return err
		}

		if util.ListContainsElement(canonicalPaths, canonicalPath) {
			unit.FlagExcluded = false
		}
	}

	return nil
}

// flagUnitDependencies processes dependencies of a unit and flags them based on include paths.
// Returns an error if dependency processing fails.
func (r *UnitResolver) flagUnitDependencies(unit *Unit, canonicalPaths []string) error {
	for _, dependency := range unit.Dependencies {
		if dependency.FlagExcluded {
			continue
		}

		if err := r.flagDependencyIncludes(dependency, unit.Path, canonicalPaths); err != nil {
			return err
		}
	}

	return nil
}

// flagDependencyIncludes marks a dependency as included if any of its include paths match the canonical paths.
// Returns an error if path resolution fails during the comparison.
func (r *UnitResolver) flagDependencyIncludes(dependency *Unit, unitPath string, canonicalPaths []string) error {
	for _, includeConfig := range dependency.Config.ProcessedIncludes {
		canonicalPath, err := util.CanonicalPath(includeConfig.Path, unitPath)
		if err != nil {
			return err
		}

		if util.ListContainsElement(canonicalPaths, canonicalPath) {
			dependency.FlagExcluded = false
		}
	}

	return nil
}

// flagExcludedUnits iterates over a unit slice and flags all units that are excluded based on the exclude block.
func (r *UnitResolver) flagExcludedUnits(l log.Logger, opts *options.TerragruntOptions, units Units) Units {
	for _, unit := range units {
		excludeConfig := unit.Config.Exclude

		if excludeConfig == nil {
			continue
		}

		if !excludeConfig.IsActionListed(opts.TerraformCommand) {
			continue
		}

		if excludeConfig.If {
			l.Debugf("Unit %s is excluded by exclude block", unit.Path)
			unit.FlagExcluded = true
		}

		if excludeConfig.ExcludeDependencies != nil && *excludeConfig.ExcludeDependencies {
			l.Debugf("Excluding dependencies for unit %s by exclude block", unit.Path)

			for _, dependency := range unit.Dependencies {
				dependency.FlagExcluded = true
			}
		}
	}

	return units
}

// flagUnitsThatRead iterates over a unit slice and flags all units that read at least one file in the specified
// file list in the TerragruntOptions UnitsReading attribute.
func (r *UnitResolver) flagUnitsThatRead(opts *options.TerragruntOptions, units Units) Units {
	// If no UnitsThatRead is specified return the units list instantly
	if len(opts.UnitsReading) == 0 {
		return units
	}

	for _, path := range opts.UnitsReading {
		if !filepath.IsAbs(path) {
			path = filepath.Join(opts.WorkingDir, path)
			path = filepath.Clean(path)
		}

		for _, unit := range units {
			if opts.DidReadFile(path, unit.Path) {
				unit.FlagExcluded = false
			}
		}
	}

	return units
}

// flagExcludedDirs iterates over a unit slice and flags all entries as excluded listed in the queue-exclude-dir CLI flag.
func (r *UnitResolver) flagExcludedDirs(l log.Logger, opts *options.TerragruntOptions, reportInstance *report.Report, units Units) Units {
	// If we don't have any excludes, we don't need to do anything.
	if len(r.excludeGlobs) == 0 {
		return units
	}

	for _, unit := range units {
		for globPath, glob := range r.excludeGlobs {
			if glob.Match(unit.Path) {
				// Mark unit itself as excluded
				unit.FlagExcluded = true
				l.Debugf("Unit %s is excluded by glob %s", unit.Path, globPath)

				if opts.Experiments.Evaluate(experiment.Report) {
					// TODO: Make an upsert option for ends,
					// so that I don't have to do this every time.
					run, err := reportInstance.EnsureRun(unit.Path)
					if err != nil {
						l.Errorf("Error ensuring run for unit %s: %v", unit.Path, err)
						continue
					}

					if err := reportInstance.EndRun(
						run.Path,
						report.WithResult(report.ResultExcluded),
						report.WithReason(report.ReasonExcludeDir),
					); err != nil {
						l.Errorf("Error ending run for unit %s: %v", unit.Path, err)
						continue
					}
				}
			}
		}

		// Mark all affected dependencies as excluded
		for _, dependency := range unit.Dependencies {
			for globPath, glob := range r.excludeGlobs {
				if glob.Match(dependency.Path) {
					l.Debugf("Dependency %s is excluded by glob %s", dependency.Path, globPath)
					dependency.FlagExcluded = true

					if opts.Experiments.Evaluate(experiment.Report) {
						run, err := reportInstance.EnsureRun(dependency.Path)
						if err != nil {
							l.Errorf("Error ensuring run for dependency %s: %v", dependency.Path, err)
							continue
						}

						if err := reportInstance.EndRun(
							run.Path,
							report.WithResult(report.ResultExcluded),
							report.WithReason(report.ReasonExcludeDir),
						); err != nil {
							l.Errorf("Error ending run for dependency %s: %v", dependency.Path, err)
							continue
						}
					}
				}
			}
		}
	}

	return units
}
