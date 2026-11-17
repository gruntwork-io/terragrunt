package flags

import "fmt"

// GlobalFlagHintError reports a flag given at the global level that only a
// command accepts, and names the command that takes it.
type GlobalFlagHintError struct {
	undefFlag string
	cmdHint   string
	flagHint  string
}

// NewGlobalFlagHintError returns a [GlobalFlagHintError] for undefFlag that
// suggests `cmdHint --flagHint` instead.
func NewGlobalFlagHintError(undefFlag, cmdHint, flagHint string) *GlobalFlagHintError {
	return &GlobalFlagHintError{
		undefFlag: undefFlag,
		cmdHint:   cmdHint,
		flagHint:  flagHint,
	}
}

func (err GlobalFlagHintError) Error() string {
	return fmt.Sprintf(
		"flag `--%s` is not a valid global flag. Did you mean to use `%s --%s`?",
		err.undefFlag,
		err.cmdHint,
		err.flagHint,
	)
}

// CommandFlagHintError reports a flag given to a command that does not accept
// it, and names the command that does.
type CommandFlagHintError struct {
	undefFlag string
	wrongCmd  string
	cmdHint   string
	flagHint  string
}

// NewCommandFlagHintError returns a [CommandFlagHintError] for undefFlag given
// to wrongCmd that suggests `cmdHint --flagHint` instead.
func NewCommandFlagHintError(wrongCmd, undefFlag, cmdHint, flagHint string) *CommandFlagHintError {
	return &CommandFlagHintError{
		undefFlag: undefFlag,
		wrongCmd:  wrongCmd,
		cmdHint:   cmdHint,
		flagHint:  flagHint,
	}
}

func (err CommandFlagHintError) Error() string {
	return fmt.Sprintf(
		"flag `--%s` is not a valid flag for `%s`. Did you mean to use `%s --%s`?",
		err.undefFlag,
		err.wrongCmd,
		err.cmdHint,
		err.flagHint,
	)
}

// PassthroughFlagHintError reports an unknown flag under `terragrunt run` and
// suggests forwarding it to tofu after `--`.
type PassthroughFlagHintError struct {
	undefFlag string
}

// NewPassthroughFlagHintError returns a [PassthroughFlagHintError] for undefFlag.
func NewPassthroughFlagHintError(undefFlag string) *PassthroughFlagHintError {
	return &PassthroughFlagHintError{undefFlag: undefFlag}
}

func (err PassthroughFlagHintError) Error() string {
	return fmt.Sprintf(
		"flag `-%s` is not a Terragrunt flag. If this is an OpenTofu/Terraform flag, use `--` to forward it (e.g., `terragrunt run -- <command> -%s`).",
		err.undefFlag,
		err.undefFlag,
	)
}
