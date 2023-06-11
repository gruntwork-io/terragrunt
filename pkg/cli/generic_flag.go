package cli

import (
	libflag "flag"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/pkg/env"
	"github.com/urfave/cli/v2"
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
	var err error
	valType := FlagType[T](new(flagType[T]))

	if flag.FlagValue, err = newGenericValue(valType, flag.Destination, flag.EnvVar); err != nil {
		return err
	}

	for _, name := range flag.Names() {
		set.Var(flag.FlagValue, name, flag.Usage)
	}
	return nil
}

// GetUsage returns the usage string for the flag.
func (flag *GenericFlag[T]) GetUsage() string {
	return flag.Usage
}

// GetEnvVars returns the env vars for this flag.
func (flag *GenericFlag[T]) GetEnvVars() []string {
	if flag.EnvVar == "" {
		return nil
	}
	return []string{flag.EnvVar}
}

// GetValue returns the flags value as string representation and an empty string if the flag takes no value at all.
func (flag *GenericFlag[T]) GetDefaultText() string {
	if flag.DefaultText == "" {
		return flag.FlagValue.GetDefaultText()
	}
	return flag.DefaultText
}

// String returns a readable representation of this value (for usage defaults).
func (flag *GenericFlag[T]) String() string {
	return cli.FlagStringer(flag)
}

// Names returns the names of the flag.
func (flag *GenericFlag[T]) Names() []string {
	return append([]string{flag.Name}, flag.Aliases...)
}

// -- generic Value
type genericValue[T comparable] struct {
	value       FlagType[T]
	defaultText string
	hasBeenSet  bool
}

func newGenericValue[T comparable](value FlagType[T], dest *T, envVar string) (FlagValue, error) {
	var nilPtr *T
	if dest == nilPtr {
		dest = new(T)
	}

	defaultText := value.Clone(dest).String()
	value = value.Clone(dest)

	if strVal, ok := env.LookupEnv(envVar); ok {
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
