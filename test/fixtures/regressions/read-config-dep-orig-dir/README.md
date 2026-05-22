# Reproducer for `run apply --all` failing on dep declared via read_terragrunt_config

This fixture reproduces the failure pattern reported by the user in the bug
thread for issue 5993. The trigger is a three-way combination:

1. A unit's effective config is assembled by `include` of a shared HCL file
   (`common/root-common.hcl`).
2. That shared file uses `read_terragrunt_config` to load a per-unit
   `unit-common.hcl`, whose `dependency` block is the only place the
   dependency is declared.
3. The `dependency.config_path` is computed via `get_original_terragrunt_dir()`.

Because the dependency block lives only inside the file pulled by
`read_terragrunt_config`, runner discovery never sees it. On `run apply --all`
the runner schedules the dependent units in parallel with their dependencies;
when `unit-a`'s parse decodes the dependency for real, `unit-b`'s outputs are
not yet available and `mock_outputs_allowed_terraform_commands` does not
include `apply`, so the parse fails.

## Manual repro

```
cp -r read-config-dep-orig-dir /tmp/repro
cd /tmp/repro
terragrunt run --all --non-interactive --tf-path tofu -- apply -auto-approve
```

The same fixture passes for:

- `run --all plan` (mocks are allowed for `plan`).
- A single-unit `terragrunt run -- apply -auto-approve` invoked from `unit-b`
  then `unit-a` in sequence.
- `apply --all` once `apply` is added to
  `mock_outputs_allowed_terraform_commands` in `common/unit-a/unit-common.hcl`.
