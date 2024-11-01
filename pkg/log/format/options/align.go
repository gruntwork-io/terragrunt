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
	leftSpaces := len(str) - len(strings.TrimLeft(str, " "))
	rightSpaces := len(str) - len(strings.TrimRight(str, " "))

	switch option.value {
	case LeftAlign:
		return strings.TrimLeft(str, " ") + strings.Repeat(" ", leftSpaces)
	case RightAlign:
		return strings.Repeat(" ", rightSpaces) + strings.TrimRight(str, " ")
	case CenterAlign:
		spaces := leftSpaces + rightSpaces

		rightSpaces = (spaces - spaces%2) / 2
		leftSpaces = spaces - rightSpaces

		return strings.Repeat(" ", leftSpaces) + strings.TrimSpace(str) + strings.Repeat(" ", rightSpaces)
	}

	return str
}

func Align(value AlignValue) Option {
	return &align{
		CommonOption: NewCommonOption[AlignValue](AlignOptionName, value, alignValues),
	}
}
