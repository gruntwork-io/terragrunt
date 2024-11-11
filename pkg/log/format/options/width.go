package options

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const WidthOptionName = "width"

type WidthOption struct {
	*CommonOption[int]
}

func (option *WidthOption) Format(_ *Data, str string) (string, error) {
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

func Width(val int) Option {
	return &WidthOption{
		CommonOption: NewCommonOption(WidthOptionName, NewIntValue(val)),
	}
}
