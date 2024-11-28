package options

import (
	"strings"
)

// AlignOptionName is the option name.
const AlignOptionName = "align"

const (
	NoneAlign AlignValue = iota
	LeftAlign
	CenterAlign
	RightAlign
)

var alignList = NewMapValue(map[AlignValue]string{ //nolint:gochecknoglobals
	LeftAlign:   "left",
	CenterAlign: "center",
	RightAlign:  "right",
})

type AlignValue byte

type AlignOption struct {
	*CommonOption[AlignValue]
}

// Format implements `Option` interface.
func (option *AlignOption) Format(_ *Data, val any) (any, error) {
	str := toString(val)

	withoutSpaces := strings.TrimSpace(str)
	spaces := len(str) - len(withoutSpaces)

	switch option.value.Get() {
	case LeftAlign:
		return withoutSpaces + strings.Repeat(" ", spaces), nil
	case RightAlign:
		return strings.Repeat(" ", spaces) + withoutSpaces, nil
	case CenterAlign:
		twoSides := 2
		rightSpaces := (spaces - spaces%2) / twoSides
		leftSpaces := spaces - rightSpaces

		return strings.Repeat(" ", leftSpaces) + strings.TrimSpace(str) + strings.Repeat(" ", rightSpaces), nil
	case NoneAlign:
	}

	return str, nil
}

// Align creates the option to align text relative to the edges.
func Align(value AlignValue) Option {
	return &AlignOption{
		CommonOption: NewCommonOption(AlignOptionName, alignList.Set(value)),
	}
}
