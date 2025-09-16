package common

import (
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/report"
)

// ParseOptionsSetter is a minimal interface for components that can accept HCL parser options.
//
// Implemented by both discovery and stack runner implementations, allowing callers to inject
// custom HCL parsing behavior (e.g., diagnostics writer, file update hooks) without coupling
// to concrete types.
type ParseOptionsSetter interface {
	SetParseOptions(parserOptions []hclparse.Option)
}

// Option applies configuration to a StackRunner.
//
// Options are small composable units used to configure runners without exposing their internals.
// They can update runtime dependencies (e.g. report collector), tweak parsing behavior via
// ParseOptionsSetter, or seed contextual configuration.
type Option interface {
	Apply(stack StackRunner)
}

// optionImpl is a lightweight Option implementation that wraps an apply function
// and optionally carries HCL parser options for propagation to ParseOptionsSetter
// implementers (e.g., discovery and stack).
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
//
// This is primarily used by discovery to extract parser options early, prior to constructing
// a stack, so discovery-time parsing uses the same configuration as execution-time parsing.
type ParseOptionsProvider interface {
	GetParseOptions() []hclparse.Option
}

// GetParseOptions returns the HCL parser options attached to the option, if any.
// Returns nil when the option does not carry parser options.
func (o optionImpl) GetParseOptions() []hclparse.Option {
	if len(o.parserOptions) > 0 {
		return o.parserOptions
	}

	return nil
}

// WithChildTerragruntConfig sets the child Terragrunt configuration on a StackRunner.
//
// Useful when running a single child unit with a pre-parsed configuration.
func WithChildTerragruntConfig(cfg *config.TerragruntConfig) Option {
	return optionImpl{
		apply: func(stack StackRunner) {
			stack.SetTerragruntConfig(cfg)
		},
	}
}

// WithParseOptions provides custom HCL parser options to both discovery and stack execution.
//
// Typical options include diagnostics writers, loggers, and file-update hooks.
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
