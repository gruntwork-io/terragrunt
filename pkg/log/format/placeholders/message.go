package placeholders

import (
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

const MessagePlaceholderName = "msg"

type message struct {
	*CommonPlaceholder
}

func (msg *message) Evaluate(data *options.Data) string {
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

func init() {
	Registered.Add(Message())
}
