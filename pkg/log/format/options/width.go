package options

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// WidthOptionName is the option name.
const WidthOptionName = "width"

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

	strLen := len(log.RemoveAllASCISeq(str))

	if width < strLen {
		return str[:width], nil
	}

	return str + strings.Repeat(" ", width-strLen), nil
}

// Width creates the option to set the column width.
func Width(val int) Option {
	return &WidthOption{
		CommonOption: NewCommonOption(WidthOptionName, NewIntValue(val)),
	}
}
