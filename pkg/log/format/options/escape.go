package options

import (
	"encoding/json"
)

// EscapeOptionName is the option name.
const EscapeOptionName = "escape"

const (
	// NoneEscape leaves the text as it is. It has no name in a format string.
	NoneEscape EscapeValue = iota
	// JSONEscape, `escape=json`, encodes the text as JSON and drops the first and
	// last characters, which for a string leaves its escaped content without quotes.
	JSONEscape
)

var escapeList = NewMapValue(map[EscapeValue]string{
	JSONEscape: "json",
})

// EscapeValue selects how [EscapeOption] escapes the text.
type EscapeValue byte

// EscapeOption is the `escape` option. It escapes the text for embedding in
// another format.
type EscapeOption struct {
	*CommonOption[EscapeValue]
}

// Format implements `Option` interface.
func (option *EscapeOption) Format(_ *Data, val any) (any, error) {
	if option.value.Get() != JSONEscape {
		return val, nil
	}

	jsonStr, err := json.Marshal(val)
	if err != nil {
		return "", err
	}

	// Trim the beginning and trailing " character.
	return string(jsonStr[1 : len(jsonStr)-1]), nil
}

// Escape creates the option to escape text.
func Escape(val EscapeValue) Option {
	return &EscapeOption{
		CommonOption: NewCommonOption(EscapeOptionName, escapeList.Set(val)),
	}
}
