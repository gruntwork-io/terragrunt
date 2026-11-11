package options

import (
	"strings"
)

// AlignOptionName is the option name.
const AlignOptionName = "align"

const (
	// NoneAlign leaves the text where it is. It has no name in a format string.
	NoneAlign AlignValue = iota
	// LeftAlign, `align=left`, moves all surrounding spaces after the text.
	LeftAlign
	// CenterAlign, `align=center`, splits the surrounding spaces around the text,
	// putting the odd one before it.
	CenterAlign
	// RightAlign, `align=right`, moves all surrounding spaces before the text.
	RightAlign
)

var alignList = NewMapValue(map[AlignValue]string{
	LeftAlign:   "left",
	CenterAlign: "center",
	RightAlign:  "right",
})

// AlignValue selects where [AlignOption] puts the text among its leading and
// trailing spaces.
type AlignValue byte

// AlignOption is the `align` option. It repositions the text within the spaces
// around it, such as the padding [WidthOption] adds.
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

		return strings.Repeat(
			" ",
			leftSpaces,
		) + strings.TrimSpace(
			str,
		) + strings.Repeat(
			" ",
			rightSpaces,
		), nil
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
