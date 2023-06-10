package cli

import (
	libflag "flag"
	"io"
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/errors"
)

const errFlagUndefined = "flag provided but not defined:"

type Flags []Flag

// Get returns a Flag by the given name.
func (flags Flags) Get(name string) Flag {
	for _, flag := range flags {
		if collections.ListContainsElement(flag.Names(), name) {
			return flag
		}
	}

	return nil
}

func (flags Flags) Len() int {
	return len(flags)
}

func (flags Flags) Less(i, j int) bool {
	if len(flags[j].Names()) == 0 {
		return false
	} else if len(flags[i].Names()) == 0 {
		return true
	}

	return lexicographicLess(flags[i].Names()[0], flags[j].Names()[0])
}

func (flags Flags) Swap(i, j int) {
	flags[i], flags[j] = flags[j], flags[i]
}

func (flags Flags) newFlagSet(name string, errorHandling libflag.ErrorHandling) (*libflag.FlagSet, error) {
	set := libflag.NewFlagSet(name, errorHandling)
	set.SetOutput(io.Discard)

	for _, flag := range flags {
		if err := flag.Apply(set); err != nil {
			return nil, err
		}
	}

	return set, nil
}

func (flags Flags) parseArgs(flagSet *libflag.FlagSet, args []string) ([]string, error) {
	var undefined []string

	if len(args) == 0 {
		return undefined, nil
	}

	for {
		err := flagSet.Parse(args)
		if err == nil {
			break
		}

		// check if the error is due to an undefined flag
		var notFound string
		errStr := err.Error()
		if !strings.HasPrefix(errStr, errFlagUndefined) {
			return nil, errors.WithStackTrace(err)
		}

		notFound = strings.Trim(strings.TrimPrefix(errStr, errFlagUndefined), " -")

		// cut off the args
		var notFoundMatch bool
		for i, arg := range args {
			if trimmed := strings.Trim(arg, "-"); trimmed == notFound {
				undefined = append(undefined, arg)
				notFoundMatch = true
				args = args[i+1:]
				break
			}

		}

		// This should be an impossible to reach code path, but in case the arg
		// splitting failed to happen, this will prevent infinite loops
		if !notFoundMatch {
			return nil, err
		}
	}

	undefined = append(undefined, flagSet.Args()...)
	return undefined, nil
}
