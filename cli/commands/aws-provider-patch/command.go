package awsproviderpatch

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	// CommandName is the name of the command.
	CommandName = "aws-provider-patch"

	// FlagNameTerragruntOverrideAttr is the name of the flag that specifies the
	// attribute to override in the provider block.
	FlagNameTerragruntOverrideAttr = "terragrunt-override-attr"
)

// NewFlags returns the flags for the aws-provider-patch command.
func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.MapFlag[string, string]{
			Name:        FlagNameTerragruntOverrideAttr,
			Destination: &opts.AwsProviderPatchOverrides,
			EnvVar:      "TERRAGRUNT_EXCLUDE_DIR",
			Usage:       "A key=value attribute to override in a provider block as part of the aws-provider-patch command. May be specified multiple times.", //nolint:lll
		},
	}
}

// NewCommand returns the aws-provider-patch command.
func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Overwrite settings on nested AWS providers to work around a Terraform bug (issue #13018).",
		Flags:  NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error { return Run(ctx, opts.OptionsFromContext(ctx)) },
	}
}
