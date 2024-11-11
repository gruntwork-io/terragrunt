package options

import (
	"strings"
)

const AlignOptionName = "align"

const (
	NoneAlign AlignValue = iota
	LeftAlign
	CenterAlign
	RightAlign
)

var alignValues = CommonMapValues[AlignValue]{ //nolint:gochecknoglobals
	LeftAlign:   "left",
	CenterAlign: "center",
	RightAlign:  "right",
}

type AlignValue byte

type AlignOption struct {
	*CommonOption[AlignValue]
}

func (option *AlignOption) Evaluate(_ *Data, str string) (string, error) {
	withoutSpaces := strings.TrimSpace(str)
	spaces := len(str) - len(withoutSpaces)

	switch option.value {
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

func Align(value AlignValue) Option {
	return &AlignOption{
		CommonOption: NewCommonOption(AlignOptionName, value, alignValues),
	}
}
