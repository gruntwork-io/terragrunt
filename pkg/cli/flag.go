package cli

import (
	"flag"
	libflag "flag"
	"strings"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/urfave/cli/v2"
)

const (
	defaultArgSep    = ","
	defaultKeyValSep = "="
)

var (
	// use to separate arguments and env vars with multiple values.
	defaultSplitter = strings.Split
)

type FlagValue interface {
	flag.Getter

	DefaultText() string

	IsSet() bool

	// optional interface to indicate boolean flags that can be
	// supplied without "=value" text
	IsBoolFlag() bool
}

// Flag is a common flag related to parsing flags in cli.
type Flag struct {
	Value FlagValue

	Name    string
	Aliases []string
	Usage   string
	EnvVar  string

	Destination any
	DefaultText string

	Splitter SplitterFunc
	ArgSep   string
	ValSep   string
}

func NewFlag(name string, dest any) cli.DocGenerationFlag {
	return &Flag{
		Name:        name,
		Destination: dest,
	}
}

// TakesValue returns true of the flag takes a value, otherwise false.
// Implements `cli.DocGenerationFlag.TakesValue` required to generate help.
func (flag *Flag) TakesValue() bool {
	return !flag.Value.IsBoolFlag()
}

// IsSet `cli.Flag.IsSet` required to generate help.
func (flag *Flag) IsSet() bool {
	return flag.Value.IsSet()
}

// GetUsage returns the usage string for the flag
func (flag *Flag) GetUsage() string {
	return flag.Usage
}

// GetValue returns the flags value as string representation and an empty
// string if the flag takes no value at all.
// Implements `cli.DocGenerationFlag.GetValue` required to generate help.
func (flag *Flag) GetValue() string {
	return flag.Value.String()
}

// GetCategory returns the category for the flag.
// Implements `cli.DocGenerationFlag.GetCategory` required to generate help.
func (flag *Flag) GetCategory() string {
	return ""
}

// GetEnvVars returns the env vars for this flag.
// Implements `cli.DocGenerationFlag.GetEnvVars` required to generate help.
func (flag *Flag) GetEnvVars() []string {
	if flag.EnvVar == "" {
		return nil
	}
	return []string{flag.EnvVar}
}

// GetValue returns the flags value as string representation and an empty string if the flag takes no value at all.
// Implements `cli.DocGenerationFlag.GetValue` required to generate help.
func (flag *Flag) GetDefaultText() string {
	if flag.DefaultText == "" {
		return flag.Value.DefaultText()
	}
	return flag.DefaultText
}

// String implements fmt.Stringer.String()
func (flag *Flag) String() string {
	return cli.FlagStringer(flag)
}

// Names `cli.Flag.Names` required to generate help.
func (flag *Flag) Names() []string {
	var names []string

	for _, name := range append([]string{flag.Name}, flag.Aliases...) {
		name = strings.TrimSpace(name)
		names = append(names, name)
	}

	return names
}

// Apply applies Flag settings to the given flag set.
func (flag *Flag) Apply(set *libflag.FlagSet) error {
	flag.normalize()

	var err error

	switch ptr := flag.Destination.(type) {
	case *string:
		valType := Type[string](new(stringType))
		flag.Value, err = newFlagGenreicValue(valType, ptr, flag.EnvVar)

	case *bool:
		valType := Type[bool](new(boolType))
		flag.Value, err = newFlagGenreicValue(valType, ptr, flag.EnvVar)

	case *int:
		valType := Type[int](new(intType))
		flag.Value, err = newFlagGenreicValue(valType, ptr, flag.EnvVar)

	case *int64:
		valType := Type[int64](new(int64Type))
		flag.Value, err = newFlagGenreicValue(valType, ptr, flag.EnvVar)

	case *[]string:
		valType := Type[string](new(stringType))
		flag.Value, err = newFlagSliceValue(valType, ptr, flag.EnvVar, flag.ArgSep, flag.Splitter)

	case *[]int:
		valType := Type[int](new(intType))
		flag.Value, err = newFlagSliceValue(valType, ptr, flag.EnvVar, flag.ArgSep, flag.Splitter)

	case *[]int64:
		valType := Type[int64](new(int64Type))
		flag.Value, err = newFlagSliceValue(valType, ptr, flag.EnvVar, flag.ArgSep, flag.Splitter)

	case *map[string]string:
		valType := Type[string](new(stringType))
		flag.Value, err = newFlagMapValue(valType, valType, ptr, flag.EnvVar, flag.ArgSep, flag.ValSep, flag.Splitter)

	case FlagValue:
		flag.Value = ptr

	default:
		return errors.Errorf("undefined flag type: %s", flag.Name)
	}

	if err != nil {
		return err
	}

	for _, name := range flag.Names() {
		set.Var(flag.Value, name, flag.Usage)
	}

	return nil
}

func (flag *Flag) normalize() {
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
