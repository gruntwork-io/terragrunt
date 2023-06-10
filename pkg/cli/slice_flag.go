package cli

import (
	libflag "flag"
	"strings"
)

type SliceFlag[T GenericType] struct {
	CommonFlag

	Name        string
	DefaultText string
	Usage       string
	Aliases     []string
	EnvVar      string

	Destination *[]T
	Splitter    SplitterFunc
	ArgSep      string
}

// Apply applies Flag settings to the given flag set.
func (flag *SliceFlag[T]) Apply(set *libflag.FlagSet) error {
	flag.normalize()

	var err error
	valType := FlagType[T](new(flagType[T]))

	if flag.FlagValue, err = newSliceValue(valType, flag.Destination, flag.EnvVar, flag.ArgSep, flag.Splitter); err != nil {
		return err
	}
	return flag.CommonFlag.Apply(set)
}

func (flag *SliceFlag[T]) normalize() {
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
}

// -- slice Value
type sliceValue[T comparable] struct {
	values      *[]T
	valueType   FlagType[T]
	defaultText string
	valSep      string
	hasBeenSet  bool
}

func newSliceValue[T comparable](valueType FlagType[T], dest *[]T, envVar string, valSep string, splitter SplitterFunc) (FlagValue, error) {
	var nilPtr *[]T
	if dest == nilPtr {
		dest = new([]T)
	}

	defaultText := (&sliceValue[T]{values: dest, valueType: valueType, valSep: valSep}).String()

	if envVal, ok := lookupEnv(envVar); ok && splitter != nil {
		value := sliceValue[T]{values: dest, valueType: valueType}

		vals := splitter(envVal, valSep)
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

	value := flag.valueType.Init(new(T), false)
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

	for _, val := range *flag.values {
		vals = append(vals, val)
	}

	return vals
}

func (flag *sliceValue[T]) String() string {
	var vals []string

	for _, val := range *flag.values {
		vals = append(vals, flag.valueType.Init(&val, false).String())
	}

	return strings.Join(vals, flag.valSep)
}
