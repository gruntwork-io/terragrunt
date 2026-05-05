# Azure Test Isolation Configuration

This document explains how to configure and use the improved Azure test isolation helpers.

## Environment Variables

The isolation helpers use the following environment variables to configure test isolation:

### Basic Configuration
- `TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT`: Storage account name (optional if using isolated storage)
- `TERRAGRUNT_AZURE_TEST_ACCESS_KEY`: Storage account access key (optional if using Azure AD auth)
- `TERRAGRUNT_AZURE_TEST_RESOURCE_GROUP`: Resource group name (optional if using isolated resource groups)
- `TERRAGRUNT_AZURE_TEST_SUBSCRIPTION_ID`: Azure subscription ID
- `TERRAGRUNT_AZURE_TEST_LOCATION`: Azure location (default: eastus)

### Isolation Configuration
- `TERRAGRUNT_AZURE_TEST_ISOLATION`: Isolation mode (default: full)
  - `full`: Complete isolation with unique containers, storage accounts, and resource groups
  - `container`: Container-level isolation only
  - `none`: No isolation (shared resources)

- `TERRAGRUNT_AZURE_TEST_ISOLATE_STORAGE`: Create isolated storage accounts per test (default: false)
- `TERRAGRUNT_AZURE_TEST_ISOLATE_RESOURCE_GROUP`: Create isolated resource groups per test (default: false)
- `TERRAGRUNT_AZURE_TEST_CLEANUP`: Enable cleanup of resources after test (default: true)

### Fallback Variables
- `ARM_SUBSCRIPTION_ID`: Azure subscription ID fallback

## Usage Examples

### Basic Container Isolation
```bash
export TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT=myteststorage
export TERRAGRUNT_AZURE_TEST_RESOURCE_GROUP=my-test-rg
export TERRAGRUNT_AZURE_TEST_SUBSCRIPTION_ID=00000000-0000-0000-0000-000000000000
export TERRAGRUNT_AZURE_TEST_ISOLATION=container
```

### Full Isolation (Recommended for Parallel Tests)
```bash
export TERRAGRUNT_AZURE_TEST_SUBSCRIPTION_ID=00000000-0000-0000-0000-000000000000
export TERRAGRUNT_AZURE_TEST_LOCATION=swedencentral
export TERRAGRUNT_AZURE_TEST_ISOLATION=full
export TERRAGRUNT_AZURE_TEST_ISOLATE_STORAGE=true
export TERRAGRUNT_AZURE_TEST_ISOLATE_RESOURCE_GROUP=true
```

### Using Azure AD Authentication
```bash
export TERRAGRUNT_AZURE_TEST_SUBSCRIPTION_ID=00000000-0000-0000-0000-000000000000
export TERRAGRUNT_AZURE_TEST_ISOLATION=full
export TERRAGRUNT_AZURE_TEST_ISOLATE_STORAGE=true
# Don't set TERRAGRUNT_AZURE_TEST_ACCESS_KEY to use Azure AD auth
```

## Test Code Examples

### Basic Test with Isolation
```go
func TestAzureWithIsolation(t *testing.T) {
    // Get isolated configuration
    config := azuretest.GetIsolatedAzureConfig(t)
    
    // Set up cleanup
    defer azuretest.CleanupAzureResources(t, config)
    
    // Create resources as needed
    azuretest.EnsureResourceGroupExists(t, config)
    azuretest.EnsureStorageAccountExists(t, config)
    
    // Get blob client
    blobClient := azuretest.GetAzureBlobClient(t, config)
    
    // Ensure container exists
    azuretest.EnsureContainerExists(t, config, blobClient)
    
    // Your test logic here...
}
```

### Parallel-Safe Test
```go
func TestAzureParallelSafe(t *testing.T) {
    t.Parallel() // Safe because of resource isolation
    
    // Get isolated configuration
    config := azuretest.GetIsolatedAzureConfig(t)
    
    // Set up cleanup
    defer azuretest.CleanupAzureResources(t, config)
    
    // Create fully isolated resources
    azuretest.EnsureResourceGroupExists(t, config)
    azuretest.EnsureStorageAccountExists(t, config)
    
    // Get blob client
    blobClient := azuretest.GetAzureBlobClient(t, config)
    
    // Ensure container exists
    azuretest.EnsureContainerExists(t, config, blobClient)
    
    // Your parallel test logic here...
}
```

## Resource Naming Convention

The isolation helpers use a predictable naming convention:

### Container Names
Format: `tg-<cleaned-test-name>-<timestamp>-<uuid>`
- Maximum length: 63 characters
- Automatically cleaned and truncated

### Storage Account Names
Format: `tg<cleanedtestname><shortened-id>`
- Maximum length: 24 characters
- Lowercase letters and numbers only
- Automatically cleaned and truncated

### Resource Group Names
Format: `terragrunt-test-<cleaned-test-name>-<timestamp>-<uuid>`
- Maximum length: 90 characters
- Automatically cleaned and truncated

## Test Tags

All resources created by the isolation helpers are tagged with:
- `terragrunt-test`: "true"
- `terragrunt-test-id`: Unique test ID
- `terragrunt-test-name`: Test name
- `terragrunt-timestamp`: Creation timestamp

These tags help with resource tracking and cleanup.

## Benefits of Using Isolation Helpers

1. **Parallel Test Execution**: Tests can run in parallel without resource conflicts
2. **Predictable Resource Names**: Consistent naming convention across all tests
3. **Automatic Cleanup**: Resources are cleaned up after each test
4. **Configurable Isolation Levels**: Choose the right level of isolation for your needs
5. **RBAC Compatibility**: Works with both access keys and Azure AD authentication
6. **Better Test Reliability**: Reduces flaky tests caused by resource conflicts

## Troubleshooting

### Common Issues

1. **Permission Errors**: Ensure your Azure credentials have sufficient permissions to create resources
2. **Resource Conflicts**: Use higher isolation levels if tests are still conflicting
3. **Cleanup Failures**: Check Azure logs for resource deletion issues
4. **Long Test Names**: Test names are automatically truncated to fit Azure naming requirements

### Debug Information

Enable verbose logging to see detailed information about resource creation and cleanup:
```bash
go test -v -count=1 -timeout 30m -tags azure ./test -run TestYourAzureTest
```

The helpers log detailed information about:
- Resource creation and existence checks
- Cleanup operations
- Isolation configuration
- Resource naming decisions
