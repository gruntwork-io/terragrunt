package gcs_test

import (
	"testing"

	gcsbackend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/gcs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_IsEqual(t *testing.T) {
	t.Parallel()

	logger := logger.CreateLogger()

	testCases := []struct { //nolint: govet
		name          string
		cfg           gcsbackend.Config
		comparableCfg gcsbackend.Config
		shouldBeEqual bool
	}{
		{
			"equal-both-empty",
			gcsbackend.Config{},
			gcsbackend.Config{},
			true,
		},
		{
			"equal-empty-and-nil",
			gcsbackend.Config{},
			nil,
			true,
		},
		{
			"equal-one-key",
			gcsbackend.Config{"foo": "bar"},
			gcsbackend.Config{"foo": "bar"},
			true,
		},
		{
			"equal-multiple-keys",
			gcsbackend.Config{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true},
			gcsbackend.Config{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true},
			true,
		},
		{
			"equal-encrypt-bool-handling",
			gcsbackend.Config{"encrypt": true},
			gcsbackend.Config{"encrypt": "true"},
			true,
		},
		{
			"equal-general-bool-handling",
			gcsbackend.Config{"something": true, "encrypt": true},
			gcsbackend.Config{"something": "true", "encrypt": "true"},
			true,
		},
		{
			"equal-ignore-gcs-labels",
			gcsbackend.Config{"foo": "bar", "gcs_bucket_labels": []map[string]string{{"foo": "bar"}}},
			gcsbackend.Config{"foo": "bar"},
			true,
		},
		{
			"unequal-values",
			gcsbackend.Config{"foo": "bar"},
			gcsbackend.Config{"foo": "different"},
			false,
		},
		{
			"unequal-non-empty-cfg-nil",
			gcsbackend.Config{"foo": "bar"},
			nil,
			false,
		},
		{
			"unequal-general-bool-handling",
			gcsbackend.Config{"something": true},
			gcsbackend.Config{"something": "false"},
			false,
		},
		{
			"equal-null-ignored",
			gcsbackend.Config{"something": "foo"},
			gcsbackend.Config{"something": "foo", "ignored-because-null": nil},
			true,
		},
		{
			"terragrunt-only-configs-remain-intact",
			gcsbackend.Config{"something": "foo", "skip_bucket_creation": true},
			gcsbackend.Config{"something": "foo"},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := tc.cfg.IsEqual(tc.comparableCfg, logger)
			assert.Equal(t, tc.shouldBeEqual, actual)
		})
	}
}

// TestParseExtendedGCSConfig_StringBoolCoercion verifies that boolean config values
// passed as strings (e.g. from HCL ternary type unification) are correctly parsed.
// See https://github.com/gruntwork-io/terragrunt/issues/5475
func TestParseExtendedGCSConfig_StringBoolCoercion(t *testing.T) {
	t.Parallel()

	testCases := []struct { //nolint: govet
		name   string
		config gcsbackend.Config
		check  func(t *testing.T, cfg *gcsbackend.ExtendedRemoteStateConfigGCS)
	}{
		{
			"skip-bucket-versioning-string-true",
			gcsbackend.Config{
				"bucket":                 "my-bucket",
				"skip_bucket_versioning": "true",
			},
			func(t *testing.T, cfg *gcsbackend.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.True(t, cfg.SkipBucketVersioning)
			},
		},
		{
			"skip-bucket-versioning-string-false",
			gcsbackend.Config{
				"bucket":                 "my-bucket",
				"skip_bucket_versioning": "false",
			},
			func(t *testing.T, cfg *gcsbackend.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.False(t, cfg.SkipBucketVersioning)
			},
		},
		{
			"skip-bucket-creation-string-true",
			gcsbackend.Config{
				"bucket":               "my-bucket",
				"skip_bucket_creation": "true",
			},
			func(t *testing.T, cfg *gcsbackend.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.True(t, cfg.SkipBucketCreation)
			},
		},
		{
			"enable-bucket-policy-only-string-true",
			gcsbackend.Config{
				"bucket":                    "my-bucket",
				"enable_bucket_policy_only": "true",
			},
			func(t *testing.T, cfg *gcsbackend.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.True(t, cfg.EnableBucketPolicyOnly)
			},
		},
		{
			"native-bool-still-works",
			gcsbackend.Config{
				"bucket":                 "my-bucket",
				"skip_bucket_versioning": true,
			},
			func(t *testing.T, cfg *gcsbackend.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.True(t, cfg.SkipBucketVersioning)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			extGCSCfg, err := tc.config.ParseExtendedGCSConfig()
			require.NoError(t, err)

			tc.check(t, extGCSCfg)
		})
	}
}
