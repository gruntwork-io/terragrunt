# PR 1: Register azure-backend experiment + documentation stubs

## Context

Maintainer feedback on [gruntwork-io/terragrunt#4307](https://github.com/gruntwork-io/terragrunt/issues/4307#issuecomment-4354333422) asks to split the Azure backend effort into incremental PRs. This document defines PR 1 only.

Scope target for PR 1:

- Register the azure-backend experiment.
- Register an azurerm backend stub that is intentionally no-op.
- Add documentation stubs aligned with current docs layout.

Out of scope for PR 1:

- Azure SDK imports and auth flows.
- Any resource management (storage account/container/blob create/update/delete).
- Any behavioral changes that require cloud credentials to validate.

## Goal

Land a trivially reviewable, low-risk foundational PR (target under ~200 LOC net change) that proves:

1. Experiment flag name exists and is discoverable.
2. Backend name azurerm is recognized by Terragrunt remote_state plumbing.
3. Current docs accurately reflect experimental status and usage intent.

## Repo-accurate paths (validated on main)

- Experiment registry: internal/experiment/experiment.go
- Remote state backend registry: internal/remotestate/remote_state.go
- Existing backend implementations: internal/remotestate/backend/s3, internal/remotestate/backend/gcs
- Experiments docs: docs/src/content/docs/04-reference/04-experiments.md
- State backend docs: docs/src/content/docs/03-features/01-units/03-state-backend.mdx

Note: experiments are currently documented in a single page (04-experiments.md), not in per-experiment files under an active/ directory.

## Detailed implementation plan

### 1) Register experiment name

File: internal/experiment/experiment.go

Changes:

- Add constant:
  - AzureBackend = "azure-backend"
- Add entry in NewExperiments():
  - {Name: AzureBackend, Status: StatusOngoing}

Reasoning:

- Keeps experiment lifecycle in one canonical registry.
- Allows CLI and env var activation without any backend logic in place yet.

### 2) Add azurerm backend stub package

Create folder:

- internal/remotestate/backend/azurerm

Create file 1:

- internal/remotestate/backend/azurerm/backend.go

Implementation:

- Define BackendName = "azurerm".
- Define Backend struct embedding *backend.CommonBackend.
- Implement NewBackend() returning CommonBackend initialized with azurerm.
- Optionally include compile-time assertion var _ backend.Backend = new(Backend) for parity with s3/gcs style.

Behavior:

- No overrides required for bootstrap/delete/migrate; inherited CommonBackend behavior remains no-op/default.

Create file 2:

- internal/remotestate/backend/azurerm/config.go

Implementation:

- type Config backend.Config
- GetTFInitArgs passthrough copy of all key/value pairs (no filtering in PR 1).

Reasoning:

- Supports init argument shaping through backend abstraction immediately.
- Defers Terragrunt-only key filtering and advanced config rules to later PRs.

### 3) Register azurerm backend in remote state registry

File: internal/remotestate/remote_state.go

Changes:

- Add import for internal/remotestate/backend/azurerm.
- Add azurerm.NewBackend() to the backends list alongside s3 and gcs.

Reasoning:

- Ensures backend = "azurerm" resolves to Terragrunt backend abstraction path.
- Keeps fallback behavior controlled by CommonBackend defaults.

### 4) Update experiments documentation

File: docs/src/content/docs/04-reference/04-experiments.md

Changes:

- Add a new section for azure-backend in the active/ongoing experiments list style used by this page.
- Include:
  - experiment name: azure-backend
  - activation examples using --experiment and TG_EXPERIMENT
  - explicit statement that current behavior is documentation/stub registration only
  - brief roadmap bullets (bootstrap/delete/migrate/dependency state reads) as future scope

Reasoning:

- Matches current docs architecture.
- Prevents user confusion about current capability level.

### 5) Add Azure note in state backend feature docs

File: docs/src/content/docs/03-features/01-units/03-state-backend.mdx

Changes:

- Extend supported backend narrative to mention azurerm as experimental.
- Add a compact Azure Storage (azurerm) section with:
  - experimental Aside callout
  - sample remote_state snippet
  - link to /reference/experiments (or in-page anchor if created)
- Wording must clarify that PR 1 does not include Terragrunt bootstrap behavior yet.

Reasoning:

- Gives users a canonical example and expectations.
- Avoids overpromising parity with s3/gcs before subsequent PRs land.

## Verification plan

### Build and static validation

- go build ./...
- make lint (or project-preferred lint target if different in CI)

### Functional smoke checks (no Azure creds required)

1. Verify experiment registration:
   - terragrunt --experiment azure-backend --help (or command invocation that parses experiment flag)
2. Verify backend parsing path:
   - Use a minimal terragrunt.hcl with remote_state.backend = "azurerm"
   - Run a non-destructive init/plan flow and confirm no unknown-backend errors from Terragrunt
3. Verify docs render/build:
   - docs build command per docs package scripts (if available)

## Acceptance criteria

- azure-backend appears in experiment registry and can be enabled.
- azurerm backend is present in remote state backend registry.
- New azurerm package compiles and passes through init args.
- Docs updated in the current docs structure:
  - 04-reference/04-experiments.md
  - 03-features/01-units/03-state-backend.mdx
- No Azure SDK dependencies added to go.mod/go.sum.
- go build ./... succeeds.

## Risk controls

- Keep implementation minimal and additive.
- Avoid touching existing s3/gcs behavior.
- No credentialed tests or live cloud operations in PR 1.
- Ensure wording in docs repeatedly marks feature as experimental/stub.

## PR checklist

- [ ] Add AzureBackend constant + NewExperiments entry
- [ ] Create internal/remotestate/backend/azurerm/backend.go
- [ ] Create internal/remotestate/backend/azurerm/config.go
- [ ] Register azurerm.NewBackend() in remote_state registry
- [ ] Update experiments docs page
- [ ] Update state backend docs page
- [ ] Run build/lint/docs checks
- [ ] Prepare PR summary with explicit out-of-scope list

## References

- Maintainer feedback: https://github.com/gruntwork-io/terragrunt/issues/4307#issuecomment-4354333422
- Experiment system: internal/experiment/experiment.go
- Backend registry: internal/remotestate/remote_state.go
- S3 pattern reference: internal/remotestate/backend/s3/backend.go
- GCS pattern reference: internal/remotestate/backend/gcs/backend.go
