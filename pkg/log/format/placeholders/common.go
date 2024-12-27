package placeholders

import (
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

// WithCommonOptions is a set of common options that are used in all placeholders.
func WithCommonOptions(opts ...options.Option) options.Options {
	return options.Options(append(opts,
		options.Content(""),
		options.Escape(options.NoneEscape),
		options.Case(options.NoneCase),
		options.Width(0),
		options.Align(options.NoneAlign),
		options.Prefix(""),
		options.Suffix(""),
		options.Color(options.NoneColor),
	))
}

type CommonPlaceholder struct {
	name string
	opts options.Options
}

// NewCommonPlaceholder creates a new Common placeholder.
func NewCommonPlaceholder(name string, opts ...options.Option) *CommonPlaceholder {
	return &CommonPlaceholder{
		name: name,
		opts: opts,
	}
}

// Name implements `Placeholder` interface.
func (common *CommonPlaceholder) Name() string {
	return common.name
}

// Options implements `Placeholder` interface.
func (common *CommonPlaceholder) Options() options.Options {
	return common.opts
}

// Format implements `Placeholder` interface.
func (common *CommonPlaceholder) Format(data *options.Data) (string, error) {
	return common.opts.Format(data, "")
}
