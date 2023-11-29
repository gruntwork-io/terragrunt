package catalog

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "catalog"

	FlagNameTerragruntJSONOut = "terragrunt-json-out"
	FlagNameWithMetadata      = "with-metadata"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                   CommandName,
		DisallowUndefinedFlags: true,
		Usage:                  "Browse and search the modules.",
		Description:            "Browse and search the modules.",
		Action:                 func(ctx *cli.Context) error { return Run(ctx, opts.OptionsFromContext(ctx)) },
	}
}
