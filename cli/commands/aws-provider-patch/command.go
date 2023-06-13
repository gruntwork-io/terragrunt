package awsproviderpatch

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "aws-provider-patch"
)

var (
	TerragruntFlagNames = append(terraform.TerragruntFlagNames,
		flags.FlagNameTerragruntOverrideAttr,
	)
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Overwrite settings on nested AWS providers to work around a Terraform bug (issue #13018).",
		Flags:  flags.NewFlags(opts).Filter(TerragruntFlagNames),
		Before: func(ctx *cli.Context) error { return ctx.App.Before(ctx) },
		Action: Action(opts),
	}
}

func Action(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		if len(opts.AwsProviderPatchOverrides) == 0 {
			return errors.WithStackTrace(MissingOverrideAttrError(flags.FlagNameTerragruntOverrideAttr))
		}

		return Run(opts)
	}
}
