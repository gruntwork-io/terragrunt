package placeholders

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

// TimePlaceholderName is the placeholder name. Example `%time()`.
const TimePlaceholderName = "time"

type timePlaceholder struct {
	*CommonPlaceholder
}

// Format implements `Placeholder` interface.
func (t *timePlaceholder) Format(data *options.Data) (string, error) {
	return t.opts.Format(data, data.Time.String())
}

// Time creates a placeholder that displays log time.
func Time(opts ...options.Option) Placeholder {
	opts = WithCommonOptions(
		options.TimeFormat(fmt.Sprintf("%s:%s:%s%s", options.Hour24Zero, options.MinZero, options.SecZero, options.MilliSec)),
	).Merge(opts...)

	return &timePlaceholder{
		CommonPlaceholder: NewCommonPlaceholder(TimePlaceholderName, opts...),
	}
}
