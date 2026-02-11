package s3_test

import (
	"testing"

	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseExtendedS3Config_StringBoolCoercion verifies that boolean config values
// passed as strings (e.g. from HCL ternary type unification) are correctly parsed.
// See https://github.com/gruntwork-io/terragrunt/issues/5475
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			extS3Cfg, err := tc.config.Normalize(log.Default()).ParseExtendedS3Config()
			require.NoError(t, err)

			tc.check(t, extS3Cfg)
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

	_, err := cfg.Normalize(log.Default()).ParseExtendedS3Config()
	require.Error(t, err)
}
