package flags

import "fmt"

var _ error = new(GlobalFlagHintError)

type GlobalFlagHintError struct {
	undefFlag string
	cmdHint   string
	flagHint  string
}

func NewGlobalFlagHintError(undefFlag, cmdHint, flagHint string) *GlobalFlagHintError {
	return &GlobalFlagHintError{
		undefFlag: undefFlag,
		cmdHint:   cmdHint,
		flagHint:  flagHint,
	}
}

func (err GlobalFlagHintError) Error() string {
	return fmt.Sprintf("flag `--%s` is not a valid global flag. Did you mean to use `%s --%s`?", err.undefFlag, err.cmdHint, err.flagHint)
}

var _ error = new(CommandFlagHintError)

type CommandFlagHintError struct {
	undefFlag string
	wrongCmd  string
	cmdHint   string
	flagHint  string
}

func NewCommandFlagHintError(wrongCmd, undefFlag, cmdHint, flagHint string) *CommandFlagHintError {
	return &CommandFlagHintError{
		undefFlag: undefFlag,
		wrongCmd:  wrongCmd,
		cmdHint:   cmdHint,
		flagHint:  flagHint,
	}
}

func (err CommandFlagHintError) Error() string {
	return fmt.Sprintf("flag `--%s` is not a valid flag for `%s`. Did you mean to use `%s --%s`?", err.undefFlag, err.wrongCmd, err.cmdHint, err.flagHint)
}
