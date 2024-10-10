package configstack

import (
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
)

// Option is type for passing options to the Stack.
type Option func(*Stack)

func WithChildTerragruntConfig(config *config.TerragruntConfig) Option {
	return func(stack *Stack) {
		stack.childTerragruntConfig = config
	}
}

func WithParseOptions(parserOptions []hclparse.Option) Option {
	return func(stack *Stack) {
		stack.parserOptions = parserOptions
	}
}
