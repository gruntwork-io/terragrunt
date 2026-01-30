# Terragrunt Copilot Instructions

Keep agents productive and PR-ready by following patterns in `docs-starlight/src/content/docs/05-community/01-contributing.mdx`.

## Architecture Overview
```
main.go                    → CLI startup, panic handling, log setup
cli/                       → Commands (run, stack, exec) via urfave/cli wrapper
  cli/commands/            → Each command in its own package
options/                   → TerragruntOptions: immutable config, always use Clone()
config/                    → HCL v2 parsing for terragrunt.hcl (hclparse/, cty helpers)
internal/
  errors/                  → ALWAYS wrap: errors.New(err) or errors.Errorf(fmt, args...)
  remotestate/backend/     → Backend interface: S3, GCS, azurerm
  azure/                   → Interface-based DI (interfaces/ → implementations/ → factory/)
  cache/, locks/           → Caching and locking primitives
pkg/log/                   → Logging: pass Logger from options, never use pkg-level log.*
engine/                    → Pluggable gRPC IaC runtime
```

## Critical Patterns

**Options are immutable** — always clone before modification:
```go
newOpts := opts.Clone()  // or opts.CloneWithConfigPath(l, path)
```

**Error wrapping** — use `internal/errors` for stack traces:
```go
import "github.com/gruntwork-io/terragrunt/internal/errors"
return errors.Errorf("failed to %s: %w", action, err)
```

**Logging** — pass logger through, never use global:
```go
func doThing(l log.Logger, opts *options.TerragruntOptions) {
    l.Debugf("processing %s", opts.WorkingDir)
}
```

**Backend interface** (`internal/remotestate/backend/backend.go`):
```go
type Backend interface {
    Name() string
    NeedsBootstrap(ctx, l, config, opts) (bool, error)
    Bootstrap(ctx, l, config, opts) error
    // ... Delete, Migrate, GetTFInitArgs
}
```

## Dev Commands
```bash
go run main.go plan                                    # smoke test
go test ./config/... ./cli/...                         # unit tests
GOFLAGS='-tags=aws' go test ./test -run 'TestAws.*'    # AWS integration
GOFLAGS='-tags=azure' go test ./test -run 'TestAzure.*' # Azure integration
make run-lint                                          # golangci-lint
make run-strict-lint                                   # strict lint (changed files only)
make fmtcheck                                          # format check
make generate-mocks                                    # regenerate mocks (go generate ./...)
```

## Test Conventions
- **Parallel tests**: always add `t.Parallel()` unless using `t.Setenv()`
- **Integration tests**: prefix `TestAws*`, `TestGcp*`, `TestAzure*` with build tags
- **Azure isolation**: see `test/helpers/azuretest/README.md` for parallel-safe config
- If parallel isn't possible: `//nolint:paralleltest` with reason comment

## Lint Quick Fixes
| Linter | Fix |
|--------|-----|
| `wsl` | Empty line before `if`/`for`/`switch` after assignment |
| `perfsprint` | `fmt.Sprintf("%s", v)` → `v`; use `strings.Builder` in loops |
| `mnd` | Magic numbers → named `const` |
| `unparam` | Unused params → `_` (e.g., `func foo(_ log.Logger)`) |
| `errorlint` | Use `errors.Is()`/`errors.As()`, wrap with `%w` |

Auto-fix: `golangci-lint run --fix --disable-all --enable=goimports,gofmt`

## Azure Backend Development
Location: `internal/remotestate/backend/azurerm/` + `internal/azure/`

1. Define interface in `internal/azure/interfaces/` (e.g., `StorageAccountService`)
2. Implement in `internal/azure/implementations/`
3. Wire via factory in `internal/azure/factory/`
4. Backend uses DI: receives services via `BackendConfig`

Key files: `internal/azure/README.md`, `internal/azure/CONFIGURATION.md`

## PR Checklist
- [ ] Docs in `docs-starlight/` updated; `markdownlint` clean
- [ ] Tests added/updated; `go test` output in PR description
- [ ] `make fmtcheck && make run-lint` pass
- [ ] Strict lint on changed files: `make run-strict-lint`
- [ ] Errors wrapped with `internal/errors`; options cloned, not mutated