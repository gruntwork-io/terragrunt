package placeholders

import (
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

const (
	// WorkDirKeyName is the log field and `%prefix` placeholder for the
	// directory of the unit the entry is about.
	WorkDirKeyName = "prefix"
	// TFPathKeyName is the log field and `%tf-path` placeholder for the tofu or
	// terraform binary that produced the entry.
	TFPathKeyName = "tf-path"
	// TFCmdArgsKeyName is the log field and `%tf-command-args` placeholder for
	// the arguments of the tofu or terraform command that produced the entry.
	TFCmdArgsKeyName = "tf-command-args"
	// TFCmdKeyName is the log field and `%tf-command` placeholder for the name
	// of the tofu or terraform command that produced the entry, such as `plan`.
	TFCmdKeyName = "tf-command"

	// Terragrunt Provider Cache Server fields.

	// CacheServerURLKeyName is the log field for the request URI the provider
	// cache server handled, with download tokens redacted.
	CacheServerURLKeyName = "url"
	// CacheServerStatusKeyName is the log field for the HTTP status the provider
	// cache server answered a request with.
	CacheServerStatusKeyName = "status"
)

type fieldPlaceholder struct {
	*CommonPlaceholder
}

// Format implements `Placeholder` interface.
func (field *fieldPlaceholder) Format(data *options.Data) (string, error) {
	if val, ok := data.Fields[field.Name()]; ok {
		return field.opts.Format(data, val)
	}

	return "", nil
}

// Field creates a placeholder that displays log field value.
func Field(fieldName string, opts ...options.Option) Placeholder {
	opts = WithCommonOptions(
		options.PathFormat(options.NonePath),
	).Merge(opts...)

	return &fieldPlaceholder{
		CommonPlaceholder: NewCommonPlaceholder(fieldName, opts...),
	}
}
