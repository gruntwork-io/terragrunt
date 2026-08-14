package iam_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	sessionNamePrefix          = "terragrunt-"
	sessionNameFreshnessWindow = time.Hour
)

// TestGetDefaultAssumeRoleSessionName pins the generated STS RoleSessionName format: the "terragrunt-" prefix CloudTrail attributes sessions by, followed by a bare current-time nanosecond suffix.
func TestGetDefaultAssumeRoleSessionName(t *testing.T) {
	t.Parallel()

	name := iam.GetDefaultAssumeRoleSessionName()

	suffix, hasPrefix := strings.CutPrefix(name, sessionNamePrefix)
	require.True(t, hasPrefix, "CloudTrail identifies terragrunt sessions by the %q prefix", sessionNamePrefix)

	nanos, err := strconv.ParseInt(suffix, 10, 64)
	require.NoError(t, err, "the suffix has to stay a bare integer, which also keeps the name inside the character set STS accepts")

	assert.Less(t, time.Since(time.Unix(0, nanos)).Abs(), sessionNameFreshnessWindow, "the suffix encodes the current time in nanoseconds so separate runs get separate STS sessions")
}

// TestDefaultAssumeRoleDuration pins the default STS DurationSeconds at the one hour callers get when they multiply the constant by time.Second.
func TestDefaultAssumeRoleDuration(t *testing.T) {
	t.Parallel()

	assert.Equal(t, time.Hour, time.Duration(iam.DefaultAssumeRoleDuration)*time.Second, "callers convert the constant with time.Second, so it has to stay a count of seconds inside the 15 minute to 12 hour range STS accepts")
}

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
		{
			name:   "flag-role-arn-only",
			target: attr,
			source: iam.RoleOptions{RoleARN: "arn:aws:iam::111111111111:role/from-flag"},
			want: iam.RoleOptions{
				RoleARN:               "arn:aws:iam::111111111111:role/from-flag",
				AssumeRoleSessionName: "attribute-session",
				AssumeRoleDuration:    1800,
				WebIdentityToken:      "attribute-token",
			},
		},
		{
			name:   "flag-session-name-only",
			target: attr,
			source: iam.RoleOptions{AssumeRoleSessionName: "flag-session"},
			want: iam.RoleOptions{
				RoleARN:               "arn:aws:iam::111111111111:role/from-attribute",
				AssumeRoleSessionName: "flag-session",
				AssumeRoleDuration:    1800,
				WebIdentityToken:      "attribute-token",
			},
		},
		{
			name:   "flag-web-identity-token-only",
			target: attr,
			source: iam.RoleOptions{WebIdentityToken: "flag-token"},
			want: iam.RoleOptions{
				RoleARN:               "arn:aws:iam::111111111111:role/from-attribute",
				AssumeRoleSessionName: "attribute-session",
				AssumeRoleDuration:    1800,
				WebIdentityToken:      "flag-token",
			},
		},
		{
			name:   "empty-target-takes-single-flag-field",
			target: iam.RoleOptions{},
			source: iam.RoleOptions{RoleARN: "arn:aws:iam::111111111111:role/from-flag"},
			want:   iam.RoleOptions{RoleARN: "arn:aws:iam::111111111111:role/from-flag"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, iam.MergeRoleOptions(tc.target, tc.source))
		})
	}
}
