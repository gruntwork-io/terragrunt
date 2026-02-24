package gcs_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/gcs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_IsEqual(t *testing.T) {
	t.Parallel()

	logger := logger.CreateLogger()

	testCases := []struct { //nolint: govet
		name          string
		cfg           gcs.Config
		comparableCfg gcs.Config
		shouldBeEqual bool
	}{
		{
			"equal-both-empty",
			gcs.Config{},
			gcs.Config{},
			true,
		},
		{
			"equal-empty-and-nil",
			gcs.Config{},
			nil,
			true,
		},
		{
			"equal-one-key",
			gcs.Config{"foo": "bar"},
			gcs.Config{"foo": "bar"},
			true,
		},
		{
			"equal-multiple-keys",
			gcs.Config{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true},
			gcs.Config{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true},
			true,
		},
		{
			"equal-encrypt-bool-handling",
			gcs.Config{"encrypt": true},
			gcs.Config{"encrypt": "true"},
			true,
		},
		{
			"equal-general-bool-handling",
			gcs.Config{"something": true, "encrypt": true},
			gcs.Config{"something": "true", "encrypt": "true"},
			true,
		},
		{
			"equal-ignore-gcs-labels",
			gcs.Config{"foo": "bar", "gcs_bucket_labels": []map[string]string{{"foo": "bar"}}},
			gcs.Config{"foo": "bar"},
			true,
		},
		{
			"unequal-values",
			gcs.Config{"foo": "bar"},
			gcs.Config{"foo": "different"},
			false,
		},
		{
			"unequal-non-empty-cfg-nil",
			gcs.Config{"foo": "bar"},
			nil,
			false,
		},
		{
			"unequal-general-bool-handling",
			gcs.Config{"something": true},
			gcs.Config{"something": "false"},
			false,
		},
		{
			"equal-null-ignored",
			gcs.Config{"something": "foo"},
			gcs.Config{"something": "foo", "ignored-because-null": nil},
			true,
		},
		{
			"terragrunt-only-configs-remain-intact",
			gcs.Config{"something": "foo", "skip_bucket_creation": true},
			gcs.Config{"something": "foo"},
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
		config gcs.Config
		check  func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS)
	}{
		{
			"skip-bucket-versioning-string-true",
			gcs.Config{
				"bucket":                 "my-bucket",
				"skip_bucket_versioning": "true",
			},
			func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.True(t, cfg.SkipBucketVersioning)
			},
		},
		{
			"skip-bucket-versioning-string-false",
			gcs.Config{
				"bucket":                 "my-bucket",
				"skip_bucket_versioning": "false",
			},
			func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.False(t, cfg.SkipBucketVersioning)
			},
		},
		{
			"skip-bucket-creation-string-true",
			gcs.Config{
				"bucket":               "my-bucket",
				"skip_bucket_creation": "true",
			},
			func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.True(t, cfg.SkipBucketCreation)
			},
		},
		{
			"enable-bucket-policy-only-string-true",
			gcs.Config{
				"bucket":                    "my-bucket",
				"enable_bucket_policy_only": "true",
			},
			func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.True(t, cfg.EnableBucketPolicyOnly)
			},
		},
		{
			"native-bool-still-works",
			gcs.Config{
				"bucket":                 "my-bucket",
				"skip_bucket_versioning": true,
			},
			func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.True(t, cfg.SkipBucketVersioning)
			},
		},
		{
			"empty-string-coerces-to-false",
			gcs.Config{
				"bucket":                 "my-bucket",
				"skip_bucket_versioning": "",
			},
			func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.False(t, cfg.SkipBucketVersioning)
			},
		},
		{
			"numeric-one-coerces-to-true",
			gcs.Config{
				"bucket":                 "my-bucket",
				"skip_bucket_versioning": "1",
			},
			func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS) {
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

// TestParseExtendedGCSConfig_InvalidStringBool verifies invalid string values
// for bool fields are rejected (e.g. "maybe" is not a valid bool).
func TestParseExtendedGCSConfig_InvalidStringBool(t *testing.T) {
	t.Parallel()

	cfg := gcs.Config{
		"bucket":                 "my-bucket",
		"skip_bucket_versioning": "maybe",
	}

	_, err := cfg.ParseExtendedGCSConfig()
	require.Error(t, err)
}
