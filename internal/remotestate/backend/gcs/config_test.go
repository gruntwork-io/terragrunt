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

	testCases := []struct {
		cfg           gcs.Config
		comparableCfg gcs.Config
		name          string
		shouldBeEqual bool
	}{
		{
			name:          "equal-both-empty",
			cfg:           gcs.Config{},
			comparableCfg: gcs.Config{},
			shouldBeEqual: true,
		},
		{
			name:          "equal-empty-and-nil",
			cfg:           gcs.Config{},
			comparableCfg: nil,
			shouldBeEqual: true,
		},
		{
			name:          "equal-one-key",
			cfg:           gcs.Config{"foo": "bar"},
			comparableCfg: gcs.Config{"foo": "bar"},
			shouldBeEqual: true,
		},
		{
			name:          "equal-multiple-keys",
			cfg:           gcs.Config{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true},
			comparableCfg: gcs.Config{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true},
			shouldBeEqual: true,
		},
		{
			name:          "equal-encrypt-bool-handling",
			cfg:           gcs.Config{"encrypt": true},
			comparableCfg: gcs.Config{"encrypt": "true"},
			shouldBeEqual: true,
		},
		{
			name:          "equal-general-bool-handling",
			cfg:           gcs.Config{"something": true, "encrypt": true},
			comparableCfg: gcs.Config{"something": "true", "encrypt": "true"},
			shouldBeEqual: true,
		},
		{
			name:          "equal-ignore-gcs-labels",
			cfg:           gcs.Config{"foo": "bar", "gcs_bucket_labels": []map[string]string{{"foo": "bar"}}},
			comparableCfg: gcs.Config{"foo": "bar"},
			shouldBeEqual: true,
		},
		{
			name:          "unequal-values",
			cfg:           gcs.Config{"foo": "bar"},
			comparableCfg: gcs.Config{"foo": "different"},
			shouldBeEqual: false,
		},
		{
			name:          "unequal-non-empty-cfg-nil",
			cfg:           gcs.Config{"foo": "bar"},
			comparableCfg: nil,
			shouldBeEqual: false,
		},
		{
			name:          "unequal-general-bool-handling",
			cfg:           gcs.Config{"something": true},
			comparableCfg: gcs.Config{"something": "false"},
			shouldBeEqual: false,
		},
		{
			name:          "equal-null-ignored",
			cfg:           gcs.Config{"something": "foo"},
			comparableCfg: gcs.Config{"something": "foo", "ignored-because-null": nil},
			shouldBeEqual: true,
		},
		{
			name:          "terragrunt-only-configs-remain-intact",
			cfg:           gcs.Config{"something": "foo", "skip_bucket_creation": true},
			comparableCfg: gcs.Config{"something": "foo"},
			shouldBeEqual: true,
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

	testCases := []struct {
		config gcs.Config
		check  func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS)
		name   string
	}{
		{
			name: "skip-bucket-versioning-string-true",
			config: gcs.Config{
				"bucket":                 "my-bucket",
				"skip_bucket_versioning": "true",
			},
			check: func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.True(t, cfg.SkipBucketVersioning)
			},
		},
		{
			name: "skip-bucket-versioning-string-false",
			config: gcs.Config{
				"bucket":                 "my-bucket",
				"skip_bucket_versioning": "false",
			},
			check: func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.False(t, cfg.SkipBucketVersioning)
			},
		},
		{
			name: "skip-bucket-creation-string-true",
			config: gcs.Config{
				"bucket":               "my-bucket",
				"skip_bucket_creation": "true",
			},
			check: func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.True(t, cfg.SkipBucketCreation)
			},
		},
		{
			name: "enable-bucket-policy-only-string-true",
			config: gcs.Config{
				"bucket":                    "my-bucket",
				"enable_bucket_policy_only": "true",
			},
			check: func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.True(t, cfg.EnableBucketPolicyOnly)
			},
		},
		{
			name: "native-bool-still-works",
			config: gcs.Config{
				"bucket":                 "my-bucket",
				"skip_bucket_versioning": true,
			},
			check: func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.True(t, cfg.SkipBucketVersioning)
			},
		},
		{
			name: "empty-string-coerces-to-false",
			config: gcs.Config{
				"bucket":                 "my-bucket",
				"skip_bucket_versioning": "",
			},
			check: func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS) {
				t.Helper()
				assert.False(t, cfg.SkipBucketVersioning)
			},
		},
		{
			name: "numeric-one-coerces-to-true",
			config: gcs.Config{
				"bucket":                 "my-bucket",
				"skip_bucket_versioning": "1",
			},
			check: func(t *testing.T, cfg *gcs.ExtendedRemoteStateConfigGCS) {
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
