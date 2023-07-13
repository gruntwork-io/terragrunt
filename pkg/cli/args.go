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

type Args struct {
	cli.Args
}

func newArgs(args []string) *Args {
	flagSet := libflag.NewFlagSet("", libflag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	flagSet.Parse(append([]string{"--"}, args...))

	return &Args{
		Args: cli.NewContext(nil, flagSet, nil).Args(),
	}

}

// Normalize formats the arguments according to the given actions.
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

func (args *Args) CommandName() string {
	name := args.First()

	if isFlag := strings.HasPrefix(name, "-"); !isFlag && name != "" {
		return name
	}

	return ""
}
