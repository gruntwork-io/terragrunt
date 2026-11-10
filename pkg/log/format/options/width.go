package options

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// WidthOptionName is the option name.
const WidthOptionName = "width"

// WidthOption is the `width` option. It pads the text with trailing spaces or
// truncates it to fill exactly its value in terminal columns, ignoring ANSI escape
// sequences. A width of zero leaves the text as it is.
type WidthOption struct {
	*CommonOption[int]
}

// Format implements `Option` interface.
func (option *WidthOption) Format(_ *Data, val any) (any, error) {
	str := toString(val)

	width := option.value.Get()
	if width == 0 {
		return str, nil
	}

	visibleLen := log.VisibleLength(str)

	if width < visibleLen {
		return log.TruncateVisible(str, width), nil
	}

	return str + strings.Repeat(" ", width-visibleLen), nil
}

// Width creates the option to set the column width.
func Width(val int) Option {
	return &WidthOption{
		CommonOption: NewCommonOption(WidthOptionName, NewIntValue(val)),
	}
}
