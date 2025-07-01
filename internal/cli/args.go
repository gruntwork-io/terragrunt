package cli

import (
	"regexp"
	"slices"
	"strings"
)

const (
	tailMinArgsLen = 2
	BuiltinCmdSep  = "--"
)

const (
	SingleDashFlag NormalizeActsType = iota
	DoubleDashFlag
)

var (
	singleDashRegexp = regexp.MustCompile(`^-([^-]|$)`)
	doubleDashRegexp = regexp.MustCompile(`^--([^-]|$)`)
)

type NormalizeActsType byte

// Args provides convenient access to CLI arguments.
type Args []string

// String implements `fmt.Stringer` interface.
func (args Args) String() string {
	return strings.Join(args, " ")
}

// Split splits `args` into two slices separated by `sep`.
func (args Args) Split(sep string) (Args, Args) {
	for i := range args {
		if args[i] == sep {
			return args[:i], args[i+1:]
		}
	}

	return args, nil
}

func (args Args) WithoutBuiltinCmdSep() Args {
	flags, nonFlags := args.Split(BuiltinCmdSep)

	return append(slices.Clone(flags), nonFlags...)
}

// Get returns the nth argument, or else a blank string
func (args Args) Get(n int) string {
	if len(args) > 0 && len(args) > n {
		return (args)[n]
	}

	return ""
}

// First returns the first argument or a blank string
func (args Args) First() string {
	return args.Get(0)
}

// Second returns the first argument or a blank string.
func (args Args) Second() string {
	return args.Get(1)
}

// Last returns the last argument or a blank string.
func (args Args) Last() string {
	return args.Get(len(args) - 1)
}

// Tail returns the rest of the arguments (not the first one)
// or else an empty string slice.
func (args Args) Tail() Args {
	if args.Len() < tailMinArgsLen {
		return []string{}
	}

	tail := []string((args)[1:])
	ret := make([]string, len(tail))
	copy(ret, tail)

	return ret
}

// Remove returns `args` with the `name` element removed.
func (args Args) Remove(name string) Args {
	result := make([]string, 0, len(args))

	for _, arg := range args {
		if arg != name {
			result = append(result, arg)
		}
	}

	return result
}

// Len returns the length of the wrapped slice
func (args Args) Len() int {
	return len(args)
}

// Present checks if there are any arguments present
func (args Args) Present() bool {
	return args.Len() != 0
}

// Slice returns a copy of the internal slice
func (args Args) Slice() []string {
	ret := make([]string, len(args))
	copy(ret, args)

	return ret
}

// Normalize formats the arguments according to the given actions.
// if the given act is:
//
//	`SingleDashFlag` - converts all arguments containing double dashes to single dashes
//	`DoubleDashFlag` - converts all arguments containing single dashes to double dashes
func (args Args) Normalize(acts ...NormalizeActsType) Args {
	strArgs := make(Args, 0, len(args.Slice()))

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

	return strArgs
}

// CommandNameN returns the nth argument from `args` that starts without a dash `-`.
func (args Args) CommandNameN(n int) string {
	var found int

	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			if found == n {
				return arg
			}

			found++
		}
	}

	return ""
}

// CommandName returns the first arg that starts without a dash `-`,
// otherwise that means the args do not consist any command and an empty string is returned.
func (args Args) CommandName() string {
	return args.CommandNameN(0)
}

// SubCommandName returns the second arg that starts without a dash `-`,
// otherwise that means the args do not consist a subcommand and an empty string is returned.
func (args Args) SubCommandName() string {
	return args.CommandNameN(1)
}

// Contains returns true if args contains the given `target` arg.
func (args Args) Contains(target string) bool {
	return slices.Contains(args, target)
}
