package placeholders

import (
	"fmt"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

// IntervalPlaceholderName is the placeholder name.
const IntervalPlaceholderName = "interval"

type intervalPlaceholder struct {
	baseTime time.Time
	*CommonPlaceholder
}

// Format implements `Placeholder` interface.
func (t *intervalPlaceholder) Format(data *options.Data) (string, error) {
	return t.opts.Format(data, fmt.Sprintf("%04d", time.Since(t.baseTime)/time.Second))
}

// Interval creates a placeholder that displays seconds that have passed since app started.
func Interval(opts ...options.Option) Placeholder {
	opts = WithCommonOptions().Merge(opts...)

	return &intervalPlaceholder{
		baseTime:          time.Now(),
		CommonPlaceholder: NewCommonPlaceholder(IntervalPlaceholderName, opts...),
	}
}
