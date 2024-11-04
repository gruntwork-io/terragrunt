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

var alignValues = CommonMapValues[AlignValue]{
	LeftAlign:   "left",
	CenterAlign: "center",
	RightAlign:  "right",
}

type AlignValue byte

type align struct {
	*CommonOption[AlignValue]
}

func (option *align) Evaluate(data *Data, str string) string {
	withoutSpaces := strings.TrimSpace(str)
	spaces := len(str) - len(withoutSpaces)

	switch option.value {
	case LeftAlign:
		return withoutSpaces + strings.Repeat(" ", spaces)
	case RightAlign:
		return strings.Repeat(" ", spaces) + withoutSpaces
	case CenterAlign:
		rightSpaces := (spaces - spaces%2) / 2
		leftSpaces := spaces - rightSpaces

		return strings.Repeat(" ", leftSpaces) + strings.TrimSpace(str) + strings.Repeat(" ", rightSpaces)
	}

	return str
}

func Align(value AlignValue) Option {
	return &align{
		CommonOption: NewCommonOption[AlignValue](AlignOptionName, value, alignValues),
	}
}
