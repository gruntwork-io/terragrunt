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

func (option *WidthOption) Evaluate(_ *Data, str string) (string, error) {
	WidthOption := int(option.value)
	if WidthOption == 0 {
		return str, nil
	}

	strLen := len(log.RemoveAllASCISeq(str))

	if WidthOption < strLen {
		return str[:WidthOption], nil
	}

	return str + strings.Repeat(" ", WidthOption-strLen), nil
}

func Width(value WidthValue) Option {
	return &WidthOption{
		CommonOption: NewCommonOption(WidthOptionName, value, value),
	}
}
