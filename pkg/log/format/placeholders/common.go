package placeholders

import (
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

func WithCommonOptions(opts ...options.Option) options.Options {
	return options.Options(append(opts,
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

func NewCommonPlaceholder(name string, opts ...options.Option) *CommonPlaceholder {
	return &CommonPlaceholder{
		name: name,
		opts: opts,
	}
}

func (common *CommonPlaceholder) Name() string {
	return common.name
}

func (common *CommonPlaceholder) GetOption(str string) options.Option {
	return common.opts.Get(str)
}

func (common *CommonPlaceholder) SetValue(str string) error {
	return nil
}

func (common *CommonPlaceholder) Evaluate(data *options.Data) string {
	return common.opts.Evaluate(data, "")
}
