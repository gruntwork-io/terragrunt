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
	var undefinedFlags []string

	for {
		err := flagSet.Parse(args)
		if err == nil {
			break
		}

		// check if the error is due to an undefined flag
		var undefined string
		errStr := err.Error()
		if !strings.HasPrefix(errStr, errFlagUndefined) {
			return nil, errors.WithStackTrace(err)
		}

		undefined = strings.Trim(strings.TrimPrefix(errStr, errFlagUndefined), " -")

		// cut off the args
		var undefinedMatch bool
		for i, arg := range args {
			if trimmed := strings.Trim(arg, "-"); trimmed == undefined {
				undefinedFlags = append(undefinedFlags, arg)
				undefinedMatch = true
				args = args[i+1:]
				break
			}

		}

		// This should be an impossible to reach code path, but in case the arg
		// splitting failed to happen, this will prevent infinite loops
		if !undefinedMatch {
			return nil, err
		}
	}

	undefinedFlags = append(undefinedFlags, flagSet.Args()...)
	return undefinedFlags, nil
}
