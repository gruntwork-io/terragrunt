package options

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const CaseOptionName = "case"

const (
	NoneCase CaseValue = iota
	UpperCase
	LowerCase
	CapitalizeCase
)

var textCaseValues = CommonMapValues[CaseValue]{ //nolint:gochecknoglobals
	UpperCase:      "upper",
	LowerCase:      "lower",
	CapitalizeCase: "capitalize",
}

type CaseValue byte

type CaseOption struct {
	*CommonOption[CaseValue]
}

func (option *CaseOption) Evaluate(_ *Data, str string) (string, error) {
	switch option.value {
	case UpperCase:
		return strings.ToUpper(str), nil
	case LowerCase:
		return strings.ToLower(str), nil
	case CapitalizeCase:
		return cases.Title(language.English, cases.Compact).String(str), nil
	case NoneCase:
	}

	return str, nil
}

func Case(value CaseValue) Option {
	return &CaseOption{
		CommonOption: NewCommonOption[CaseValue](CaseOptionName, value, textCaseValues),
	}
}
