package cli

import (
	libflag "flag"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
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

	// The name of the flag.
	Name string
	// The default value of the flag to display in the help, if it is empty, the value is taken from `Destination`.
	DefaultText string
	// A short usage description to display in help.
	Usage string
	// Aliases are usually used for the short flag name, like `-h`.
	Aliases []string
	// The names of the env variables that are parsed and assigned to `Destination` before the flag value.
	EnvVars []string
	// Action is a function that is called when the flag is specified. It is executed only after all command flags have been parsed.
	Action FlagActionFunc[[]T]
	// FlagSetterFunc represents function type that is called when the flag is specified.
	// Executed during value parsing, in case of an error the returned error is wrapped with the flag or environment variable name.
	Setter FlagSetterFunc[T]
	// Destination is a pointer to which the value of the flag or env var is assigned.
	// It also uses as the default value displayed in the help.
	Destination *[]T
	// The func used to split the EvnVar, by default `strings.Split`
	Splitter SplitterFunc
	// The EnvVarSep value is passed to the Splitter function as an argument.
	EnvVarSep string
	// Hidden hides the flag from the help, if set to true.
	Hidden bool
}

// Apply applies Flag settings to the given flag set.
func (flag *SliceFlag[T]) Apply(set *libflag.FlagSet) error {
	if flag.Destination == nil {
		flag.Destination = new([]T)
	}

	if flag.Splitter == nil {
		flag.Splitter = FlagSplitter
	}

	if flag.EnvVarSep == "" {
		flag.EnvVarSep = SliceFlagEnvVarSep
	}

	var (
		err      error
		envVar   string
		envValue *string
	)

	valType := FlagType[T](new(genericType[T]))

	for _, envVar = range flag.EnvVars {
		if val := flag.LookupEnv(envVar); val != nil {
			envValue = val

			break
		}
	}

	if flag.FlagValue, err = newSliceValue(valType, envValue, flag.EnvVarSep, flag.Splitter, flag.Destination, flag.Setter); err != nil {
		if envValue != nil {
			return errors.Errorf("invalid value %q for env var %s: %w", *envValue, envVar, err)
		}

		return err
	}

	for _, name := range flag.Names() {
		set.Var(flag.FlagValue, name, flag.Usage)
	}

	return nil
}

// GetHidden returns true if the flag should be hidden from the help.
func (flag *SliceFlag[T]) GetHidden() bool {
	return flag.Hidden
}

// GetUsage returns the usage string for the flag.
func (flag *SliceFlag[T]) GetUsage() string {
	return flag.Usage
}

// GetEnvVars returns the env vars for this flag.
func (flag *SliceFlag[T]) GetEnvVars() []string {
	return flag.EnvVars
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
		return flag.Action(ctx, *flag.Destination)
	}

	return nil
}

// -- slice Value
type sliceValue[T comparable] struct {
	values        *[]T
	setter        FlagSetterFunc[T]
	valueType     FlagType[T]
	defaultText   string
	valSep        string
	hasBeenSet    bool
	envHasBeenSet bool
}

func newSliceValue[T comparable](valueType FlagType[T], envValue *string, valSep string, splitter SplitterFunc, dest *[]T, setter FlagSetterFunc[T]) (FlagValue, error) {
	var nilPtr *[]T
	if dest == nilPtr {
		dest = new([]T)
	}

	defaultText := (&sliceValue[T]{values: dest, setter: setter, valueType: valueType, valSep: valSep}).String()

	var envHasBeenSet bool

	if envValue != nil && splitter != nil {
		value := sliceValue[T]{values: dest, valueType: valueType}

		vals := splitter(*envValue, valSep)
		for _, val := range vals {
			if err := value.Set(val); err != nil {
				return nil, err
			}

			envHasBeenSet = true
		}
	}

	return &sliceValue[T]{
		values:        dest,
		setter:        setter,
		valueType:     valueType,
		defaultText:   defaultText,
		valSep:        valSep,
		envHasBeenSet: envHasBeenSet,
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

	if flag.setter != nil {
		return flag.setter(value.Get().(T))
	}

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
	return flag.hasBeenSet || flag.envHasBeenSet
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
