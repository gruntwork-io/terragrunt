// Package options represents a set of placeholders options.
package options

import (
	"reflect"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

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
	Format(data *Data, val any) (any, error)
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
func (opts Options) Format(data *Data, val any) (string, error) {
	var err error

	for _, opt := range opts {
		val, err = opt.Format(data, val)
		if val == "" || err != nil {
			return "", err
		}
	}

	return toString(val), nil
}
