# Azure Backend Design

This document explains the design decisions and architecture of the Azure Storage backend for Terragrunt remote state.

## Overview

The Azure backend (`internal/remotestate/backend/azurerm/`) provides remote state storage using Azure Blob Storage. It supports:

- Storage account and container auto-creation
- Azure AD authentication (default and required)
- RBAC role assignment for data plane operations
- State migration between containers
- Telemetry and error handling

## Architecture

### Dependency Injection Pattern

The Azure backend uses interface-based dependency injection to:

1. **Enable testability** - Services can be mocked for unit tests without hitting Azure APIs
2. **Support multiple implementations** - Production vs test implementations
3. **Centralize service creation** - Factory pattern manages service lifecycle

```
┌─────────────────────────────────────────────────────────────┐
│                      Backend                                │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              AzureServiceContainer                   │   │
│  │  (interfaces.AzureServiceContainer)                  │   │
│  │  ┌─────────────────┐ ┌─────────────────┐            │   │
│  │  │ StorageAccount  │ │   BlobService   │            │   │
│  │  │    Service      │ │                 │            │   │
│  │  └─────────────────┘ └─────────────────┘            │   │
│  │  ┌─────────────────┐ ┌─────────────────┐            │   │
│  │  │  RBACService    │ │ Authentication  │            │   │
│  │  │                 │ │    Service      │            │   │
│  │  └─────────────────┘ └─────────────────┘            │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### Key Interfaces

Located in `internal/azure/interfaces/`:

- **`AzureServiceContainer`** - Factory for creating Azure services
- **`StorageAccountService`** - Storage account CRUD operations
- **`BlobService`** - Blob container and object operations
- **`RBACService`** - Role assignment for Azure AD auth
- **`AuthenticationService`** - Credential management

### Implementation Locations

| Component | Location | Purpose |
|-----------|----------|---------|
| Interfaces | `internal/azure/interfaces/` | Service contracts |
| Factory | `internal/azure/factory/` | Service creation with caching |
| Implementations | `internal/azure/implementations/` | Production Azure SDK code |
| Backend | `internal/remotestate/backend/azurerm/` | Backend interface implementation |

## Why DI Over Direct SDK Calls?

The S3 backend uses direct AWS SDK calls. We chose DI for Azure because:

1. **Azure SDK complexity** - Azure requires more ceremony (credential chains, retry policies, multi-step operations)
2. **Testing requirements** - Azure integration tests are slower and require real resources
3. **Authentication diversity** - Azure supports many auth methods (Azure AD, MSI, SAS tokens, CLI)
4. **RBAC integration** - Data plane operations require role assignments

### Trade-offs

| Aspect | Direct SDK (S3 style) | DI (Azure style) |
|--------|----------------------|------------------|
| Code simplicity | ✅ Simpler | ❌ More abstractions |
| Testability | ❌ Needs mocking SDK | ✅ Interface mocking |
| Flexibility | ❌ Coupled to SDK | ✅ Swappable implementations |
| Learning curve | ✅ Familiar patterns | ❌ More indirection |

## File Structure

```
internal/
├── azure/
│   ├── azureauth/       # Authentication helpers
│   ├── azurehelper/     # Low-level Azure operations
│   ├── azureutil/       # Error handling utilities
│   ├── factory/         # Service factory with caching
│   ├── implementations/ # Production implementations
│   ├── interfaces/      # Service interfaces
│   └── types/           # Shared types
└── remotestate/backend/azurerm/
    ├── backend.go       # Main backend implementation
    ├── backend_di.go    # DI setup helpers
    ├── config.go        # Configuration parsing
    ├── errors.go        # Error types
    ├── retry.go         # Retry logic
    └── telemetry.go     # Telemetry collection
```

## Error Handling

Errors flow through multiple layers:

1. **Azure SDK errors** - Raw `azcore.ResponseError`
2. **Wrapped errors** - `azErrors.AzureError` with classification
3. **Backend errors** - User-friendly messages

```go
// Error classification enables smart retry decisions
type ErrorClassification string

const (
    ErrorClassTransient     ErrorClassification = "transient"
    ErrorClassConfiguration ErrorClassification = "configuration"
    ErrorClassPermissions   ErrorClassification = "permissions"
    ErrorClassNotFound      ErrorClassification = "not_found"
)
```

## Retry Strategy

The backend uses exponential backoff with jitter for transient errors:

- Initial delay: 1 second
- Max delay: 30 seconds
- Max attempts: 3
- Backoff multiplier: 2.0
- Jitter: ±25%

## Future Improvements

1. **Split backend.go** - Currently 2000+ lines, should be split into:
   - `bootstrap.go` - Storage account/container creation
   - `delete.go` - Deletion operations
   - `migrate.go` - State migration
   - `services.go` - Service initialization

2. **Reduce interface surface** - Some interfaces have methods that could be internal

3. **Align with S3 patterns** - Consider simplifying where Azure SDK allows
