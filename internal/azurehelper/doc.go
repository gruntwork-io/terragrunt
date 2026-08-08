// Package azurehelper provides Azure SDK client construction and helpers for
// the azurerm remote state backend: a builder for credential resolution and
// concrete client types for blobs, storage accounts, and resource groups.
//
// Four Azure platform concepts require explicit code here:
//
//  1. Resource groups. Every storage account belongs to a named resource
//     group in a subscription. See resource_group.go.
//  2. Six authentication modes. The Azure SDK does not auto-chain
//     SAS / access-key / service-principal / MSI / OIDC / AAD; the builder
//     enumerates and prioritizes them. See config.go Build().
//  3. Sovereign clouds (public, US Gov, China) with distinct endpoint
//     suffixes and AAD authorities. See blob.go endpointSuffixForCloud.
//  4. Soft-delete and blob versioning as separate stateful API calls
//     against BlobServiceProperties, independent of account creation.
//     See storage_account.go (EnableSoftDelete, EnableVersioning).
//
// Backend bootstrap/delete/migrate orchestration lives in
// internal/remotestate/backend/azurerm, which consumes this package. See
// docs/src/data/experiments/azure-backend.mdx for the experiment status.
package azurehelper
