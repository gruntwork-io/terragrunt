package cli

import (
	libflag "flag"
	"fmt"
	"strconv"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/urfave/cli/v2"
)

// GenericFlag implements Flag
var _ Flag = new(GenericFlag[string])

type GenericType interface {
	string | int | int64 | uint
}

type GenericFlag[T GenericType] struct {
	flag

	// Action is a function that is called when the flag is specified. It is executed only after all command flags have been parsed.
	Action FlagActionFunc[T]

	// Setter allows to set a value to any type by calling its `func(bool) error` function.
	Setter FlagSetterFunc[T]

	// Destination is a pointer to which the value of the flag or env var is assigned.
	Destination *T

	// Name is the name of the flag.
	Name string

	// DefaultText is the default value of the flag to display in the help, if it is empty, the value is taken from `Destination`.
	DefaultText string

	// Usage is a short usage description to display in help.
	Usage string

	// Aliases are usually used for the short flag name, like `-h`.
	Aliases []string

	// EnvVars are the names of the env variables that are parsed and assigned to `Destination` before the flag value.
	EnvVars []string

	// Hidden hides the flag from the help.
	Hidden bool
}

// Apply applies Flag settings to the given flag set.
func (flag *GenericFlag[T]) Apply(set *libflag.FlagSet) error {
	if flag.FlagValue != nil {
		return ApplyFlag(flag, set)
	}

	if flag.Destination == nil {
		flag.Destination = new(T)
	}

	valueType := &genericVar[T]{dest: flag.Destination}
	value := newGenericValue(valueType, flag.Setter)

	flag.FlagValue = &flagValue{
		value:            value,
		initialTextValue: value.String(),
	}

	return ApplyFlag(flag, set)
}

// GetHidden returns true if the flag should be hidden from the help.
func (flag *GenericFlag[T]) GetHidden() bool {
	return flag.Hidden
}

// GetUsage returns the usage string for the flag.
func (flag *GenericFlag[T]) GetUsage() string {
	return flag.Usage
}

// GetEnvVars implements `cli.Flag` interface.
func (flag *GenericFlag[T]) GetEnvVars() []string {
	return flag.EnvVars
}

// GetDefaultText returns the flags value as string representation and an empty string if the flag takes no value at all.
func (flag *GenericFlag[T]) GetDefaultText() string {
	if flag.DefaultText == "" && flag.FlagValue != nil {
		return flag.GetInitialTextValue()
	}

	return flag.DefaultText
}

// String returns a readable representation of this value (for usage defaults).
func (flag *GenericFlag[T]) String() string {
	return cli.FlagStringer(flag)
}

// Names returns the names of the flag.
func (flag *GenericFlag[T]) Names() []string {
	if flag.Name == "" {
		return flag.Aliases
	}

	return append([]string{flag.Name}, flag.Aliases...)
}

// RunAction implements ActionableFlag.RunAction
func (flag *GenericFlag[T]) RunAction(ctx *Context) error {
	dest := flag.Destination
	if dest == nil {
		dest = new(T)
	}

	if flag.Action != nil {
		return flag.Action(ctx, *dest)
	}

	return nil
}

var _ = Value(new(genericValue[string]))

// -- generic Value
type genericValue[T comparable] struct {
	setter FlagSetterFunc[T]
	value  FlagVariable[T]
}

func newGenericValue[T comparable](value FlagVariable[T], setter FlagSetterFunc[T]) *genericValue[T] {
	return &genericValue[T]{
		setter: setter,
		value:  value,
	}
}

func (flag *genericValue[T]) Reset() {}

func (flag *genericValue[T]) Set(str string) error {
	if err := flag.value.Set(str); err != nil {
		return err
	}

	if flag.setter != nil {
		return flag.setter(flag.Get().(T))
	}

	return nil
}

func (flag *genericValue[T]) Get() any {
	return flag.value.Get()
}

func (flag *genericValue[T]) String() string {
	return flag.value.String()
}

var _ = FlagVariable[string](new(genericVar[string]))

// -- generic Type
type genericVar[T comparable] struct {
	dest *T
}

func (val *genericVar[T]) Clone(dest *T) FlagVariable[T] {
	if dest == nil {
		dest = new(T)
	}

	return &genericVar[T]{dest: dest}
}

func (val *genericVar[T]) Set(str string) error {
	if val.dest == nil {
		val.dest = new(T)
	}

	switch dest := (any)(val.dest).(type) {
	case *string:
		*dest = str

	case *bool:
		v, err := strconv.ParseBool(str)
		if err != nil {
			return errors.New(InvalidValueError{underlyingError: err, msg: `must be one of: "0", "1", "f", "t", "false", "true"`})
		}

		*dest = v

	case *int:
		v, err := strconv.ParseInt(str, 0, strconv.IntSize)
		if err != nil {
			return errors.New(InvalidValueError{underlyingError: err, msg: "must be 32-bit integer"})
		}

		*dest = int(v)

	case *uint:
		v, err := strconv.ParseUint(str, 10, 64)
		if err != nil {
			return errors.New(InvalidValueError{underlyingError: err, msg: "must be 32-bit unsigned integer"})
		}

		*dest = uint(v)

	case *int64:
		v, err := strconv.ParseInt(str, 0, 64)
		if err != nil {
			return errors.New(InvalidValueError{underlyingError: err, msg: "must be 64-bit integer"})
		}

		*dest = v

	default:
		return errors.Errorf("flag type %T is undefined", dest)
	}

	return nil
}

func (val *genericVar[T]) Get() any {
	if val.dest == nil {
		return *new(T)
	}

	return *val.dest
}

// String returns a readable representation of this value
func (val *genericVar[T]) String() string {
	if val.dest == nil {
		return ""
	}

	format := "%v"
	if _, ok := val.Get().(bool); ok {
		format = "%t"
	}

	return fmt.Sprintf(format, *val.dest)
}
