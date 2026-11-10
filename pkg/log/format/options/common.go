package options

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// CommonOption is the name and value every option embeds. Each option type
// supplies its own Format.
type CommonOption[T comparable] struct {
	value OptionValue[T]
	name  string
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

// StringValue is an option value that accepts any text.
type StringValue string

// NewStringValue returns a [StringValue] set to val.
func NewStringValue(val string) *StringValue {
	v := StringValue(val)
	return &v
}

// Parse sets the value to str.
func (val *StringValue) Parse(str string) error {
	*val = StringValue(str)

	return nil
}

// Get returns the value.
func (val *StringValue) Get() string {
	return string(*val)
}

// IntValue is an option value that accepts a decimal integer.
type IntValue int

// NewIntValue returns an [IntValue] set to val.
func NewIntValue(val int) *IntValue {
	v := IntValue(val)
	return &v
}

// Parse sets the value to the integer in str.
//
// Returns an error when str is not a decimal integer.
func (val *IntValue) Parse(str string) error {
	v, err := strconv.Atoi(str)
	if err != nil {
		return fmt.Errorf("incorrect option value: %s", str)
	}

	*val = IntValue(v)

	return nil
}

// Get returns the value.
func (val *IntValue) Get() int {
	return int(*val)
}

// MapValue is an option value chosen by name from a fixed set.
type MapValue[T comparable] struct {
	list  map[T]string
	value T
}

// NewMapValue returns a [MapValue] that accepts the names in list, each selecting
// its key. The selected value starts as the zero value of T.
func NewMapValue[T comparable](list map[T]string) MapValue[T] {
	return MapValue[T]{
		list: list,
	}
}

// Get returns the selected value.
func (val *MapValue[T]) Get() T {
	return val.value
}

// Set returns a copy of val that selects v.
func (val MapValue[T]) Set(v T) *MapValue[T] {
	val.value = v

	return &val
}

// Parse selects the value named str.
//
// Returns an error listing the accepted names when str is not one of them.
func (val *MapValue[T]) Parse(str string) error {
	for v, name := range val.list {
		if name == str {
			val.value = v
			return nil
		}
	}

	list := slices.Sorted(maps.Values(val.list))

	return fmt.Errorf("available values: %s", strings.Join(list, ","))
}

// Filter returns a [MapValue] that accepts only the names of vals that val
// accepts. Its selected value is the zero value of T.
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
