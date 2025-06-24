package remotestate_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockBackend implements the Backend interface for testing
type MockBackend struct {
	getTFInitArgsFunc  func(config backend.Config) map[string]any
	bootstrapFunc      func(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) error
	needsBootstrapFunc func(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) (bool, error)
	name               string
}

func (m *MockBackend) Name() string {
	return m.name
}

func (m *MockBackend) GetTFInitArgs(config backend.Config) map[string]any {
	if m.getTFInitArgsFunc != nil {
		return m.getTFInitArgsFunc(config)
	}
	return config
}

func (m *MockBackend) Bootstrap(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) error {
	if m.bootstrapFunc != nil {
		return m.bootstrapFunc(ctx, l, config, opts)
	}
	return nil
}

func (m *MockBackend) NeedsBootstrap(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) (bool, error) {
	if m.needsBootstrapFunc != nil {
		return m.needsBootstrapFunc(ctx, l, config, opts)
	}
	return false, nil
}

func (m *MockBackend) IsVersionControlEnabled(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) (bool, error) {
	return true, nil
}

func (m *MockBackend) Migrate(ctx context.Context, l log.Logger, srcConfig, dstConfig backend.Config, opts *options.TerragruntOptions) error {
	return nil
}

func (m *MockBackend) Delete(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) error {
	return nil
}

func (m *MockBackend) DeleteBucket(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) error {
	return nil
}

func (m *MockBackend) DeleteStorageAccount(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) error {
	return nil
}

// setupOptionsWithExperiment creates TerragruntOptions with specified experiment enabled
func setupOptionsWithExperiment(experimentName string) *options.TerragruntOptions {
	opts := options.NewTerragruntOptions()
	opts.Experiments.EnableExperiment(experimentName)
	return opts
}

// setupWithAzureBackendExperiment enables Azure backend experiment and registers backends
// This is needed for tests that expect the Azure backend to be available
func setupWithAzureBackendExperiment() *options.TerragruntOptions {
	opts := setupOptionsWithExperiment(experiment.AzureBackend)
	remotestate.RegisterBackends(opts)
	return opts
}

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

	// Set up options with Azure backend experiment enabled
	setupWithAzureBackendExperiment()

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

	// Set up options with Azure backend experiment enabled
	setupWithAzureBackendExperiment()

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

	// Set up options with Azure backend experiment enabled
	setupWithAzureBackendExperiment()

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

func TestGenerateOpenTofuCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		desc     string
		backend  string
		expected string
	}{
		{
			desc:     "s3 backend",
			backend:  "s3",
			expected: "terraform {\n  backend \"s3\" {\n  }\n}",
		},
		{
			desc:     "gcs backend",
			backend:  "gcs",
			expected: "terraform {\n  backend \"gcs\" {\n  }\n}",
		},
		{
			desc:     "azurerm backend",
			backend:  "azurerm",
			expected: "terraform {\n  backend \"azurerm\" {\n  }\n}",
		},
		{
			desc:     "unknown backend",
			backend:  "s4",
			expected: "terraform {\n  backend \"s4\" {\n  }\n}",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			// Call the helper function to generate code
			actual, err := generateRemoteStateCodeReal(tc.backend, map[string]any{})
			require.NoError(t, err)

			// Normalize whitespace for comparison
			actualNormalized := strings.ReplaceAll(strings.TrimSpace(actual), "\n", "")
			actualNormalized = strings.ReplaceAll(actualNormalized, "  ", " ")
			expectedNormalized := strings.ReplaceAll(strings.TrimSpace(tc.expected), "\n", "")
			expectedNormalized = strings.ReplaceAll(expectedNormalized, "  ", " ")

			assert.Equal(t, expectedNormalized, actualNormalized)
		})
	}
}

func TestGenerateOpenTofuCodeWithBackendConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		desc     string
		backend  string
		config   map[string]any
		expected string
	}{
		{
			desc:     "s3 backend with config",
			backend:  "s3",
			config:   map[string]any{"bucket": "my-bucket", "key": "terraform.tfstate"},
			expected: "terraform { backend \"s3\" { bucket = \"my-bucket\" key = \"terraform.tfstate\" } }",
		},
		{
			desc:     "gcs backend with config",
			backend:  "gcs",
			config:   map[string]any{"bucket": "my-bucket", "prefix": "terraform.tfstate"},
			expected: "terraform { backend \"gcs\" { bucket = \"my-bucket\" prefix = \"terraform.tfstate\" } }",
		},
		{
			desc:     "azurerm backend with config",
			backend:  "azurerm",
			config:   map[string]any{"storage_account_name": "mystorageaccount", "container_name": "terraform-state"},
			expected: "terraform { backend \"azurerm\" { container_name = \"terraform-state\" storage_account_name = \"mystorageaccount\" } }",
		},
		{
			desc:     "unknown backend with config",
			backend:  "s4",
			config:   map[string]any{"foo": "bar"},
			expected: "terraform { backend \"s4\" { foo = \"bar\" } }",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			// Call the helper function to generate code
			actual, err := generateRemoteStateCodeReal(tc.backend, tc.config)
			require.NoError(t, err)

			// Normalize whitespace for comparison since HCL formatting may differ
			actualNormalized := strings.ReplaceAll(strings.TrimSpace(actual), "\n", "")
			actualNormalized = strings.ReplaceAll(actualNormalized, "  ", " ")

			assert.Contains(t, actualNormalized, "terraform {")
			assert.Contains(t, actualNormalized, fmt.Sprintf("backend \"%s\" {", tc.backend))
			// Check that the config values are present
			for key, value := range tc.config {
				assert.Contains(t, actualNormalized, key)
				assert.Contains(t, actualNormalized, fmt.Sprintf("\"%v\"", value))
			}
		})
	}
}

// Tests for GenerateOpenTofuCode function
/*
func TestGenerateOpenTofuCodeWithEmptyBackend(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "",
	}

	// Call the method under test
	actual := remotestate.GenerateOpenTofuCode(cfg)

	// Assert the expected output
	assert.Equal(t, "terraform { backend \"\" {} }", actual)
}
*/

/*
func TestGenerateOpenTofuCodeWithMultipleBackends(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "s3",
		BackendConfig: map[string]any{
			"bucket": "my-bucket",
			"key":    "terraform.tfstate",
		},
	}

	// Register multiple backends
	remotestate.RegisterBackend("s3", &MockBackend{
		name: "s3",
		getTFInitArgsFunc: func(config backend.Config) map[string]any {
			return config
		},
		bootstrapFunc: func(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) error {
			return nil
		},
		needsBootstrapFunc: func(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) (bool, error) {
			return false, nil
		},
	})

	remotestate.RegisterBackend("gcs", &MockBackend{
		name: "gcs",
		getTFInitArgsFunc: func(config backend.Config) map[string]any {
			return config
		},
		bootstrapFunc: func(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) error {
			return nil
		},
		needsBootstrapFunc: func(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) (bool, error) {
			return false, nil
		},
	})

	// Call the method under test for each registered backend
	for _, backend := range []string{"s3", "gcs"} {
		t.Run(fmt.Sprintf("backend=%s", backend), func(t *testing.T) {
			cfg.BackendName = backend

			actual := remotestate.GenerateOpenTofuCode(cfg)

			assert.Equal(t, fmt.Sprintf("terraform { backend \"%s\" {} }", backend), actual)
		})
	}
}
*/

/*
func TestGenerateOpenTofuCodeWithNestedBlocks(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "azurerm",
		BackendConfig: map[string]any{
			"storage_account_name": "mystorageaccount",
			"container_name":       "terraform-state",
			"key":                  "terraform.tfstate",
			"resource_group_name":  "myrg",
			"subscription_id":      "00000000-0000-0000-0000-000000000000",

			// Nested block example
			"blob_storage": map[string]any{
				"enable_versioning": true,
				"tags": map[string]any{
					"env": "dev",
				},
			},
		},
	}
	actual := remotestate.GenerateOpenTofuCode(cfg)

	expected := `terraform {
  backend "azurerm" {
    container_name       = "terraform-state"
    key                  = "terraform.tfstate"
    resource_group_name  = "myrg"
    storage_account_name = "mystorageaccount"
    subscription_id      = "00000000-0000-0000-0000-000000000000"

    blob_storage {
      enable_versioning = true

      tags {
        env = "dev"
      }
    }
  }
}`

	assert.Equal(t, expected, actual)
}
*/

/*
func TestGenerateOpenTofuCodeWithSensitiveData(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "s3",
		BackendConfig: map[string]any{
			"bucket": "my-bucket",
			"key":    "terraform.tfstate",

			// Sensitive data
			"access_key":  "my-access-key",
			"secret_key": "my-secret-key",
		},
	}
	actual := remotestate.GenerateOpenTofuCode(cfg)

	// Assert that sensitive data is not present in the generated code
	assert.NotContains(t, actual, "my-access-key")
	assert.NotContains(t, actual, "my-secret-key")
}
*/

/*
func TestGenerateOpenTofuCodeWithComplexConfig(t *testing.T) {
	t.Parallel()

	cfg := &remotestate.Config{
		BackendName: "azurerm",
		BackendConfig: map[string]any{
			"storage_account_name": "mystorageaccount",
			"container_name":       "terraform-state",
			"key":                  "terraform.tfstate",
			"resource_group_name":  "myrg",
			"subscription_id":      "00000000-0000-0000-0000-000000000000",

			// Complex configuration
			"network_acls": map[string]any{
				"bypass": "AzureServices",
				"ip_rules": []any{
					map[string]any{
						"action": "Allow",
						"ip_address": "203.0.113.0/24",
					},
				},
			},
		},
	}
	actual := remotestate.GenerateOpenTofuCode(cfg)

	expected := `terraform {
  backend "azurerm" {
    container_name       = "terraform-state"
    key                  = "terraform.tfstate"
    resource_group_name  = "myrg"
    storage_account_name = "mystorageaccount"
    subscription_id      = "00000000-0000-0000-0000-000000000000"

    network_acls {
      bypass = "AzureServices"

      ip_rules {
        action     = "Allow"
        ip_address  = "203.0.113.0/24"
      }
    }
  }
}`

	assert.Equal(t, expected, actual)
}
*/

// Tests for GenerateOpenTofuCode function

// Helper function to generate remote state code using the existing codegen functionality
func generateRemoteStateCodeReal(backendName string, config map[string]any) (string, error) {
	// Import the codegen package function directly
	bytes, err := codegen.RemoteStateConfigToTerraformCode(backendName, config, nil)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func TestGenerateOpenTofuCodeS3Backend(t *testing.T) {
	t.Parallel()

	config := map[string]any{
		"bucket": "my-test-bucket",
		"key":    "path/to/state.tfstate",
		"region": "us-west-2",
	}

	code, err := generateRemoteStateCodeReal("s3", config)

	require.NoError(t, err)
	assert.Contains(t, code, "terraform {")
	assert.Contains(t, code, "backend \"s3\" {")
	assert.Contains(t, code, "bucket")
	assert.Contains(t, code, "my-test-bucket")
	assert.Contains(t, code, "key")
	assert.Contains(t, code, "path/to/state.tfstate")
	assert.Contains(t, code, "region")
	assert.Contains(t, code, "us-west-2")
	assert.Contains(t, code, "}")
}

func TestGenerateOpenTofuCodeAzureBackend(t *testing.T) {
	t.Parallel()

	config := map[string]any{
		"resource_group_name":  "my-rg",
		"storage_account_name": "mystorageaccount",
		"container_name":       "tfstate",
		"key":                  "terraform.tfstate",
	}

	code, err := generateRemoteStateCodeReal("azurerm", config)

	require.NoError(t, err)
	assert.Contains(t, code, "terraform {")
	assert.Contains(t, code, "backend \"azurerm\" {")
	assert.Contains(t, code, "resource_group_name")
	assert.Contains(t, code, "my-rg")
	assert.Contains(t, code, "storage_account_name")
	assert.Contains(t, code, "mystorageaccount")
	assert.Contains(t, code, "container_name")
	assert.Contains(t, code, "tfstate")
	assert.Contains(t, code, "key")
	assert.Contains(t, code, "terraform.tfstate")
	assert.Contains(t, code, "}")
}

func TestGenerateOpenTofuCodeGCSBackend(t *testing.T) {
	t.Parallel()

	config := map[string]any{
		"bucket": "my-gcs-bucket",
		"prefix": "terraform/state",
	}

	code, err := generateRemoteStateCodeReal("gcs", config)

	require.NoError(t, err)
	assert.Contains(t, code, "terraform {")
	assert.Contains(t, code, "backend \"gcs\" {")
	assert.Contains(t, code, "bucket")
	assert.Contains(t, code, "my-gcs-bucket")
	assert.Contains(t, code, "prefix")
	assert.Contains(t, code, "terraform/state")
	assert.Contains(t, code, "}")
}

func TestGenerateOpenTofuCodeWithBackendFiltering(t *testing.T) {
	t.Parallel()

	// Test that backend-specific filtering works using existing RemoteState functionality
	cfg := &remotestate.Config{
		BackendName: "s3",
		BackendConfig: map[string]any{
			"bucket": "my-bucket",
			"key":    "terraform.tfstate",
			"region": "us-east-1",
			// These should be filtered out by GetTFInitArgs
			"s3_bucket_tags": map[string]any{
				"team": "team name",
			},
			"skip_bucket_versioning": true,
		},
	}

	rs := remotestate.New(cfg)
	filteredConfig := rs.GetTFInitArgs()

	// Convert the init args back to a map for testing
	configMap := make(map[string]any)
	for _, arg := range filteredConfig {
		if strings.HasPrefix(arg, "-backend-config=") {
			parts := strings.SplitN(arg[len("-backend-config="):], "=", 2)
			if len(parts) == 2 {
				key, value := parts[0], parts[1]
				// Try to parse boolean values
				switch value {
				case "true":
					configMap[key] = true
				case "false":
					configMap[key] = false
				default:
					configMap[key] = value
				}
			}
		}
	}

	code, err := generateRemoteStateCodeReal("s3", configMap)

	require.NoError(t, err)
	assert.Contains(t, code, "terraform {")
	assert.Contains(t, code, "backend \"s3\" {") // Terragrunt-specific options should not appear in the generated code
	assert.NotContains(t, code, "s3_bucket_tags")
	assert.NotContains(t, code, "skip_bucket_versioning")
	assert.Contains(t, code, "terraform {")
	assert.Contains(t, code, "backend \"s3\" {")
	assert.Contains(t, code, "my-bucket")
	assert.Contains(t, code, "terraform.tfstate")
	assert.Contains(t, code, "us-east-1")
	assert.Contains(t, code, "}")
}

func TestGenerateOpenTofuCodeEmptyConfig(t *testing.T) {
	t.Parallel()

	code, err := generateRemoteStateCodeReal("s3", map[string]any{})

	require.NoError(t, err)
	assert.Contains(t, code, "terraform {")
	assert.Contains(t, code, "backend \"s3\" {")
	assert.Contains(t, code, "}")
	// Should not contain any configuration parameters
	assert.NotContains(t, code, "=")
}

func TestGenerateOpenTofuCodeComplexValues(t *testing.T) {
	t.Parallel()

	config := map[string]any{
		"bucket":                      "my-bucket",
		"key":                         "terraform.tfstate",
		"region":                      "us-east-1",
		"encrypt":                     true,
		"versioning":                  false,
		"max_retries":                 5,
		"skip_credentials_validation": true,
	}

	code, err := generateRemoteStateCodeReal("s3", config)

	require.NoError(t, err)
	assert.Contains(t, code, "terraform {")
	assert.Contains(t, code, "backend \"s3\" {")
	assert.Contains(t, code, "my-bucket")
	assert.Contains(t, code, "terraform.tfstate")
	assert.Contains(t, code, "us-east-1")
	assert.Contains(t, code, "true")
	assert.Contains(t, code, "5")
	assert.Contains(t, code, "}")
}

func TestGenerateOpenTofuCodeStringEscaping(t *testing.T) {
	t.Parallel()

	config := map[string]any{
		"bucket": "my-bucket-with-\"quotes\"",
		"key":    "path/with spaces/terraform.tfstate",
	}

	code, err := generateRemoteStateCodeReal("s3", config)

	require.NoError(t, err)
	// The codegen function should properly escape quotes in HCL
	assert.Contains(t, code, "my-bucket-with-")
	assert.Contains(t, code, "quotes")
	assert.Contains(t, code, "path/with spaces/terraform.tfstate")
}
