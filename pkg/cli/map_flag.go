package cli

import (
	libflag "flag"
	"strings"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/pkg/maps"
)

type MapFlag[K, V GenericType] struct {
	CommonFlag

	Name        string
	DefaultText string
	Usage       string
	Aliases     []string
	EnvVar      string

	Destination *map[K]V
	Splitter    SplitterFunc
	ArgSep      string
	ValSep      string
}

// Apply applies Flag settings to the given flag set.
func (flag *MapFlag[K, V]) Apply(set *libflag.FlagSet) error {
	flag.normalize()

	var err error
	keyType := FlagType[K](new(flagType[K]))
	valType := FlagType[V](new(flagType[V]))

	if flag.FlagValue, err = newMapValue(keyType, valType, flag.Destination, flag.EnvVar, flag.ArgSep, flag.ValSep, flag.Splitter); err != nil {
		return err
	}
	return flag.CommonFlag.Apply(set)
}

func (flag *MapFlag[K, V]) normalize() {
	flag.CommonFlag.Name = flag.Name
	flag.CommonFlag.DefaultText = flag.DefaultText
	flag.CommonFlag.Usage = flag.Usage
	flag.CommonFlag.Aliases = flag.Aliases
	flag.CommonFlag.EnvVar = flag.EnvVar

	if flag.Splitter == nil {
		flag.Splitter = defaultSplitter
	}

	if flag.ArgSep == "" {
		flag.ArgSep = defaultArgSep
	}

	if flag.ValSep == "" {
		flag.ValSep = defaultKeyValSep
	}
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

func newMapValue[K, V comparable](keyType FlagType[K], valType FlagType[V], dest *map[K]V, envVar string, argSep, valSep string, splitter SplitterFunc) (FlagValue, error) {
	var nilPtr *map[K]V
	if dest == nilPtr {
		val := make(map[K]V)
		dest = &val
	}

	defaultText := (&mapValue[K, V]{values: dest, keyType: keyType, valType: valType, argSep: argSep, valSep: valSep, splitter: splitter}).String()

	if envVal, ok := lookupEnv(envVar); ok && splitter != nil {
		value := mapValue[K, V]{values: dest, keyType: keyType, valType: valType, argSep: argSep, valSep: valSep, splitter: splitter}

		args := splitter(envVal, argSep)
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
	if len(parts) != 2 {
		return errors.Errorf("valid format: key%svalue", flag.valSep)
	}

	key := flag.keyType.Init(new(K), false)
	if err := key.Set(parts[0]); err != nil {
		return err
	}

	val := flag.valType.Init(new(V), false)
	if err := val.Set(parts[1]); err != nil {
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
	var vals map[K]V

	for key, val := range *flag.values {
		vals[key] = val
	}

	return vals
}

func (flag *mapValue[K, V]) String() string {
	return maps.Join(*flag.values, flag.argSep, flag.valSep)
}
