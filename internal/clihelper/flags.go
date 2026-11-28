package clihelper

import (
	"context"
	libflag "flag"
	"io"
	"slices"
	"sort"
)

// Flags is the list of flags a command or app accepts.
type Flags []Flag

// Parse calls [Flag.Parse] on each flag with args and env, stopping at the first error.
func (flags Flags) Parse(args Args, env map[string]string) error {
	for _, flag := range flags {
		if err := flag.Parse(args, env); err != nil {
			return err
		}
	}

	return nil
}

// NewFlagSet returns a standard flag set named cmdName that holds every flag, applied with
// [Flags.Apply]. The flag set writes no output.
func (flags Flags) NewFlagSet(
	cmdName string,
	env map[string]string,
	errHandler func(err error) error,
) (*libflag.FlagSet, error) {
	flagSet := libflag.NewFlagSet(cmdName, libflag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	err := flags.Apply(flagSet, env, errHandler)

	return flagSet, err
}

// Apply registers every flag with flagSet, resolving env vars against env. Each apply error
// goes to errHandler, and the first error errHandler returns stops the loop.
func (flags Flags) Apply(
	flagSet *libflag.FlagSet,
	env map[string]string,
	errHandler func(err error) error,
) error {
	for _, flag := range flags {
		if err := flag.Apply(flagSet, env); err != nil {
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
		if slices.Contains(flag.Names(), name) {
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
			if slices.Contains(flag.Names(), name) {
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

// Len returns the number of flags.
func (flags Flags) Len() int {
	return len(flags)
}

// Less orders flags by first name with [LexicographicLess]. Flags without a name sort first.
func (flags Flags) Less(i, j int) bool {
	if len(flags[j].Names()) == 0 {
		return false
	} else if len(flags[i].Names()) == 0 {
		return true
	}

	return LexicographicLess(flags[i].Names()[0], flags[j].Names()[0])
}

// Swap swaps the flags at i and j.
func (flags Flags) Swap(i, j int) {
	flags[i], flags[j] = flags[j], flags[i]
}

// RunActions runs the action of each flag that was set, stopping at the first error.
func (flags Flags) RunActions(ctx context.Context, cliCtx *Context) error {
	for _, flag := range flags {
		if flag.Value().IsSet() {
			if err := flag.RunAction(ctx, cliCtx); err != nil {
				return err
			}
		}
	}

	return nil
}

// Sort sorts flags in place by first name and returns them.
func (flags Flags) Sort() Flags {
	sort.Sort(flags)

	return flags
}

// WithSubcommandScope returns the flags that may also appear after a subcommand of the command
// that defines them.
func (flags Flags) WithSubcommandScope() Flags {
	var filtered Flags

	for _, flag := range flags {
		if flag.AllowedSubcommandScope() {
			filtered = append(filtered, flag)
		}
	}

	return filtered
}

// Names returns the names and aliases of every flag.
func (flags Flags) Names() []string {
	names := make([]string, 0, len(flags))

	for _, flag := range flags {
		names = append(names, flag.Names()...)
	}

	return names
}
