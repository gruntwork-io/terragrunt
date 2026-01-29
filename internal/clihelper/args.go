package clihelper

import (
	"regexp"
	"slices"
	"strings"
)

const (
	tailMinArgsLen = 2
	BuiltinCmdSep  = "--"

	// minFlagValuePairLen is the minimum number of elements needed for a space-separated flag-value pair (e.g., "-var", "foo=bar")
	minFlagValuePairLen = 2

	// minFlagLen is the minimum length for a valid flag (e.g., "-x" has length 2)
	minFlagLen = 2
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

// Second returns the second argument or a blank string.
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

// booleanFlagsMap contains flags that don't take values (boolean flags).
// Unknown flags are assumed to take space-separated values for safer parsing
// of new Terraform/Tofu flags without requiring updates to this list.
var booleanFlagsMap = map[string]struct{}{
	"allow-missing":     {},
	"auto-approve":      {},
	"check":             {},
	"compact-warnings":  {},
	"destroy":           {},
	"detailed-exitcode": {},
	"diff":              {},
	"force-copy":        {},
	"get":               {},
	"h":                 {},
	"help":              {},
	"input":             {},
	"json":              {},
	"list":              {},
	"lock":              {},
	"migrate-state":     {},
	"no-color":          {},
	"raw":               {},
	"reconfigure":       {},
	"recursive":         {},
	"refresh":           {},
	"refresh-only":      {},
	"update":            {},
	"upgrade":           {},
	"v":                 {},
	"version":           {},
	"write-out":         {},
}

// normalizeFlag strips leading dashes from a flag name.
func normalizeFlag(flag string) string {
	return strings.TrimLeft(flag, "-")
}

// IacArgs represents parsed IaC (terraform/tofu) CLI arguments
// with separate command, flags, and arguments fields.
// Provides a builder pattern for constructing CLI arguments.
//
// Structure: [Command] [SubCommand...] [Flags...] [Arguments...]
// - Command: main terraform command (e.g., "apply", "providers")
// - SubCommand: non-flag args before any flags (e.g., "lock" in "providers lock")
// - Flags: all flags with their values
// - Arguments: non-flag args after flags (e.g., plan files)
type IacArgs struct {
	Command    string   // e.g., "apply", "plan", "providers"
	SubCommand []string // e.g., "lock" in "providers lock -platform=..."
	Flags      []string // e.g., "-input=false", "-auto-approve"
	Arguments  []string // e.g., plan files, resource addresses
}

// NewIacArgs creates IacArgs from strings, parsing command/flags/arguments.
func NewIacArgs(args ...string) *IacArgs {
	result := &IacArgs{
		SubCommand: make([]string, 0),
		Flags:      make([]string, 0),
		Arguments:  make([]string, 0),
	}
	result.parse(args)

	return result
}

// SetCommand sets the command and returns self for chaining.
func (a *IacArgs) SetCommand(cmd string) *IacArgs {
	a.Command = cmd

	return a
}

// AppendFlag adds flag(s) and returns self for chaining.
func (a *IacArgs) AppendFlag(flags ...string) *IacArgs {
	a.Flags = append(a.Flags, flags...)

	return a
}

// InsertFlag inserts flag(s) at position and returns self for chaining.
func (a *IacArgs) InsertFlag(pos int, flags ...string) *IacArgs {
	a.Flags = slices.Insert(a.Flags, pos, flags...)

	return a
}

// AppendArgument adds argument(s) and returns self for chaining.
func (a *IacArgs) AppendArgument(args ...string) *IacArgs {
	a.Arguments = append(a.Arguments, args...)

	return a
}

// InsertArgument inserts an argument at position and returns self for chaining.
func (a *IacArgs) InsertArgument(pos int, arg string) *IacArgs {
	a.Arguments = slices.Insert(a.Arguments, pos, arg)

	return a
}

// InsertArguments inserts arguments at position and returns self for chaining.
func (a *IacArgs) InsertArguments(pos int, args ...string) *IacArgs {
	a.Arguments = slices.Insert(a.Arguments, pos, args...)

	return a
}

// AppendSubCommand adds subcommand(s) and returns self for chaining.
func (a *IacArgs) AppendSubCommand(subs ...string) *IacArgs {
	a.SubCommand = append(a.SubCommand, subs...)

	return a
}

// AddFlagIfNotPresent adds a flag only if not already present.
func (a *IacArgs) AddFlagIfNotPresent(flag string) *IacArgs {
	if !slices.Contains(a.Flags, flag) {
		a.Flags = append(a.Flags, flag)
	}

	return a
}

// HasFlag checks if flag exists (handles -flag, --flag and -flag=value formats).
// Note: Values starting with "-" (like -module.resource) are indistinguishable from flags.
func (a *IacArgs) HasFlag(name string) bool {
	target := normalizeFlag(name)

	return slices.ContainsFunc(a.Flags, func(f string) bool {
		if !strings.HasPrefix(f, "-") {
			return false
		}
		return normalizeFlag(extractFlagName(f)) == target
	})
}

// RemoveFlag removes a flag by name (handles -flag, --flag, -flag=value, and space-separated -flag value).
func (a *IacArgs) RemoveFlag(name string) *IacArgs {
	newFlags := make([]string, 0, len(a.Flags))
	target := normalizeFlag(name)

	for i := 0; i < len(a.Flags); i++ {
		f := a.Flags[i]

		// Only treat tokens starting with "-" as potential flags
		if !strings.HasPrefix(f, "-") {
			newFlags = append(newFlags, f)
			continue
		}

		current := normalizeFlag(extractFlagName(f))

		if current == target {
			// If exact match (no =value) and next entry is a value (doesn't start with "-"), skip it too.
			// BUT: only if it's NOT a boolean flag.
			if !strings.Contains(f, "=") && i+1 < len(a.Flags) && !strings.HasPrefix(a.Flags[i+1], "-") {
				if _, isBool := booleanFlagsMap[current]; !isBool {
					i++ // skip the value
				}
			}

			continue
		}

		newFlags = append(newFlags, f)
	}

	a.Flags = newFlags

	return a
}

// HasPlanFile checks if a plan file is already specified in args.
// Checks for -out= flag (plan command) or any argument present (apply/destroy).
func (a *IacArgs) HasPlanFile() bool {
	// Check for -out= flag (used with plan command)
	if a.HasFlag("-out") {
		return true
	}

	// For apply/destroy: any argument is assumed to be a plan file
	// (can't reliably check file existence - path may be relative or created later)
	return len(a.Arguments) > 0
}

// MergeFlags merges flags from another IacArgs, adding only flags not already present.
// Handles both -flag=value and space-separated -flag value formats.
// Returns self for chaining.
func (a *IacArgs) MergeFlags(other *IacArgs) *IacArgs {
	for i := 0; i < len(other.Flags); i++ {
		flag := other.Flags[i]

		// Check if this is a flag with space-separated value
		hasValue := i+1 < len(other.Flags) &&
			!strings.HasPrefix(other.Flags[i+1], "-") &&
			!strings.Contains(flag, "=") &&
			strings.HasPrefix(flag, "-")

		if hasValue {
			value := other.Flags[i+1]
			if !a.hasFlagWithValue(flag, value) {
				a.Flags = append(a.Flags, flag, value)
			}

			i++ // skip the value in iteration

			continue
		}

		if a.Contains(flag) {
			continue
		}

		a.Flags = append(a.Flags, flag)
	}

	return a
}

// hasFlagWithValue checks if a flag with specific value exists in either format.
func (a *IacArgs) hasFlagWithValue(flag, value string) bool {
	// Check -flag=value format
	if slices.Contains(a.Flags, flag+"="+value) {
		return true
	}

	if len(a.Flags) < minFlagValuePairLen {
		return false
	}

	// Check space-separated format
	for i := 0; i < len(a.Flags)-1; i++ {
		if a.Flags[i] == flag && a.Flags[i+1] == value {
			return true
		}
	}

	return false
}

// Slice returns args in correct order: [command] [flags...] [arguments...]
func (a *IacArgs) Slice() []string {
	result := make([]string, 0, 1+len(a.SubCommand)+len(a.Flags)+len(a.Arguments))

	if a.Command != "" {
		result = append(result, a.Command)
	}

	result = append(result, a.SubCommand...)
	result = append(result, a.Flags...)
	result = append(result, a.Arguments...)

	return result
}

// Clone returns a deep copy of IacArgs.
// Note: This performs a deep copy of slices (Command, SubCommand, Flags, Arguments).
// If IacArgs is extended with pointer fields or nested structs in the future,
// this method must be updated to ensure deep copying of those fields as well.
func (a *IacArgs) Clone() *IacArgs {
	return &IacArgs{
		Command:    a.Command,
		SubCommand: slices.Clone(a.SubCommand),
		Flags:      slices.Clone(a.Flags),
		Arguments:  slices.Clone(a.Arguments),
	}
}

// First returns the command (first element).
func (a *IacArgs) First() string {
	return a.Command
}

// Second returns the second element (first subcommand, first flag, or first argument).
func (a *IacArgs) Second() string {
	if len(a.SubCommand) > 0 {
		return a.SubCommand[0]
	}

	if len(a.Flags) > 0 {
		return a.Flags[0]
	}

	if len(a.Arguments) > 0 {
		return a.Arguments[0]
	}

	return ""
}

// Last returns the last element (last argument, or last flag, or command).
func (a *IacArgs) Last() string {
	if len(a.Arguments) > 0 {
		return a.Arguments[len(a.Arguments)-1]
	}

	if len(a.Flags) > 0 {
		return a.Flags[len(a.Flags)-1]
	}

	return a.Command
}

// Tail returns everything except the command (subcommand, flags, and arguments) as a slice.
func (a *IacArgs) Tail() []string {
	result := make([]string, 0, len(a.SubCommand)+len(a.Flags)+len(a.Arguments))
	result = append(result, a.SubCommand...)
	result = append(result, a.Flags...)
	result = append(result, a.Arguments...)

	return result
}

// Contains checks if the args contain the target (in command, subcommand, flags, or arguments).
func (a *IacArgs) Contains(target string) bool {
	return a.Command == target ||
		slices.Contains(a.SubCommand, target) ||
		slices.Contains(a.Flags, target) ||
		slices.Contains(a.Arguments, target)
}

// Normalize formats the flags according to the given actions.
func (a *IacArgs) Normalize(acts ...NormalizeActsType) *IacArgs {
	result := a.Clone()
	result.Flags = make([]string, 0, len(a.Flags))

	for _, flag := range a.Flags {
		normalized := flag

		for _, act := range acts {
			switch act {
			case SingleDashFlag:
				if doubleDashRegexp.MatchString(normalized) {
					normalized = normalized[1:]
				}
			case DoubleDashFlag:
				if singleDashRegexp.MatchString(normalized) {
					normalized = "-" + normalized
				}
			}
		}

		result.Flags = append(result.Flags, normalized)
	}

	return result
}

// parse parses raw args into Command/SubCommand/Flags/Arguments.
// Known terraform subcommands before any flags go to SubCommand (stay in place).
// Other non-flag args go to Arguments (appear at end).
func (a *IacArgs) parse(args []string) {
	skipNext := false
	seenFlag := false

	for i, arg := range args {
		if skipNext {
			skipNext = false

			continue
		}

		if strings.HasPrefix(arg, "-") {
			seenFlag = true
			skipNext = a.processFlag(arg, args, i)

			continue
		}

		if a.Command == "" {
			a.Command = arg

			continue
		}

		// Known subcommands before flags -> SubCommand (e.g., "lock" in "providers lock")
		// All other non-flag args -> Arguments (e.g., plan files, resource addresses)
		if !seenFlag && IsKnownSubCommand(arg) {
			a.SubCommand = append(a.SubCommand, arg)
		} else {
			a.Arguments = append(a.Arguments, arg)
		}
	}
}

// knownSubCommands lists terraform subcommands that appear after the main command.
// These should NOT be reordered to the end like plan files.
// Maintainers: Add new Terraform/OpenTofu subcommands here as they are introduced.
var knownSubCommands = []string{
	// providers subcommands
	"lock", "mirror", "schema",
	// state subcommands
	"list", "mv", "pull", "push", "replace-provider", "rm", "show",
	// workspace subcommands
	"delete", "new", "select",
	// force-unlock takes an argument, not a subcommand
}

// IsKnownSubCommand returns true if arg is a known terraform subcommand.
func IsKnownSubCommand(arg string) bool {
	return slices.Contains(knownSubCommands, arg)
}

// processFlag handles flag parsing, returns true if next arg should be skipped.
// Unknown flags are assumed to take space-separated values for forward compatibility.
func (a *IacArgs) processFlag(arg string, args []string, i int) bool {
	if len(arg) < minFlagLen {
		// Malformed flag (just "-" or empty), treat as argument or ignore
		a.Flags = append(a.Flags, arg)
		return false
	}

	flagName := extractFlagName(arg)

	// Flag with inline value (-flag=value) - self-contained
	if strings.Contains(arg, "=") {
		a.Flags = append(a.Flags, arg)
		return false
	}

	// Known boolean flag - self-contained
	if _, ok := booleanFlagsMap[normalizeFlag(flagName)]; ok {
		a.Flags = append(a.Flags, arg)
		return false
	}

	// Check if next arg looks like a value (doesn't start with -)
	if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
		a.Flags = append(a.Flags, arg)
		return false
	}

	// Assume unknown flag takes a space-separated value
	a.Flags = append(a.Flags, arg, args[i+1])

	return true
}

// extractFlagName gets flag name before = if present.
func extractFlagName(arg string) string {
	name, _, _ := strings.Cut(arg, "=")
	return name
}
