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
	filesToCheck := append(opts.ModulesThatInclude, opts.UnitsReading...)
	if len(filesToCheck) == 0 {
		return components
	}

	normalizedPaths := make([]string, 0, len(filesToCheck))

	for _, path := range filesToCheck {
		normalized := path

		if !filepath.IsAbs(normalized) {
			normalized = util.JoinPath(opts.WorkingDir, normalized)
		}

		normalizedPaths = append(normalizedPaths, util.CleanPath(normalized))
	}

	for _, normalizedPath := range normalizedPaths {
		for _, c := range components {
			unit, ok := c.(*component.Unit)
			if !ok {
				continue
			}

			if util.ListContainsElement(unit.Reading(), normalizedPath) {
				unit.SetExcluded(false)
				continue
			}

			cfg := unit.Config()
			if cfg == nil {
				continue
			}

			for _, includeConfig := range cfg.ProcessedIncludes {
				includePath := includeConfig.Path
				if !filepath.IsAbs(includePath) {
					includePath = util.JoinPath(opts.WorkingDir, includePath)
				}

				includePath = util.CleanPath(includePath)

				if includePath != normalizedPath {
					continue
				}

				unit.SetExcluded(false)

				break
			}
		}
	}

	return components
}

// applyExcludeDirs marks units as excluded when they match --queue-exclude-dir patterns.
func (d *Discovery) applyExcludeDirs(opts *options.TerragruntOptions, components component.Components) component.Components {
	if len(d.compiledExcludePatterns) == 0 || len(opts.ExcludeDirs) == 0 {
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
