package cli

import (
	"sort"

	"github.com/gruntwork-io/go-commons/collections"
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

// Filter returns a list of flags filtered by the given names.
func (flags Flags) Filter(names []string) Flags {
	var filtered Flags

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
func (flags *Flags) Add(newFlags ...Flag) {
	*flags = append(*flags, newFlags...)
}

// VisibleFlags returns a slice of the Flags.
// Used by `urfave/cli` package to generate help.
func (flags Flags) VisibleFlags() Flags {
	return flags
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
			if flag, ok := flag.(ActionableFlag); ok {
				if err := flag.RunAction(ctx); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (flags Flags) Sort() Flags {
	sort.Sort(flags)

	return flags
}
