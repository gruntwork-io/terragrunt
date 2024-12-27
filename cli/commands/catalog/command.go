// Package catalog provides the ability to interact with a catalog of OpenTofu/Terraform modules
// via the `terragrunt catalog` command.
package catalog

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "catalog"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return scaffold.NewFlags(opts).Filter(
		scaffold.RootFileNameFlagName,
		scaffold.NoIncludeRootFlagName,
	)
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                 CommandName,
		Usage:                "Launch the user interface for searching and managing your module catalog.",
		ErrorOnUndefinedFlag: true,
		Flags:                NewFlags(opts),
		Action: func(ctx *cli.Context) error {
			var repoPath string

			if val := ctx.Args().Get(0); val != "" {
				repoPath = val
			}

			if opts.ScaffoldRootFileName == "" {
				opts.ScaffoldRootFileName = scaffold.GetDefaultRootFileName(opts)
			}

			return Run(ctx, opts.OptionsFromContext(ctx), repoPath)
		},
	}
}
