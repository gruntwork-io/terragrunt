package cli

import (
	libflag "flag"
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/urfave/cli/v2"
)

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

	Name        string
	DefaultText string
	Usage       string
	Aliases     []string
	Action      ActionFunc
	EnvVar      string

	Destination *map[K]V
	Splitter    SplitterFunc
	EnvVarSep   string
	KeyValSep   string
}

// Apply applies Flag settings to the given flag set.
func (flag *MapFlag[K, V]) Apply(set *libflag.FlagSet) error {
	if flag.Splitter == nil {
		flag.Splitter = FlagSplitter
	}

	if flag.EnvVarSep == "" {
		flag.EnvVarSep = MapFlagEnvVarSep
	}

	if flag.KeyValSep == "" {
		flag.KeyValSep = MapFlagKeyValSep
	}

	var err error
	keyType := FlagType[K](new(genericType[K]))
	valType := FlagType[V](new(genericType[V]))

	if flag.FlagValue, err = newMapValue(keyType, valType, flag.LookupEnv(flag.EnvVar), flag.EnvVarSep, flag.KeyValSep, flag.Splitter, flag.Destination); err != nil {
		return err
	}

	for _, name := range flag.Names() {
		set.Var(flag.FlagValue, name, flag.Usage)
	}
	return nil
}

// GetUsage returns the usage string for the flag.
func (flag *MapFlag[K, V]) GetUsage() string {
	return flag.Usage
}

// GetEnvVars returns the env vars for this flag.
func (flag *MapFlag[K, V]) GetEnvVars() []string {
	if flag.EnvVar == "" {
		return nil
	}
	return []string{flag.EnvVar}
}

// GetDefaultText returns the flags value as string representation and an empty string if the flag takes no value at all.
func (flag *MapFlag[K, V]) GetDefaultText() string {
	if flag.DefaultText == "" && flag.FlagValue != nil {
		return flag.FlagValue.GetDefaultText()
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
		return flag.Action(ctx)
	}

	return nil
}

type mapValue[K, V comparable] struct {
	values         *map[K]V
	keyType        FlagType[K]
	valType        FlagType[V]
	defaultText    string
	argSep, valSep string
	splitter       SplitterFunc
	hasBeenSet     bool
}

func newMapValue[K, V comparable](keyType FlagType[K], valType FlagType[V], envValue *string, argSep, valSep string, splitter SplitterFunc, dest *map[K]V) (FlagValue, error) {
	var nilPtr *map[K]V
	if dest == nilPtr {
		val := make(map[K]V)
		dest = &val
	}

	defaultText := (&mapValue[K, V]{values: dest, keyType: keyType, valType: valType, argSep: argSep, valSep: valSep, splitter: splitter}).String()

	if envValue != nil && splitter != nil {
		value := mapValue[K, V]{values: dest, keyType: keyType, valType: valType, argSep: argSep, valSep: valSep, splitter: splitter}

		args := splitter(*envValue, argSep)
		for _, arg := range args {
			if err := value.Set(strings.TrimSpace(arg)); err != nil {
				return nil, err
			}
		}
	}

	return &mapValue[K, V]{
		values:      dest,
		keyType:     keyType,
		valType:     valType,
		defaultText: defaultText,
		argSep:      argSep,
		valSep:      valSep,
		splitter:    splitter,
	}, nil
}

func (flag *mapValue[K, V]) Set(str string) error {
	if !flag.hasBeenSet {
		flag.hasBeenSet = true

		// may contain a default value or an env var, so it needs to be cleared before the first setting.
		*flag.values = map[K]V{}
	}

	parts := flag.splitter(str, flag.valSep)
	if len(parts) != flatPatsCount {
		return errors.WithStackTrace(NewInvalidKeyValueError(flag.valSep, str))
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
	return nil
}

func (flag *mapValue[K, V]) GetDefaultText() string {
	if flag.IsBoolFlag() {
		return ""
	}
	return flag.defaultText
}

func (flag *mapValue[K, V]) IsBoolFlag() bool {
	return false
}

func (flag *mapValue[K, V]) IsSet() bool {
	return flag.hasBeenSet
}

func (flag *mapValue[K, V]) Get() any {
	var vals = map[K]V{}

	for key, val := range *flag.values {
		vals[key] = val
	}

	return vals
}

// String returns a readable representation of this value
func (flag *mapValue[K, V]) String() string {
	if flag.values == nil {
		return ""
	}
	return collections.MapJoin(*flag.values, flag.argSep, flag.valSep)
}
