// Package azurehelper provides Azure SDK client construction and helpers for
// the azurerm remote state backend. It pattern-matches internal/awshelper and
// internal/gcphelper: a flat package, a builder for credential resolution, and
// concrete client types — no interfaces, factories, or adapter layers.
//
// # Why this package is larger than awshelper/gcphelper
//
// Five Azure platform concepts require explicit code that has no AWS or GCP
// equivalent in the existing helpers:
//
//  1. Resource groups. Every storage account belongs to a named resource
//     group in a subscription. See resource_group.go.
//  2. RBAC role-assignment propagation. Azure replicates role assignments
//     asynchronously across data-plane services (up to ~5 minutes). When
//     use_azuread_auth = true, bootstrap must assign a role and poll until
//     it takes effect. See rbac.go (RBACRetryDelay, RBACMaxRetries,
//     RBACPropagationTimeout).
//  3. Six authentication modes. The Azure SDK does not auto-chain
//     SAS / access-key / service-principal / MSI / OIDC / AAD; the builder
//     enumerates and prioritizes them. See config.go Build().
//  4. Sovereign clouds (public, US Gov, China) with distinct endpoint
//     suffixes and AAD authorities. See blob.go endpointSuffixForCloud.
//  5. Soft-delete and blob versioning as separate stateful API calls
//     against BlobServiceProperties, independent of account creation.
//     See storage_account.go (EnableSoftDelete, EnableVersioning).
//
// # Out of scope
//
// No distributed locking — Azure exposes blob leases but Terragrunt does not
// wire them here. No backend bootstrap/delete/migrate orchestration: that
// lives in internal/remotestate/backend/azurerm, which consumes this package.
//
// See docs/src/data/experiments/azure-backend.mdx for the experiment status
// and the stabilization checklist.
package azurehelper
