package options

import (
	"encoding/json"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

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

func (option *EscapeOption) Format(_ *Data, str string) (string, error) {
	if option.value.Get() != JSONEscape {
		return str, nil
	}

	b, err := json.Marshal(str)
	if err != nil {
		return "", errors.New(err)
	}

	// Trim the beginning and trailing " character.
	return string(b[1 : len(b)-1]), nil
}

func Escape(val EscapeValue) Option {
	return &EscapeOption{
		CommonOption: NewCommonOption(EscapeOptionName, escapeList.Set(val)),
	}
}
