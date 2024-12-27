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
	// Parse parses and sets the value of the option.
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

// Parse parsers the given `str` to configure the `opts` and returns the offset index.
func (opts Options) Parse(str string) (string, error) {
	str, ok := isNext(str)
	if !ok {
		return str, nil
	}

	parts := strings.SplitN(str, OptNameValueSep, splitIntoNameAndValue)
	if len(parts) != splitIntoNameAndValue {
		return "", errors.Errorf("invalid option %q", str)
	}

	name := strings.TrimSpace(parts[0])
	if name == "" {
		return "", errors.New("empty option name")
	}

	opt := opts.Get(name)
	if opt == nil {
		return "", errors.Errorf("invalid option name %q, available names: %s", name, strings.Join(opts.Names(), ","))
	}

	str = parts[1]

	var quoted byte

	for index := range str {
		// Skip quoted text, e.g. `%(content='level()')`.
		if isQuoted(str[:index], &quoted) {
			continue
		}

		if !strings.HasSuffix(str[:index+1], OptSep) && !strings.HasSuffix(str[:index+1], OptEndSign) {
			continue
		}

		val := strings.TrimSpace(str[:index])
		val = strings.Trim(val, "'")
		val = strings.Trim(val, "\"")

		if err := opt.ParseValue(val); err != nil {
			return "", errors.Errorf("invalid value %q for option %q: %w", val, opt.Name(), err)
		}

		return opts.Parse(OptStartSign + str[index:])
	}

	return "", errors.Errorf("invalid option %q", str)
}

func isNext(str string) (string, bool) {
	if len(str) == 0 || !strings.HasPrefix(str, OptStartSign) {
		return str, false
	}

	str = strings.TrimLeftFunc(str[1:], unicode.IsSpace)

	switch {
	case strings.HasPrefix(str, OptEndSign):
		return str[1:], false
	case strings.HasPrefix(str, OptSep):
		return str[1:], true
	}

	return str, true
}

func isQuoted(str string, quoted *byte) bool {
	strlen := len(str)

	if strlen == 0 {
		return false
	}

	char := str[strlen-1]

	if char == '"' || char == '\'' {
		if *quoted == 0 {
			*quoted = char
		} else if *quoted == char && (strlen < 2 || str[strlen-2] != '\\') {
			*quoted = 0
		}
	}

	return *quoted != 0
}
