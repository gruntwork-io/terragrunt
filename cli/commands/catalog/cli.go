// Package catalog provides the ability to interact with a catalog of OpenTofu/Terraform modules
// via the `terragrunt catalog` command.
package catalog

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "catalog"
)

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	return shared.NewScaffoldingFlags(opts, prefix)
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:  CommandName,
		Usage: "Launch the user interface for searching and managing your module catalog.",
		Flags: NewFlags(opts, nil),
		Action: func(ctx *cli.Context) error {
			var repoPath string

			if val := ctx.Args().Get(0); val != "" {
				repoPath = val
			}

			if opts.ScaffoldRootFileName == "" {
				opts.ScaffoldRootFileName = scaffold.GetDefaultRootFileName(ctx, opts)
			}

			return Run(ctx, l, opts.OptionsFromContext(ctx), repoPath)
		},
	}
}
