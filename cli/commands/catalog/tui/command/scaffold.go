package command

import (
	"fmt"
	"io"
)

type Scaffold struct {
	moduelDir string
}

func NewScaffold(moduelDir string) *Scaffold {
	return &Scaffold{
		moduelDir: moduelDir,
	}
}

func (cmd *Scaffold) Run() error {
	fmt.Println("run Scaffold")
	return nil
}

func (cmd *Scaffold) SetStdin(io.Reader) {
}

func (cmd *Scaffold) SetStdout(io.Writer) {
}

func (cmd *Scaffold) SetStderr(io.Writer) {
}
