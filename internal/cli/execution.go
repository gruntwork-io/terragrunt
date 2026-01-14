package cli

import "slices"

// CommandWithArgs holds the command and arguments for a process.
type CommandWithArgs struct {
	Cmd  string
	Args []string
}

// CmdWithArgs returns a slice representing the command and arguments to run a process.
func (e *CommandWithArgs) CmdWithArgs() []string {
	if e == nil {
		return nil
	}

	return append([]string{e.Cmd}, e.Args...)
}

// FirstArg returns the first argument, or an empty string if there are no args.
func (e *CommandWithArgs) FirstArg() string {
	if e == nil || len(e.Args) == 0 {
		return ""
	}

	return e.Args[0]
}

// LastArg returns the last argument, or an empty string if no args.
func (e *CommandWithArgs) LastArg() string {
	if e == nil || len(e.Args) == 0 {
		return ""
	}

	return e.Args[len(e.Args)-1]
}

// HasArg checks if args contain the specified argument.
func (e *CommandWithArgs) HasArg(arg string) bool {
	if e == nil {
		return false
	}

	return slices.Contains(e.Args, arg)
}

// InsertArg inserts an argument at the specified position.
// Does nothing if the argument already exists or if e is nil.
func (e *CommandWithArgs) InsertArg(arg string, position int) {
	if e == nil || e.HasArg(arg) {
		return
	}

	e.Args = slices.Insert(e.Args, position, arg)
}

// AppendArg appends an argument to the end of the args list.
func (e *CommandWithArgs) AppendArg(arg string) {
	if e == nil {
		return
	}

	e.Args = append(e.Args, arg)
}

// Clone creates a deep copy of the TerraformExecution.
func (e *CommandWithArgs) Clone() *CommandWithArgs {
	if e == nil {
		return nil
	}

	return &CommandWithArgs{
		Cmd:  e.Cmd,
		Args: slices.Clone(e.Args),
	}
}
