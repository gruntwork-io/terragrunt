package placeholders

import (
	"fmt"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

const IntervalPlaceholderName = "interval"

type intervalPlaceholder struct {
	baseTime time.Time
	*CommonPlaceholder
}

func (t *intervalPlaceholder) Evaluate(data *options.Data) string {
	return t.opts.Evaluate(data, fmt.Sprintf("%04d", time.Since(t.baseTime)/time.Second))
}

func Interval(opts ...options.Option) Placeholder {
	opts = WithCommonOptions().Merge(opts...)

	return &intervalPlaceholder{
		baseTime:          time.Now(),
		CommonPlaceholder: NewCommonPlaceholder(IntervalPlaceholderName, opts...),
	}
}

func init() {
	Registered.Add(Interval())
}
