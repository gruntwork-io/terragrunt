package discovery

import (
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/options"
)

// applyQueueFilters marks discovered units as excluded or included based on queue-related CLI flags and config.
// The runner consumes the exclusion markers instead of re-evaluating the filters.
func (d *Discovery) applyQueueFilters(opts *options.TerragruntOptions, components component.Components) component.Components {
	components = d.applyExcludeModules(opts, components)

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
