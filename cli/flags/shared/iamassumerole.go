package shared

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	IAMAssumeRoleFlagName                 = "iam-assume-role"
	IAMAssumeRoleDurationFlagName         = "iam-assume-role-duration"
	IAMAssumeRoleSessionNameFlagName      = "iam-assume-role-session-name"
	IAMAssumeRoleWebIdentityTokenFlagName = "iam-assume-role-web-identity-token"
)

// NewIAMAssumeRoleFlags creates flags for IAM assume role configuration.
func NewIAMAssumeRoleFlags(opts *options.TerragruntOptions, prefix flags.Prefix, commandName string) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	var terragruntPrefixControl flags.RegisterStrictControlsFunc
	if commandName != "" {
		terragruntPrefixControl = flags.StrictControlsByCommand(opts.StrictControls, commandName)
	} else {
		terragruntPrefixControl = flags.StrictControlsByGlobalFlags(opts.StrictControls)
	}

	return cli.Flags{
		flags.NewFlag(
			&cli.GenericFlag[string]{
				Name:        IAMAssumeRoleFlagName,
				EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleFlagName),
				Destination: &opts.IAMRoleOptions.RoleARN,
				Usage:       "Assume the specified IAM role before executing OpenTofu/Terraform.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("iam-role"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.GenericFlag[int64]{
				Name:        IAMAssumeRoleDurationFlagName,
				EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleDurationFlagName),
				Destination: &opts.IAMRoleOptions.AssumeRoleDuration,
				Usage:       "Session duration for IAM Assume Role session.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("iam-assume-role-duration"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.GenericFlag[string]{
				Name:        IAMAssumeRoleSessionNameFlagName,
				EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleSessionNameFlagName),
				Destination: &opts.IAMRoleOptions.AssumeRoleSessionName,
				Usage:       "Name for the IAM Assumed Role session.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("iam-assume-role-session-name"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.GenericFlag[string]{
				Name:        IAMAssumeRoleWebIdentityTokenFlagName,
				EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleWebIdentityTokenFlagName),
				Destination: &opts.IAMRoleOptions.WebIdentityToken,
				Usage:       "For AssumeRoleWithWebIdentity, the WebIdentity token.",
			},
			flags.WithDeprecatedEnvVars(
				append(
					terragruntPrefix.EnvVars("iam-web-identity-token"),
					terragruntPrefix.EnvVars("iam-assume-role-web-identity-token")...,
				),
				terragruntPrefixControl,
			),
		),
	}
}
