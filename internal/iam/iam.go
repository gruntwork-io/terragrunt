// Package iam provides shared types for IAM role configuration used across
// multiple packages (options, config, awshelper, etc.).
package iam

// RoleOptions represents options that are used by Terragrunt to assume an IAM role.
type RoleOptions struct {
	RoleARN               string
	WebIdentityToken      string
	AssumeRoleSessionName string
	AssumeRoleDuration    int64
}

// MergeRoleOptions merges the source IAM role options into the target, preferring
// non-zero source values.
func MergeRoleOptions(target RoleOptions, source RoleOptions) RoleOptions {
	out := target

	if source.RoleARN != "" {
		out.RoleARN = source.RoleARN
	}

	if source.AssumeRoleDuration != 0 {
		out.AssumeRoleDuration = source.AssumeRoleDuration
	}

	if source.AssumeRoleSessionName != "" {
		out.AssumeRoleSessionName = source.AssumeRoleSessionName
	}

	if source.WebIdentityToken != "" {
		out.WebIdentityToken = source.WebIdentityToken
	}

	return out
}
