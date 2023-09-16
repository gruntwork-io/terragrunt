package cli

import (
	"regexp"
	"strings"
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

// Args provides convenient access to CLI arguments.
type Args []string

// Get returns the nth argument, or else a blank string
func (args *Args) Get(n int) string {
	if len(*args) > n {
		return (*args)[n]
	}
	return ""
}

// First returns the first argument, or else a blank string
func (args *Args) First() string {
	return args.Get(0)
}

// Tail returns the rest of the arguments (not the first one)
// or else an empty string slice
func (args *Args) Tail() []string {
	if args.Len() >= 2 {
		tail := []string((*args)[1:])
		ret := make([]string, len(tail))
		copy(ret, tail)
		return ret
	}
	return []string{}
}

// Len returns the length of the wrapped slice
func (args *Args) Len() int {
	return len(*args)
}

// Present checks if there are any arguments present
func (args *Args) Present() bool {
	return args.Len() != 0
}

// Slice returns a copy of the internal slice
func (args *Args) Slice() []string {
	ret := make([]string, len(*args))
	copy(ret, *args)
	return ret
}

// Normalize formats the arguments according to the given actions.
// if the given act is:
//
//	`SingleDashFlag` - converts all arguments containing double dashes to single dashes
//	`DoubleDashFlag` - converts all arguments containing signle dashes to double dashes
func (args *Args) Normalize(acts ...NormalizeActsType) *Args {
	var strArgs Args

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

	return &strArgs
}

// CommandName returns the first value if it starts without a dash `-`, otherwise that means the args do not consist any command and an empty string is returned.
func (args *Args) CommandName() string {
	name := args.First()

	if !strings.HasPrefix(name, "-") {
		return name
	}

	return ""
}
