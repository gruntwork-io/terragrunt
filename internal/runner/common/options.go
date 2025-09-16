package common

import (
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/report"
)

// ParseOptionsSetter is a minimal interface for types that can accept parser options.
// Both stack runners and discovery implement SetParseOptions with this signature.
type ParseOptionsSetter interface {
	SetParseOptions(parserOptions []hclparse.Option)
}

// Option applies configuration to the StackRunner.
type Option interface {
	Apply(stack StackRunner)
}

type optionImpl struct {
	apply           func(StackRunner)
	parserOptions   []hclparse.Option
	hasParseOptions bool
}

func (o optionImpl) Apply(stack StackRunner) {
	if o.apply != nil {
		o.apply(stack)
	}
}

// ParseOptionsProvider allows extracting parser options from an Option without applying it to a stack.
type ParseOptionsProvider interface {
	GetParseOptions() ([]hclparse.Option, bool)
}

// GetParseOptions returns parser options when the option was created by WithParseOptions.
func (o optionImpl) GetParseOptions() ([]hclparse.Option, bool) {
	if o.hasParseOptions {
		return o.parserOptions, true
	}

	return nil, false
}

// WithChildTerragruntConfig sets the TerragruntConfig on any Stack implementation.
func WithChildTerragruntConfig(cfg *config.TerragruntConfig) Option {
	return optionImpl{
		apply: func(stack StackRunner) {
			stack.SetTerragruntConfig(cfg)
		},
	}
}

// WithParseOptions sets custom HCL parser options on both discovery and stack.
func WithParseOptions(parserOptions []hclparse.Option) Option {
	return optionImpl{
		apply: func(stack StackRunner) {
			stack.SetParseOptions(parserOptions)
		},
		parserOptions:   parserOptions,
		hasParseOptions: true,
	}
}

// WithReport attaches a report collector to the stack only.
func WithReport(r *report.Report) Option {
	return optionImpl{
		apply: func(stack StackRunner) {
			stack.SetReport(r)
		},
	}
}
