package backend_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
)

func TestConfig_IsEqual(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		existingBackend backend.Config
		cfg             backend.Config
		name            string
		expected        bool
	}{
		{
			name:            "both empty",
			existingBackend: backend.Config{},
			cfg:             backend.Config{},
			expected:        true,
		},
		{
			name:            "identical S3 configs",
			existingBackend: backend.Config{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			cfg:             backend.Config{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			expected:        true,
		},
		{
			name: "identical GCS configs",
			existingBackend: backend.Config{
				"project":  "foo-123456",
				"location": "europe-west3",
				"bucket":   "foo",
				"prefix":   "bar",
			},
			cfg: backend.Config{
				"project":  "foo-123456",
				"location": "europe-west3",
				"bucket":   "foo",
				"prefix":   "bar",
			},
			expected: true,
		},
		{
			name:            "different s3 bucket values",
			existingBackend: backend.Config{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			cfg:             backend.Config{"bucket": "different", "key": "bar", "region": "us-east-1"},
			expected:        false,
		},
		{
			name: "different gcs bucket values",
			existingBackend: backend.Config{
				"project":  "foo-123456",
				"location": "europe-west3",
				"bucket":   "foo",
				"prefix":   "bar",
			},
			cfg: backend.Config{
				"project":  "foo-123456",
				"location": "europe-west3",
				"bucket":   "different",
				"prefix":   "bar",
			},
			expected: false,
		},
		{
			name:            "different s3 key values",
			existingBackend: backend.Config{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			cfg:             backend.Config{"bucket": "foo", "key": "different", "region": "us-east-1"},
			expected:        false,
		},
		{
			name: "different gcs prefix values",
			existingBackend: backend.Config{
				"project":  "foo-123456",
				"location": "europe-west3",
				"bucket":   "foo",
				"prefix":   "bar",
			},
			cfg: backend.Config{
				"project":  "foo-123456",
				"location": "europe-west3",
				"bucket":   "foo",
				"prefix":   "different",
			},
			expected: false,
		},
		{
			name:            "different s3 region values",
			existingBackend: backend.Config{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			cfg:             backend.Config{"bucket": "foo", "key": "bar", "region": "different"},
			expected:        false,
		},
		{
			name: "different gcs location values",
			existingBackend: backend.Config{
				"project":  "foo-123456",
				"location": "europe-west3",
				"bucket":   "foo",
				"prefix":   "bar",
			},
			cfg: backend.Config{
				"project":  "foo-123456",
				"location": "different",
				"bucket":   "foo",
				"prefix":   "bar",
			},
			expected: false,
		},
		{
			name:            "different boolean values and boolean conversion",
			existingBackend: backend.Config{"something": "true"},
			cfg:             backend.Config{"something": false},
			expected:        false,
		},
		{
			name:            "different gcs boolean values and boolean conversion",
			existingBackend: backend.Config{"something": "true"},
			cfg:             backend.Config{"something": false},
			expected:        false,
		},
		{
			name:            "null values ignored",
			existingBackend: backend.Config{"something": "foo", "set-to-nil-should-be-ignored": nil},
			cfg:             backend.Config{"something": "foo"},
			expected:        true,
		},
		{
			name:            "gcs null values ignored",
			existingBackend: backend.Config{"something": "foo", "set-to-nil-should-be-ignored": nil},
			cfg:             backend.Config{"something": "foo"},
			expected:        true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := tc.cfg.IsEqual(tc.existingBackend, "", log.Default())
			assert.Equal(
				t,
				tc.expected,
				actual,
				"Expect differsFrom to return %t but got %t for existingRemoteState %v and remoteStateFromTerragruntConfig %v",
				tc.expected,
				actual,
				tc.existingBackend,
				tc.cfg,
			)
		})
	}
}
