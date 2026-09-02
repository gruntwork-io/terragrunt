package s3_test

import (
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

	testCases := []struct {
		config s3backend.Config
		check  func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3)
		name   string
	}{
		{
			name: "use-lockfile-string-true",
			config: s3backend.Config{
				"bucket":       "my-bucket",
				"key":          "my-key",
				"region":       "us-east-1",
				"use_lockfile": "true",
			},
			check: func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.True(t, cfg.RemoteStateConfigS3.UseLockfile)
			},
		},
		{
			name: "use-lockfile-string-false",
			config: s3backend.Config{
				"bucket":       "my-bucket",
				"key":          "my-key",
				"region":       "us-east-1",
				"use_lockfile": "false",
			},
			check: func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.False(t, cfg.RemoteStateConfigS3.UseLockfile)
			},
		},
		{
			name: "encrypt-string-true",
			config: s3backend.Config{
				"bucket":  "my-bucket",
				"key":     "my-key",
				"region":  "us-east-1",
				"encrypt": "true",
			},
			check: func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.True(t, cfg.RemoteStateConfigS3.Encrypt)
			},
		},
		{
			name: "force-path-style-string-true",
			config: s3backend.Config{
				"bucket":           "my-bucket",
				"key":              "my-key",
				"region":           "us-east-1",
				"force_path_style": "true",
			},
			check: func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.True(t, cfg.RemoteStateConfigS3.S3ForcePathStyle)
			},
		},
		{
			name: "skip-bucket-versioning-string-true",
			config: s3backend.Config{
				"bucket":                 "my-bucket",
				"key":                    "my-key",
				"region":                 "us-east-1",
				"skip_bucket_versioning": "true",
			},
			check: func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.True(t, cfg.SkipBucketVersioning)
			},
		},
		{
			name: "native-bool-still-works",
			config: s3backend.Config{
				"bucket":       "my-bucket",
				"key":          "my-key",
				"region":       "us-east-1",
				"use_lockfile": true,
			},
			check: func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.True(t, cfg.RemoteStateConfigS3.UseLockfile)
			},
		},
		{
			name: "empty-string-coerces-to-false",
			config: s3backend.Config{
				"bucket":       "my-bucket",
				"key":          "my-key",
				"region":       "us-east-1",
				"use_lockfile": "",
			},
			check: func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
				t.Helper()
				assert.False(t, cfg.RemoteStateConfigS3.UseLockfile)
			},
		},
		{
			name: "numeric-one-coerces-to-true",
			config: s3backend.Config{
				"bucket":       "my-bucket",
				"key":          "my-key",
				"region":       "us-east-1",
				"use_lockfile": "1",
			},
			check: func(t *testing.T, cfg *s3backend.ExtendedRemoteStateConfigS3) {
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
