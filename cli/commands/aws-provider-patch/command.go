package awsproviderpatch

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "aws-provider-patch"

	FlagNameTerragruntOverrideAttr = "terragrunt-override-attr"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.MapFlag[string, string]{
			Name:        FlagNameTerragruntOverrideAttr,
			Destination: &opts.AwsProviderPatchOverrides,
			EnvVar:      "TERRAGRUNT_EXCLUDE_DIR",
			Usage:       "A key=value attribute to override in a provider block as part of the aws-provider-patch command. May be specified multiple times.",
		},
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Overwrite settings on nested AWS providers to work around a Terraform bug (issue #13018).",
		Flags:  NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error { return Run(opts.OptionsFromContext(ctx)) },
	}
}
