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
	rightSpaces := int(option.value)

	if rightSpaces -= len(log.RemoveAllASCISeq(str)); rightSpaces < 1 {
		return str
	}

	return str + strings.Repeat(" ", rightSpaces)
}

func Width(value WidthValue) Option {
	return &width{
		CommonOption: NewCommonOption[WidthValue](WidthOptionName, value, value),
	}
}
