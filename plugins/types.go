package plugins

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"os/exec"
)

// RunCmd struct used to send the command to the plugin
type RunCmd struct {
	TerragruntOptions *options.TerragruntOptions
	WorkingDir        string
	Cmd               *exec.Cmd
	AllocatePseudoTty bool
}

// RunCmdResponse command execution response
type RunCmdResponse struct {
	Output *shell.CmdOutput
}

// Plugin interface
type Plugin interface {
	Init(opts *options.TerragruntOptions) error
	Run(runCmd *RunCmd) (*RunCmdResponse, error)
}
