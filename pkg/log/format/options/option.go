// Package options represents a set of placeholders options.
package options

import (
	"reflect"
	"strings"
	"unicode"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Constants for parsing options.
const (
	OptNameValueSep = "="
	OptSep          = ","
	OptStartSign    = "("
	OptEndSign      = ")"
)

const splitIntoNameAndValue = 2

// OptionValue contains the value of the option.
type OptionValue[T any] interface {
	// Configure parses and sets the value of the option.
	Parse(str string) error
	// Get returns the value of the option.
	Get() T
}

// Option represents a value modifier of placeholders.
type Option interface {
	// Name returns the name of the option.
	Name() string
	// Format formats the given string.
	Format(data *Data, str any) (any, error)
	// ParseValue parses and sets the value of the option.
	ParseValue(str string) error
}

// Data is a log entry data.
type Data struct {
	*log.Entry
	BaseDir        string
	DisableColors  bool
	RelativePather *RelativePather
	PresetColorFn  func() ColorValue
}

// Options is a set of Options.
type Options []Option

// Get returns the option with the given name.
func (opts Options) Get(name string) Option {
	for _, opt := range opts {
		if opt.Name() == name {
			return opt
		}
	}

	return nil
}

// Names returns names of the options.
func (opts Options) Names() []string {
	var names = make([]string, len(opts))

	for i, opt := range opts {
		names[i] = opt.Name()
	}

	return names
}

// Merge replaces options with the same name and adds new ones to the end.
func (opts Options) Merge(withOpts ...Option) Options {
	for i := range opts {
		for t := range withOpts {
			if reflect.TypeOf(opts[i]) == reflect.TypeOf(withOpts[t]) {
				opts[i] = withOpts[t]
				withOpts = append(withOpts[:t], withOpts[t+1:]...)

				break
			}
		}
	}

	return append(opts, withOpts...)
}

// Format returns the formatted value.
func (opts Options) Format(data *Data, str any) (string, error) {
	var err error

	for _, opt := range opts {
		str, err = opt.Format(data, str)
		if str == "" || err != nil {
			return "", err
		}
	}

	return toString(str), nil
}

// Configure parsers the given `str` to configure the `opts` and returns the rest of the given `str`.
//
// e.g. (color=green, case=upper) some-text" sets `color` option to `green`, `case` option to `upper` and returns " some-text".
func (opts Options) Configure(str string) (string, error) {
	if len(str) == 0 || !strings.HasPrefix(str, OptStartSign) {
		return str, nil
	}

	str = str[1:]

	for {
		var (
			ok  bool
			err error
		)

		if str, ok = nextOption(str); !ok {
			return str, nil
		}

		parts := strings.SplitN(str, OptNameValueSep, splitIntoNameAndValue)
		if len(parts) != splitIntoNameAndValue {
			return "", errors.New(NewInvalidOptionError(str))
		}

		name := strings.TrimSpace(parts[0])

		if name == "" {
			return "", errors.New(NewEmptyOptionNameError(str))
		}

		opt := opts.Get(name)
		if opt == nil {
			return "", errors.New(NewInvalidOptionNameError(name, opts))
		}

		if str, err = setOptionValue(opt, parts[1]); err != nil {
			return "", err
		}
	}
}

// setOptionValue parses the given `str` and sets the value for the given `opt` and returns the rest of the given `str`.
//
// e.g. "green, case=upper) some-text" sets "green" to the option and returns ", case=upper) some-text".
// e.g. "' quoted value ') some-text" sets " quoted value " to the option and returns ") some-text".
func setOptionValue(opt Option, str string) (string, error) {
	var quoteChar byte

	for index := range str {
		if quoteOpened(str[:index], &quoteChar) {
			continue
		}

		lastSign := str[index : index+1]
		if !strings.HasSuffix(lastSign, OptSep) && !strings.HasSuffix(lastSign, OptEndSign) {
			continue
		}

		val := strings.TrimSpace(str[:index])
		val = strings.Trim(val, "'")
		val = strings.Trim(val, "\"")

		if err := opt.ParseValue(val); err != nil {
			return "", errors.New(NewInvalidOptionValueError(opt, val, err))
		}

		return str[index:], nil
	}

	return str, nil
}

// nextOption returns true if the given `str` contains one more option
// and returns the given `str` without separator sign "," or ")".
//
// e.g. ",color=green) some-text" returns "color=green) some-text" and `true`.
// e.g. "(color=green) some-text" returns "color=green) some-text" and `true`.
// e.g. ") some-text"  returns " some-text" and `false`.
func nextOption(str string) (string, bool) {
	str = strings.TrimLeftFunc(str, unicode.IsSpace)

	switch {
	case strings.HasPrefix(str, OptEndSign):
		return str[1:], false
	case strings.HasPrefix(str, OptSep):
		return str[1:], true
	}

	return str, true
}

// quoteOpened returns true if the given `str` contains an unclosed quote.
//
// e.g. "%(content=' level" return `true`.
// e.g. "%(content=' level '" return `false`.
// e.g. "%(content=\" level" return `true`.
func quoteOpened(str string, quoteChar *byte) bool {
	strlen := len(str)

	if strlen == 0 {
		return false
	}

	char := str[strlen-1]

	if char == '"' || char == '\'' {
		if *quoteChar == 0 {
			*quoteChar = char
		} else if *quoteChar == char && (strlen < 2 || str[strlen-2] != '\\') {
			*quoteChar = 0
		}
	}

	return *quoteChar != 0
}
