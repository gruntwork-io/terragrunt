// Package options implements placeholders options.
package options

import (
	"reflect"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

type Options []Option

func (options Options) Get(name string) Option {
	for _, option := range options {
		if option.Name() == name {
			return option
		}
	}

	return nil
}

func (options Options) Merge(withOptions ...Option) Options {
	for i := range options {
		for t := range withOptions {
			if reflect.TypeOf(options[i]) == reflect.TypeOf(withOptions[t]) {
				options[i] = withOptions[t]
				withOptions = append(withOptions[:t], withOptions[t+1:]...)

				break
			}
		}
	}

	return append(options, withOptions...)
}

func (options Options) Evaluate(data *Data, str string) (string, error) {
	var err error

	for _, option := range options {
		str, err = option.Evaluate(data, str)
		if str == "" || err != nil {
			return "", err
		}
	}

	return str, nil
}

type OptionValues[Value any] interface {
	Parse(str string) (Value, error)
}

type Option interface {
	Name() string
	Evaluate(data *Data, str string) (string, error)
	ParseValue(str string) error
}

type Data struct {
	*log.Entry
	BaseDir        string
	DisableColors  bool
	RelativePather *RelativePather
	AutoColorFn    func() ColorValue
}
