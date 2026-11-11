package shared

import (
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// IAMAssumeRoleFlagName is the flag name for `--iam-assume-role`, the ARN of the IAM role to assume
	// before running tofu.
	IAMAssumeRoleFlagName = "iam-assume-role"
	// IAMAssumeRoleDurationFlagName is the flag name for `--iam-assume-role-duration`, the session duration
	// in seconds.
	IAMAssumeRoleDurationFlagName = "iam-assume-role-duration"
	// IAMAssumeRoleSessionNameFlagName is the flag name for `--iam-assume-role-session-name`, the name of
	// the assumed role session.
	IAMAssumeRoleSessionNameFlagName = "iam-assume-role-session-name"
	// IAMAssumeRoleWebIdentityTokenFlagName is the flag name for `--iam-assume-role-web-identity-token`,
	// the token Terragrunt passes to AssumeRoleWithWebIdentity.
	IAMAssumeRoleWebIdentityTokenFlagName = "iam-assume-role-web-identity-token"
)

// NewIAMAssumeRoleFlags creates flags for IAM assume role configuration.
func NewIAMAssumeRoleFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	return clihelper.Flags{
		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        IAMAssumeRoleFlagName,
				EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleFlagName),
				Destination: &opts.IAMRoleOptions.RoleARN,
				Usage:       "Assume the specified IAM role before executing OpenTofu/Terraform.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("iam-role"), opts.StrictControls),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[int64]{
				Name:        IAMAssumeRoleDurationFlagName,
				EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleDurationFlagName),
				Destination: &opts.IAMRoleOptions.AssumeRoleDuration,
				Usage:       "Session duration for IAM Assume Role session.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("iam-assume-role-duration"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        IAMAssumeRoleSessionNameFlagName,
				EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleSessionNameFlagName),
				Destination: &opts.IAMRoleOptions.AssumeRoleSessionName,
				Usage:       "Name for the IAM Assumed Role session.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("iam-assume-role-session-name"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        IAMAssumeRoleWebIdentityTokenFlagName,
				EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleWebIdentityTokenFlagName),
				Destination: &opts.IAMRoleOptions.WebIdentityToken,
				Usage:       "For AssumeRoleWithWebIdentity, the WebIdentity token.",
			},
			flags.WithDeprecatedEnvVars(
				slices.Concat(
					terragruntPrefix.EnvVars("iam-web-identity-token"),
					terragruntPrefix.EnvVars("iam-assume-role-web-identity-token"),
				),
				opts.StrictControls,
			),
		),
	}
}
