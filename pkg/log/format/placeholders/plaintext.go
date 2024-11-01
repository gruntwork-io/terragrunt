package placeholders

import (
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

const PlainTextPlaceholderName = ""

type plainText struct {
	*CommonPlaceholder
	value string
}

func (plainText *plainText) Evaluate(data *options.Data) string {
	return plainText.opts.Evaluate(data, plainText.value)
}

func PlainText(value string, opts ...options.Option) Placeholder {
	opts = WithCommonOptions().Merge(opts...)

	return &plainText{
		CommonPlaceholder: NewCommonPlaceholder(PlainTextPlaceholderName, opts...),
		value:             value,
	}
}
