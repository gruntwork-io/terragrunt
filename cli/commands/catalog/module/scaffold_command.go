package module

import (
	"io"
)

type ScaffoldCommand struct {
	moduelDir string
}

func NewScaffoldCommand(moduelDir string) *ScaffoldCommand {
	return &ScaffoldCommand{
		moduelDir: moduelDir,
	}
}

func (cmd *ScaffoldCommand) Run() error {
	return nil
}

func (cmd *ScaffoldCommand) SetStdin(io.Reader) {
}

func (cmd *ScaffoldCommand) SetStdout(io.Writer) {
}

func (cmd *ScaffoldCommand) SetStderr(io.Writer) {
}
