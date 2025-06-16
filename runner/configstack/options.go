package configstack

import (
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/runner"
)

// Option is a function that modifies a Stack.
type Option func(runner.Stack)

// WithChildTerragruntConfig sets the TerragruntConfig on any Stack implementation.
func WithChildTerragruntConfig(config *config.TerragruntConfig) Option {
	return func(stack runner.Stack) {
		stack.SetTerragruntConfig(config)
	}
}

// WithParseOptions sets the parserOptions on any Stack implementation.
func WithParseOptions(parserOptions []hclparse.Option) Option {
	return func(stack runner.Stack) {
		stack.SetParseOptions(parserOptions)
	}
}

func WithReport(report *report.Report) Option {
	return func(stack runner.Stack) {
		stack.SetReport(report)
	}
}
