package command

import (
	"fmt"
	"io"
)

type Scaffold struct {
	moduleDir string
}

func NewScaffold(moduleDir string) *Scaffold {
	return &Scaffold{
		moduleDir: moduleDir,
	}
}

func (cmd *Scaffold) Run() error {
	fmt.Println("run Scaffold", cmd.moduleDir)
	return nil
}

func (cmd *Scaffold) SetStdin(io.Reader) {
}

func (cmd *Scaffold) SetStdout(io.Writer) {
}

func (cmd *Scaffold) SetStderr(io.Writer) {
}
