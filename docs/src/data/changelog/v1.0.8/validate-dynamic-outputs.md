---
version: "v1.0.8"
category: "bug-fixes"
---

#### `validate` no longer fails on `dependency.outputs` references

`terragrunt validate` previously failed with "Unsupported attribute" when a configuration referenced `dependency.<name>.outputs.<key>` without `mock_outputs`. 

The fixes in https://github.com/gruntwork-io/terragrunt/pull/5827 fixed this issue for `terragrunt hcl validate` however the fix was not working when running `terragrunt validate` as when running this command `pctx.SkipOutput` appeared to not be set to `true` as expected.
