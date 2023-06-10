package cli

import (
	libflag "flag"

	"github.com/gruntwork-io/terragrunt/errors"
)

type GenericType interface {
	string | int | int64
}

type GenericFlag[T GenericType] struct {
	CommonFlag

	Name        string
	DefaultText string
	Usage       string
	Aliases     []string
	EnvVar      string

	Destination *T
}

// Apply applies Flag settings to the given flag set.
func (flag *GenericFlag[T]) Apply(set *libflag.FlagSet) error {
	flag.normalize()

	var err error
	valType := FlagType[T](new(flagType[T]))

	if flag.FlagValue, err = newGenreicValue(valType, flag.Destination, flag.EnvVar, false); err != nil {
		return err
	}
	return flag.CommonFlag.Apply(set)
}

func (flag *GenericFlag[T]) normalize() {
	flag.CommonFlag.Name = flag.Name
	flag.CommonFlag.DefaultText = flag.DefaultText
	flag.CommonFlag.Usage = flag.Usage
	flag.CommonFlag.Aliases = flag.Aliases
	flag.CommonFlag.EnvVar = flag.EnvVar
}

// -- generic Value
type genericValue[T comparable] struct {
	value       FlagType[T]
	defaultText string
	hasBeenSet  bool
}

func newGenreicValue[T comparable](value FlagType[T], dest *T, envVar string, negative bool) (FlagValue, error) {
	var nilPtr *T
	if dest == nilPtr {
		dest = new(T)
	}

	defaultText := value.Init(dest, false).String()
	value = value.Init(dest, negative)

	if strVal, ok := lookupEnv(envVar); ok {
		if err := value.Set(strVal); err != nil {
			return nil, err
		}
	}

	return &genericValue[T]{
		value:       value,
		defaultText: defaultText,
	}, nil
}

func (flag *genericValue[T]) Set(str string) error {
	if flag.hasBeenSet {
		return errors.Errorf("setting the flag multiple times")
	}
	flag.hasBeenSet = true

	return flag.value.Set(str)
}

func (flag *genericValue[T]) Get() any {
	return flag.value.Get()
}

func (flag *genericValue[T]) IsBoolFlag() bool {
	_, ok := flag.Get().(bool)
	return ok
}

func (flag *genericValue[T]) IsSet() bool {
	return flag.hasBeenSet
}

func (flag *genericValue[T]) String() string {
	return flag.value.String()
}

func (flag *genericValue[T]) GetDefaultText() string {
	if flag.IsBoolFlag() {
		return ""
	}
	return flag.defaultText
}
