package options

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// CaseOptionName is the option name.
const CaseOptionName = "case"

const (
	// NoneCase leaves the case as it is. It has no name in a format string.
	NoneCase CaseValue = iota
	// UpperCase, `case=upper`, converts the text to upper case.
	UpperCase
	// LowerCase, `case=lower`, converts the text to lower case.
	LowerCase
	// CapitalizeCase, `case=capitalize`, upper-cases the first letter of each word
	// and lower-cases the rest.
	CapitalizeCase
)

var caseList = NewMapValue(map[CaseValue]string{
	UpperCase:      "upper",
	LowerCase:      "lower",
	CapitalizeCase: "capitalize",
})

// CaseValue selects how [CaseOption] changes the case of the text.
type CaseValue byte

// CaseOption is the `case` option. It changes the letter case of the text.
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
