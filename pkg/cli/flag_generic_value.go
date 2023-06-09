package cli

import (
	"github.com/gruntwork-io/terragrunt/errors"
)

type flagGenericValue[T comparable] struct {
	value       Type[T]
	defaultText string
	hasBeenSet  bool
}

func newFlagGenreicValue[T comparable](value Type[T], ptr *T, envVar string) (FlagValue, error) {
	var nilPtr *T
	if ptr == nilPtr {
		ptr = new(T)
	}

	defaultText := value.Init(ptr).String()
	value = value.Init(ptr)

	if strVal, ok := lookupEnv(envVar); ok {
		if err := value.Set(strVal); err != nil {
			return nil, err
		}
	}

	return &flagGenericValue[T]{
		value:       value,
		defaultText: defaultText,
	}, nil
}

func (flag *flagGenericValue[T]) Set(str string) error {
	if flag.hasBeenSet {
		return errors.Errorf("the flag set multiple times")
	}
	flag.hasBeenSet = true

	return flag.value.Set(str)
}

func (flag *flagGenericValue[T]) Get() any {
	return flag.value.Get()
}

func (flag *flagGenericValue[T]) IsBoolFlag() bool {
	_, ok := flag.Get().(bool)
	return ok
}

func (flag *flagGenericValue[T]) IsSet() bool {
	return flag.hasBeenSet
}

func (flag *flagGenericValue[T]) String() string {
	return flag.value.String()
}

func (flag *flagGenericValue[T]) DefaultText() string {
	if flag.IsBoolFlag() {
		return ""
	}
	return flag.defaultText
}
