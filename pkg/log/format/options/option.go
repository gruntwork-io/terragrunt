// Package options represents a set of placeholders options.
package options

import (
	"reflect"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// OptionValue contains the value of the option.
type OptionValue[T any] interface {
	Parse(str string) error
	Get() T
}

// Option represents a value modifier of placeholders.
type Option interface {
	// Name returns the name of the option.
	Name() string
	// Format formats the given string.
	Format(data *Data, str string) (string, error)
	// ParseValue parses and sets the value of the option.
	ParseValue(str string) error
}

type Data struct {
	*log.Entry
	BaseDir        string
	DisableColors  bool
	RelativePather *RelativePather
	AutoColorFn    func() ColorValue
}

type Options []Option

func (opts Options) Get(name string) Option {
	for _, opt := range opts {
		if opt.Name() == name {
			return opt
		}
	}

	return nil
}

func (opts Options) Names() []string {
	var names = make([]string, len(opts))

	for i, opt := range opts {
		names[i] = opt.Name()
	}

	return names
}

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

func (opts Options) Format(data *Data, str string) (string, error) {
	var err error

	for _, opt := range opts {
		str, err = opt.Format(data, str)
		if str == "" || err != nil {
			return "", err
		}
	}

	return str, nil
}
