package command

import (
	"io"

	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

type Scaffold struct {
	moduleDir         string
	terragruntOptions *options.TerragruntOptions
}

func NewScaffold(moduleDir string, opts *options.TerragruntOptions) *Scaffold {
	return &Scaffold{
		moduleDir:         moduleDir,
		terragruntOptions: opts,
	}
}

func (cmd *Scaffold) Run() error {
	log.Infof("Run Scaffold for the module: %q", cmd.moduleDir)

	return scaffold.Run(cmd.terragruntOptions, cmd.moduleDir, "")
}

func (cmd *Scaffold) SetStdin(io.Reader) {
}

func (cmd *Scaffold) SetStdout(io.Writer) {
}

func (cmd *Scaffold) SetStderr(io.Writer) {
}
