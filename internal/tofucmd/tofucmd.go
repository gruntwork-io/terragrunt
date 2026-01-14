package tofucmd

import "slices"

// TofuCommand holds the command and arguments for a tofu process.
type TofuCommand struct {
	// Cmd is the subcommand of tofu to run (e.g. "plan", "apply", "destroy", etc.)
	Cmd string

	// Args is the arguments to pass to the subcommand (e.g. "-destroy", "-auto-approve", etc.).
	Args []string
}

// ProcessArgs returns a slice representing the actual arguments passed to the tofu command.
func (e *TofuCommand) ProcessArgs() []string {
	if e == nil {
		return nil
	}

	return append([]string{e.Cmd}, e.Args...)
}

// FirstArg returns the first argument, or an empty string if there are no args.
func (e *TofuCommand) FirstArg() string {
	if e == nil || len(e.Args) == 0 {
		return ""
	}

	return e.Args[0]
}

// LastArg returns the last argument, or an empty string if no args.
func (e *TofuCommand) LastArg() string {
	if e == nil || len(e.Args) == 0 {
		return ""
	}

	return e.Args[len(e.Args)-1]
}

// HasArg checks if args contain the specified argument.
func (e *TofuCommand) HasArg(arg string) bool {
	if e == nil {
		return false
	}

	return slices.Contains(e.Args, arg)
}

// InsertArg inserts an argument at the specified position.
// Does nothing if the argument already exists or if e is nil.
func (e *TofuCommand) InsertArg(arg string, position int) {
	if e == nil || e.HasArg(arg) {
		return
	}

	e.Args = slices.Insert(e.Args, position, arg)
}

// AppendArg appends an argument to the end of the args list.
func (e *TofuCommand) AppendArg(arg string) {
	if e == nil {
		return
	}

	e.Args = append(e.Args, arg)
}

// Clone creates a deep copy of the TerraformExecution.
func (e *TofuCommand) Clone() *TofuCommand {
	if e == nil {
		return nil
	}

	return &TofuCommand{
		Cmd:  e.Cmd,
		Args: slices.Clone(e.Args),
	}
}
