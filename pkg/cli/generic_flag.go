package cli

import (
	libflag "flag"
	"fmt"
	"strconv"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/urfave/cli/v2"
)

type GenericType interface {
	string | int | int64
}

type GenericFlag[T GenericType] struct {
	flag

	// The name of the flag.
	Name string
	// The default value of the flag to display in the help, if it is empty, the value is taken from `Destination`.
	DefaultText string
	// A short usage description to display in help.
	Usage string
	// Aliases are usually used for the short flag name, like `-h`.
	Aliases []string
	// The name of the env variable that is parsed and assigned to `Destination` before the flag value.
	EnvVar string
	// The action to execute when flag is specified
	Action ActionFunc
	// The pointer to which the value of the flag or env var is assigned.
	// It also uses as the default value displayed in the help.
	Destination *T
}

// Apply applies Flag settings to the given flag set.
func (flag *GenericFlag[T]) Apply(set *libflag.FlagSet) error {
	var err error

	valType := FlagType[T](new(genericType[T]))

	if flag.FlagValue, err = newGenericValue(valType, flag.LookupEnv(flag.EnvVar), flag.Destination); err != nil {
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

// GetDefaultText returns the flags value as string representation and an empty string if the flag takes no value at all.
func (flag *GenericFlag[T]) GetDefaultText() string {
	if flag.DefaultText == "" && flag.FlagValue != nil {
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

// RunAction implements ActionableFlag.RunAction
func (flag *GenericFlag[T]) RunAction(ctx *Context) error {
	if flag.Action != nil {
		return flag.Action(ctx)
	}

	return nil
}

// -- generic Value
type genericValue[T comparable] struct {
	value       FlagType[T]
	defaultText string
	hasBeenSet  bool
}

func newGenericValue[T comparable](value FlagType[T], envValue *string, dest *T) (FlagValue, error) {
	var nilPtr *T
	if dest == nilPtr {
		dest = new(T)
	}

	defaultText := value.Clone(dest).String()
	value = value.Clone(dest)

	if envValue != nil {
		if err := value.Set(*envValue); err != nil {
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
	if flag.value == nil {
		return ""
	}
	return flag.value.String()
}

func (flag *genericValue[T]) GetDefaultText() string {
	if val, ok := flag.Get().(bool); ok && !val {
		return ""
	}
	return flag.defaultText
}

// -- generic Type
type genericType[T comparable] struct {
	dest *T
}

func (val *genericType[T]) Clone(dest *T) FlagType[T] {
	return &genericType[T]{dest: dest}
}

func (val *genericType[T]) Set(str string) error {
	switch dest := (interface{})(val.dest).(type) {
	case *string:
		*dest = str

	case *bool:
		v, err := strconv.ParseBool(str)
		if err != nil {
			return errors.Errorf("error parse: %w", err)
		}
		*dest = v

	case *int:
		v, err := strconv.ParseInt(str, 0, strconv.IntSize)
		if err != nil {
			return errors.Errorf("error parse: %w", err)
		}
		*dest = int(v)

	case *int64:
		v, err := strconv.ParseInt(str, 0, 64)
		if err != nil {
			return errors.Errorf("error parse: %w", err)
		}
		*dest = v

	default:
		return errors.Errorf("flag type %T is undefined", dest)
	}

	return nil
}

func (val *genericType[T]) Get() any { return *val.dest }

// String returns a readable representation of this value
func (val *genericType[T]) String() string {
	if *val.dest == *new(T) {
		return ""
	}

	format := "%v"
	if _, ok := val.Get().(bool); ok {
		format = "%t"
	}

	return fmt.Sprintf(format, *val.dest)
}
