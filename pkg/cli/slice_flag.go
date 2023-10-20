package cli

import (
	libflag "flag"
	"strings"

	"github.com/urfave/cli/v2"
)

var (
	SliceFlagEnvVarSep = ","
)

type SliceFlagType interface {
	GenericType
}

// SliceFlag is a multiple flag.
type SliceFlag[T SliceFlagType] struct {
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
	Destination *[]T
	// The func used to split the EvnVar, by default `strings.Split`
	Splitter SplitterFunc
	// The Env Var separator that is passed to the Splitter function as an argument
	EnvVarSep string
}

// Apply applies Flag settings to the given flag set.
func (flag *SliceFlag[T]) Apply(set *libflag.FlagSet) error {
	if flag.Splitter == nil {
		flag.Splitter = FlagSplitter
	}

	if flag.EnvVarSep == "" {
		flag.EnvVarSep = SliceFlagEnvVarSep
	}

	var err error
	valType := FlagType[T](new(genericType[T]))

	if flag.FlagValue, err = newSliceValue(valType, flag.LookupEnv(flag.EnvVar), flag.EnvVarSep, flag.Splitter, flag.Destination); err != nil {
		return err
	}

	for _, name := range flag.Names() {
		set.Var(flag.FlagValue, name, flag.Usage)
	}
	return nil
}

// GetUsage returns the usage string for the flag.
func (flag *SliceFlag[T]) GetUsage() string {
	return flag.Usage
}

// GetEnvVars returns the env vars for this flag.
func (flag *SliceFlag[T]) GetEnvVars() []string {
	if flag.EnvVar == "" {
		return nil
	}
	return []string{flag.EnvVar}
}

// GetDefaultText returns the flags value as string representation and an empty string if the flag takes no value at all.
func (flag *SliceFlag[T]) GetDefaultText() string {
	if flag.DefaultText == "" && flag.FlagValue != nil {
		return flag.FlagValue.GetDefaultText()
	}
	return flag.DefaultText
}

// String returns a readable representation of this value (for usage defaults).
func (flag *SliceFlag[T]) String() string {
	return cli.FlagStringer(flag)
}

// Names returns the names of the flag.
func (flag *SliceFlag[T]) Names() []string {
	return append([]string{flag.Name}, flag.Aliases...)
}

// RunAction implements ActionableFlag.RunAction
func (flag *SliceFlag[T]) RunAction(ctx *Context) error {
	if flag.Action != nil {
		return flag.Action(ctx)
	}

	return nil
}

// -- slice Value
type sliceValue[T comparable] struct {
	values      *[]T
	valueType   FlagType[T]
	defaultText string
	valSep      string
	hasBeenSet  bool
}

func newSliceValue[T comparable](valueType FlagType[T], envValue *string, valSep string, splitter SplitterFunc, dest *[]T) (FlagValue, error) {
	var nilPtr *[]T
	if dest == nilPtr {
		dest = new([]T)
	}

	defaultText := (&sliceValue[T]{values: dest, valueType: valueType, valSep: valSep}).String()

	if envValue != nil && splitter != nil {
		value := sliceValue[T]{values: dest, valueType: valueType}

		vals := splitter(*envValue, valSep)
		for _, val := range vals {
			if err := value.Set(val); err != nil {
				return nil, err
			}
		}
	}

	return &sliceValue[T]{
		values:      dest,
		valueType:   valueType,
		defaultText: defaultText,
		valSep:      valSep,
	}, nil
}

func (flag *sliceValue[T]) Set(str string) error {
	if !flag.hasBeenSet {
		flag.hasBeenSet = true

		// may contain a default value or an env var, so it needs to be cleared before the first setting.
		*flag.values = []T{}
	}

	value := flag.valueType.Clone(new(T))
	if err := value.Set(str); err != nil {
		return err
	}

	*flag.values = append(*flag.values, value.Get().(T))
	return nil
}

func (flag *sliceValue[T]) GetDefaultText() string {
	if flag.IsBoolFlag() {
		return ""
	}
	return flag.defaultText
}

func (flag *sliceValue[T]) IsBoolFlag() bool {
	return false
}

func (flag *sliceValue[T]) IsSet() bool {
	return flag.hasBeenSet
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

	var vals []string

	for _, val := range *flag.values {
		vals = append(vals, flag.valueType.Clone(&val).String())
	}

	return strings.Join(vals, flag.valSep)
}
