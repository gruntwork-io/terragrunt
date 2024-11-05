package options

import (
	"encoding/json"
	"fmt"
)

const EscapeOptionName = "escape"

const (
	NoneEscape EscapeValue = iota
	JSONEscape
)

var textEscapeValues = CommonMapValues[EscapeValue]{
	JSONEscape: "json",
}

type EscapeValue byte

type textEscape struct {
	*CommonOption[EscapeValue]
}

func (option *textEscape) Evaluate(data *Data, str string) string {
	if option.value != JSONEscape {
		return str
	}

	b, err := json.Marshal(str)
	if err != nil {
		fmt.Printf("Failed to marhsal %q, %v\n", str, err)
	}

	// Trim the beginning and trailing " character.
	return string(b[1 : len(b)-1])
}

func Escape(value EscapeValue) Option {
	return &textEscape{
		CommonOption: NewCommonOption[EscapeValue](EscapeOptionName, value, textEscapeValues),
	}
}
