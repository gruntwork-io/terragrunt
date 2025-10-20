# Dependency Output in Generate Block Regression Test

**Location**: `test/fixtures/regressions/dependency-generate/`

This fixture reproduces **GitHub Issue #4962**: Dependency output in generate block regression in v0.89.0+

## The Bug

Since Terragrunt v0.89.0, using a `dependency` output reference within a `generate` block results in an error when using `run --all plan`:

```
ERROR  Unsuitable value: value must be known
```

### Affected Versions
- **Broken**: v0.89.0 and later (including v0.90.0)
- **Last working version**: v0.88.1

### Symptoms

1. ❌ **FAILS**: `terragrunt run --all plan` - produces "Unsuitable value: value must be known"
2. ✅ **WORKS**: `terragrunt plan` (direct run in module without --all)
3. ✅ **WORKS**: Using same dependency output in `inputs` instead of `generate` block

## Directory Structure

```
test/fixtures/regressions/dependency-generate/
├── README.md
├── other/
│   └── terragrunt.hcl          # Produces outputs with secrets
├── testing/
│   └── terragrunt.hcl          # Depends on "other", uses output in generate block
└── modules/
    ├── other-module/
    │   └── main.tf             # Outputs secrets.test_provider_token
    └── test-module/
        └── main.tf             # Consumes the token
```

## How to Run Tests

```bash
cd /projects/gruntwork/code-review/terragrunt
go test -v -run TestDependencyOutputInGenerate -timeout 10m ./test
```

### Individual Tests

1. **TestDependencyOutputInGenerateBlock** - Main regression test using `run --all plan`
   - Should FAIL on v0.89.0+ (reproduces the bug)
   - Should PASS on v0.88.1 and earlier

2. **TestDependencyOutputInGenerateBlockDirectRun** - Tests direct `plan` without `--all`
   - Should PASS on all versions (this always works)

3. **TestDependencyOutputInInputsStillWorks** - Tests using outputs in `inputs` instead
   - Should PASS on all versions (workaround)

## The Problem Configuration

The issue occurs with this pattern in `testing/terragrunt.hcl`:

```hcl
dependency "other" {
  config_path = "../other"
}

generate "provider_test" {
  path      = "provider_test.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
provider "test" {
  api_token = "${dependency.other.outputs.secrets.test_provider_token}"
}
EOF
}
```

## Expected Behavior

The `generate` block should be able to access dependency outputs just like `inputs` can, regardless of whether using `run --all` or direct execution.

## Root Cause

When using `run --all`, Terragrunt processes modules in parallel/batched mode. Starting in v0.89.0, dependency outputs are not being properly resolved before generating files, causing the "value must be known" error.

## Related Issues

- GitHub Issue: #4962
- Introduced in: v0.89.0
- Related to: Parallel execution and dependency resolution ordering
