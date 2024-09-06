package log

import (
	"strings"

	"github.com/pkg/errors"
)

// These are different output formats.
const (
	//
	JSONFormat Format = iota
	JSONFormatIndent
	//
	PrettyFormat
	PrettyFormatNoColor
	//
	KeyValueFormat
)

var outputFormatNames = map[Format]string{
	JSONFormat:          "json",
	JSONFormatIndent:    "json-indent",
	PrettyFormat:        "pretty",
	PrettyFormatNoColor: "pretty-no-color",
	KeyValueFormat:      "key-value",
}

// Format type
type Format uint32

// ParseFormat takes a string and returns the Format constant.
func ParseFormat(str string) (Format, error) {
	for format, name := range outputFormatNames {
		if strings.EqualFold(name, str) {
			return format, nil
		}
	}

	return Format(0), errors.Errorf("not a valid log format: %q", str)
}

// String implements fmt.Stringer.
func (format Format) String() string {
	if name, ok := outputFormatNames[format]; ok {
		return name
	}

	return ""
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (format *Format) UnmarshalText(b []byte) error {
	fmt, err := ParseFormat(string(b))
	if err != nil {
		return err
	}

	*format = fmt

	return nil
}

// MarshalText implements encoding.MarshalText.
func (format Format) MarshalText() ([]byte, error) {
	if name := format.String(); name != "" {
		return []byte(name), nil
	}

	return nil, errors.Errorf("not a valid log format %q", format)
}
