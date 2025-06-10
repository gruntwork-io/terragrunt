package backend_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
)

func TestConfig_IsEqual(t *testing.T) {
	t.Parallel()
	testCases := []struct { //nolint: govet
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
			"identical Azure configs",
			backend.Config{"storage_account_name": "myaccount", "container_name": "states", "key": "prod/terraform.tfstate"},
			backend.Config{"storage_account_name": "myaccount", "container_name": "states", "key": "prod/terraform.tfstate"},
			true,
		},
		{
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
		{
			"different Azure storage account names",
			backend.Config{"storage_account_name": "myaccount", "container_name": "states", "key": "prod/terraform.tfstate"},
			backend.Config{"storage_account_name": "different", "container_name": "states", "key": "prod/terraform.tfstate"},
			false,
		},
		{
			"different Azure container names",
			backend.Config{"storage_account_name": "myaccount", "container_name": "states", "key": "prod/terraform.tfstate"},
			backend.Config{"storage_account_name": "myaccount", "container_name": "different", "key": "prod/terraform.tfstate"},
			false,
		},
		{
			"different Azure blob keys",
			backend.Config{"storage_account_name": "myaccount", "container_name": "states", "key": "prod/terraform.tfstate"},
			backend.Config{"storage_account_name": "myaccount", "container_name": "states", "key": "different/terraform.tfstate"},
			false,
		},
		{
			"Azure null environment ignored",
			backend.Config{"storage_account_name": "myaccount", "container_name": "states", "key": "prod/terraform.tfstate", "environment": nil},
			backend.Config{"storage_account_name": "myaccount", "container_name": "states", "key": "prod/terraform.tfstate"},
			true,
		},
		{
			"different Azure environments",
			backend.Config{"storage_account_name": "myaccount", "container_name": "states", "key": "prod/terraform.tfstate", "environment": "public"},
			backend.Config{"storage_account_name": "myaccount", "container_name": "states", "key": "prod/terraform.tfstate", "environment": "usgovernment"},
			false,
		},
		{
			"identical Azure configs with connection string",
			backend.Config{"storage_account_name": "myaccount", "container_name": "states", "key": "prod/terraform.tfstate", "connection_string": "DefaultEndpointsProtocol=https;AccountName=..."},
			backend.Config{"storage_account_name": "myaccount", "container_name": "states", "key": "prod/terraform.tfstate", "connection_string": "DefaultEndpointsProtocol=https;AccountName=..."},
			true,
		},
		{
			"different Azure connection strings",
			backend.Config{"storage_account_name": "myaccount", "container_name": "states", "key": "prod/terraform.tfstate", "connection_string": "DefaultEndpointsProtocol=https;AccountName=..."},
			backend.Config{"storage_account_name": "myaccount", "container_name": "states", "key": "prod/terraform.tfstate", "connection_string": "DefaultEndpointsProtocol=https;AccountName=different"},
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := tc.cfg.IsEqual(tc.existingBackend, "", log.Default())
			assert.Equal(t, tc.expected, actual, "Expect differsFrom to return %t but got %t for existingRemoteState %v and remoteStateFromTerragruntConfig %v", tc.expected, actual, tc.existingBackend, tc.cfg)
		})
	}
}

func TestConfig_AzureBackendComparison(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		existingBackend backend.Config
		newConfig       backend.Config
		expected        bool
	}{
		{
			"authentication method change - access key to MSI",
			backend.Config{
				"storage_account_name": "myaccount",
				"container_name":      "states",
				"key":                "terraform.tfstate",
				"access_key":         "abcd1234",
			},
			backend.Config{
				"storage_account_name": "myaccount",
				"container_name":      "states",
				"key":                "terraform.tfstate",
				"use_msi":           true,
			},
			false,
		},
		{
			"authentication method unchanged - MSI",
			backend.Config{
				"storage_account_name": "myaccount",
				"container_name":      "states",
				"key":                "terraform.tfstate",
				"use_msi":           true,
			},
			backend.Config{
				"storage_account_name": "myaccount",
				"container_name":      "states",
				"key":                "terraform.tfstate",
				"use_msi":           true,
			},
			true,
		},
		{
			"client config credentials change",
			backend.Config{
				"storage_account_name": "myaccount",
				"container_name":      "states",
				"key":                "terraform.tfstate",
				"client_id":          "old-id",
				"client_secret":      "old-secret",
				"tenant_id":          "tenant-id",
			},
			backend.Config{
				"storage_account_name": "myaccount",
				"container_name":      "states",
				"key":                "terraform.tfstate",
				"client_id":          "new-id",
				"client_secret":      "new-secret",
				"tenant_id":          "tenant-id",
			},
			false,
		},
		{
			"endpoint configuration change",
			backend.Config{
				"storage_account_name": "myaccount",
				"container_name":      "states",
				"key":                "terraform.tfstate",
				"endpoint":           "core.windows.net",
			},
			backend.Config{
				"storage_account_name": "myaccount",
				"container_name":      "states",
				"key":                "terraform.tfstate",
				"endpoint":           "core.chinacloudapi.cn",
			},
			false,
		},
		{
			"subscription and tenant change",
			backend.Config{
				"storage_account_name": "myaccount",
				"container_name":      "states",
				"key":                "terraform.tfstate",
				"subscription_id":    "sub1",
				"tenant_id":          "tenant1",
			},
			backend.Config{
				"storage_account_name": "myaccount",
				"container_name":      "states",
				"key":                "terraform.tfstate",
				"subscription_id":    "sub2",
				"tenant_id":          "tenant2",
			},
			false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log := log.Default()
			actual := tc.newConfig.IsEqual(tc.existingBackend, "", log)
			assert.Equal(t, tc.expected, actual, "Expected IsEqual to return %t but got %t for existingBackend %v and newConfig %v", tc.expected, actual, tc.existingBackend, tc.newConfig)
		})
	}
}
