package config

import "github.com/gruntwork-io/terragrunt/internal/strict"

// Option is a functional option for NewParsingContext.
type Option func(*ParsingContext)

// WithStrictControls sets the strict controls for the parsing context.
func WithStrictControls(controls strict.Controls) Option {
	return func(pctx *ParsingContext) {
		pctx.StrictControls = controls
	}
}
