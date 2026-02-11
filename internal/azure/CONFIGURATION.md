# Azure Configuration Guide

This document provides a comprehensive guide to configuring Terragrunt for use with Azure services, particularly for remote state storage.

## Quick Start

To use Azure as your Terragrunt backend, add this configuration to your `terragrunt.hcl`:

```hcl
remote_state {
  backend = "azurerm"
  config = {
    # Required Configuration
    storage_account_name = "mystorageaccount"
    container_name      = "terraform-state"
    resource_group_name = "terraform-rg"
    subscription_id     = "12345678-1234-1234-1234-123456789abc"
    key                = "prod/terraform.tfstate"

    # Optional but Recommended
    enable_versioning   = true
    use_azuread_auth   = true  # Uses Azure CLI or environment credentials
    
    # Automatic Storage Account Creation (Optional)
    create_storage_account_if_not_exists = true
    location           = "eastus"
    account_tier      = "Standard"
    replication_type  = "LRS"
  }
}
```

## Authentication Methods

There are several ways to authenticate with Azure. Below are the recommended approaches in order of preference:

### 1. Azure AD Authentication (Recommended)

The recommended authentication method that uses Azure Active Directory with automatic credential discovery.

```hcl
remote_state {
  backend = "azurerm"
  config = {
    use_azuread_auth = true
    # Other required fields...
  }
}
```

When using Azure AD authentication, Terragrunt will use the current logged-in Azure CLI credentials or environment variables.

**For local development:** Use Azure CLI

```bash
az login
```

**For CI/CD environments:** Use environment variables

```bash
export ARM_CLIENT_ID="your-app-registration-id"
export ARM_CLIENT_SECRET="your-client-secret"
export ARM_TENANT_ID="your-tenant-id"
export ARM_SUBSCRIPTION_ID="your-subscription-id"
```

#### Credential Sources (in order of precedence)

1. Environment variables (AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID)
2. Azure CLI authentication
3. Managed Service Identity (when running on Azure)
4. Visual Studio authentication
5. Visual Studio Code authentication

### 2. Managed Service Identity (MSI)

Uses Azure's Managed Service Identity for authentication when running on Azure resources.

```hcl
remote_state {
  backend = "azurerm"
  config = {
    use_msi = true
    # Other required fields...
  }
}
```

#### Supported Azure Resources

- Azure Virtual Machines
- Azure App Service
- Azure Function Apps
- Azure Container Instances
- Azure Kubernetes Service

### 3. Service Principal Authentication

Uses an explicit Azure AD application (service principal) for authentication.

```hcl
remote_state {
  backend = "azurerm"
  config = {
    subscription_id  = "12345678-1234-1234-1234-123456789abc"
    tenant_id        = "87654321-4321-4321-4321-210987654321"
    client_id        = "11111111-1111-1111-1111-111111111111"
    client_secret    = "your-client-secret"
    # Other required fields...
  }
}
```

#### Environment Variables

- `AZURE_CLIENT_ID` or `ARM_CLIENT_ID`: Service Principal client ID
- `AZURE_CLIENT_SECRET` or `ARM_CLIENT_SECRET`: Service Principal client secret
- `AZURE_TENANT_ID` or `ARM_TENANT_ID`: Azure AD tenant ID
- `AZURE_SUBSCRIPTION_ID` or `ARM_SUBSCRIPTION_ID`: Azure subscription ID

### 4. SAS Token Authentication

Uses a Shared Access Signature token for storage-specific authentication.

```hcl
remote_state {
  backend = "azurerm"
  config = {
    storage_account_name = "mystorageaccount"
    sas_token           = "?sv=2021-06-08&ss=b&srt=sco&sp=rwdlacx&se=2023-12-31T23:59:59Z&sig=..."
    # Other required fields...
  }
}
```

### 5. Troubleshooting Authentication

To test your Azure credentials:

```bash
az account show
```

## Configuration Options

### Required Configuration

| Option | Description |
|--------|-------------|
| `storage_account_name` | Name of the Azure Storage Account to store state files |
| `container_name` | Name of the Blob Container to store state files |
| `resource_group_name` | Name of the Resource Group containing the Storage Account |
| `subscription_id` | Azure Subscription ID |
| `key` | Path to the state file within the container |

### Optional Configuration

| Option | Default | Description |
|--------|---------|-------------|
| `enable_versioning` | `false` | Enable blob versioning for state files |
| `use_azuread_auth` | `false` | Use Azure AD authentication (recommended) |
| `use_msi` | `false` | Use Managed Service Identity for authentication |
| `sas_token` | `""` | SAS token for Storage Account access |
| `snapshot` | `""` | Use a specific state file snapshot version |
| `client_id` | `""` | Service Principal Client ID |
| `client_secret` | `""` | Service Principal Client Secret |
| `tenant_id` | `""` | Azure AD Tenant ID |

### Storage Account Creation Options

These options are only used when `create_storage_account_if_not_exists = true`:

| Option | Default | Description |
|--------|---------|-------------|
| `location` | Required | Azure region for the Storage Account |
| `account_tier` | `"Standard"` | Performance tier (`Standard` or `Premium`) |
| `replication_type` | `"LRS"` | Replication type (`LRS`, `GRS`, `RAGRS`, etc.) |
| `account_kind` | `"StorageV2"` | Storage Account kind |
| `access_tier` | `"Hot"` | Access tier (`Hot` or `Cool`) |
| `min_tls_version` | `"TLS1_2"` | Minimum TLS version |
| `allow_blob_public_access` | `false` | Allow public access to blobs |
| `enable_https_traffic_only` | `true` | Allow only HTTPS traffic |

## Advanced Features

### State File Versioning

Blob versioning provides point-in-time recovery of state files. To enable:

```hcl
remote_state {
  backend = "azurerm"
  config = {
    enable_versioning = true
    # Other configuration...
  }
}
```

### Working with State Snapshots

To retrieve a specific version of the state file:

```hcl
remote_state {
  backend = "azurerm"
  config = {
    snapshot = "2023-01-01T00:00:00.0000000Z"
    # Other configuration...
  }
}
```

### Cross-Subscription Access

To access storage accounts in different subscriptions:

```hcl
remote_state {
  backend = "azurerm"
  config = {
    subscription_id = "target-subscription-id"
    # Optional: Specify credentials for the target subscription
    use_azuread_auth = true  # Recommended
    # Other configuration...
  }
}
```

## Best Practices

1. Always use Azure AD authentication (`use_azuread_auth = true`) in production environments
2. Enable versioning (`enable_versioning = true`) to protect against accidental state file corruption
3. Use Managed Identity (`use_msi = true`) in Azure-hosted environments (VMs, AKS, etc.)
4. Consider using cross-region replication (`replication_type = "RAGRS"`) for critical environments
5. Implement proper RBAC permissions on the Storage Account and Container
6. Use different containers for different environments (dev/staging/prod)
7. Enable soft delete on the Storage Account for additional protection

## Troubleshooting

### Common Issues

1. **Authentication Failures**
   - Verify Azure CLI login status with `az account show`
   - Check environment variables are set correctly
   - Ensure service principal has correct permissions

2. **Storage Account Access**
   - Verify resource group and storage account exist
   - Check network access rules and firewall settings
   - Validate RBAC permissions

3. **State File Locking**
   - Check for abandoned locks in the blob's lease status
   - Verify network connectivity to Azure Storage
   - Ensure proper permissions for lease operations

### Getting Help

- Review Azure Storage logs and metrics
- Check Terragrunt debug logs with `TF_LOG=DEBUG terragrunt <command>`
- File issues on the [Terragrunt GitHub repository](https://github.com/gruntwork-io/terragrunt)

## Azure Backend Configuration

This document explains how to configure Terragrunt to use the Azure storage backend for remote state management.

## Table of Contents

- [Configuration Options](#configuration-options)
  - [Required Configuration](#required-configuration)
  - [Optional Configuration](#optional-configuration)
  - [Storage Account Creation Options](#storage-account-creation-options)
- [Authentication Methods](#authentication-methods)
- [Advanced Features](#advanced-features)
- [Cloud Environments](#cloud-environments)
- [Best Practices](#best-practices)
- [Environment Variables](#environment-variables)
- [Troubleshooting](#troubleshooting)
- [Migration Guide](#migration-guide)
- [References](#references)

## Key Features

The Azure backend offers several important features:

- **Blob Versioning**: Built-in support for state file versioning
- **Enhanced Error Handling**: Better error classification and retry logic
- **Improved Authentication**: Multiple robust authentication methods
- **Automatic Bootstrapping**: Can create and configure storage accounts automatically

## Configuration Options

### Required Configuration

These settings are required for any Azure backend configuration:

```hcl
remote_state {
  backend = "azurerm"
  config = {
    storage_account_name = "mystorageaccount"  # 3-24 chars, lowercase alphanumeric
    container_name      = "terraform-state"     # 3-63 chars, lowercase with hyphens
    resource_group_name = "terraform-rg"        # Resource group containing storage
    subscription_id     = "your-sub-id"         # Azure subscription ID
    key                = "path/to/state.tfstate" # State file path in container
  }
}
```

### Recommended Security Settings

Enable these settings for production environments:

```hcl
remote_state {
  backend = "azurerm"
  config = {
    # ... required settings ...

    # Security settings
    enable_versioning          = true    # Enable blob versioning
    use_azuread_auth          = true    # Use Azure AD auth
    allow_blob_public_access  = false   # Disable public access
    
    # Optional: Configure managed identity
    use_msi                   = true    # Use managed identity if available
  }
}
```

### Automatic Storage Account Creation

The backend can automatically create and configure the storage account:

```hcl
remote_state {
  backend = "azurerm"
  config = {
    # ... required settings ...

    # Bootstrap settings
    create_storage_account_if_not_exists = true   # Create if missing
    location                             = "eastus" # Azure region
    account_kind                         = "StorageV2"
    account_tier                         = "Standard"
    access_tier                          = "Hot"
    replication_type                     = "LRS"
    
    # Optional: Add tags
    storage_account_tags = {
      Environment = "production"
      Terraform   = "true"
    }
  }
}
```

#### Example Configuration

```hcl
remote_state {
  backend = "azurerm"
  config = {
    # Remote state configuration
    storage_account_name = "mystorageaccount"
    container_name       = "terraform-state"
    resource_group_name  = "terraform-rg"
    subscription_id      = "12345678-1234-1234-1234-123456789abc"
    key                  = "prod/terraform.tfstate"
    use_azuread_auth     = true
    
    # Storage account bootstrapping
    location                         = "eastus"
    account_kind                     = "StorageV2"
    account_tier                     = "Standard"
    access_tier                      = "Hot"
    replication_type                 = "LRS"
    enable_versioning                = true
    allow_blob_public_access         = false
    create_storage_account_if_not_exists = true
    skip_storage_account_update      = false
    disable_blob_public_access       = true
    
    storage_account_tags = {
      Environment = "production"
      Team        = "platform"
      CostCenter  = "engineering"
    }
  }
}
```

## Cloud Environments

Terragrunt supports multiple Azure cloud environments:

### Azure Public Cloud (Default)

```hcl
config = {
  environment = "public"
  # or
  environment = "AzurePublicCloud"
}
```

### Azure US Government Cloud

```hcl
config = {
  environment = "government"
  # or
  environment = "AzureUSGovernmentCloud"
}
```

### Azure China Cloud

```hcl
config = {
  environment = "china"
  # or
  environment = "AzureChinaCloud"
}
```

### Azure German Cloud (Deprecated)

```hcl
config = {
  environment = "german"
  # or
  environment = "AzureGermanCloud"
}
```

## Best Practices

### Security

1. **Use Azure AD authentication** as the default method
2. **Disable public blob access** for state storage
3. **Enable blob versioning** for state file protection
4. **Use SAS tokens** for time-limited access
5. **Store secrets securely** using Azure Key Vault or environment variables
6. **Enable storage account encryption** with customer-managed keys when required

### Performance

1. **Choose appropriate storage tier** based on access patterns
2. **Select optimal replication strategy** for your durability requirements
3. **Configure retry policies** for transient error handling
4. **Use regional storage** close to your compute resources

### Cost Optimization

1. **Use Standard tier** for most use cases
2. **Choose LRS replication** for development environments
3. **Implement lifecycle policies** for old state file versions
4. **Monitor storage costs** with appropriate tagging

### Operational

1. **Use consistent naming conventions** for storage accounts and containers
2. **Implement proper tagging** for resource organization
3. **Set up monitoring** for storage account access and errors
4. **Document authentication methods** used in different environments

## Environment Variables

The following environment variables are supported:

### Authentication

- `AZURE_CLIENT_ID` / `ARM_CLIENT_ID`: Service Principal client ID
- `AZURE_CLIENT_SECRET` / `ARM_CLIENT_SECRET`: Service Principal client secret
- `AZURE_TENANT_ID` / `ARM_TENANT_ID`: Azure AD tenant ID
- `AZURE_SUBSCRIPTION_ID` / `ARM_SUBSCRIPTION_ID`: Azure subscription ID

### Configuration

- `AZURE_ENVIRONMENT` / `ARM_ENVIRONMENT`: Azure cloud environment

### MSI-specific

- `MSI_ENDPOINT`: Custom MSI endpoint URL
- `MSI_RESOURCE_ID`: User-assigned managed identity resource ID

## Troubleshooting

### Common Issues

#### Authentication Failures

1. **Check credential precedence**: Ensure the correct authentication method is being used
2. **Verify environment variables**: Confirm that required variables are set correctly
3. **Check permissions**: Ensure the principal has necessary permissions on the storage account
4. **Validate subscription ID**: Confirm the subscription ID is correct and accessible

#### Storage Account Issues

1. **Check account name uniqueness**: Storage account names must be globally unique
2. **Verify resource group**: Ensure the resource group exists and is accessible
3. **Check permissions**: Ensure proper Storage Blob Data Contributor permissions
4. **Validate container name**: Container names must follow Azure naming conventions

#### Network Issues

1. **Check firewall rules**: Ensure storage account firewall allows access
2. **Verify private endpoints**: Confirm private endpoint configuration if used
3. **Check DNS resolution**: Ensure storage account endpoint is resolvable
4. **Validate proxy settings**: Check if proxy configuration is interfering

### Debug Configuration

Enable detailed logging to troubleshoot configuration issues:

```bash
export TF_LOG=DEBUG
export TERRAGRUNT_LOG_LEVEL=debug
```

## Migration Guide

### From Legacy Configuration

If you're migrating from older Terragrunt versions:

1. **Update authentication method**: Switch to Azure AD authentication
2. **Review security settings**: Ensure public access is disabled
3. **Enable versioning**: Turn on blob versioning for state protection
4. **Update retry configuration**: Use new retry settings for better reliability

### From Other Backends

When migrating from other backends:

1. **Plan the migration**: Use `terraform state pull` and `terraform state push`
2. **Test thoroughly**: Validate state file integrity after migration
3. **Update team documentation**: Ensure all team members understand new configuration
4. **Monitor usage**: Watch for any access pattern changes

## References

- [Azure Storage Account Documentation](https://docs.microsoft.com/en-us/azure/storage/common/storage-account-overview)
- [Azure AD Authentication](https://docs.microsoft.com/en-us/azure/active-directory/authentication/)
- [Managed Service Identity](https://docs.microsoft.com/en-us/azure/active-directory/managed-identities-azure-resources/)
- [Azure Cloud Environments](https://docs.microsoft.com/en-us/azure/azure-government/documentation-government-developer-guide)
- [Terragrunt Documentation](https://terragrunt.gruntwork.io/docs/)
