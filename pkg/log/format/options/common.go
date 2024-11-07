package options

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"golang.org/x/exp/maps"
)

type CommonOption[T comparable] struct {
	name   string
	value  T
	values OptionValues[T]
}

func NewCommonOption[T comparable](name string, value T, values OptionValues[T]) *CommonOption[T] {
	return &CommonOption[T]{
		name:   name,
		value:  value,
		values: values,
	}
}

func (option *CommonOption[T]) Name() string {
	return option.name
}

func (option *CommonOption[T]) Value() T {
	return option.value
}

func (option *CommonOption[T]) String() string {
	return fmt.Sprintf("%v", option.value)
}

func (option *CommonOption[T]) Evaluate(_ *Data, str string) (string, error) {
	return str, nil
}

func (option *CommonOption[T]) ParseValue(str string) error {
	val, err := option.values.Parse(str)
	if err != nil {
		return err
	}

	option.value = val

	return nil
}

type CommonMapValues[T comparable] map[T]string

func (valNames CommonMapValues[T]) Parse(str string) (T, error) {
	for val, name := range valNames {
		if name == str {
			return val, nil
		}
	}

	t := new(T)

	return *t, errors.Errorf("available values: %s", strings.Join(maps.Values(valNames), ","))
}

func (valNames CommonMapValues[T]) Filter(vals ...T) CommonMapValues[T] {
	filtered := make(map[T]string, len(vals))

	for _, val := range vals {
		if name, ok := valNames[val]; ok {
			filtered[val] = name
		}
	}

	return filtered
}
