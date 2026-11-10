// Package awsproviderpatch provides the `aws-provider-patch` command.
//
// The `aws-provider-patch` command finds all Terraform modules nested in the current code (i.e., in the .terraform/modules
// folder), looks for provider "aws" { ... } blocks in those modules, and overwrites the attributes in those provider
// blocks with the attributes specified in terragrntOptions.
//
// For example, if were running Terragrunt against code that contained a module:
//
//	module "example" {
//	  source = "<URL>"
//	}
//
// When you run 'init', Terraform would download the code for that module into .terraform/modules. This function would
// scan that module code for provider blocks:
//
//	provider "aws" {
//	   region = var.aws_region
//	}
//
// And if AwsProviderPatchOverrides in opts was set to map[string]string{"region": "us-east-1"}, then this
// method would update the module code to:
//
//	provider "aws" {
//	   region = "us-east-1"
//	}
//
// This is a temporary workaround for a Terraform bug (https://github.com/hashicorp/terraform/issues/13018) where
// any dynamic values in nested provider blocks are not handled correctly when you call 'terraform import', so by
// temporarily hard-coding them, we can allow 'import' to work.
package awsproviderpatch

import (
	"context"

	runcmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// CommandName is the name of the `terragrunt aws-provider-patch` command.
	CommandName = "aws-provider-patch"

	// OverrideAttrFlagName is the name of the flag that sets a key=value
	// attribute to overwrite in nested provider "aws" blocks.
	OverrideAttrFlagName = "override-attr"
)

// NewFlags returns the flags of `terragrunt aws-provider-patch`, bound to opts.
func NewFlags(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	return clihelper.Flags{
		flags.NewFlag(
			&clihelper.MapFlag[string, string]{
				Name:        OverrideAttrFlagName,
				EnvVars:     tgPrefix.EnvVars(OverrideAttrFlagName),
				Destination: &opts.AwsProviderPatchOverrides,
				Usage:       "A key=value attribute to override in a provider block as part of the aws-provider-patch command. May be specified multiple times.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("override-attr"),
				opts.StrictControls,
			),
		),
	}
}

// NewCommand returns the hidden, deprecated `terragrunt aws-provider-patch`
// command, which fails when its deprecated-command strict control is enabled.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	control := controls.NewDeprecatedCommand(CommandName)
	opts.StrictControls.FilterByNames(controls.DeprecatedCommands, controls.CLIRedesign, CommandName).
		AddSubcontrols(control)

	cmdFlags := append(runcmd.NewFlags(l, opts, v, nil), NewFlags(l, opts, nil)...)
	cmdFlags = append(cmdFlags, shared.NewAllFlag(opts, nil))

	cmd := &clihelper.Command{
		Name:   CommandName,
		Usage:  "Overwrite settings on nested AWS providers to work around a Terraform bug (issue #13018).",
		Hidden: true,
		Flags:  cmdFlags,
		Before: func(ctx context.Context, _ *clihelper.Context) error {
			if err := control.Evaluate(ctx); err != nil {
				return clihelper.NewExitError(err, clihelper.ExitCodeGeneralError)
			}

			return nil
		},
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			return Run(ctx, l, v, opts.OptionsFromContext(ctx))
		},
		DisabledErrorOnUndefinedFlag: true,
	}

	return cmd
}
