package options

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// CaseOptionName is the option name.
const CaseOptionName = "case"

const (
	NoneCase CaseValue = iota
	UpperCase
	LowerCase
	CapitalizeCase
)

var caseList = NewMapValue(map[CaseValue]string{ //nolint:gochecknoglobals
	UpperCase:      "upper",
	LowerCase:      "lower",
	CapitalizeCase: "capitalize",
})

type CaseValue byte

type CaseOption struct {
	*CommonOption[CaseValue]
}

// Format implements `Option` interface.
func (option *CaseOption) Format(_ *Data, val any) (any, error) {
	str := toString(val)

	switch option.value.Get() {
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

// Case creates the option to change the case of text.
func Case(value CaseValue) Option {
	return &CaseOption{
		CommonOption: NewCommonOption(CaseOptionName, caseList.Set(value)),
	}
}
