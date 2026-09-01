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

	testCases := []struct {
		config        backend.Config
		expected      map[string]any
		name          string
		shouldBeEqual bool
	}{
		{
			name:          "empty-no-values",
			config:        backend.Config{},
			expected:      map[string]any{},
			shouldBeEqual: true,
		},
		{
			name: "valid-s3-configuration-keys",
			config: backend.Config{
				"bucket":  "foo",
				"encrypt": "bar",
				"key":     "baz",
				"region":  "quux",
			},
			expected: map[string]any{
				"bucket":  "foo",
				"encrypt": "bar",
				"key":     "baz",
				"region":  "quux",
			},
			shouldBeEqual: true,
		},
		{
			name: "terragrunt-keys-filtered",
			config: backend.Config{
				"bucket":                      "foo",
				"encrypt":                     "bar",
				"key":                         "baz",
				"region":                      "quux",
				"skip_credentials_validation": true,
				"s3_bucket_tags":              map[string]string{},
			},
			expected: map[string]any{
				"bucket":                      "foo",
				"encrypt":                     "bar",
				"key":                         "baz",
				"region":                      "quux",
				"skip_credentials_validation": true,
			},
			shouldBeEqual: true,
		},
		{
			name: "empty-no-values-all-terragrunt-keys-filtered",
			config: backend.Config{
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
			expected:      map[string]any{},
			shouldBeEqual: true,
		},
		{
			name: "lock-table-replaced-with-dynamodb-table",
			config: backend.Config{
				"bucket":     "foo",
				"encrypt":    "bar",
				"key":        "baz",
				"region":     "quux",
				"lock_table": "xyzzy",
			},
			expected: map[string]any{
				"bucket":         "foo",
				"encrypt":        "bar",
				"key":            "baz",
				"region":         "quux",
				"dynamodb_table": "xyzzy",
			},
			shouldBeEqual: true,
		},
		{
			name: "dynamodb-table-not-replaced-with-lock-table",
			config: backend.Config{
				"bucket":         "foo",
				"encrypt":        "bar",
				"key":            "baz",
				"region":         "quux",
				"dynamodb_table": "xyzzy",
			},
			expected: map[string]any{
				"bucket":     "foo",
				"encrypt":    "bar",
				"key":        "baz",
				"region":     "quux",
				"lock_table": "xyzzy",
			},
			shouldBeEqual: false,
		},
		{
			name: "assume-role",
			config: backend.Config{
				"bucket": "foo",
				"assume_role": map[string]any{
					"role_arn":     "arn:aws:iam::123:role/role",
					"external_id":  "123",
					"session_name": "qwe",
				},
			},
			expected: map[string]any{
				"bucket":      "foo",
				"assume_role": "{external_id=\"123\",role_arn=\"arn:aws:iam::123:role/role\",session_name=\"qwe\"}",
			},
			shouldBeEqual: true,
		},
		{
			name: "use-lockfile-native-s3-locking",
			config: backend.Config{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": true,
			},
			expected: map[string]any{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": true,
			},
			shouldBeEqual: true,
		},
		{
			name: "use-lockfile-false",
			config: backend.Config{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": false,
			},
			expected: map[string]any{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": false,
			},
			shouldBeEqual: true,
		},
		{
			name: "dual-locking-dynamodb-and-s3",
			config: backend.Config{
				"bucket":         "foo",
				"key":            "bar",
				"region":         "us-east-1",
				"dynamodb_table": "my-lock-table",
				"use_lockfile":   true,
			},
			expected: map[string]any{
				"bucket":         "foo",
				"key":            "bar",
				"region":         "us-east-1",
				"dynamodb_table": "my-lock-table",
				"use_lockfile":   true,
			},
			shouldBeEqual: true,
		},
		{
			name: "string-bool-use-lockfile-true",
			config: backend.Config{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": "true",
			},
			expected: map[string]any{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": true,
			},
			shouldBeEqual: true,
		},
		{
			name: "string-bool-use-lockfile-false",
			config: backend.Config{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": "false",
			},
			expected: map[string]any{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": false,
			},
			shouldBeEqual: true,
		},
		{
			name: "string-bool-encrypt-and-use-lockfile",
			config: backend.Config{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"encrypt":      "true",
				"use_lockfile": "true",
			},
			expected: map[string]any{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"encrypt":      true,
				"use_lockfile": true,
			},
			shouldBeEqual: true,
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
