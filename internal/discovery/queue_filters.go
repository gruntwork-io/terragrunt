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
	components = d.applyIncludeDirs(opts, components)
	components = d.flagUnitsThatRead(opts, components)
	components = d.applyExcludeDirs(opts, components)
	components = d.applyExcludeModules(opts, components)

	return components
}

func (d *Discovery) matchesInclude(path string) bool {
	cleanPath := util.CleanPath(path)

	for _, pattern := range d.compiledIncludePatterns {
		if pattern.Compiled.Match(cleanPath) {
			return true
		}
	}

	for _, raw := range d.includeDirs {
		if util.HasPathPrefix(cleanPath, util.CleanPath(raw)) {
			return true
		}
	}

	return false
}

func (d *Discovery) matchesExclude(path string) bool {
	cleanPath := util.CleanPath(path)

	for _, pattern := range d.compiledExcludePatterns {
		if pattern.Compiled.Match(cleanPath) {
			return true
		}
	}

	for _, raw := range d.excludeDirs {
		if util.HasPathPrefix(cleanPath, util.CleanPath(raw)) {
			return true
		}
	}

	return false
}

// applyIncludeDirs mirrors the runner's include-dir handling for ExcludeByDefault and StrictInclude flags.
func (d *Discovery) applyIncludeDirs(opts *options.TerragruntOptions, components component.Components) component.Components {
	if !opts.ExcludeByDefault {
		return components
	}

	for _, c := range components {
		unit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		unit.SetExcluded(true)

		if len(d.compiledIncludePatterns) == 0 {
			continue
		}

		if d.matchesInclude(unit.Path()) {
			unit.SetExcluded(false)
		}
	}

	if opts.StrictInclude {
		return components
	}

	for _, c := range components {
		unit, ok := c.(*component.Unit)
		if !ok || unit.Excluded() {
			continue
		}

		for _, dep := range unit.Dependencies() {
			depUnit, ok := dep.(*component.Unit)
			if !ok {
				continue
			}

			depUnit.SetExcluded(false)
		}
	}

	return components
}

// flagUnitsThatRead un-excludes units that read files listed via --modules-that-include/--units-reading.
func (d *Discovery) flagUnitsThatRead(opts *options.TerragruntOptions, components component.Components) component.Components {
	if len(opts.ModulesThatInclude) == 0 && len(opts.UnitsReading) == 0 {
		return components
	}

	normalizedReading := make([]string, 0, len(opts.UnitsReading))
	for _, path := range opts.UnitsReading {
		if !filepath.IsAbs(path) {
			path = util.JoinPath(opts.WorkingDir, path)
		}

		normalizedReading = append(normalizedReading, util.CleanPath(path))
	}

	normalizedIncluding := make([]string, 0, len(opts.ModulesThatInclude))
	for _, path := range opts.ModulesThatInclude {
		if !filepath.IsAbs(path) {
			path = util.JoinPath(opts.WorkingDir, path)
		}

		normalizedIncluding = append(normalizedIncluding, util.CleanPath(path))
	}

	// Track any units that were already included (e.g., via include dirs) so we can preserve them after resetting exclusions.
	preIncluded := make(map[string]struct{})

	if len(normalizedReading) > 0 || len(normalizedIncluding) > 0 {
		for _, c := range components {
			if unit, ok := c.(*component.Unit); ok {
				if !unit.Excluded() {
					preIncluded[unit.Path()] = struct{}{}
				}

				unit.SetExcluded(true)
			}
		}
	}

	// Un-exclude units that explicitly read any of the requested files.
	if len(normalizedReading) > 0 {
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
					readPath = util.JoinPath(opts.WorkingDir, readPath)
				}

				readPath = util.CleanPath(readPath)

				if _, ok := readingSet[readPath]; ok {
					unit.SetExcluded(false)
					break
				}
			}
		}
	}

	// Un-exclude units that include any of the requested files (modules-that-include).
	if len(normalizedIncluding) > 0 {
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
					includePath = util.JoinPath(opts.WorkingDir, includePath)
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

	// Re-apply any prior inclusions from include directories.
	for _, c := range components {
		if unit, ok := c.(*component.Unit); ok {
			if _, wasIncluded := preIncluded[unit.Path()]; wasIncluded {
				unit.SetExcluded(false)
			}
		}
	}

	return components
}

// applyExcludeDirs marks units as excluded when they match --queue-exclude-dir patterns.
func (d *Discovery) applyExcludeDirs(_ *options.TerragruntOptions, components component.Components) component.Components {
	if len(d.compiledExcludePatterns) == 0 && len(d.excludeDirs) == 0 {
		return components
	}

	for _, c := range components {
		unit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		if d.matchesExclude(unit.Path()) {
			unit.SetExcluded(true)
		}

		for _, dep := range unit.Dependencies() {
			depUnit, ok := dep.(*component.Unit)
			if !ok {
				continue
			}

			if d.matchesExclude(depUnit.Path()) {
				depUnit.SetExcluded(true)
			}
		}
	}

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
