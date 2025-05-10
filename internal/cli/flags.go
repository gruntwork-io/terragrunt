package cli

import (
	libflag "flag"
	"io"
	"sort"
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/errors"
)

type Flags []Flag

var IgnoringUndefinedFlagErrorHandler = func(err error) error {
	if errors.As(err, new(UndefinedFlagError)) {
		return nil
	}

	return err
}

func (flags Flags) FilterBySourceType(sourceType FlagValueSourceType) Flags {
	var filtered Flags

	for _, flag := range flags {
		if flag.Value().SourceType() == sourceType {
			filtered = append(filtered, flag)
		}
	}

	return filtered
}

// Parse parses the given `args` to the `flags` and returns the rest of args for which no flags were found.
// Essentially this is a wrapper for `flagSet.Parse` Golang flag parser,
// which allows us to parse all `args` in a few tries and not fall on the first failure.
func (flags Flags) Parse(args Args, errHandler FlagErrorHandler) (Args, error) {
	flagSet := libflag.NewFlagSet("", libflag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	if err := flags.Apply(flagSet, errHandler); err != nil {
		return nil, err
	}

	args, builtinCmd := args.Split(BuiltinCmdSep)

	const maxFlagsParse = 1000 // Maximum flags parse

	var (
		undefArgs Args
		err       error
	)

	for range maxFlagsParse {
		if !args.Present() {
			break
		}

		if err = flagSet.Parse(args); err != nil {
			errStr := err.Error()

			if strings.HasPrefix(errStr, ErrMsgFlagHelpRequested) {
				return append(undefArgs, "-h"), nil
			}

			if strings.HasPrefix(errStr, ErrMsgFlagUndefined) {
				undefArg := strings.Trim(strings.TrimPrefix(errStr, ErrMsgFlagUndefined), " -")
				err = UndefinedFlagError{CmdName: undefArgs.First(), Arg: undefArg}

				for i, arg := range args {
					// `--var=input=from_env` trims to `var`
					trimmed := strings.SplitN(strings.Trim(arg, "-"), "=", 2)[0] //nolint:mnd
					if trimmed == undefArg {
						undefArgs = append(undefArgs, arg)
						args = args[i+1:]

						break
					}
				}
			}
		} else if args = Args(flagSet.Args()); args.Present() {
			undefArgs = append(undefArgs, args.First())
			args = args.Tail()
		}

		if errHandler != nil {
			err = errHandler(err)
		}

		if err != nil {
			break
		}
	}

	if err != nil && errors.As(err, new(FatalFlagError)) {
		return nil, err
	}

	undefArgs = append(undefArgs, args...)

	if len(builtinCmd) > 0 {
		undefArgs = append(undefArgs, BuiltinCmdSep)
		undefArgs = append(undefArgs, builtinCmd...)
	}

	return undefArgs, err
}

func (flags Flags) ApplyConfig(cfgGetter FlagConfigGetter) error {
	for _, flag := range flags {
		if err := ApplyConfig(flag, cfgGetter); err != nil {
			return err
		}
	}

	return nil
}

func (flags Flags) Apply(flagSet *libflag.FlagSet, errHandler FlagErrorHandler) error {
	for _, flag := range flags {
		if err := flag.Apply(flagSet); err != nil {
			if errHandler == nil {
				return err
			}

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
