package cli

import (
	"strings"
)

type flagSliceValue[T comparable] struct {
	values      *[]T
	valueType   Type[T]
	defaultText string
	valSep      string
	hasBeenSet  bool
}

func newFlagSliceValue[T comparable](valueType Type[T], ptr *[]T, envVar string, valSep string, splitter SplitterFunc) (FlagValue, error) {
	var nilPtr *[]T
	if ptr == nilPtr {
		ptr = new([]T)
	}

	defaultText := (&flagSliceValue[T]{values: ptr, valueType: valueType, valSep: valSep}).String()

	if envVal, ok := lookupEnv(envVar); ok && splitter != nil {
		value := flagSliceValue[T]{values: ptr, valueType: valueType}

		vals := splitter(envVal, valSep)
		for _, val := range vals {
			if err := value.Set(val); err != nil {
				return nil, err
			}
		}
	}

	return &flagSliceValue[T]{
		values:      ptr,
		valueType:   valueType,
		defaultText: defaultText,
		valSep:      valSep,
	}, nil
}

func (flag *flagSliceValue[T]) Set(str string) error {
	if !flag.hasBeenSet {
		flag.hasBeenSet = true

		// may contain a default value or an env var, so it needs to be cleared before the first setting.
		*flag.values = []T{}
	}

	value := flag.valueType.Init(new(T))
	if err := value.Set(str); err != nil {
		return err
	}

	*flag.values = append(*flag.values, value.Get().(T))
	return nil
}

func (flag *flagSliceValue[T]) DefaultText() string {
	if flag.IsBoolFlag() {
		return ""
	}
	return flag.defaultText
}

func (flag *flagSliceValue[T]) IsBoolFlag() bool {
	return false
}

func (flag *flagSliceValue[T]) IsSet() bool {
	return flag.hasBeenSet
}

func (flag *flagSliceValue[T]) Get() any {
	var vals []T

	for _, val := range *flag.values {
		vals = append(vals, val)
	}

	return vals
}

func (flag *flagSliceValue[T]) String() string {
	var vals []string

	for _, val := range *flag.values {
		vals = append(vals, flag.valueType.Init(&val).String())
	}

	return strings.Join(vals, flag.valSep)
}
