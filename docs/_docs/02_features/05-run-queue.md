---
layout: collection-browser-doc
title: Run Queue
category: features
categories_url: features
excerpt: Learn how Terragrunt orchestrates multiple concurrent OpenTofu/Terraform runs.
tags: ["run queue"]
order: 205
nav_title: Documentation
nav_title_link: /docs/
slug: run-queue
---

Terragrunt's "Run Queue" is the mechanism it uses to manage the run order and concurrency when running OpenTofu/Terraform commands across multiple Terragrunt [units](/docs/features/units). This is particularly relevant when using the [`run --all`](/docs/reference/cli-options#all) or [`run --graph`](/docs/reference/cli-options#graph) commands.

## How it Works: The Dependency Graph (DAG)

At its core, the Run Queue relies on a [Directed Acyclic Graph (DAG)](/docs/getting-started/terminology#directed-acyclic-graph-dag) built from the dependencies defined between your Terragrunt units. These dependencies are typically established using [`dependency`](/docs/reference/config-blocks-and-attributes#dependency) or [`dependencies`](/docs/reference/config-blocks-and-attributes#dependencies) blocks in your `terragrunt.hcl` files.

Terragrunt analyzes these dependencies to determine the correct order of operations:

1. **Discovery:** Terragrunt discovers configurations that might be relevant to a run based on the current working directory.
2. **Constructing the Queue:** Based on the command being run, Terragrunt creates an ordered queue.
    - For commands like `plan` or `apply`, dependencies are run *before* the units that depend on them.
    - For commands like `destroy`, dependent units are run *before* their dependencies.
3. **Runs:** Terragrunt dequeues the units in the queue and runs them, respecting the queue order. By default, it runs units concurrently up to a certain limit (controlled by the [`--parallelism`](/docs/reference/cli-options#parallelism) flag), but it will always wait for a unit's dependencies (or dependents for destroys) to complete successfully before running that unit.

### Example DAG

Consider a setup where:

- Unit "dependent" depends on unit "dependency".
- Unit "dependency" depends on unit "ancestor-dependency".
- Unit "independent" has no dependencies nor dependents.

Represented textually, the file structure might look like:

```tree
root
├── ancestor-dependency
│   └── terragrunt.hcl
├── independent
│   └── terragrunt.hcl
└── subtree
    ├── dependency
    │   └── terragrunt.hcl
    └── dependent
        └── terragrunt.hcl
```

![Example DAG](/assets/img/05-run-queue-0.svg)

Assuming a current working directory of the `root` directory, Terragrunt would run units in the following order:

- **`run --all plan` Order:** Terragrunt would run `independent` and `ancestor-dependency` concurrently. Once `ancestor-dependency` finishes, `dependency` would run. Once `dependency` finishes, `dependent` would run.
- **`run --all destroy` Order:** Terragrunt would run `dependent` and `independent` concurrently. Once `dependent` finishes, `dependency` would run. Once `dependency` finishes, `ancestor-dependency` would run.

## Controlling the Queue

Several flags allow you to customize how Terragrunt builds and executes the run queue. By default, Terragrunt will include all units that are in the current working directory.

### Include by default

By default, when using the `--all` flag, Terragrunt will include all units that are in the current working directory, and any external dependencies.

Certain flags trigger "Exclude by default" behavior, meaning that Terragrunt will no longer automatically include all units in the current working directory, and will instead rely on discovering configurations based on the provided queue control flags.

Those flags will be discussed in the next section.

### Filtering Units

You can control which units are included or excluded from the queue:

- [`--queue-include-dir`](/docs/reference/cli-options#queue-include-dir): Specify glob patterns for directories to *include*. Can be used multiple times.

  e.g. `terragrunt run --all plan --queue-include-dir "subtree/*"`

  Include units within the `subtree` directory (along with their dependencies), i.e., `subtree/dependent`, `subtree/dependency` and `ancestor-dependency`.
  > **Note:** Using the `--queue-include-dir` automatically triggers "Exclude by default" behavior, as mentioned above.
  >
  > **Note:** `ancestor-dependency` is still included by default here because it's a dependency of `dependency`. Using [`--queue-strict-include`](/docs/reference/cli-options#queue-strict-include) would prevent that.

- [`--queue-exclude-dir`](/docs/reference/cli-options#queue-exclude-dir): Specify glob patterns for directories to *exclude*. Can be specified multiple times.

  e.g. `terragrunt run --all plan --queue-exclude-dir "independent"`

  Exclude the `independent` unit. `ancestor-dependency`, `subtree/dependency`, and `subtree/dependent` would still be processed according to their dependencies.
  > **Note:** Dependencies of excluded units will still be included unless they are also explicitly excluded. In this example, excluding `subtree/dependency` would not automatically exclude `ancestor-dependency`.

- [`--queue-excludes-file`](/docs/reference/cli-options#queue-excludes-file): Provide a file containing a list of directories to exclude.

  e.g. `terragrunt run --all plan --queue-excludes-file ".tg-excludes"`

  ```text
  # .tg-excludes

  independent
  subtree/dependency
  ```

  Exclude `independent` and `subtree/dependency` from the run, only running `subtree/dependent` and `ancestor-dependency`.
  > **Tip:** The default value for this flag is `.terragrunt-excludes`. Populate a file named this in your project root to exclude units from being run by default, without using the `--queue-excludes-file` flag.

- [`--queue-strict-include`](/docs/reference/cli-options#queue-strict-include): Only include units matching `--queue-include-dir`.

  e.g. `terragrunt run --all plan --queue-include-dir "subtree/dependency" --queue-strict-include`

  Only include the `subtree/dependency` unit. Its dependency, `ancestor-dependency`, will be excluded because it does not match the include and strict mode is enabled.

- [`--queue-include-external`](/docs/reference/cli-options#queue-include-external): Include external dependencies (those outside the current working directory) by default.

  e.g. `terragrunt run --all plan --working-dir subtree --queue-include-external`

  Include `ancestor-dependency` in addition to the `subtree/dependent` and `subtree/dependency` units.
  > **Note:** The `--queue-include-external` flag is simply a convenience flag to avoid the interactive prompt to request inclusion of external dependencies. By default, Terragrunt will wait for user input to determine whether or not external dependencies should be included.

- [`--queue-exclude-external`](/docs/reference/cli-options#queue-exclude-external): Exclude external dependencies.

  e.g. `terragrunt run --all plan --working-dir subtree --queue-exclude-external`

  Exclude `ancestor-dependency` from the run. Only run `subtree/dependent` and `subtree/dependency`.
  > **Note:** This flag is simply a convenience flag to avoid the interactive prompt to request exclusion of external dependencies. By default, Terragrunt will wait for user input to determine whether or not external dependencies should be excluded.

- [`--queue-include-units-reading`](/docs/reference/cli-options#queue-include-units-reading): Include units that read a specific file (via [includes](/docs/reference/config-blocks-and-attributes#include) or HCL functions like [`mark_as_read`](/docs/reference/built-in-functions#mark_as_read)).

  e.g. `terragrunt run --all plan --queue-include-units-reading "subtree/common.hcl"`

  Example `subtree/dependent/terragrunt.hcl`:

  ```hcl
  dependency "dep" {
    config_path  = "../dependency"
    skip_outputs = true
  }

  include "common" {
    path = find_in_parent_folders("common.hcl")
  }
  ```

  Example `subtree/dependency/terragrunt.hcl`:

  ```hcl
  dependency "dep" {
    config_path  = "../../ancestor-dependency"
    skip_outputs = true
  }

  include "common" {
    path = find_in_parent_folders("common.hcl")
  }
  ```

  Example `subtree/common.hcl`:

  ```hcl
  # Intentionally empty
  ```

  Include `subtree/dependent` and `subtree/dependency` (the units that read `subtree/common.hcl`) in the run.
  > **Tip:** Sometimes, it can be impossible for Terragrunt to know that certain files are read by a unit (e.g. if the file is read in the OpenTofu/Terraform module, not in Terragrunt configuration). In cases like this, you can use the [`mark_as_read`](/docs/reference/built-in-functions#mark_as_read) HCL function to explicitly tell Terragrunt that a unit reads a file.

### Modifying Order and Error Handling

- [`--queue-construct-as`](/docs/reference/cli-options#queue-construct-as) (`--as`): Build the run queue *as if* a particular command was run. Useful for performing dry-runs of [`run`](/docs/reference/cli-options#run) using discovery commands, like [`find`](/docs/reference/cli-options#find) and [`list`](/docs/reference/cli-options#list).
  e.g. `terragrunt list --queue-construct-as destroy`
  This lists the units in the order they'd be processed for `run --all destroy`:

  ```bash
  $ terragrunt list --as destroy -l
  Type  Path
  unit  independent
  unit  subtree/dependent
  unit  subtree/dependency
  unit  ancestor-dependency
  ```

  ```bash
  $ terragrunt list --as plan -l
  Type  Path
  unit  ancestor-dependency
  unit  independent
  unit  subtree/dependency
  unit  subtree/dependent
  ```

- [`--queue-ignore-dag-order`](/docs/reference/cli-options#queue-ignore-dag-order): Execute units concurrently without respecting the dependency order.

  e.g. `terragrunt run --all plan --queue-ignore-dag-order`

  Run `plan` on `ancestor-dependency`, `subtree/dependency`, `subtree/dependent`, and `independent` all concurrently, without waiting for their defined dependencies. For instance, `subtree/dependent`'s plan would not wait for `subtree/dependency`'s plan to complete.
  > **Caution:** This flag is useful for faster runs in stateless commands like `validate` or `plan`, but is **dangerous** for commands that modify state like `apply` or `destroy`. You might encounter failed applies if unit dependencies are not applied before dependents, and conversely, failed destroys if unit dependents are not destroyed before dependencies.

- [`--queue-ignore-errors`](/docs/reference/cli-options#queue-ignore-errors): Continue processing the queue even if some units fail.

  e.g. `terragrunt run --all plan --queue-ignore-errors`

  If `ancestor-dependency`'s plan fails, Terragrunt will still attempt to run `plan` for `subtree/dependency`, then `subtree/dependent`, and also for `independent`.
  > **Caution:** This flag is useful for identifying all errors at once, but can lead to inconsistent state if used with `apply` or `destroy`. You might encounter failed applies if unit dependencies are not applied successfully before dependents, and conversely, failed destroys if unit dependents are not destroyed successfully before dependencies.

## Important Considerations

> **Caution:** When using `run --all plan` with units that have dependencies (e.g. via `dependency` or `dependencies` blocks), the command will fail if those dependencies have never been deployed. This is because Terragrunt cannot resolve dependency outputs without existing state.
>
> To work around this issue, use [mock outputs in dependency blocks](/docs/reference/config-blocks-and-attributes#dependency).
>
> **Caution:** Do not set `TF_PLUGIN_CACHE_DIR` when using `run --all` (unless using OpenTofu >= 1.10).
>
> This can cause concurrent access issues with the provider cache. Instead, use Terragrunt's built-in [Provider Cache Server](/docs/features/provider-cache-server/).
>
> **Caution:** When using `run --all` with `apply` or `destroy`, Terragrunt automatically adds the `-auto-approve` flag due to limitations with shared stdin making individual approvals impossible. Use [`--no-auto-approve`](/docs/reference/cli-options#no-auto-approve) to override this, but be aware you might need alternative approval workflows.
