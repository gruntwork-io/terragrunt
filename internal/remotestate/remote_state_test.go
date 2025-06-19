package remotestate_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/stretchr/testify/assert"
)

/**
 * Test for s3, also tests that the terragrunt-specific options are not passed on to terraform
 */
func TestGetTFInitArgs(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "s3",
		BackendConfig: map[string]any{
			"encrypt": true,
			"bucket":  "my-bucket",
			"key":     "terraform.tfstate",
			"region":  "us-east-1",

			"s3_bucket_tags": map[string]any{
				"team":    "team name",
				"name":    "Terraform state storage",
				"service": "Terraform"},

			"dynamodb_table_tags": map[string]any{
				"team":    "team name",
				"name":    "Terraform lock table",
				"service": "Terraform"},

			"accesslogging_bucket_tags": map[string]any{
				"team":    "team name",
				"name":    "Terraform access log storage",
				"service": "Terraform"},

			"skip_bucket_versioning": true,

			"shared_credentials_file": "my-file",
			"force_path_style":        true,
		},
	}
	args := remotestate.New(cfg).GetTFInitArgs()

	// must not contain s3_bucket_tags or dynamodb_table_tags or accesslogging_bucket_tags or skip_bucket_versioning
	assertTerraformInitArgsEqual(t, args, "-backend-config=encrypt=true -backend-config=bucket=my-bucket -backend-config=key=terraform.tfstate -backend-config=region=us-east-1 -backend-config=force_path_style=true -backend-config=shared_credentials_file=my-file")
}

func TestGetTFInitArgsForGCS(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "gcs",
		BackendConfig: map[string]any{
			"project":  "my-project-123456",
			"location": "US",
			"bucket":   "my-bucket",
			"prefix":   "terraform.tfstate",

			"gcs_bucket_labels": map[string]any{
				"team":    "team name",
				"name":    "Terraform state storage",
				"service": "Terraform"},

			"skip_bucket_versioning": true,

			"credentials":  "my-file",
			"access_token": "xxxxxxxx",
		},
	}
	args := remotestate.New(cfg).GetTFInitArgs()

	// must not contain project, location gcs_bucket_labels or skip_bucket_versioning
	assertTerraformInitArgsEqual(t, args, "-backend-config=bucket=my-bucket -backend-config=prefix=terraform.tfstate -backend-config=credentials=my-file -backend-config=access_token=xxxxxxxx")
}

func TestGetTFInitArgsUnknownBackend(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "s4",
		BackendConfig: map[string]any{
			"encrypt": true,
			"bucket":  "my-bucket",
			"key":     "terraform.tfstate",
			"region":  "us-east-1"},
	}
	args := remotestate.New(cfg).GetTFInitArgs()

	// no Backend initializer available, but command line args should still be passed on
	assertTerraformInitArgsEqual(t, args, "-backend-config=encrypt=true -backend-config=bucket=my-bucket -backend-config=key=terraform.tfstate -backend-config=region=us-east-1")
}

func TestGetTFInitArgsInitDisabled(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "s3",
		DisableInit: true,
		BackendConfig: map[string]any{
			"encrypt": true,
			"bucket":  "my-bucket",
			"key":     "terraform.tfstate",
			"region":  "us-east-1"},
	}
	args := remotestate.New(cfg).GetTFInitArgs()

	assertTerraformInitArgsEqual(t, args, "-backend=false")
}

func TestGetTFInitArgsNoBackendConfigs(t *testing.T) {
	t.Parallel()

	cfgs := []*remotestate.Config{
		{BackendName: "s3"},
		{BackendName: "gcs"},
	}

	for _, cfg := range cfgs {
		args := remotestate.New(cfg).GetTFInitArgs()
		assert.Empty(t, args)
	}
}

func TestGetTFInitArgsForAzureRM(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "azurerm",
		BackendConfig: map[string]any{
			"storage_account_name": "mystorageaccount",
			"container_name":       "terraform-state",
			"key":                  "terraform.tfstate",
			"resource_group_name":  "myrg",
			"subscription_id":      "00000000-0000-0000-0000-000000000000",

			// Azure AD auth should be enabled by default
			"use_azuread_auth": true,

			// These options should be filtered and not passed to terraform init
			"create_storage_account_if_not_exists": true,
			"enable_versioning":                    true,
			"allow_blob_public_access":             false,
			"storage_account_tags": map[string]any{
				"team":    "terragrunt",
				"purpose": "testing",
			},
		},
	}
	args := remotestate.New(cfg).GetTFInitArgs()

	// Must not contain storage_account_tags, create_storage_account_if_not_exists, enable_versioning, resource_group_name, etc.
	assertTerraformInitArgsEqual(t, args, "-backend-config=storage_account_name=mystorageaccount "+
		"-backend-config=container_name=terraform-state "+
		"-backend-config=key=terraform.tfstate "+
		"-backend-config=subscription_id=00000000-0000-0000-0000-000000000000 "+
		"-backend-config=use_azuread_auth=true")
}

func TestGetTFInitArgsForAzureRMWithExistingAccount(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "azurerm",
		BackendConfig: map[string]any{
			"storage_account_name": "existingaccount",
			"container_name":       "terraform-state",
			"key":                  "terraform.tfstate",
			"use_azuread_auth":     true,

			// No storage account bootstrap options
		},
	}
	args := remotestate.New(cfg).GetTFInitArgs()

	assertTerraformInitArgsEqual(t, args, "-backend-config=storage_account_name=existingaccount "+
		"-backend-config=container_name=terraform-state "+
		"-backend-config=key=terraform.tfstate "+
		"-backend-config=use_azuread_auth=true")
}

func TestGetTFInitArgsForAzureRMWithFullBootstrap(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "azurerm",
		BackendConfig: map[string]any{
			"storage_account_name": "newaccount",
			"container_name":       "terraform-state",
			"key":                  "terraform.tfstate",
			"subscription_id":      "00000000-0000-0000-0000-000000000000",
			"resource_group_name":  "myrg",
			"use_azuread_auth":     true,

			// Full set of bootstrap options
			"create_storage_account_if_not_exists": true,
			"enable_versioning":                    true,
			"allow_blob_public_access":             false,
			"enable_hierarchical_namespace":        false,
			"account_kind":                         "StorageV2",
			"account_tier":                         "Standard",
			"access_tier":                          "Hot",
			"replication_type":                     "LRS",
			"location":                             "eastus",
			"storage_account_tags": map[string]any{
				"team":    "terragrunt",
				"purpose": "testing",
				"env":     "dev",
			},
		},
	}
	args := remotestate.New(cfg).GetTFInitArgs()

	// None of the bootstrap options and resource_group_name should be passed to terraform init
	assertTerraformInitArgsEqual(t, args, "-backend-config=storage_account_name=newaccount "+
		"-backend-config=container_name=terraform-state "+
		"-backend-config=key=terraform.tfstate "+
		"-backend-config=subscription_id=00000000-0000-0000-0000-000000000000 "+
		"-backend-config=use_azuread_auth=true")
}

func assertTerraformInitArgsEqual(t *testing.T, actualArgs []string, expectedArgs string) {
	t.Helper()

	expected := strings.Split(expectedArgs, " ")
	assert.Len(t, actualArgs, len(expected))

	for _, expectedArg := range expected {
		assert.Contains(t, actualArgs, expectedArg)
	}
}
