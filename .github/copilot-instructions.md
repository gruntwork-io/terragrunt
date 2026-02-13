# Terragrunt Copilot Instructions

Keep agents productive and PR-ready by following patterns in `docs-starlight/src/content/docs/05-community/01-contributing.mdx`.

## Architecture Overview

```
main.go                         → CLI startup, logger setup, app.RunContext()
internal/cli/commands/           → Command registration (run, stack, find, list, catalog, scaffold…)
internal/clihelper/              → urfave/cli v2 wrapper; undefined flags/commands pass through to TF
internal/cli/flags/              → Flag system with deprecation + strict control integration
pkg/options/                     → TerragruntOptions (~120 fields): immutable, always Clone()
pkg/config/                      → HCL v2 parsing for terragrunt.hcl (includes, dependencies, locals)
internal/discovery/              → Concurrent directory walking to find terragrunt.hcl files
internal/component/              → Unit (single config) and Stack (collection) model
internal/queue/                  → DAG-ordered deque: apply = front→back, destroy = back→front
internal/runner/runnerpool/      → Semaphore-based concurrent controller for run --all
internal/runner/runcfg/          → Import-cycle breaker: run package imports runcfg, never pkg/config
internal/remotestate/backend/    → Backend interface: S3, GCS, azurerm
internal/azure/                  → Interface-based DI (interfaces/ → implementations/ → factory/)
internal/errors/                 → ALWAYS wrap: errors.New(err) or errors.Errorf(fmt, args...)
internal/experiment/             → Feature flags: opts.Experiments.Evaluate("name") → bool
internal/strict/                 → Strict controls: deprecation warnings/errors for flags, envs, configs
internal/view/                   → Output rendering (HumanRender, JSONRender) for diagnostics
pkg/log/                         → Logging: pass Logger from options, never use pkg-level log.*
internal/engine/                 → Pluggable gRPC IaC runtime
```

## Critical Patterns

**Options are immutable** — always clone before modification:
```go
newOpts := opts.Clone()
// or with path (returns new logger + opts + error):
l, newOpts, err := opts.CloneWithConfigPath(l, path)
```

**Error wrapping** — use `internal/errors` for stack traces (backed by `go-errors`):
```go
import "github.com/gruntwork-io/terragrunt/internal/errors"
return errors.Errorf("failed to %s: %w", action, err)
// Multi-errors: errs := &errors.MultiError{}; errs.Append(err); return errs.ErrorOrNil()
```

**Logging** — pass logger as first param, never use global:
```go
func doThing(l log.Logger, opts *options.TerragruntOptions) {
    l.Debugf("processing %s", opts.WorkingDir)
}
```

**Context propagation** — `context.Context` is always the first parameter. Multiple values live in it:
logger, telemetry, detailed exit codes, 6 config caches (HCL, config, output, etc.), engine runtime.

**`runcfg` import cycle** — the `internal/runner/run` package must never import `pkg/config`.
Convert at the boundary: `tgConfig.ToRunConfig(l)` → use `runcfg.RunConfig` inside `run/`.

**CLI command structure** — each command in `internal/cli/commands/<name>/`:
- `cli.go` — command definition, flags, action
- `run.go` or `action.go` — business logic
- `flags.go` — flag declarations (optional)
- Top-level shortcuts: `terragrunt plan` = `terragrunt run -- plan`
- Undefined flags/subcommands pass through to OpenTofu/Terraform transparently

**Experiments and strict controls**:
```go
if opts.Experiments.Evaluate("azure-backend") { /* new behavior */ }
// Strict controls: used by deprecated flags/envs. In strict mode, deprecated usage → error.
```

## Dev Commands

```bash
go run main.go plan                                    # smoke test
go test ./pkg/config/... ./internal/cli/...            # unit tests
GOFLAGS='-tags=aws' go test ./test -run 'TestAws.*'    # AWS integration
GOFLAGS='-tags=azure' go test ./test -run 'TestAzure.*' # Azure integration
make build                                             # CGO_ENABLED=0 binary with version ldflags
make run-lint                                          # golangci-lint (auto-discovers build tags)
make run-strict-lint                                   # strict lint (changed files vs origin/main only)
make fmtcheck                                          # pre-commit goimports check
make generate-mocks                                    # go generate ./...
```

## Test Conventions

- **Parallel**: always `t.Parallel()` unless using `t.Setenv()`. If not possible: `//nolint:paralleltest` with reason.
- **Integration tests**: in `test/`, prefixed `TestAws*`/`TestGcp*`/`TestAzure*`, require build tags (`aws`, `gcp`, `azure`).
- **Test package**: `package test_test` (external test package).
- **Fixtures**: `test/fixtures/<scenario>/` — self-contained terragrunt configs (~200+ scenarios).
- **Key helpers** (in `test/helpers/package.go`):
  ```go
  helpers.RunTerragrunt(t, "terragrunt plan --working-dir ...")              // full in-process CLI run
  helpers.RunTerragruntCommandWithOutput(t, cmd)                             // → (stdout, stderr, error)
  helpers.CopyEnvironment(t, "fixtures/some-fixture")                        // copy to temp dir (always do this!)
  helpers.CleanupTerraformFolder(t, path)                                    // remove .terraform + state
  helpers.CreateTmpTerragruntConfig(t, rootPath, tgConfig, tmpPath)          // temp config with placeholders
  helpers.ValidateOutput(t, outputs, key, expectedValue)                     // check TF JSON output
  ```
- Tests auto-inject `--log-format=key-value` for stable output.
- `helpers.WrappedBinary()` returns `"tofu"` or `"terraform"` based on what's installed.

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

## Gotchas

- `Unit.Execution` is `nil` during discovery; only populated before execution — don't access it early.
- Env var prefix: `TG_*` is current, `TERRAGRUNT_*` is deprecated (strict control).
- `clone:"shadowcopy"` tag on struct fields = shallow copy during `opts.Clone()` (used for interfaces, `*version.Version`, `*xsync.MapOf`).
- Queue entry ordering is alphabetical among peers for deterministic DAG execution.
- Partial parsing during `run --all` avoids fetching dependency outputs during graph construction.

## PR Checklist

- [ ] Docs in `docs-starlight/` updated; `markdownlint` clean
- [ ] Tests added/updated; `go test` output in PR description
- [ ] `make fmtcheck && make run-lint` pass
- [ ] Strict lint on changed files: `make run-strict-lint`
- [ ] Errors wrapped with `internal/errors`; options cloned, not mutated