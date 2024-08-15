package catalog

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	// CommandName is the name of the command.
	CommandName = "catalog"
)

// NewCommand returns a new instance of the catalog command.
func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                   CommandName,
		DisallowUndefinedFlags: true,
		Usage:                  "Launch the user interface for searching and managing your module catalog.",
		Action: func(ctx *cli.Context) error {
			var repoPath string

			if val := ctx.Args().Get(0); val != "" {
				repoPath = val
			}

			return Run(ctx, opts.OptionsFromContext(ctx), repoPath)
		},
	}
}
