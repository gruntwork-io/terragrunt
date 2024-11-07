package cli

import (
	libflag "flag"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
)

var (
	// FlagSplitter uses to separate arguments and env vars with multiple values.
	FlagSplitter = strings.Split
)

type FlagType[T any] interface {
	libflag.Getter
	Clone(dest *T) FlagType[T]
}

type FlagValue interface {
	libflag.Getter

	GetDefaultText() string

	IsSet() bool

	// optional interface to indicate boolean flags that can be
	// supplied without "=value" text
	IsBoolFlag() bool
}

type Flag interface {
	Value() FlagValue
	GetHidden() bool
	RunAction(*Context) error
	// `urfave/cli/v2` uses to generate help
	cli.DocGenerationFlag
}

type LookupEnvFuncType func(key string) (string, bool)

// flag is a common flag related to parsing flags in cli.
type flag struct {
	FlagValue     FlagValue
	LookupEnvFunc LookupEnvFuncType
}

func (flag *flag) LookupEnv(envVar string) *string {
	var value *string

	if flag.LookupEnvFunc == nil {
		flag.LookupEnvFunc = os.LookupEnv
	}

	if val, ok := flag.LookupEnvFunc(envVar); ok {
		value = &val
	}

	return value
}

func (flag *flag) Value() FlagValue {
	return flag.FlagValue
}

// TakesValue returns true of the flag takes a value, otherwise false.
// Implements `cli.DocGenerationFlag.TakesValue` required to generate help.
func (flag *flag) TakesValue() bool {
	return flag.FlagValue != nil && !flag.FlagValue.IsBoolFlag()
}

// IsSet returns true if the flag was set either evn, by env var or arg flag.
// Implements `cli.flag.IsSet` required to generate help.
func (flag *flag) IsSet() bool {
	return flag.FlagValue.IsSet()
}

// GetValue returns the flags value as string representation and an empty
// string if the flag takes no value at all.
// Implements `cli.DocGenerationFlag.GetValue` required to generate help.
func (flag *flag) GetValue() string {
	return flag.FlagValue.String()
}

// GetCategory returns the category for the flag.
// Implements `cli.DocGenerationFlag.GetCategory` required to generate help.
func (flag *flag) GetCategory() string {
	return ""
}
