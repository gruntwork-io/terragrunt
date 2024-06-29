package configstack

import "github.com/gruntwork-io/terragrunt/config"

type Option func(Stack) Stack

func WithChildTerragruntConfig(config *config.TerragruntConfig) Option {
	return func(stack Stack) Stack {
		stack.childTerragruntConfig = config
		return stack
	}
}
