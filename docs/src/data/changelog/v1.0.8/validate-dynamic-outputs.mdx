---
version: "v1.0.8"
category: "bug-fixes"
---

#### `validate` no longer fails on `dependency.outputs` references

`terragrunt validate` previously failed with "Unsupported attribute" when a configuration referenced `dependency.<name>.outputs.<key>` without `mock_outputs`. 

The fixes in https://github.com/gruntwork-io/terragrunt/pull/5827 resolved this issue for `terragrunt hcl validate` by checking `pctx.SkipOutput` when deciding to use `cty.DynamicVal` for dependency outputs. However, that fix did not handle cases where a dependency block specifies `skip_outputs = true` but the global `pctx.SkipOutput` is false. The implementation now checks both `pctx.SkipOutput` and `dependencyConfig.SkipOutputs` to ensure dynamic values are used whenever output resolution is skipped at either level.
