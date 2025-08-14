package cli

import (
	libflag "flag"
	"fmt"
	"os"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/urfave/cli/v2"
)

var (
	// FlagSplitter uses to separate arguments and env vars with multiple values.
	FlagSplitter = strings.Split
)

// FlagStringer converts a flag definition to a string. This is used by help
// to display a flag.
var FlagStringer = cli.FlagStringer //nolint:gochecknoglobals

// FlagSetterFunc represents function type that is called when the flag is specified.
// Unlike `FlagActionFunc` where the function is called after the value has been parsed and assigned to the `Destination` field,
// `FlagSetterFunc` is called earlier, during the variable parsing.
// if `FlagSetterFunc` returns the error, it will be wrapped with the flag or environment variable name.
// Example:
// `fmt.Errorf("invalid value \"invalid-value\" for env var TG_ENV_VAR: %w", err)`
// Therefore, using `FlagSetterFunc` is preferable to `FlagActionFunc` when you need to indicate in the error from where the value came from.
// If the flag has multiple values, `FlagSetterFunc` will be called for each value.
type FlagSetterFunc[T any] func(value T) error

type MapFlagSetterFunc[K any, V any] func(key K, value V) error

// FlagActionFunc represents function type that is called when the flag is specified.
// Executed after flag have been parsed  and assigned to the `Destination` field.
type FlagActionFunc[T any] func(ctx *Context, value T) error

type FlagVariable[T any] interface {
	libflag.Getter
	Clone(dest *T) FlagVariable[T]
}

type FlagValue interface {
	fmt.Stringer

	Get() any

	Set(str string) error

	Getter(name string) FlagValueGetter

	GetName() string

	GetInitialTextValue() string

	// IsSet returns true if the flag was set either by env var or CLI arg.
	IsSet() bool

	// IsArgSet returns true if the flag was set by CLI arg.
	IsArgSet() bool

	// IsEnvSet returns true if the flag was set by env var.
	IsEnvSet() bool

	// IsBoolFlag returns true if the flag is of type bool.
	IsBoolFlag() bool

	// IsNegativeBoolFlag returns true if the boolean flag's value should be inverted.
	// Example: For a flag with Negative=true, when set to true it returns false, and vice versa.
	IsNegativeBoolFlag() bool

	// MultipleSet returns true if the flag allows multiple assignments, such as slice/map.
	MultipleSet() bool
}

type Flag interface {
	// `urfave/cli/v2` uses to generate help
	cli.DocGenerationFlag

	// Value returns the `FlagValue` interface for interacting with the flag value.
	Value() FlagValue

	// GetHidden returns true if the flag is hidden.
	GetHidden() bool

	// RunAction runs the flag action.
	RunAction(ctx *Context) error

	// LookupEnv gets and splits the environment variable depending on the flag type: common, map, slice.
	LookupEnv(envVar string) []string

	// AllowedSubcommandScope returns true if the flag is allowed to be specified in subcommands,
	// and not only after the command it belongs to.
	AllowedSubcommandScope() bool

	// Parse parses the given args and environment variables to set the flag value.
	Parse(args Args) error
}

type LookupEnvFuncType func(key string) []string

type FlagValueGetter interface {
	libflag.Getter

	EnvSet(str string) error
}

type flagValueGetter struct {
	*flagValue
	valueName string
}

func (flag *flagValueGetter) EnvSet(val string) error {
	var err error

	if !flag.envHasBeenSet {
		// may contain a default value or an env var, so it needs to be cleared before the first setting.
		flag.value.Reset()
		flag.envHasBeenSet = true
	} else if !flag.multipleSet {
		err = errors.New(ErrMultipleTimesSettingEnvVar)
	}

	flag.name = flag.valueName

	if err := flag.value.Set(val); err != nil {
		return err
	}

	return err
}

func (flag *flagValueGetter) Set(val string) error {
	var err error

	if !flag.hasBeenSet {
		// may contain a default value or an env var, so it needs to be cleared before the first setting.
		flag.value.Reset()
		flag.hasBeenSet = true
	} else if !flag.multipleSet {
		err = errors.New(ErrMultipleTimesSettingFlag)
	}

	flag.name = flag.valueName

	if err := flag.value.Set(val); err != nil {
		return err
	}

	return err
}

type Value interface {
	libflag.Getter
	Reset()
}

// flag is a common flag related to parsing flags in cli.
type flagValue struct {
	value            Value
	name             string
	initialTextValue string
	multipleSet      bool
	hasBeenSet       bool
	envHasBeenSet    bool
	negative         bool
}

func (flag *flagValue) MultipleSet() bool {
	return flag.multipleSet
}

// IsBoolFlag implements `cli.FlagValue` interface.
func (flag *flagValue) IsBoolFlag() bool {
	_, ok := flag.value.Get().(bool)
	return ok
}

// IsNegativeBoolFlag implements `cli.FlagValue` interface.
func (flag *flagValue) IsNegativeBoolFlag() bool {
	return flag.negative
}

func (flag *flagValue) Get() any {
	return flag.value.Get()
}

func (flag *flagValue) Set(str string) error {
	return (&flagValueGetter{flagValue: flag}).Set(str)
}

func (flag *flagValue) String() string {
	if val := flag.value.Get(); val == nil {
		return ""
	}

	return flag.value.String()
}

func (flag *flagValue) GetInitialTextValue() string {
	return flag.initialTextValue
}

func (flag *flagValue) IsSet() bool {
	return flag.hasBeenSet || flag.envHasBeenSet
}

func (flag *flagValue) IsArgSet() bool {
	return flag.hasBeenSet
}

func (flag *flagValue) IsEnvSet() bool {
	return flag.envHasBeenSet
}

func (flag *flagValue) GetName() string {
	return flag.name
}

func (flag *flagValue) Getter(name string) FlagValueGetter {
	return &flagValueGetter{flagValue: flag, valueName: name}
}

// flag is a common flag related to parsing flags in cli.
type flag struct {
	FlagValue
	LookupEnvFunc LookupEnvFuncType
}

// Parse implements `Flag` interface.
func (flag *flag) Parse(args Args) error {
	return nil
}

func (flag *flag) LookupEnv(envVar string) []string {
	if flag.LookupEnvFunc == nil {
		flag.LookupEnvFunc = func(key string) []string {
			if val, ok := os.LookupEnv(key); ok {
				return []string{val}
			}

			return nil
		}
	}

	return flag.LookupEnvFunc(envVar)
}

func (flag *flag) Value() FlagValue {
	return flag.FlagValue
}

// TakesValue returns true if the flag needs to be given a value.
// Implements `cli.DocGenerationFlag.TakesValue` required to generate help.
func (flag *flag) TakesValue() bool {
	return true
}

// GetValue returns the flags value as string representation and an empty
// string if the flag takes no value at all.
// Implements `cli.DocGenerationFlag.GetValue` required to generate help.
func (flag *flag) GetValue() string {
	return flag.String()
}

// GetCategory returns the category for the flag.
// Implements `cli.DocGenerationFlag.GetCategory` required to generate help.
func (flag *flag) GetCategory() string {
	return ""
}

// AllowedSubcommandScope implements `cli.Flag` interface.
func (flag *flag) AllowedSubcommandScope() bool {
	return true
}

func (flag *flag) SplitValue(val string) []string {
	return []string{val}
}

func ApplyFlag(flag Flag, set *libflag.FlagSet) error {
	for _, name := range flag.GetEnvVars() {
		for _, val := range flag.LookupEnv(name) {
			if val == "" || (flag.Value().IsEnvSet() && !flag.Value().MultipleSet()) {
				continue
			}

			if err := flag.Value().Getter(name).EnvSet(val); err != nil {
				return errors.Errorf("invalid value %q for env var %s: %w", val, name, err)
			}
		}
	}

	for _, name := range flag.Names() {
		if name != "" {
			set.Var(flag.Value().Getter(name), name, flag.GetUsage())
		}
	}

	return nil
}
