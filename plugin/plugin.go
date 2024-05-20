package plugin

import (
	"context"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"os/exec"
)

type Plugin interface {
	Run(ctx context.Context,
		terragruntOptions *options.TerragruntOptions,
		workingDir string,
		allocatePseudoTty bool,
		cmd *exec.Cmd) (*shell.CmdOutput, error)
}
