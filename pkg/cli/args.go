package cli

import (
	libflag "flag"

	"github.com/urfave/cli/v2"
)

const (
	OnePrefixFlag    NormalizeActsType = iota
	DoublePrefixFlag NormalizeActsType = iota
)

type NormalizeActsType byte

type Args struct {
	cli.Args
}

// Normalize formats the arguments according to the given actions.
func (args *Args) Normalize(acts ...NormalizeActsType) *Args {
	var strArgs []string

	for _, arg := range args.Slice() {
		arg := arg

		for _, act := range acts {
			switch act {
			case OnePrefixFlag:
				if len(arg) >= 3 && arg[0:2] == "--" && arg[2] != '-' {
					arg = arg[1:]
				}
			case DoublePrefixFlag:
				if len(arg) >= 2 && arg[0] == '-' && arg[1] != '-' {
					arg = "-" + arg
				}
			}
		}

		strArgs = append(strArgs, arg)
	}

	return newArgs(strArgs)
}

func newArgs(args []string) *Args {
	flagSet := libflag.NewFlagSet("", libflag.ContinueOnError)
	flagSet.Parse(args)

	return &Args{
		Args: cli.NewContext(nil, flagSet, nil).Args(),
	}

}
