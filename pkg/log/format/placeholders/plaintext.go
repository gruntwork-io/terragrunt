package placeholders

import (
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

// PlainTextPlaceholderName is the placeholder name.
const PlainTextPlaceholderName = ""

type plainText struct {
	*CommonPlaceholder
}

// PlainText creates a placeholder that displays plaintext.
// Although plaintext can be used as is without placeholder, this allows you to format the contant,
// for example set a color: `%(contant='just text',color=green)`.
func PlainText(value string, opts ...options.Option) Placeholder {
	opts = WithCommonOptions(
		options.Content(options.ContentValue(value)),
	).Merge(opts...)

	return &plainText{
		CommonPlaceholder: NewCommonPlaceholder(PlainTextPlaceholderName, opts...),
	}
}
