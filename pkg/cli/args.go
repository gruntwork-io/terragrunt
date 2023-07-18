package cli

import (
	libflag "flag"
	"io"
	"regexp"
	"strings"

	"github.com/urfave/cli/v2"
)

const (
	SingleDashFlag NormalizeActsType = iota
	DoubleDashFlag NormalizeActsType = iota
)

var (
	singleDashRegexp = regexp.MustCompile(`^-[^-]`)
	doubleDashRegexp = regexp.MustCompile(`^--[^-]`)
)

type NormalizeActsType byte

// Args is a wrapper for `urfave`'s `cli.Args` interface, that provides convenient access to CLI arguments.
type Args struct {
	cli.Args
}

func newArgs(args []string) *Args {
	// This is the only way to avoid duplicating code from the private struct `urfave`'s `cli.args` that implements `cli.Args` interface.
	flagSet := libflag.NewFlagSet("", libflag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	flagSet.Parse(append([]string{"--"}, args...))

	return &Args{
		Args: cli.NewContext(nil, flagSet, nil).Args(),
	}

}

// Normalize formats the arguments according to the given actions.
// if the given act is:
//
//	`SingleDashFlag` - converts all arguments containing double dashes to single dashes
//	`DoubleDashFlag` - converts all arguments containing signle dashes to double dashes
func (args *Args) Normalize(acts ...NormalizeActsType) *Args {
	var strArgs []string

	for _, arg := range args.Slice() {
		for _, act := range acts {
			switch act {
			case SingleDashFlag:
				if doubleDashRegexp.MatchString(arg) {
					arg = arg[1:]
				}
			case DoubleDashFlag:
				if singleDashRegexp.MatchString(arg) {
					arg = "-" + arg
				}
			}
		}

		strArgs = append(strArgs, arg)
	}

	return newArgs(strArgs)
}

// CommandName returns the first value if it starts without a dash `-`, otherwise that means the args do not consist any command and an empty string is returned.
func (args *Args) CommandName() string {
	name := args.First()

	if isFlag := strings.HasPrefix(name, "-"); !isFlag && name != "" {
		return name
	}

	return ""
}
