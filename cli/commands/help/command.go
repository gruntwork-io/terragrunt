// Package help represents the help CLI command that works the same as the `--help` flag.
package help

import (
	"github.com/gruntwork-io/terragrunt/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "help"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Show help.",
		Hidden: true,
		Action: func(ctx *cli.Context) error {
			return global.HelpAction(ctx, opts)
		},
	}
}
