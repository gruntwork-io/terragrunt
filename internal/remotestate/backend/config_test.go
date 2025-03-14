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
		name            string
		existingBackend backend.Config
		cfg             backend.Config
		expected        bool
	}{
		{
			"both empty",
			backend.Config{},
			backend.Config{},
			true,
		},
		{
			"identical S3 configs",
			backend.Config{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			backend.Config{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			true,
		}, {
			"identical GCS configs",
			backend.Config{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			backend.Config{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			true,
		}, {
			"different s3 bucket values",
			backend.Config{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			backend.Config{"bucket": "different", "key": "bar", "region": "us-east-1"},
			false,
		}, {
			"different gcs bucket values",
			backend.Config{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			backend.Config{"project": "foo-123456", "location": "europe-west3", "bucket": "different", "prefix": "bar"},
			false,
		}, {
			"different s3 key values",
			backend.Config{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			backend.Config{"bucket": "foo", "key": "different", "region": "us-east-1"},
			false,
		}, {
			"different gcs prefix values",
			backend.Config{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			backend.Config{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "different"},
			false,
		}, {
			"different s3 region values",
			backend.Config{"bucket": "foo", "key": "bar", "region": "us-east-1"},
			backend.Config{"bucket": "foo", "key": "bar", "region": "different"},
			false,
		}, {
			"different gcs location values",
			backend.Config{"project": "foo-123456", "location": "europe-west3", "bucket": "foo", "prefix": "bar"},
			backend.Config{"project": "foo-123456", "location": "different", "bucket": "foo", "prefix": "bar"},
			false,
		},
		{
			"different boolean values and boolean conversion",
			backend.Config{"something": "true"},
			backend.Config{"something": false},
			false,
		},
		{
			"different gcs boolean values and boolean conversion",
			backend.Config{"something": "true"},
			backend.Config{"something": false},
			false,
		},
		{
			"null values ignored",
			backend.Config{"something": "foo", "set-to-nil-should-be-ignored": nil},
			backend.Config{"something": "foo"},
			true,
		},
		{
			"gcs null values ignored",
			backend.Config{"something": "foo", "set-to-nil-should-be-ignored": nil},
			backend.Config{"something": "foo"},
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			actual := testCase.cfg.IsEqual(testCase.existingBackend, "", log.Default())
			assert.Equal(t, testCase.expected, actual, "Expect differsFrom to return %t but got %t for existingRemoteState %v and remoteStateFromTerragruntConfig %v", testCase.expected, actual, testCase.existingBackend, testCase.cfg)
		})
	}
}
