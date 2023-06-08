package cli

import (
	libflag "flag"
	"io"

	"github.com/gruntwork-io/terragrunt/errors"

	"github.com/gruntwork-io/go-commons/collections"
)

type Flags []*Flag

// Get returns a Flag by the given name.
func (flags Flags) Get(name string) *Flag {
	for _, flag := range flags {
		if flag.Name == name {
			return flag
		}
		if collections.ListContainsElement(flag.Aliases, name) {
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

func (flags Flags) normalize(set *libflag.FlagSet) error {
	visited := make(map[string]bool)
	set.Visit(func(f *libflag.Flag) {
		visited[f.Name] = true
	})

	for _, flag := range flags {
		var firstVisit string

		for _, name := range flag.Names() {
			if !visited[name] {
				continue
			}

			if firstVisit != "" {
				return errors.Errorf("cannot use two forms of the same flag: %q %q", firstVisit, name)
			}
			firstVisit = name
		}
	}

	return nil
}
