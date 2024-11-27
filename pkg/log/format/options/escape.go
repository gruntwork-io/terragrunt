package options

import (
	"encoding/json"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// EscapeOptionName is the option name.
const EscapeOptionName = "escape"

const (
	NoneEscape EscapeValue = iota
	JSONEscape
)

var escapeList = NewMapValue(map[EscapeValue]string{ //nolint:gochecknoglobals
	JSONEscape: "json",
})

type EscapeValue byte

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
		return "", errors.New(err)
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
