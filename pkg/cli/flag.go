package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/pkg/maps"
	"github.com/gruntwork-io/terragrunt/pkg/os"
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

// Flag is a common flag related to parsing flags in cli.
type Flag struct {
	Name   string
	Usage  string
	EnvVar string

	Destination any
	defaultText string

	Splitter  SplitterFunc
	ArgSep    string
	KeyValSep string
}

// TakesValue returns true of the flag takes a value, otherwise false.
// Implements `cli.DocGenerationFlag.TakesValue` required to generate help.
func (flag *Flag) TakesValue() bool {
	_, ok := flag.Destination.(*bool)
	return !ok
}

// GetUsage returns the usage string for the flag
func (flag *Flag) GetUsage() string {
	return flag.Usage
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
func (flag *Flag) GetValue() string {
	if val := fmt.Sprintf("%s", flag.Destination); val != "" {
		return val
	}

	return flag.defaultText
}

// GetDefaultText returns the default text for this flag
// Implements `cli.DocGenerationFlag.GetDefaultText` required to generate help.
func (flag *Flag) GetDefaultText() string {
	if _, ok := flag.Destination.(*bool); ok {
		return ""
	}
	return flag.defaultText
}

// String implements fmt.Stringer.String()
func (flag *Flag) String() string {
	return cli.FlagStringer(flag)
}

// Names `cli.Flag.Names` required to generate help.
func (flag *Flag) Names() []string {
	return []string{flag.Name}
}

// IsSet `cli.Flag.IsSet` required to generate help.
func (flag *Flag) IsSet() bool {
	return flag.defaultText != fmt.Sprintf("%s", flag.Destination)
}

// Apply applies Flag settings to the given flag set.
func (flag *Flag) Apply(set *flag.FlagSet) error {
	if err := flag.validate(); err != nil {
		return err
	}

	switch ptr := flag.Destination.(type) {
	case *string:
		flag.defaultText = fmt.Sprintf("%v", *ptr)

		envVal := os.GetStringEnv(flag.EnvVar, *ptr)

		val := newStringValue(envVal, ptr)
		set.Var(val, flag.Name, flag.Usage)

	case *bool:
		flag.defaultText = fmt.Sprintf("%v", *ptr)

		envVal, err := os.GetBoolEnv(flag.EnvVar, *ptr)
		if err != nil {
			return err
		}

		val := newBoolValue(envVal, ptr)
		set.Var(val, flag.Name, flag.Usage)

	case *int:
		flag.defaultText = fmt.Sprintf("%v", *ptr)

		envVal, err := os.GetIntEnv(flag.EnvVar, *ptr)
		if err != nil {
			return err
		}

		val := newIntValue(envVal, ptr)
		set.Var(val, flag.Name, flag.Usage)

	case *[]string:
		flag.defaultText = strings.Join(*ptr, flag.ArgSep)

		envVal := os.GetStringSliceEnv(flag.EnvVar, flag.ArgSep, flag.Splitter, *ptr)

		val := newStringSliceValue(envVal, ptr, flag.ArgSep)
		set.Var(val, flag.Name, flag.Usage)

	case *map[string]string:
		flag.defaultText = maps.Join(*ptr, flag.ArgSep, flag.KeyValSep)

		envVal, err := os.GetStringMapEnv(flag.EnvVar, flag.ArgSep, flag.KeyValSep, flag.Splitter, *ptr)
		if err != nil {
			return err
		}

		val := newStringMapValue(envVal, ptr, flag.ArgSep, flag.KeyValSep, flag.Splitter)
		set.Var(val, flag.Name, flag.Usage)
	}

	return nil
}

func (flag *Flag) validate() error {
	if flag.Splitter == nil {
		flag.Splitter = defaultSplitter
	}

	if flag.ArgSep == "" {
		flag.ArgSep = defaultArgSep
	}

	if flag.KeyValSep == "" {
		flag.KeyValSep = defaultKeyValSep
	}

	if flag.Destination == nil {
		err := fmt.Errorf("must be defined `Destination` field for flag %s", flag.Name)
		return errors.WithStackTrace(err)
	}

	return nil
}

func NewFlag(name string, dest any) *Flag {
	return &Flag{
		Name:        name,
		Destination: dest,
	}
}
