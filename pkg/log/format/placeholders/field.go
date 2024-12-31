package placeholders

import (
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

const (
	WorkDirKeyName     = "prefix"
	DownloadDirKeyName = "download-dir"
	TFPathKeyName      = "tf-path"
	TFCmdArgsKeyName   = "tf-command-args"
	TFCmdKeyName       = "tf-command"
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
