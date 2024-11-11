package placeholders

import (
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

// MessagePlaceholderName is the placeholder name.
const MessagePlaceholderName = "msg"

type message struct {
	*CommonPlaceholder
}

// Evaluate implements `Placeholder` interface.
func (msg *message) Evaluate(data *options.Data) (string, error) {
	return msg.opts.Evaluate(data, data.Message)
}

func Message(opts ...options.Option) Placeholder {
	opts = WithCommonOptions(
		options.PathFormat(options.NonePath, options.RelativePath),
	).Merge(opts...)

	return &message{
		CommonPlaceholder: NewCommonPlaceholder(MessagePlaceholderName, opts...),
	}
}
