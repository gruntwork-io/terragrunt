package placeholders

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

const TimePlaceholderName = "time"

type timePlaceholder struct {
	*CommonPlaceholder
}

func (t *timePlaceholder) Evaluate(data *options.Data) (string, error) {
	return t.opts.Evaluate(data, data.Time.String())
}

func Time(opts ...options.Option) Placeholder {
	opts = WithCommonOptions(
		options.TimeFormat(fmt.Sprintf("%s:%s:%s%s", options.Hour24Zero, options.MinZero, options.SecZero, options.MilliSec)),
	).Merge(opts...)

	return &timePlaceholder{
		CommonPlaceholder: NewCommonPlaceholder(TimePlaceholderName, opts...),
	}
}
