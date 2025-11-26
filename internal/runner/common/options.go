package common

import (
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/report"
)

// ParseOptionsSetter is a minimal interface for components that can accept HCL parser options.
type ParseOptionsSetter interface {
	SetParseOptions(parserOptions []hclparse.Option)
}

// Option applies configuration to a StackRunner.
type Option interface {
	Apply(stack StackRunner)
}

// optionImpl is a lightweight Option implementation that wraps an apply function
// and optionally carries HCL parser options.
type optionImpl struct {
	apply         func(StackRunner)
	parserOptions []hclparse.Option
}

func (o optionImpl) Apply(stack StackRunner) {
	if o.apply != nil {
		o.apply(stack)
	}
}

// ParseOptionsProvider exposes HCL parser options carried by an Option.
type ParseOptionsProvider interface {
	GetParseOptions() []hclparse.Option
}

// GetParseOptions returns the HCL parser options attached to the option, if any.
func (o optionImpl) GetParseOptions() []hclparse.Option {
	if len(o.parserOptions) > 0 {
		return o.parserOptions
	}

	return nil
}

// WithChildTerragruntConfig sets the child Terragrunt configuration on a StackRunner.
func WithChildTerragruntConfig(cfg *config.TerragruntConfig) Option {
	return optionImpl{
		apply: func(stack StackRunner) {
			stack.SetTerragruntConfig(cfg)
		},
	}
}

// WithParseOptions provides custom HCL parser options to both discovery and stack execution.
func WithParseOptions(parserOptions []hclparse.Option) Option {
	return optionImpl{
		apply: func(stack StackRunner) {
			stack.SetParseOptions(parserOptions)
		},
		parserOptions: parserOptions,
	}
}

// WithReport attaches a report collector to the stack, enabling run summaries and metrics.
func WithReport(r *report.Report) Option {
	return optionImpl{
		apply: func(stack StackRunner) {
			stack.SetReport(r)
		},
	}
}

// UnitFiltersSetter is an interface for stack runners that support unit filtering.
type UnitFiltersSetter interface {
	SetUnitFilters(filters ...UnitFilter)
}

// WithUnitFilters provides unit filters to customize unit exclusions after resolution.
func WithUnitFilters(filters ...UnitFilter) Option {
	return optionImpl{
		apply: func(stack StackRunner) {
			if setter, ok := stack.(UnitFiltersSetter); ok {
				setter.SetUnitFilters(filters...)
			}
		},
	}
}
