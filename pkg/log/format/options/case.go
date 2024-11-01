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

var textCaseValues = CommonMapValues[CaseValue]{
	UpperCase:      "upper",
	LowerCase:      "lower",
	CapitalizeCase: "capitalize",
}

type CaseValue byte

type textCase struct {
	*CommonOption[CaseValue]
}

func (option *textCase) Evaluate(data *Data, str string) string {
	switch option.value {
	case UpperCase:
		return strings.ToUpper(str)
	case LowerCase:
		return strings.ToLower(str)
	case CapitalizeCase:
		return cases.Title(language.English, cases.Compact).String(str)
	}

	return str
}

func Case(value CaseValue) Option {
	return &textCase{
		CommonOption: NewCommonOption[CaseValue](CaseOptionName, value, textCaseValues),
	}
}
