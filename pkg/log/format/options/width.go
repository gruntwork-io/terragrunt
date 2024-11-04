package options

import (
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const WidthOptionName = "width"

type WidthValue int

func (val WidthValue) Parse(str string) (WidthValue, error) {
	if val, err := strconv.Atoi(str); err == nil {
		return WidthValue(val), nil
	}

	return val, errors.Errorf("incorrect option value: %s", str)
}

type width struct {
	*CommonOption[WidthValue]
}

func (option *width) Evaluate(data *Data, str string) string {
	width := int(option.value)
	if width == 0 {
		return str
	}

	strLen := len(log.RemoveAllASCISeq(str))

	if width < strLen {
		return str[:width]
	}

	return str + strings.Repeat(" ", width-strLen)
}

func Width(value WidthValue) Option {
	return &width{
		CommonOption: NewCommonOption[WidthValue](WidthOptionName, value, value),
	}
}
