# Terragrunt Copilot Instructions

Keep agents productive and PR-ready by following the patterns documented in `docs-starlight/src/content/docs/05-community/01-contributing.mdx`.

## Architecture map
- `main.go` wires CLI startup, log configuration, and panic handling.
- `cli/` registers commands (`run`, `stack`, `exec`, etc.) and wraps `options.TerragruntOptions`.
- `config/` parses `terragrunt.hcl` via HCL v2 helpers (`config/hclparse`, `cty` utilities) and owns remoting/state metadata.
- `configstack/` builds dependency-aware module graphs; `runner_pool_stack.go` is the experimental parallel executor.
- `engine/` hosts the pluggable gRPC IaC runtime and download/cache logic.
- `internal/` holds shared services:
  - `internal/errors` — wrap all errors with `errors.New(err)` or `errors.Errorf(fmt, args...)` for stack traces.
  - `internal/remotestate/backend/` — pluggable backends (S3, GCS, Azure) implementing the `Backend` interface.
  - `internal/azure/` — interface-based Azure services (`interfaces/`, `implementations/`, `factory/`). See `internal/azure/README.md`.
  - `internal/cache`, `internal/locks`, `internal/providercache` — caching and locking primitives.

## Contribution workflow highlights
1. **Start with an issue or RFC** so the change has buy-in (`.github/ISSUE_TEMPLATE/02-rfc.yml`).
2. **Docs-first**: update Starlight content in `docs-starlight/` before code. Run `markdownlint --disable MD013 MD024 -- docs-starlight/src/content` locally.
3. **Tests-first**: add failing tests, then implement. Prefer focused `go test ./path/...` and include the command + output in the PR description (link to a Gist per the guideline).
4. **Keep configs immutable**: create new `config.TerragruntConfig` instances instead of mutating parsed structs.
5. **Error discipline**: wrap new failures with `errors.New(err)` or `errors.Errorf(...)` from `internal/errors`; expose typed errors for callers.
6. **Logging**: emit through `pkg/log`, passing the module-specific logger supplied by options.

## Daily dev loop
```bash
go run main.go plan                   # quick manual smoke
go test ./config/... ./configstack/... ./cli/...   # unit suites
go test ./test -run 'TestAws*'        # scoped integration (set GOFLAGS tags as needed)
golangci-lint run                     # default lint (Makefile target: make run-lint)
golangci-lint run -c .strict.golangci.yml --new-from-rev origin/main ./...  # strict lint
make fmtcheck && gofmt -w <files>     # formatting gate
```

Windows-specific work requires long-path support and a bash shell (CI scripts live in `.github/scripts/windows-setup.ps1`).

## Integration test matrix
- AWS: prefix tests with `TestAws*`, enable via `GOFLAGS='-tags=aws'` and configure credentials.
- GCP: prefix `TestGcp*`, require JSON service key (`GCLOUD_SERVICE_KEY`, `GOOGLE_CLOUD_PROJECT`, etc.).
- Azure: configure `TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT` and run `GOFLAGS='-tags=azure' go test -v ./test/integration_azure_test.go`. See `test/helpers/azuretest/README.md` for isolation config.
- Race tests live in `test/race_test.go` (prefix `WithRacing`); CI runs them with `-race`.

## Lint triage & mechanical fixes
- Run `golangci-lint run` locally; focus first on mechanical linters (`wsl`, `goimports`, `perfsprint`, `mnd`, `ineffassign`, `unused`) that can be resolved without semantic changes.
- **`wsl` (whitespace/cuddling)**: add empty lines between assignments/returns and the following control block.
- **`perfsprint`**: replace `fmt.Sprintf("%s", value)` with `value`; use `strings.Builder` for string concatenation in loops.
- **`mnd`**: lift repeated literals into named `const` values near their usage.
- **`paralleltest` / `tparallel` / `thelper`**: mark subtests with `t.Parallel()` unless they use `t.Setenv()`. If parallel isn't possible, add `//nolint:paralleltest` with a reason comment.
- **`contextcheck`, `errcheck`, `errorlint`**: ensure returned contexts/errors are propagated—wrap new errors with `internal/errors` helpers and bubble them up.
- **`unparam`**: unused function parameters should be replaced with `_` (e.g., `func foo(_ log.Logger)`).
- After mechanical edits, run `golangci-lint run --fix --disable-all --enable=goimports --enable=gofmt` to let auto-fixable linters clean imports/formatting.

## Azure backend patterns
The Azure remote state backend (`internal/remotestate/backend/azurerm/`) follows interface-based dependency injection:
- **Interfaces** in `internal/azure/interfaces/` define contracts (`StorageAccountService`, `BlobService`, `RBACService`).
- **Implementations** in `internal/azure/implementations/` provide production Azure SDK wrappers.
- **Factory** in `internal/azure/factory/` creates services via `AzureServiceFactory`.
- When adding Azure features: define interface methods first, implement in `implementations/`, wire via factory.
- SAS token auth is data-plane only; management operations require Azure AD or service principal.
- For cognitive complexity issues, extract helper functions (see `extractStorageClientConfig`, `resolveSubscriptionID` patterns).

## Ready-to-merge checklist
- Docs updated alongside code, with markdown lint clean.
- New/updated tests pass locally; capture output for reviewer.
- `gofmt`, default lint, and strict lint (for touched files) are green.
- No raw `fmt`/`log` usage; errors are wrapped; options cloned before mutation.
- Large downloads or cache changes respect `internal/cache` and provider cache helpers.

Following these guardrails keeps generated patches in line with maintainer expectations and speeds up review.