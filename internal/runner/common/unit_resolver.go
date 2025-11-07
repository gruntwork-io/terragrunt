// Package common provides primitives to build and filter Terragrunt Terraform units and their dependencies.
// UnitResolver converts discovery results into executable units, resolves/wires dependencies, applies include/exclude rules and custom filters, and records telemetry.
//
// Usage
//
//  1. Create the resolver
//     ctx := context.Background()
//     resolver, err := NewUnitResolver(ctx, stack)
//     if err != nil { /* handle error */ }
//
//  2. Optionally add filters (applied after dependency wiring)
//     resolver = resolver.WithFilters(
//     /* examples: FilterByGraph(...), FilterByPaths(...), custom UnitFilter funcs */
//     )
//
//  3. Resolve from discovery output
//     units, err := resolver.ResolveFromDiscovery(ctx, logger, discoveredComponents)
//     if err != nil { /* handle error */ }
//
//  4. Iterate results
//     for _, u := range units {
//     if u.FlagExcluded { continue }
//     // use u.Config, u.Dependencies, u.TerragruntOptions, etc.
//     }
//
// Notes
//   - Include/Exclude: CLI include/exclude patterns are honored when the "double-star" strict control is enabled.
//     Globs are compiled relative to WorkingDir and matched against unit paths.
//   - Telemetry: resolver stages are wrapped with telemetry to aid performance diagnostics.
package common

import (
	"context"
	"path/filepath"

	"github.com/gobwas/glob"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/util"
)

// UnitResolver provides common functionality for resolving Terraform units from Terragrunt configuration files.
type UnitResolver struct {
	Stack             *Stack
	includeGlobs      map[string]glob.Glob
	excludeGlobs      map[string]glob.Glob
	filters           []UnitFilter
	doubleStarEnabled bool
}

// NewUnitResolver creates a new UnitResolver with the given stack.
func NewUnitResolver(ctx context.Context, stack *Stack) (*UnitResolver, error) {
	var (
		includeGlobs      map[string]glob.Glob
		excludeGlobs      map[string]glob.Glob
		doubleStarEnabled = false
	)

	// Check if double-star strict control is enabled
	if stack.TerragruntOptions.StrictControls.FilterByNames(controls.DoubleStar).SuppressWarning().Evaluate(ctx) != nil {
		var err error

		doubleStarEnabled = true

		// Compile globs only when double-star is enabled
		includeGlobs, err = util.CompileGlobs(stack.TerragruntOptions.WorkingDir, stack.TerragruntOptions.IncludeDirs...)
		if err != nil {
			return nil, errors.Errorf("invalid include dirs: %w", err)
		}

		excludeGlobs, err = util.CompileGlobs(stack.TerragruntOptions.WorkingDir, stack.TerragruntOptions.ExcludeDirs...)
		if err != nil {
			return nil, errors.Errorf("invalid exclude dirs: %w", err)
		}
	}

	return &UnitResolver{
		Stack:             stack,
		doubleStarEnabled: doubleStarEnabled,
		includeGlobs:      includeGlobs,
		excludeGlobs:      excludeGlobs,
		filters:           []UnitFilter{},
	}, nil
}

// WithFilters adds unit filters to the resolver.
// Filters are applied after units are resolved but before the queue is built.
func (r *UnitResolver) WithFilters(filters ...UnitFilter) *UnitResolver {
	r.filters = append(r.filters, filters...)
	return r
}

// ResolveFromDiscovery builds units starting from discovery-parsed components, avoiding re-parsing
// for initially discovered units. It preserves the same filtering and dependency resolution pipeline.
// Discovery has already found and parsed all dependencies including external ones.
func (r *UnitResolver) ResolveFromDiscovery(ctx context.Context, l log.Logger, discovered component.Components) (Units, error) {
	unitsMap, err := r.telemetryBuildUnitsFromDiscovery(ctx, l, discovered)
	if err != nil {
		return nil, err
	}

	// Build the canonical config paths list for cross-linking
	// Discovery already found all units including external dependencies
	canonicalTerragruntConfigPaths := make([]string, 0, len(discovered))
	for _, c := range discovered {
		if c.Kind() == component.StackKind {
			continue
		}

		fname := r.determineTerragruntConfigFilename()
		configPath := filepath.Join(c.Path(), fname)

		canonicalPath, err := util.CanonicalPath(configPath, ".")
		if err != nil {
			return nil, errors.Errorf("canonicalizing terragrunt config path %q for unit %s: %w", configPath, c.Path(), err)
		}

		canonicalTerragruntConfigPaths = append(canonicalTerragruntConfigPaths, canonicalPath)
	}

	// Convert from discovery domain to runner domain
	// Discovery found all dependencies as Component interfaces, but runner needs concrete *Unit pointers
	var crossLinkedUnits Units

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "convert_discovery_to_runner", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		var linkErr error

		crossLinkedUnits, linkErr = unitsMap.ConvertDiscoveryToRunner(canonicalTerragruntConfigPaths)

		return linkErr
	})
	if err != nil {
		return nil, err
	}

	// Flag external dependencies and prompt user for confirmation
	// This must happen AFTER cross-linking so Dependencies field is populated
	// Convert Units list back to map for flagging
	crossLinkedMap := make(UnitsMap)
	for _, unit := range crossLinkedUnits {
		crossLinkedMap[unit.Component.Path()] = unit
	}

	if err := r.telemetryFlagExternalDependencies(ctx, l, crossLinkedMap); err != nil {
		return nil, err
	}

	// Process units-reading BEFORE exclude dirs/blocks so that explicit CLI excludes
	// (e.g., --queue-exclude-dir) can take precedence over inclusions by units-reading.
	// This handles both --units-that-include and legacy ModulesThatInclude flags.
	// Discovery already tracked all files read during parsing, so we check against unit.Reading.
	withUnitsIncluded, err := r.telemetryApplyIncludeDirs(ctx, l, crossLinkedUnits)
	if err != nil {
		return nil, err
	}

	withUnitsRead, err := r.telemetryFlagUnitsThatRead(ctx, withUnitsIncluded)
	if err != nil {
		return nil, err
	}

	// Process --queue-exclude-dir BEFORE exclude blocks so that CLI flags take precedence
	// This ensures units excluded via CLI get the correct reason in reports
	withUnitsExcludedByDirs, err := r.telemetryApplyExcludeDirs(ctx, l, withUnitsRead)
	if err != nil {
		return nil, err
	}

	withExcludedUnits, err := r.telemetryApplyExcludeModules(ctx, l, withUnitsExcludedByDirs)
	if err != nil {
		return nil, err
	}

	// Apply custom filters after standard resolution logic
	filteredUnits, err := r.telemetryApplyFilters(ctx, l, withExcludedUnits)
	if err != nil {
		return nil, err
	}

	return filteredUnits, nil
}

// telemetryBuildUnitsFromDiscovery wraps buildUnitsFromDiscovery in telemetry collection.
func (r *UnitResolver) telemetryBuildUnitsFromDiscovery(ctx context.Context, l log.Logger, discovered []component.Component) (UnitsMap, error) {
	var unitsMap UnitsMap

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "build_units_from_discovery", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
		"unit_count":  len(discovered),
	}, func(ctx context.Context) error {
		result, err := r.buildUnitsFromDiscovery(l, discovered)
		if err != nil {
			return err
		}

		unitsMap = result

		return nil
	})

	return unitsMap, err
}

// buildUnitsFromDiscovery constructs UnitsMap from discovery-parsed components without re-parsing,
// performing only the minimal parsing necessary to obtain missing fields (e.g., Terraform.source).
//
// This is the first stage of the unit resolution pipeline. It converts discovery components into
// Unit structs, preserving already-parsed configuration data to avoid redundant file I/O.
//
// The method:
//  1. Filters out non-units (e.g., stacks)
//  2. Skips units with parse errors from discovery
//  3. Determines the correct config file name (terragrunt.hcl or custom)
//  4. Resolves unit paths to canonical form
//  5. Checks if units should be excluded based on CLI flags (setting FlagExcluded=true)
//  6. Reuses parsed config from discovery (including TerraformSource and ErrorsBlock)
//  7. Sets up download directories for each unit
//  8. Skips units without Terraform source or TF files
//
// Units excluded at this stage have FlagExcluded=true and minimal configuration.
// They are still included in the UnitsMap for dependency resolution but won't be executed.
func (r *UnitResolver) buildUnitsFromDiscovery(l log.Logger, discovered component.Components) (UnitsMap, error) {
	units := make(UnitsMap)

	for _, c := range discovered {
		// Only handle terraform units; skip stacks and anything else
		if c.Kind() == component.StackKind {
			continue
		}

		dUnit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		// Get the config that discovery already parsed
		terragruntConfig := dUnit.Config()
		if terragruntConfig == nil {
			// Skip configurations that discovery could not parse
			l.Warnf("Skipping unit at %s due to parse error", dUnit.Path())
			continue
		}

		// Determine the actual config file path
		terragruntConfigPath := dUnit.Path()
		if util.IsDir(terragruntConfigPath) {
			fname := r.determineTerragruntConfigFilename()
			terragruntConfigPath = filepath.Join(dUnit.Path(), fname)
		}

		unitPath, err := r.resolveUnitPath(terragruntConfigPath)
		if err != nil {
			return nil, err
		}

		// Prepare options with proper working dir
		l, opts, err := r.Stack.TerragruntOptions.CloneWithConfigPath(l, terragruntConfigPath)
		if err != nil {
			return nil, err
		}

		opts.OriginalTerragruntConfigPath = terragruntConfigPath

		// Exclusion check - create a temporary unit for matching
		unitToExclude := &Unit{Component: component.NewUnit(unitPath).WithOpts(opts)}
		excludeFn := r.createPathMatcherFunc("exclude", opts, l)

		if excludeFn(unitToExclude) {
			units[unitPath] = unitToExclude

			continue
		}

		// Determine effective source and setup download dir
		terragruntSource, err := config.GetTerragruntSourceForModule(r.Stack.TerragruntOptions.Source, unitPath, terragruntConfig)
		if err != nil {
			return nil, err
		}

		opts.Source = terragruntSource

		// Update the config's source with the mapped source so that logging shows the correct URL
		if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.Source != nil && terragruntSource != "" {
			terragruntConfig.Terraform.Source = &terragruntSource
		}

		if err = r.setupDownloadDir(terragruntConfigPath, opts, l); err != nil {
			return nil, err
		}

		// Check for TF files in the directory or any of its subdirectories
		dir := filepath.Dir(terragruntConfigPath)

		hasFiles, err := util.DirContainsTFFiles(dir)
		if err != nil {
			return nil, err
		}

		if (terragruntConfig.Terraform == nil || terragruntConfig.Terraform.Source == nil || *terragruntConfig.Terraform.Source == "") && !hasFiles {
			l.Debugf("Unit %s does not have an associated terraform configuration and will be skipped.", filepath.Dir(terragruntConfigPath))
			continue
		}

		c := component.NewUnit(unitPath).WithConfig(terragruntConfig)
		c.SetReading(dUnit.Reading()...)
		// Preserve the external flag from discovery component
		if dUnit.External() {
			c.SetExternal()
		}

		// Set discovery context if available from discovered unit, otherwise create from TerragruntOptions
		// This is needed for component.Unit.Excluded() to properly check exclude blocks and prevent_destroy
		if dUnit.DiscoveryContext() != nil {
			c.SetDiscoveryContext(dUnit.DiscoveryContext())
		} else {
			c.SetDiscoveryContext(&component.DiscoveryContext{
				Cmd:  r.Stack.TerragruntOptions.TerraformCommand,
				Args: r.Stack.TerragruntOptions.TerraformCliArgs,
			})
		}

		units[unitPath] = &Unit{
			Component: c.WithOpts(opts),
		}
	}

	return units, nil
}

// telemetryFlagExternalDependencies flags external dependencies and prompts user for confirmation.
// Discovery has already found and parsed external dependencies, so this only handles user prompts.
func (r *UnitResolver) telemetryFlagExternalDependencies(ctx context.Context, l log.Logger, unitsMap UnitsMap) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_external_dependencies", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(ctx context.Context) error {
		return r.flagExternalDependencies(ctx, l, unitsMap)
	})
}
