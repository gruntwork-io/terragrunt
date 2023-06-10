package cli

import (
	"flag"
	"strings"

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

type Flag interface {
	Value() FlagValue
	cli.DocGenerationFlag
}

type FlagValue interface {
	flag.Getter

	GetDefaultText() string

	IsSet() bool

	// optional interface to indicate boolean flags that can be
	// supplied without "=value" text
	IsBoolFlag() bool
}
