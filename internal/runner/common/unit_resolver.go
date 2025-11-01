// Package common provides core abstractions for running Terraform/Terragrunt in parallel.
//
// # Architecture Overview
//
// The package revolves around three key concepts:
//   - Unit: A single Terraform module with its Terragrunt configuration
//   - Stack: A collection of units with dependencies between them
//   - UnitResolver: Builds units from discovery, applying filters and resolving dependencies
//
// # Unit Resolution Pipeline
//
// UnitResolver follows a multi-stage pipeline when building units from discovery:
//  1. buildUnitsFromDiscovery: Convert discovered components to units
//  2. resolveExternalDependencies: Find and confirm external dependencies
//  3. crossLinkDependencies: Wire up dependency pointers between units
//  4. flagIncludedDirs: Apply include patterns (if ExcludeByDefault mode)
//  5. flagUnitsThatAreIncluded: Mark units that include specific files
//  6. flagUnitsThatRead: Mark units that read specific files
//  7. flagExcludedDirs: Apply exclude patterns from CLI flags
//  8. flagExcludedUnits: Apply exclude blocks from Terragrunt configs
//  9. applyFilters: Run custom filters (e.g., graph filtering)
//
// # Exclusion Precedence
//
// Units can be excluded through multiple mechanisms, applied in this order:
//  1. CLI --terragrunt-exclude-dir (highest precedence)
//  2. Exclude blocks in terragrunt.hcl files
//  3. Custom filters (e.g., graph filter)
//  4. Include patterns (when ExcludeByDefault mode)
//
// When reporting exclusions, earlier mechanisms take precedence to avoid
// duplicate or conflicting report entries.
//
// # Telemetry
//
// Most resolver operations are wrapped in telemetry collection via the
// telemetry* methods. These track operation duration and provide context
// for debugging performance issues.
package common

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gobwas/glob"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	// doubleStarFeatureName is the strict control feature name for glob pattern support.
	doubleStarFeatureName = "double-star"
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
		includeGlobs map[string]glob.Glob
		excludeGlobs map[string]glob.Glob
		err          error
	)

	// Always compile globs for include/exclude dirs to support pattern matching
	includeGlobs, err = util.CompileGlobs(stack.TerragruntOptions.WorkingDir, stack.TerragruntOptions.IncludeDirs...)
	if err != nil {
		return nil, fmt.Errorf("invalid include dirs: %w", err)
	}

	excludeGlobs, err = util.CompileGlobs(stack.TerragruntOptions.WorkingDir, stack.TerragruntOptions.ExcludeDirs...)
	if err != nil {
		return nil, fmt.Errorf("invalid exclude dirs: %w", err)
	}

	// Check if double-star strict control is enabled (for backwards compatibility logging)
	doubleStarEnabled := stack.TerragruntOptions.StrictControls.FilterByNames(doubleStarFeatureName).SuppressWarning().Evaluate(ctx) != nil

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
func (r *UnitResolver) ResolveFromDiscovery(ctx context.Context, l log.Logger, discovered []component.Component) (Units, error) {
	unitsMap, err := r.telemetryBuildUnitsFromDiscovery(ctx, l, discovered)
	if err != nil {
		return nil, err
	}

	externalDependencies, err := r.telemetryResolveExternalDependencies(ctx, l, unitsMap)
	if err != nil {
		return nil, err
	}

	// Build the canonical config paths list for cross-linking
	// Include both discovered units and external dependencies
	canonicalTerragruntConfigPaths := make([]string, 0, len(discovered)+len(externalDependencies))
	for _, c := range discovered {
		if c.Kind() == component.StackKind {
			continue
		}
		// Mirror runner logic for file name
		fname := config.DefaultTerragruntConfigPath
		if r.Stack.TerragruntOptions.TerragruntConfigPath != "" && !util.IsDir(r.Stack.TerragruntOptions.TerragruntConfigPath) {
			fname = filepath.Base(r.Stack.TerragruntOptions.TerragruntConfigPath)
		}

		canonicalPath, err := util.CanonicalPath(filepath.Join(c.Path(), fname), ".")
		if err == nil {
			canonicalTerragruntConfigPaths = append(canonicalTerragruntConfigPaths, canonicalPath)
		}
	}

	// Add external dependencies to canonical paths list
	for _, extDep := range externalDependencies {
		// Use the actual TerragruntConfigPath from the unit's options
		// This handles non-default config file names correctly
		if extDep.TerragruntOptions != nil && extDep.TerragruntOptions.TerragruntConfigPath != "" {
			canonicalPath, err := util.CanonicalPath(extDep.TerragruntOptions.TerragruntConfigPath, ".")
			if err == nil {
				canonicalTerragruntConfigPaths = append(canonicalTerragruntConfigPaths, canonicalPath)
			}
		}
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

	withUnitsRead, err := r.telemetryFlagUnitsThatRead(ctx, withUnitsThatAreIncludedByOthers)
	if err != nil {
		return nil, err
	}

	withUnitsExcludedByDirs, err := r.telemetryFlagExcludedDirs(ctx, l, withUnitsRead)
	if err != nil {
		return nil, err
	}

	withExcludedUnits, err := r.telemetryFlagExcludedUnits(ctx, l, withUnitsExcludedByDirs)
	if err != nil {
		return nil, err
	}

	filteredUnits, err := r.telemetryApplyFilters(ctx, withExcludedUnits)
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
//  1. Filters out non-terraform units (e.g., stacks)
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
func (r *UnitResolver) buildUnitsFromDiscovery(l log.Logger, discovered []component.Component) (UnitsMap, error) {
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
		// Discovery may return either a directory path or a file path depending on the config filename
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
		tempUnit := &Unit{Path: unitPath}
		excludeFn := r.createPathMatcherFunc("exclude", opts, l)

		if excludeFn(tempUnit) {
			units[unitPath] = &Unit{Path: unitPath, Logger: l, TerragruntOptions: opts, FlagExcluded: true}

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

		// Check for TF files only in the immediate directory (not subdirectories)
		// to avoid including parent units that don't have their own terraform code
		dir := filepath.Dir(terragruntConfigPath)

		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}

		hasFiles := false

		for _, entry := range entries {
			if !entry.IsDir() && util.IsTFFile(filepath.Join(dir, entry.Name())) {
				hasFiles = true
				break
			}
		}

		if (terragruntConfig.Terraform == nil || terragruntConfig.Terraform.Source == nil || *terragruntConfig.Terraform.Source == "") && !hasFiles {
			l.Debugf("Unit %s does not have an associated terraform configuration and will be skipped.", filepath.Dir(terragruntConfigPath))
			continue
		}

		units[unitPath] = &Unit{Path: unitPath, Logger: l, Config: *terragruntConfig, TerragruntOptions: opts, Reading: dUnit.Reading()}
	}

	return units, nil
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
