package placeholders

import (
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

const (
	WorkDirKeyName     = "prefix"
	DownloadDirKeyName = "downloaddir"
	TFPathKeyName      = "tfpath"
)

type fieldPlaceholder struct {
	*CommonPlaceholder
}

func (field *fieldPlaceholder) Evaluate(data *options.Data) string {
	if val, ok := data.Fields[field.Name()]; ok {
		if val, ok := val.(string); ok {
			return field.opts.Evaluate(data, val)
		}
	}

	return ""
}

func Field(fieldName string, opts ...options.Option) Placeholder {
	opts = WithCommonOptions(
		options.PathFormat(options.NonePath),
	).Merge(opts...)

	return &fieldPlaceholder{
		CommonPlaceholder: NewCommonPlaceholder(fieldName, opts...),
	}
}

func init() {
	Registered.Add(
		Field(WorkDirKeyName, options.PathFormat(options.NonePath, options.RelativePath, options.RelativeModulePath, options.ModulePath)),
		Field(TFPathKeyName, options.PathFormat(options.NonePath, options.FilenamePath, options.DirectoryPath)),
	)
}
