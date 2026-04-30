# Azure Configuration Guide

This document provides a comprehensive guide to configuring Terragrunt for use with Azure services, particularly for remote state storage.

> **Note:** The Azure backend requires the `azure-backend` experiment flag to be enabled.
> Use `--experiment azure-backend` or set `TG_EXPERIMENT=azure-backend` in your environment.

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

#### Credential Sources (DefaultAzureCredential order)

1. Environment variables (AZURE_CLIENT_ID, AZURE_TENANT_ID, AZURE_CLIENT_SECRET or AZURE_CLIENT_CERTIFICATE_PATH)
2. Workload Identity
3. Managed Identity
4. Azure CLI
5. Azure Developer CLI (azd)

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
| `endpoint` | `""` | Custom endpoint URL for the Azure Storage service |
| `access_key` | `""` | Storage account access key (passed through to Terraform) |
| `environment` | `"public"` | Azure cloud environment (`public`, `government`, `china`) |
| `disable_blob_public_access` | `false` | Disable public access to blobs (bootstrap-only) |
| `account_replication_type` | `""` | Alternative name for `replication_type` (bootstrap-only) |

### Storage Account Creation Options

These options are only used when `create_storage_account_if_not_exists = true`:

| Option | Default | Description |
|--------|---------|-------------|
| `location` | Required | Azure region for the Storage Account |
| `account_tier` | `"Standard"` | Performance tier (`Standard` or `Premium`) |
| `replication_type` | `"LRS"` | Replication type (`LRS`, `GRS`, `RAGRS`, etc.) |
| `account_kind` | `"StorageV2"` | Storage Account kind |
| `access_tier` | `"Hot"` | Access tier (`Hot` or `Cool`) |
| `allow_blob_public_access` | `false` | Allow public access to blobs |
| `skip_storage_account_update` | `false` | Skip updating existing storage account properties |
| `storage_account_tags` | `{}` | Custom metadata tags to apply to the storage account |

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

## RBAC Permissions

### Bootstrap Permissions

When using `create_storage_account_if_not_exists = true`, the authenticated principal requires:

- **`Contributor`** role on the resource group (to create storage accounts, containers, and manage properties)

### Data Plane Operations with Azure AD

When using `use_azuread_auth = true`, the authenticated principal requires one of:

- **`Storage Blob Data Contributor`** on the storage account (read, write, delete blobs)
- **`Storage Blob Data Owner`** on the storage account (full control including RBAC management)

### Access Without Azure AD

When using access keys or SAS tokens (`sas_token` or `access_key`), no Azure RBAC role assignments are needed. Authentication is handled via the storage account's shared key or the SAS token's embedded permissions.

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

## References

- [Azure Storage Account Documentation](https://learn.microsoft.com/en-us/azure/storage/common/storage-account-overview)
- [Azure AD Authentication](https://learn.microsoft.com/en-us/azure/active-directory/authentication/)
- [Managed Service Identity](https://learn.microsoft.com/en-us/azure/active-directory/managed-identities-azure-resources/)
- [Azure Cloud Environments](https://learn.microsoft.com/en-us/azure/azure-government/documentation-government-developer-guide)
- [Terragrunt Documentation](https://terragrunt.gruntwork.io/docs/)
