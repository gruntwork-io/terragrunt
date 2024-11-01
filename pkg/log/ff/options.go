package format

import "github.com/gruntwork-io/terragrunt/pkg/log/format/config"

type Option func(*Formatter)

func WithPresetConfigs(cfgs ...*config.Config) Option {
	return func(formatter *Formatter) {
		formatter.presetConfigs = cfgs
	}
}

func WithSelectedConfig(name string) Option {
	return func(formatter *Formatter) {
		formatter.selectedConfigName = name
	}
}

// WithQuoteCharacter overrides the default quoting character " with something else. For example: ', or `.
func WithQuoteCharacter(quoteCharacter string) Option {
	return func(formatter *Formatter) {
		formatter.quoteCharacter = quoteCharacter
	}
}

// WithQuoteEmptyFields wraps empty fields in quotes if true.
func WithQuoteEmptyFields() Option {
	return func(formatter *Formatter) {
		formatter.quoteEmptyFields = true
	}
}
