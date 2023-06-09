package cli

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/pkg/maps"
)

type flagMapValue[K, V comparable] struct {
	values         *map[K]V
	keyType        Type[K]
	valType        Type[V]
	defaultText    string
	argSep, valSep string
	splitter       SplitterFunc
	hasBeenSet     bool
}

func newFlagMapValue[K, V comparable](keyType Type[K], valType Type[V], ptr *map[K]V, envVar string, argSep, valSep string, splitter SplitterFunc) (FlagValue, error) {
	var nilPtr *map[K]V
	if ptr == nilPtr {
		val := make(map[K]V)
		ptr = &val
	}

	defaultText := (&flagMapValue[K, V]{values: ptr, keyType: keyType, valType: valType, argSep: argSep, valSep: valSep, splitter: splitter}).String()

	if envVal, ok := lookupEnv(envVar); ok && splitter != nil {
		value := flagMapValue[K, V]{values: ptr, keyType: keyType, valType: valType, argSep: argSep, valSep: valSep, splitter: splitter}

		args := splitter(envVal, argSep)
		for _, arg := range args {
			if err := value.Set(strings.TrimSpace(arg)); err != nil {
				return nil, err
			}
		}
	}

	return &flagMapValue[K, V]{
		values:      ptr,
		keyType:     keyType,
		valType:     valType,
		defaultText: defaultText,
		argSep:      argSep,
		valSep:      valSep,
		splitter:    splitter,
	}, nil
}

func (flag *flagMapValue[K, V]) Set(str string) error {
	if !flag.hasBeenSet {
		flag.hasBeenSet = true

		// may contain a default value or an env var, so it needs to be cleared before the first setting.
		*flag.values = map[K]V{}
	}

	parts := flag.splitter(str, flag.valSep)
	if len(parts) != 2 {
		return errors.Errorf("valid format: key%svalue", flag.valSep)
	}

	key := flag.keyType.Init(new(K))
	if err := key.Set(parts[0]); err != nil {
		return err
	}

	val := flag.valType.Init(new(V))
	if err := val.Set(parts[1]); err != nil {
		return err
	}

	(*flag.values)[key.Get().(K)] = val.Get().(V)
	return nil
}

func (flag *flagMapValue[K, V]) DefaultText() string {
	if flag.IsBoolFlag() {
		return ""
	}
	return flag.defaultText
}

func (flag *flagMapValue[K, V]) IsBoolFlag() bool {
	return false
}

func (flag *flagMapValue[K, V]) IsSet() bool {
	return flag.hasBeenSet
}

func (flag *flagMapValue[K, V]) Get() any {
	var vals map[K]V

	for key, val := range *flag.values {
		vals[key] = val
	}

	return vals
}

func (flag *flagMapValue[K, V]) String() string {
	return maps.Join(*flag.values, flag.argSep, flag.valSep)
}
