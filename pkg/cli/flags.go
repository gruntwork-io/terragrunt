package cli

import (
	"flag"
	"io"
)

type Flags []*Flag

func (flags Flags) flagSet(name string) (*flag.FlagSet, error) {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(io.Discard)

	for _, flag := range flags {
		if err := flag.Apply(set); err != nil {
			return nil, err
		}
	}

	return set, nil
}

// BoolFlags returns a set of boolean Flags.
func (flags Flags) BoolFlags() Flags {
	var boolFlags Flags

	for _, flag := range flags {
		if _, ok := flag.Destination.(*bool); ok {
			boolFlags = append(boolFlags, flag)
		}
	}

	return boolFlags
}

// Get returns a Flag by the given name.
func (flags Flags) Get(name string) *Flag {
	for _, flag := range flags {
		if flag.Name == name {
			return flag
		}
	}

	return nil
}
