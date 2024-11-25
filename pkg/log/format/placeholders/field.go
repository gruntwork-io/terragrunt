package placeholders

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

const (
	WorkDirKeyName     = "prefix"
	DownloadDirKeyName = "download-dir"
	TFPathKeyName      = "tf-path"
	TFCmdArgsKeyName   = "tf-command-args"
)

type fieldPlaceholder struct {
	*CommonPlaceholder
}

// Format implements `Placeholder` interface.
func (field *fieldPlaceholder) Format(data *options.Data) (string, error) {
	if val, ok := data.Fields[field.Name()]; ok {
		switch val := val.(type) {
		case string:
			return field.opts.Format(data, val)
		case []string:
			return field.opts.Format(data, strings.Join(val, " "))
		}
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
