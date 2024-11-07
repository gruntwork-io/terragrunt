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

type WidthOption struct {
	*CommonOption[WidthValue]
}

func (option *WidthOption) Evaluate(data *Data, str string) string {
	WidthOption := int(option.value)
	if WidthOption == 0 {
		return str
	}

	strLen := len(log.RemoveAllASCISeq(str))

	if WidthOption < strLen {
		return str[:WidthOption]
	}

	return str + strings.Repeat(" ", WidthOption-strLen)
}

func Width(value WidthValue) Option {
	return &WidthOption{
		CommonOption: NewCommonOption[WidthValue](WidthOptionName, value, value),
	}
}
