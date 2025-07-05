package cli

import (
	libflag "flag"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
)

// SliceFlag implements Flag
var _ Flag = new(SliceFlag[string])

var (
	SliceFlagEnvVarSep = ","
)

type SliceFlagType interface {
	GenericType
}

// SliceFlag is a multiple flag.
type SliceFlag[T SliceFlagType] struct {
	flag

	// Action is a function that is called when the flag is specified. It is executed only after all command flags have been parsed.
	Action FlagActionFunc[[]T]

	// Setter represents the function that is called when the flag is specified.
	Setter FlagSetterFunc[T]

	// Destination is a pointer to which the value of the flag or env var is assigned.
	Destination *[]T

	// Splitter represents the function that is called when the flag is specified.
	Splitter SplitterFunc

	// Name is the name of the flag.
	Name string

	// DefaultText is the default value of the flag to display in the help, if it is empty, the value is taken from `Destination`.
	DefaultText string

	// Usage is a short usage description to display in help.
	Usage string

	// EnvVarSep is the separator used to split the env var value.
	EnvVarSep string

	// Aliases are usually used for the short flag name, like `-h`.
	Aliases []string

	// EnvVars are the names of the env variables that are parsed and assigned to `Destination` before the flag value.
	EnvVars []string

	// Hidden hides the flag from the help.
	Hidden bool
}

// Apply applies Flag settings to the given flag set.
func (flag *SliceFlag[T]) Apply(set *libflag.FlagSet) error {
	if flag.FlagValue != nil {
		return ApplyFlag(flag, set)
	}

	if flag.Destination == nil {
		flag.Destination = new([]T)
	}

	if flag.Splitter == nil {
		flag.Splitter = FlagSplitter
	}

	if flag.EnvVarSep == "" {
		flag.EnvVarSep = SliceFlagEnvVarSep
	}

	if flag.LookupEnvFunc == nil {
		flag.LookupEnvFunc = func(key string) []string {
			if val, ok := os.LookupEnv(key); ok {
				return flag.Splitter(val, flag.EnvVarSep)
			}

			return nil
		}
	}

	valueType := FlagVariable[T](new(genericVar[T]))
	value := newSliceValue(valueType, flag.EnvVarSep, flag.Destination, flag.Setter)

	flag.FlagValue = &flagValue{
		multipleSet:      true,
		value:            value,
		initialTextValue: value.String(),
	}

	return ApplyFlag(flag, set)
}

// GetHidden returns true if the flag should be hidden from the help.
func (flag *SliceFlag[T]) GetHidden() bool {
	return flag.Hidden
}

// GetUsage returns the usage string for the flag.
func (flag *SliceFlag[T]) GetUsage() string {
	return flag.Usage
}

// GetEnvVars implements `cli.Flag` interface.
func (flag *SliceFlag[T]) GetEnvVars() []string {
	return flag.EnvVars
}

// GetDefaultText returns the flags value as string representation and an empty string if the flag takes no value at all.
func (flag *SliceFlag[T]) GetDefaultText() string {
	if flag.DefaultText == "" && flag.FlagValue != nil {
		return flag.GetInitialTextValue()
	}

	return flag.DefaultText
}

// String returns a readable representation of this value (for usage defaults).
func (flag *SliceFlag[T]) String() string {
	return cli.FlagStringer(flag)
}

// Names returns the names of the flag.
func (flag *SliceFlag[T]) Names() []string {
	if flag.Name == "" {
		return flag.Aliases
	}

	return append([]string{flag.Name}, flag.Aliases...)
}

// RunAction implements ActionableFlag.RunAction
func (flag *SliceFlag[T]) RunAction(ctx *Context) error {
	if flag.Action != nil {
		return flag.Action(ctx, *flag.Destination)
	}

	return nil
}

var _ = Value(new(sliceValue[string]))

// -- slice Value
type sliceValue[T comparable] struct {
	values    *[]T
	valueType FlagVariable[T]
	setter    FlagSetterFunc[T]
	valSep    string
}

func newSliceValue[T comparable](valueType FlagVariable[T], valSep string, dest *[]T, setter FlagSetterFunc[T]) *sliceValue[T] {
	return &sliceValue[T]{
		values:    dest,
		valueType: valueType,
		valSep:    valSep,
		setter:    setter,
	}
}

func (flag *sliceValue[T]) Reset() {
	*flag.values = []T{}
}

func (flag *sliceValue[T]) Set(str string) error {
	value := flag.valueType.Clone(new(T))
	if err := value.Set(str); err != nil {
		return err
	}

	*flag.values = append(*flag.values, value.Get().(T))

	if flag.setter != nil {
		return flag.setter(value.Get().(T))
	}

	return nil
}

func (flag *sliceValue[T]) Get() any {
	var vals []T

	vals = append(vals, *flag.values...)

	return vals
}

// String returns a readable representation of this value
func (flag *sliceValue[T]) String() string {
	if flag.values == nil {
		return ""
	}

	var vals = make([]string, 0, len(*flag.values))

	for _, val := range *flag.values {
		vals = append(vals, flag.valueType.Clone(&val).String())
	}

	return strings.Join(vals, flag.valSep)
}
