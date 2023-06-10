package cli

import (
	libflag "flag"
	"strings"

	"github.com/urfave/cli/v2"
)

// CommonFlag is a common flag related to parsing flags in cli.
type CommonFlag struct {
	FlagValue

	Name        string
	Aliases     []string
	DefaultText string
	Usage       string
	EnvVar      string
}

func (flag *CommonFlag) Apply(set *libflag.FlagSet) error {
	for _, name := range flag.Names() {
		set.Var(flag.FlagValue, name, flag.Usage)
	}
	return nil
}

// TakesValue returns true of the flag takes a value, otherwise false.
// Implements `cli.DocGenerationFlag.TakesValue` required to generate help.
func (flag *CommonFlag) TakesValue() bool {
	return !flag.FlagValue.IsBoolFlag()
}

// IsSet `cli.CommonFlag.IsSet` required to generate help.
func (flag *CommonFlag) IsSet() bool {
	return flag.FlagValue.IsSet()
}

// GetUsage returns the usage string for the flag
func (flag *CommonFlag) GetUsage() string {
	return flag.Usage
}

// GetValue returns the flags value as string representation and an empty
// string if the flag takes no value at all.
// Implements `cli.DocGenerationFlag.GetValue` required to generate help.
func (flag *CommonFlag) GetValue() string {
	return flag.FlagValue.String()
}

// GetCategory returns the category for the flag.
// Implements `cli.DocGenerationFlag.GetCategory` required to generate help.
func (flag *CommonFlag) GetCategory() string {
	return ""
}

// GetEnvVars returns the env vars for this flag.
// Implements `cli.DocGenerationFlag.GetEnvVars` required to generate help.
func (flag *CommonFlag) GetEnvVars() []string {
	if flag.EnvVar == "" {
		return nil
	}
	return []string{flag.EnvVar}
}

// GetValue returns the flags value as string representation and an empty string if the flag takes no value at all.
// Implements `cli.DocGenerationFlag.GetValue` required to generate help.
func (flag *CommonFlag) GetDefaultText() string {
	if flag.DefaultText == "" {
		return flag.FlagValue.GetDefaultText()
	}
	return flag.DefaultText
}

// String implements fmt.Stringer.String()
func (flag *CommonFlag) String() string {
	return cli.FlagStringer(flag)
}

// Names `cli.CommonFlag.Names` required to generate help.
func (flag *CommonFlag) Names() []string {
	var names []string

	for _, name := range append([]string{flag.Name}, flag.Aliases...) {
		name = strings.TrimSpace(name)
		names = append(names, name)
	}

	return names
}
