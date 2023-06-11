package awsproviderpatch

import (
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	cmdAWSProviderPatch = "aws-provider-patch"

	flagTerragruntOverrideAttr = "terragrunt-override-attr"
)

func NewCommand(globalOpts *options.TerragruntOptions) *cli.Command {
	opts := NewOptions(globalOpts)

	command := &cli.Command{
		Name:  cmdAWSProviderPatch,
		Usage: "Overwrite settings on nested AWS providers to work around a Terraform bug (issue #13018).",
		Flags: cli.Flags{},
		Action: func(ctx *cli.Context) error {
			if len(opts.AwsProviderPatchOverrides) == 0 {
				return errors.Errorf("You must specify at least one provider attribute to override via the --%s option.", flagTerragruntOverrideAttr)
			}

			return Run(opts)
		},
	}

	command.AddFlags(
		&cli.MapFlag[string, string]{
			Name:        flagTerragruntOverrideAttr,
			Destination: &opts.AwsProviderPatchOverrides,
			EnvVar:      "TERRAGRUNT_EXCLUDE_DIR",
			Usage:       "A key=value attribute to override in a provider block as part of the aws-provider-patch command. May be specified multiple times.",
		},
	)

	return command
}
