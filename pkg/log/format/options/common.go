package options

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"golang.org/x/exp/maps"
)

type CommonOption[T comparable] struct {
	name  string
	value OptionValue[T]
}

// NewCommonOption creates a new Common option.
func NewCommonOption[T comparable](name string, value OptionValue[T]) *CommonOption[T] {
	return &CommonOption[T]{
		name:  name,
		value: value,
	}
}

// String implements `fmt.Stringer` interface.
func (option *CommonOption[T]) String() string {
	return fmt.Sprintf("%v", option.value.Get())
}

// Name implements `Option` interface.
func (option *CommonOption[T]) Name() string {
	return option.name
}

// Format implements `Option` interface.
func (option *CommonOption[T]) Format(_ *Data, str string) (string, error) {
	return str, nil
}

// ParseValue implements `Option` interface.
func (option *CommonOption[T]) ParseValue(str string) error {
	return option.value.Parse(str)
}

type StringValue string

func NewStringValue(val string) *StringValue {
	v := StringValue(val)
	return &v
}

func (val *StringValue) Parse(str string) error {
	*val = StringValue(str)

	return nil
}

func (val *StringValue) Get() string {
	return string(*val)
}

type IntValue int

func NewIntValue(val int) *IntValue {
	v := IntValue(val)
	return &v
}

func (val *IntValue) Parse(str string) error {
	v, err := strconv.Atoi(str)
	if err != nil {
		return errors.Errorf("incorrect option value: %s", str)
	}

	*val = IntValue(v)

	return nil
}

func (val *IntValue) Get() int {
	return int(*val)
}

type MapValue[T comparable] struct {
	list  map[T]string
	value T
}

func NewMapValue[T comparable](list map[T]string) MapValue[T] {
	return MapValue[T]{
		list: list,
	}
}

func (val *MapValue[T]) Get() T {
	return val.value
}

func (val MapValue[T]) Set(v T) *MapValue[T] {
	val.value = v

	return &val
}

func (val *MapValue[T]) Parse(str string) error {
	for v, name := range val.list {
		if name == str {
			val.value = v
			return nil
		}
	}

	list := maps.Values(val.list)
	sort.Strings(list)

	return errors.Errorf("available values: %s", strings.Join(list, ","))
}

func (val *MapValue[T]) Filter(vals ...T) MapValue[T] {
	newVal := MapValue[T]{
		list: make(map[T]string, len(vals)),
	}

	for _, v := range vals {
		if name, ok := val.list[v]; ok {
			newVal.list[v] = name
		}
	}

	return newVal
}
