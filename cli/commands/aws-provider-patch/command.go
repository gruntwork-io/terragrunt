package awsproviderpatch

import (
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "aws-provider-patch"

	FlagNameTerragruntOverrideAttr = "terragrunt-override-attr"
)

func NewCommand(globalOpts *options.TerragruntOptions) *cli.Command {
	opts := NewOptions(globalOpts)

	command := &cli.Command{
		Name:  CommandName,
		Usage: "Overwrite settings on nested AWS providers to work around a Terraform bug (issue #13018).",
		Flags: cli.Flags{},
		Action: func(ctx *cli.Context) error {
			if len(opts.AwsProviderPatchOverrides) == 0 {
				return errors.WithStackTrace(MissingOverrideAttrError(FlagNameTerragruntOverrideAttr))
			}

			return Run(opts)
		},
	}

	command.AddFlags(
		&cli.MapFlag[string, string]{
			Name:        FlagNameTerragruntOverrideAttr,
			Destination: &opts.AwsProviderPatchOverrides,
			EnvVar:      "TERRAGRUNT_EXCLUDE_DIR",
			Usage:       "A key=value attribute to override in a provider block as part of the aws-provider-patch command. May be specified multiple times.",
		},
	)

	return command
}
