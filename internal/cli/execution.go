package cli

import "slices"

// TerraformExecution holds the command and arguments for a terraform execution.
// This is passed separately from TerragruntOptions to make the data flow explicit.
//
// ARCHITECTURE:
//   - DiscoveryContext.Cmd/Args is the source of truth (mutable during discovery/preparation)
//   - TerraformExecution is created from DiscoveryContext via ToExecution()
//   - TerraformExecution is passed to run.Run() for execution
//   - TerragruntOptions remains for configuration only, not for command/args
type TerraformExecution struct {
	Cmd  string   // The terraform command (plan, apply, destroy, etc.)
	Args []string // Arguments after the command
}

// TerraformCliArgs returns the full CLI args [cmd, args...] for passing to terraform.
func (e *TerraformExecution) TerraformCliArgs() []string {
	if e == nil {
		return nil
	}

	return append([]string{e.Cmd}, e.Args...)
}

// First returns the command name (same as Cmd, for compatibility with cli.Args).
func (e *TerraformExecution) First() string {
	if e == nil {
		return ""
	}

	return e.Cmd
}

// Second returns the second argument (first arg after command), or empty string if no args.
func (e *TerraformExecution) Second() string {
	if e == nil || len(e.Args) == 0 {
		return ""
	}

	return e.Args[0]
}

// Last returns the last argument, or empty string if no args.
func (e *TerraformExecution) Last() string {
	if e == nil || len(e.Args) == 0 {
		return ""
	}

	return e.Args[len(e.Args)-1]
}

// HasArg checks if args contain the specified argument.
func (e *TerraformExecution) HasArg(arg string) bool {
	if e == nil {
		return false
	}

	return slices.Contains(e.Args, arg)
}

// InsertArg inserts an argument at the specified position.
// Does nothing if the argument already exists or if e is nil.
func (e *TerraformExecution) InsertArg(arg string, position int) {
	if e == nil || e.HasArg(arg) {
		return
	}

	e.Args = slices.Insert(e.Args, position, arg)
}

// AppendArg appends an argument to the end of the args list.
func (e *TerraformExecution) AppendArg(arg string) {
	if e == nil {
		return
	}

	e.Args = append(e.Args, arg)
}

// Clone creates a deep copy of the TerraformExecution.
func (e *TerraformExecution) Clone() *TerraformExecution {
	if e == nil {
		return nil
	}

	return &TerraformExecution{
		Cmd:  e.Cmd,
		Args: slices.Clone(e.Args),
	}
}
