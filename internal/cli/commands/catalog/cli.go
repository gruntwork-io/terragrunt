// Package catalog provides the ability to interact with a catalog of OpenTofu/Terraform modules
// via the `terragrunt catalog` command.
package catalog

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "catalog"
)

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	return shared.NewScaffoldingFlags(opts, prefix)
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	return &clihelper.Command{
		Name:  CommandName,
		Usage: "Launch the user interface for searching and managing your module catalog.",
		Flags: NewFlags(opts, nil),
		Action: func(ctx context.Context, cliCtx *clihelper.Context) error {
			var repoPath string

			if val := cliCtx.Args().Get(0); val != "" {
				repoPath = val
			}

			if opts.ScaffoldRootFileName == "" {
				opts.ScaffoldRootFileName = scaffold.GetDefaultRootFileName(ctx, opts)
			}

			return Run(ctx, l, opts.OptionsFromContext(ctx), repoPath)
		},
	}
}
