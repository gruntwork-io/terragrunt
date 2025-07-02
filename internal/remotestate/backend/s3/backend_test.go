package s3_test

import (
	"testing"

	backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"

	"github.com/stretchr/testify/assert"
)

func TestBackend_GetTFInitArgs(t *testing.T) {
	t.Parallel()

	remoteBackend := s3backend.NewBackend()

	testCases := []struct { //nolint: govet
		name          string
		config        backend.Config
		expected      map[string]any
		shouldBeEqual bool
	}{
		{
			"empty-no-values",
			backend.Config{},
			map[string]any{},
			true,
		},
		{
			"valid-s3-configuration-keys",
			backend.Config{
				"bucket":  "foo",
				"encrypt": "bar",
				"key":     "baz",
				"region":  "quux",
			},
			map[string]any{
				"bucket":  "foo",
				"encrypt": "bar",
				"key":     "baz",
				"region":  "quux",
			},
			true,
		},
		{
			"terragrunt-keys-filtered",
			backend.Config{
				"bucket":                      "foo",
				"encrypt":                     "bar",
				"key":                         "baz",
				"region":                      "quux",
				"skip_credentials_validation": true,
				"s3_bucket_tags":              map[string]string{},
			},
			map[string]any{
				"bucket":                      "foo",
				"encrypt":                     "bar",
				"key":                         "baz",
				"region":                      "quux",
				"skip_credentials_validation": true,
			},
			true,
		},
		{
			"empty-no-values-all-terragrunt-keys-filtered",
			backend.Config{
				"s3_bucket_tags":                                    map[string]string{},
				"dynamodb_table_tags":                               map[string]string{},
				"accesslogging_bucket_tags":                         map[string]string{},
				"skip_bucket_versioning":                            true,
				"skip_bucket_ssencryption":                          false,
				"skip_bucket_root_access":                           false,
				"skip_bucket_enforced_tls":                          false,
				"skip_bucket_public_access_blocking":                false,
				"disable_bucket_update":                             true,
				"enable_lock_table_ssencryption":                    true,
				"disable_aws_client_checksums":                      false,
				"accesslogging_bucket_name":                         "test",
				"accesslogging_target_object_partition_date_source": "EventTime",
				"accesslogging_target_prefix":                       "test",
				"skip_accesslogging_bucket_acl":                     false,
				"skip_accesslogging_bucket_enforced_tls":            false,
				"skip_accesslogging_bucket_public_access_blocking":  false,
				"skip_accesslogging_bucket_ssencryption":            false,
			},
			map[string]any{},
			true,
		},
		{
			"lock-table-replaced-with-dynamodb-table",
			backend.Config{
				"bucket":     "foo",
				"encrypt":    "bar",
				"key":        "baz",
				"region":     "quux",
				"lock_table": "xyzzy",
			},
			map[string]any{
				"bucket":         "foo",
				"encrypt":        "bar",
				"key":            "baz",
				"region":         "quux",
				"dynamodb_table": "xyzzy",
			},
			true,
		},
		{
			"dynamodb-table-not-replaced-with-lock-table",
			backend.Config{
				"bucket":         "foo",
				"encrypt":        "bar",
				"key":            "baz",
				"region":         "quux",
				"dynamodb_table": "xyzzy",
			},
			map[string]any{
				"bucket":     "foo",
				"encrypt":    "bar",
				"key":        "baz",
				"region":     "quux",
				"lock_table": "xyzzy",
			},
			false,
		},
		{
			"assume-role",
			backend.Config{
				"bucket": "foo",
				"assume_role": map[string]any{
					"role_arn":     "arn:aws:iam::123:role/role",
					"external_id":  "123",
					"session_name": "qwe",
				},
			},
			map[string]any{
				"bucket":      "foo",
				"assume_role": "{external_id=\"123\",role_arn=\"arn:aws:iam::123:role/role\",session_name=\"qwe\"}",
			},
			true,
		},
		{
			"use-lockfile-native-s3-locking",
			backend.Config{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": true,
			},
			map[string]any{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": true,
			},
			true,
		},
		{
			"use-lockfile-false",
			backend.Config{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": false,
			},
			map[string]any{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": false,
			},
			true,
		},
		{
			"dual-locking-dynamodb-and-s3",
			backend.Config{
				"bucket":         "foo",
				"key":            "bar",
				"region":         "us-east-1",
				"dynamodb_table": "my-lock-table",
				"use_lockfile":   true,
			},
			map[string]any{
				"bucket":         "foo",
				"key":            "bar",
				"region":         "us-east-1",
				"dynamodb_table": "my-lock-table",
				"use_lockfile":   true,
			},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := remoteBackend.GetTFInitArgs(tc.config)

			if !tc.shouldBeEqual {
				assert.NotEqual(t, tc.expected, actual)
				return
			}

			assert.Equal(t, tc.expected, actual)
		})
	}
}
