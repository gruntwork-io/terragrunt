package command

import (
	"io"

	"github.com/gruntwork-io/terragrunt/pkg/log"
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
	log.Infof("Run Scaffold for the module: %q", cmd.moduleDir)
	return nil
}

func (cmd *Scaffold) SetStdin(io.Reader) {
}

func (cmd *Scaffold) SetStdout(io.Writer) {
}

func (cmd *Scaffold) SetStderr(io.Writer) {
}
