package placeholders

import (
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

// MessagePlaceholderName is the placeholder name.
const MessagePlaceholderName = "msg"

type message struct {
	*CommonPlaceholder
}

// Format implements `Placeholder` interface.
func (msg *message) Format(data *options.Data) (string, error) {
	return msg.opts.Format(data, data.Message)
}

// Message creates a placeholder that displays log message.
func Message(opts ...options.Option) Placeholder {
	opts = WithCommonOptions(
		options.PathFormat(options.NonePath, options.RelativePath),
	).Merge(opts...)

	return &message{
		CommonPlaceholder: NewCommonPlaceholder(MessagePlaceholderName, opts...),
	}
}
