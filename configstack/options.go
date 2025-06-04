package configstack

import (
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
)

// Option is a function that modifies a Stack.
type Option func(Stack)

// WithChildTerragruntConfig sets the TerragruntConfig on any Stack implementation.
func WithChildTerragruntConfig(config *config.TerragruntConfig) Option {
	return func(stack Stack) {
		stack.SetTerragruntConfig(config)
	}
}

// WithParseOptions sets the parserOptions on any Stack implementation.
func WithParseOptions(parserOptions []hclparse.Option) Option {
	return func(stack Stack) {
		stack.SetParseOptions(parserOptions)
	}
}
