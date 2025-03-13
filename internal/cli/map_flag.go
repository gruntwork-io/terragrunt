package cli

import (
	libflag "flag"
	"os"
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/urfave/cli/v2"
	"maps"
)

// MapFlag implements Flag
var _ Flag = new(MapFlag[string, string])

var (
	MapFlagEnvVarSep = ","
	MapFlagKeyValSep = "="
	flatPatsCount    = 2
)

type MapFlagKeyType interface {
	GenericType
}

type MapFlagValueType interface {
	GenericType | bool
}

// MapFlag is a key value flag.
type MapFlag[K MapFlagKeyType, V MapFlagValueType] struct {
	flag

	// The name of the flag.
	Name string
	// The default value of the flag to display in the help, if it is empty, the value is taken from `Destination`.
	DefaultText string
	// A short usage description to display in help.
	Usage string
	// Aliases are usually used for the short flag name, like `-h`.
	Aliases []string
	// Action is a function that is called when the flag is specified. It is executed only after all command flags have been parsed.
	Action FlagActionFunc[map[K]V]
	// FlagSetterFunc represents function type that is called when the flag is specified.
	// Executed during value parsing, in case of an error the returned error is wrapped with the flag or environment variable name.
	Setter MapFlagSetterFunc[K, V]
	// The names of the env variables that are parsed and assigned to `Destination` before the flag value.
	EnvVars []string
	// Destination is a pointer to which the value of the flag or env var is assigned.
	// It also uses as the default value displayed in the help.
	Destination *map[K]V
	// The func used to split the EvnVar, by default `strings.Split`
	Splitter SplitterFunc
	// The EnvVarSep value is passed to the Splitter function as an argument to split the args.
	EnvVarSep string
	// The KeyValSep value is passed to the Splitter function as an argument to split `key` and `val` of the arg.
	KeyValSep string
	// Hidden hides the flag from the help, if set to true.
	Hidden bool
}

// Apply applies Flag settings to the given flag set.
func (flag *MapFlag[K, V]) Apply(set *libflag.FlagSet) error {
	if flag.FlagValue != nil {
		return ApplyFlag(flag, set)
	}

	if flag.Destination == nil {
		dest := make(map[K]V)
		flag.Destination = &dest
	}

	if flag.Splitter == nil {
		flag.Splitter = FlagSplitter
	}

	if flag.EnvVarSep == "" {
		flag.EnvVarSep = MapFlagEnvVarSep
	}

	if flag.KeyValSep == "" {
		flag.KeyValSep = MapFlagKeyValSep
	}

	if flag.LookupEnvFunc == nil {
		flag.LookupEnvFunc = func(key string) []string {
			if val, ok := os.LookupEnv(key); ok {
				return flag.Splitter(val, flag.EnvVarSep)
			}

			return nil
		}
	}

	keyType := FlagVariable[K](new(genericVar[K]))
	valType := FlagVariable[V](new(genericVar[V]))

	value := newMapValue(keyType, valType, flag.EnvVarSep, flag.KeyValSep, flag.Splitter, flag.Destination, flag.Setter)

	flag.FlagValue = &flagValue{
		multipleSet:      true,
		value:            value,
		initialTextValue: value.String(),
	}

	return ApplyFlag(flag, set)
}

// GetHidden returns true if the flag should be hidden from the help.
func (flag *MapFlag[K, V]) GetHidden() bool {
	return flag.Hidden
}

// GetUsage returns the usage string for the flag.
func (flag *MapFlag[K, V]) GetUsage() string {
	return flag.Usage
}

// GetEnvVars implements `cli.Flag` interface.
func (flag *MapFlag[K, V]) GetEnvVars() []string {
	return flag.EnvVars
}

// GetDefaultText returns the flags value as string representation and an empty string if the flag takes no value at all.
func (flag *MapFlag[K, V]) GetDefaultText() string {
	if flag.DefaultText == "" && flag.FlagValue != nil {
		return flag.FlagValue.GetInitialTextValue()
	}

	return flag.DefaultText
}

// String returns a readable representation of this value (for usage defaults).
func (flag *MapFlag[K, V]) String() string {
	return cli.FlagStringer(flag)
}

// Names returns the names of the flag.
func (flag *MapFlag[K, V]) Names() []string {
	return append([]string{flag.Name}, flag.Aliases...)
}

// RunAction implements ActionableFlag.RunAction
func (flag *MapFlag[K, V]) RunAction(ctx *Context) error {
	if flag.Action != nil {
		return flag.Action(ctx, *flag.Destination)
	}

	return nil
}

var _ = Value(new(mapValue[string, string]))

type mapValue[K, V comparable] struct {
	values         *map[K]V
	setter         MapFlagSetterFunc[K, V]
	keyType        FlagVariable[K]
	valType        FlagVariable[V]
	argSep, valSep string
	splitter       SplitterFunc
}

func newMapValue[K, V comparable](keyType FlagVariable[K], valType FlagVariable[V], argSep, valSep string, splitter SplitterFunc, dest *map[K]V, setter MapFlagSetterFunc[K, V]) *mapValue[K, V] {
	return &mapValue[K, V]{
		values:   dest,
		setter:   setter,
		keyType:  keyType,
		valType:  valType,
		argSep:   argSep,
		valSep:   valSep,
		splitter: splitter,
	}
}

func (flag *mapValue[K, V]) Reset() {
	*flag.values = map[K]V{}
}

func (flag *mapValue[K, V]) Set(str string) error {
	parts := flag.splitter(str, flag.valSep)
	if len(parts) != flatPatsCount {
		return errors.New(NewInvalidKeyValueError(flag.valSep, str))
	}

	key := flag.keyType.Clone(new(K))
	if err := key.Set(strings.TrimSpace(parts[0])); err != nil {
		return err
	}

	val := flag.valType.Clone(new(V))
	if err := val.Set(strings.TrimSpace(parts[1])); err != nil {
		return err
	}

	(*flag.values)[key.Get().(K)] = val.Get().(V)

	if flag.setter != nil {
		return flag.setter(key.Get().(K), val.Get().(V))
	}

	return nil
}

func (flag *mapValue[K, V]) Get() any {
	var vals = map[K]V{}

	maps.Copy(vals, *flag.values)

	return vals
}

// String returns a readable representation of this value
func (flag *mapValue[K, V]) String() string {
	if flag.values == nil {
		return ""
	}

	return collections.MapJoin(*flag.values, flag.argSep, flag.valSep)
}
