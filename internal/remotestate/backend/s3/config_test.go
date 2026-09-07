package s3_test

import (
	"fmt"
	"testing"

	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseExtendedS3Config_StringBoolCoercion verifies that boolean config values
// passed as strings (e.g. from HCL ternary type unification) are correctly parsed.
func TestParseExtendedS3Config_StringBoolCoercion(t *testing.T) {
	t.Parallel()

	testCases := []struct { //nolint: govet
		name   string
		config s3backend.Config
		check  func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3)
	}{
		{
			"use-lockfile-string-true",
			s3backend.Config{
				"bucket":       "my-bucket",
				"key":          "my-key",
				"region":       "us-east-1",
				"use_lockfile": "true",
			},
			func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.True(t, cfg.RemoteStateConfigS3.UseLockfile)
			},
		},
		{
			"use-lockfile-string-false",
			s3backend.Config{
				"bucket":       "my-bucket",
				"key":          "my-key",
				"region":       "us-east-1",
				"use_lockfile": "false",
			},
			func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.False(t, cfg.RemoteStateConfigS3.UseLockfile)
			},
		},
		{
			"encrypt-string-true",
			s3backend.Config{
				"bucket":  "my-bucket",
				"key":     "my-key",
				"region":  "us-east-1",
				"encrypt": "true",
			},
			func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.True(t, cfg.RemoteStateConfigS3.Encrypt)
			},
		},
		{
			"force-path-style-string-true",
			s3backend.Config{
				"bucket":           "my-bucket",
				"key":              "my-key",
				"region":           "us-east-1",
				"force_path_style": "true",
			},
			func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.True(t, cfg.RemoteStateConfigS3.S3ForcePathStyle)
			},
		},
		{
			"skip-bucket-versioning-string-true",
			s3backend.Config{
				"bucket":                 "my-bucket",
				"key":                    "my-key",
				"region":                 "us-east-1",
				"skip_bucket_versioning": "true",
			},
			func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.True(t, cfg.SkipBucketVersioning)
			},
		},
		{
			"enable-bucket-root-access-string-true",
			s3backend.Config{
				"bucket":                    "my-bucket",
				"key":                       "my-key",
				"region":                    "us-east-1",
				"enable_bucket_root_access": "true",
			},
			func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.True(t, cfg.EnableBucketRootAccess)
			},
		},
		{
			"native-bool-still-works",
			s3backend.Config{
				"bucket":       "my-bucket",
				"key":          "my-key",
				"region":       "us-east-1",
				"use_lockfile": true,
			},
			func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.True(t, cfg.RemoteStateConfigS3.UseLockfile)
			},
		},
		{
			"empty-string-coerces-to-false",
			s3backend.Config{
				"bucket":       "my-bucket",
				"key":          "my-key",
				"region":       "us-east-1",
				"use_lockfile": "",
			},
			func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.False(t, cfg.RemoteStateConfigS3.UseLockfile)
			},
		},
		{
			"numeric-one-coerces-to-true",
			s3backend.Config{
				"bucket":       "my-bucket",
				"key":          "my-key",
				"region":       "us-east-1",
				"use_lockfile": "1",
			},
			func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.True(t, cfg.RemoteStateConfigS3.UseLockfile)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			extS3Cfg, err := tc.config.Normalize(logger.CreateLogger()).ParseExtendedS3Config()
			require.NoError(t, err)

			tc.check(t, extS3Cfg)
		})
	}
}

// TestParseExtendedS3Config_SkipBucketRootAccessIsInert verifies that `skip_bucket_root_access`
// still parses but no longer decides whether the root access statement is written. `false` used to
// mean "write the statement", so neither value may enable it now.
func TestParseExtendedS3Config_SkipBucketRootAccessIsInert(t *testing.T) {
	t.Parallel()

	for _, skip := range []bool{true, false} {
		t.Run(fmt.Sprintf("skip-%t", skip), func(t *testing.T) {
			t.Parallel()

			cfg := s3backend.Config{
				"bucket":                  "my-bucket",
				"key":                     "my-key",
				"region":                  "us-east-1",
				"skip_bucket_root_access": skip,
			}

			extS3Cfg, err := cfg.Normalize(logger.CreateLogger()).ParseExtendedS3Config()
			require.NoError(t, err)

			assert.False(t, extS3Cfg.EnableBucketRootAccess)
		})
	}
}

// TestParseExtendedS3Config_InvalidStringBool verifies that WeakDecode rejects
// invalid string values for bool fields (e.g. "maybe" is not a valid bool).
func TestParseExtendedS3Config_InvalidStringBool(t *testing.T) {
	t.Parallel()

	cfg := s3backend.Config{
		"bucket":       "my-bucket",
		"key":          "my-key",
		"region":       "us-east-1",
		"use_lockfile": "maybe",
	}

	_, err := cfg.Normalize(logger.CreateLogger()).ParseExtendedS3Config()
	require.Error(t, err)
}
