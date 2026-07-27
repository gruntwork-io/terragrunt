package iam_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/stretchr/testify/assert"
)

// TestMergeRoleOptions pins the precedence rule callers rely on when combining
// the iam_role config attribute (target) with the --iam-assume-role CLI flag or
// TG_IAM_ASSUME_ROLE env options (source): every non-zero source field wins,
// and zero source fields keep the target's value.
func TestMergeRoleOptions(t *testing.T) {
	t.Parallel()

	attr := iam.RoleOptions{
		RoleARN:               "arn:aws:iam::111111111111:role/from-attribute",
		AssumeRoleSessionName: "attribute-session",
		AssumeRoleDuration:    1800,
		WebIdentityToken:      "attribute-token",
	}
	flag := iam.RoleOptions{
		RoleARN:               "arn:aws:iam::111111111111:role/from-flag",
		AssumeRoleSessionName: "flag-session",
		AssumeRoleDuration:    900,
		WebIdentityToken:      "flag-token",
	}

	testCases := []struct {
		name   string
		target iam.RoleOptions
		source iam.RoleOptions
		want   iam.RoleOptions
	}{
		{
			name:   "both-empty",
			target: iam.RoleOptions{},
			source: iam.RoleOptions{},
			want:   iam.RoleOptions{},
		},
		{
			name:   "attribute-only",
			target: attr,
			source: iam.RoleOptions{},
			want:   attr,
		},
		{
			name:   "flag-only",
			target: iam.RoleOptions{},
			source: flag,
			want:   flag,
		},
		{
			name:   "flag-overrides-attribute",
			target: attr,
			source: flag,
			want:   flag,
		},
		{
			name:   "partial-flag-keeps-remaining-attribute-fields",
			target: attr,
			source: iam.RoleOptions{AssumeRoleDuration: 900},
			want: iam.RoleOptions{
				RoleARN:               "arn:aws:iam::111111111111:role/from-attribute",
				AssumeRoleSessionName: "attribute-session",
				AssumeRoleDuration:    900,
				WebIdentityToken:      "attribute-token",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, iam.MergeRoleOptions(tc.target, tc.source))
		})
	}
}
