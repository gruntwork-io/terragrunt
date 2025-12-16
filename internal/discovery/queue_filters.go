package discovery

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// applyQueueFilters marks discovered units as excluded or included based on queue-related CLI flags and config.
// The runner consumes the exclusion markers instead of re-evaluating the filters.
func (d *Discovery) applyQueueFilters(opts *options.TerragruntOptions, components component.Components) component.Components {
	components = d.flagUnitsThatRead(opts, components)
	components = d.applyExcludeModules(opts, components)

	return components
}

// normalizePaths converts paths to canonical absolute paths relative to workDir.
// Uses resolvePath for consistent symlink resolution with memoization.
func normalizePaths(workDir string, paths []string) []string {
	normalized := make([]string, 0, len(paths))

	for _, path := range paths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(workDir, path)
		}

		path = util.CleanPath(path)

		// Use resolvePath for memoized symlink resolution (macOS /var -> /private/var)
		path = resolvePath(path)

		normalized = append(normalized, path)
	}

	return normalized
}

// capturePreIncluded records the paths of units that are currently not excluded.
func capturePreIncluded(components component.Components) map[string]struct{} {
	preIncluded := make(map[string]struct{})

	for _, c := range components {
		unit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		if !unit.Excluded() {
			preIncluded[unit.Path()] = struct{}{}
		}
	}

	return preIncluded
}

// resetAllUnitsExcluded marks all units as excluded.
func resetAllUnitsExcluded(components component.Components) {
	for _, c := range components {
		if unit, ok := c.(*component.Unit); ok {
			unit.SetExcluded(true)
		}
	}
}

// unexcludeUnitsReading un-excludes units that read any of the normalized file paths.
func unexcludeUnitsReading(components component.Components, normalizedReading []string, workDir string) {
	if len(normalizedReading) == 0 {
		return
	}

	readingSet := make(map[string]struct{}, len(normalizedReading))
	for _, r := range normalizedReading {
		readingSet[r] = struct{}{}
	}

	for _, c := range components {
		unit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		for _, readPath := range unit.Reading() {
			if !filepath.IsAbs(readPath) {
				readPath = filepath.Join(workDir, readPath)
			}

			readPath = util.CleanPath(readPath)

			// Also resolve symlinks for consistent comparison
			readPath = resolvePath(readPath)

			if _, ok := readingSet[readPath]; ok {
				unit.SetExcluded(false)

				break
			}
		}
	}
}

// unexcludeModulesThatInclude un-excludes units whose processed includes match any of the normalized paths.
func unexcludeModulesThatInclude(components component.Components, normalizedIncluding []string, workDir string) {
	if len(normalizedIncluding) == 0 {
		return
	}

	for _, c := range components {
		unit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		cfg := unit.Config()
		if cfg == nil || len(cfg.ProcessedIncludes) == 0 {
			continue
		}

		for _, includeConfig := range cfg.ProcessedIncludes {
			includePath := includeConfig.Path
			if !filepath.IsAbs(includePath) {
				includePath = filepath.Join(workDir, includePath)
			}

			includePath = util.CleanPath(includePath)

			for _, normalizedPath := range normalizedIncluding {
				if includePath == normalizedPath {
					unit.SetExcluded(false)

					break
				}
			}
		}
	}
}

// restorePreIncluded re-applies prior inclusions by un-excluding units that were previously included.
func restorePreIncluded(components component.Components, preIncluded map[string]struct{}) {
	for _, c := range components {
		unit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		if _, wasIncluded := preIncluded[unit.Path()]; wasIncluded {
			unit.SetExcluded(false)
		}
	}
}

// flagUnitsThatRead un-excludes units that read files listed via --modules-that-include/--units-reading.
func (d *Discovery) flagUnitsThatRead(opts *options.TerragruntOptions, components component.Components) component.Components {
	if len(opts.ModulesThatInclude) == 0 && len(opts.UnitsReading) == 0 {
		return components
	}

	// Normalize paths using discoveryContext.WorkingDir which is always set correctly
	workingDir := d.discoveryContext.WorkingDir
	normalizedReading := normalizePaths(workingDir, opts.UnitsReading)
	normalizedIncluding := normalizePaths(workingDir, opts.ModulesThatInclude)

	// Capture pre-included units before resetting
	preIncluded := capturePreIncluded(components)

	// Reset all units to excluded
	resetAllUnitsExcluded(components)

	// Un-exclude units that read the requested files
	unexcludeUnitsReading(components, normalizedReading, workingDir)

	// Un-exclude units that include the requested files
	unexcludeModulesThatInclude(components, normalizedIncluding, workingDir)

	// Restore prior inclusions
	restorePreIncluded(components, preIncluded)

	return components
}

// applyExcludeModules marks units (and optionally their dependencies) excluded via terragrunt exclude blocks.
func (d *Discovery) applyExcludeModules(opts *options.TerragruntOptions, components component.Components) component.Components {
	for _, c := range components {
		unit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		cfg := unit.Config()
		if cfg == nil || cfg.Exclude == nil {
			continue
		}

		if !cfg.Exclude.IsActionListed(opts.TerraformCommand) {
			continue
		}

		if cfg.Exclude.If {
			unit.SetExcluded(true)
		}

		if cfg.Exclude.ExcludeDependencies != nil && *cfg.Exclude.ExcludeDependencies {
			for _, dep := range unit.Dependencies() {
				depUnit, ok := dep.(*component.Unit)
				if !ok {
					continue
				}

				depUnit.SetExcluded(true)
			}
		}
	}

	return components
}
