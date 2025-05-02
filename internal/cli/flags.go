package cli

import (
	libflag "flag"
	"io"
	"sort"

	"github.com/gruntwork-io/go-commons/collections"
)

type Flags []Flag

func (flags Flags) Parse(args Args) error {
	for _, flag := range flags {
		if err := flag.Parse(args); err != nil {
			return err
		}
	}

	return nil
}

func (flags Flags) NewFlagSet(cmdName string, errHandler func(err error) error) (*libflag.FlagSet, error) {
	flagSet := libflag.NewFlagSet(cmdName, libflag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	err := flags.Apply(flagSet, errHandler)

	return flagSet, err
}

func (flags Flags) Apply(flagSet *libflag.FlagSet, errHandler func(err error) error) error {
	for _, flag := range flags {
		if err := flag.Apply(flagSet); err != nil {
			if err = errHandler(err); err != nil {
				return err
			}
		}
	}

	return nil
}

// Get returns a Flag by the given name.
func (flags Flags) Get(name string) Flag {
	for _, flag := range flags {
		if collections.ListContainsElement(flag.Names(), name) {
			return flag
		}
	}

	return nil
}

// Filter returns a list of flags filtered by the given names.
func (flags Flags) Filter(names ...string) Flags {
	var filtered = make(Flags, 0, len(names))

	for _, flag := range flags {
		for _, name := range names {
			if collections.ListContainsElement(flag.Names(), name) {
				filtered = append(filtered, flag)
			}
		}
	}

	return filtered
}

// Add adds a new flag to the list.
func (flags Flags) Add(newFlags ...Flag) Flags {
	return append(flags, newFlags...)
}

// VisibleFlags returns a slice of the Flags.
// Used by `urfave/cli` package to generate help.
func (flags Flags) VisibleFlags() Flags {
	var visibleFlags = make(Flags, 0, len(flags))

	for _, flag := range flags {
		if !flag.GetHidden() && len(flag.Names()) > 0 {
			visibleFlags = append(visibleFlags, flag)
		}
	}

	return visibleFlags
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

	return LexicographicLess(flags[i].Names()[0], flags[j].Names()[0])
}

func (flags Flags) Swap(i, j int) {
	flags[i], flags[j] = flags[j], flags[i]
}

func (flags Flags) RunActions(ctx *Context) error {
	for _, flag := range flags {
		if flag.Value().IsSet() {
			if err := flag.RunAction(ctx); err != nil {
				return err
			}
		}
	}

	return nil
}

func (flags Flags) Sort() Flags {
	sort.Sort(flags)

	return flags
}

func (flags Flags) WithSubcommandScope() Flags {
	var filtered Flags

	for _, flag := range flags {
		if flag.AllowedSubcommandScope() {
			filtered = append(filtered, flag)
		}
	}

	return filtered
}

func (flags Flags) Names() []string {
	var names []string

	for _, flag := range flags {
		names = append(names, flag.Names()...)
	}

	return names
}
