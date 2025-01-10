// Package defaultcmd represents the default CLI command.
package defaultcmd

import (
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"

	runCmd "github.com/gruntwork-io/terragrunt/cli/commands/run"
)

const (
	CommandName     = ""
	CommandHelpName = "*"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:     CommandName,
		HelpName: CommandHelpName,
		Usage:    "Terragrunt forwards all other commands directly to OpenTofu/Terraform",
		Flags:    runCmd.NewFlags(opts),
		Action:   runCmd.Action(opts),
	}
}
