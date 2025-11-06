---
title: Experiments
description: Opt-in to experimental features before they're stable.
slug: docs/reference/experiments
sidebar:
  order: 4
---

Terragrunt supports operating in a mode referred to as "Experiment Mode".

Experiment Mode is a set of controls that can be enabled to opt in to experimental features before they're stable.
These features are subject to change and may be removed or altered at any time.
They generally provide early access to new features or changes that are being considered for inclusion in future releases.

Those experiments will be documented here so that you know the following:

1. What the experiment is.
2. What the experiment does.
3. How to provide feedback on the experiment.
4. What criteria must be met for the experiment to be considered stable.

Sometimes, the criteria for an experiment to be considered stable is unknown, as there may not be a clear path to stabilization. In that case, this will be noted in the experiment documentation, and collaboration with the community will be encouraged to help determine the future of the experiment.

## Controlling Experiment Mode

The simplest way to enable experiment mode is to set the [experiment-mode](/docs/reference/experiments) flag.

This will enable experiment mode for all Terragrunt commands, for all experiments (note that this isn't generally recommended, unless you are following Terragrunt development closely and are prepared for the possibility of breaking changes).

```bash
terragrunt plan --experiment-mode
```

You can also use the environment variable, which can be more useful in CI/CD pipelines:

```bash
TG_EXPERIMENT_MODE='true' terragrunt plan
```

Instead of enabling experiment mode, you can also enable specific experiments by setting the [experiment](/docs/reference/experiments)
flag to a value that's specific to an experiment.
This can allow you to experiment with a specific unstable feature that you think might be useful to you.

```bash
terragrunt plan --experiment symlinks
```

Again, you can also use the environment variable, which can be more useful in CI/CD pipelines:

```bash
TG_EXPERIMENT='symlinks' terragrunt plan
```

You can also enable multiple experiments at once.

```bash
terragrunt --experiment symlinks plan
```

Including the environment variable:

```bash
TG_EXPERIMENT='symlinks,stacks' terragrunt plan
```

## Active Experiments

The following experiments are available:

- [symlinks](#symlinks)
- [cas](#cas)
- [filter-flag](#filter-flag)

### symlinks

Support symlink resolution for Terragrunt units.

#### symlinks - What it does

By default, Terragrunt will ignore symlinks when determining which units it should run. By enabling this experiment, Terragrunt will resolve symlinks and add them to the list of units being run.

#### symlinks - How to provide feedback

Provide your feedback on the [Experiment: Symlinks](https://github.com/gruntwork-io/terragrunt/discussions/3671) discussion.

#### symlinks - Criteria for stabilization

To stabilize this feature, the following need to be resolved, at a minimum:

- [ ] Ensure that symlink support continues to work for users referencing symlinks in flags. See [#3622](https://github.com/gruntwork-io/terragrunt/issues/3622).
  - [ ] Add integration tests for all filesystem flags to confirm support with symlinks (or document the fact that they cannot be supported).
- [ ] Ensure that MacOS integration tests still work. See [#3616](https://github.com/gruntwork-io/terragrunt/issues/3616).
  - [ ] Add integration tests for MacOS in CI.

### `cas`

Support for Terragrunt Content Addressable Storage (CAS).

#### `cas` - What it does

Allow Terragrunt to store and retrieve Git repositories from a Content Addressable Storage (CAS) system.

The CAS is used to speed up both catalog cloning and OpenTofu/Terraform source cloning by avoiding redundant downloads of Git repositories.

#### `cas` - How to provide feedback

Share your experience with this feature in the [CAS](https://github.com/gruntwork-io/terragrunt/discussions/3939) Feedback GitHub Discussion.
Feedback is crucial for ensuring the feature meets real-world use cases. Please include:

- Any bugs or issues encountered (including logs or stack traces if possible).
- Suggestions for additional improvements or enhancements.

#### `cas` - Criteria for stabilization

To transition the `cas` feature to a stable release, the following must be addressed:

- [x] Add support for storing and retrieving catalog repositories from the CAS.
- [x] Add support for storing and retrieving OpenTofu/Terraform modules from the CAS.
- [ ] Add support for storing and retrieving Unit/Stack configurations from the CAS.

### `filter-flag`

Support for sophisticated unit and stack filtering using the `--filter` flag.

#### `filter-flag` - What it does

The `--filter` flag provides a sophisticated querying syntax for targeting units and stacks in Terragrunt commands. This unified approach replaces the need for multiple queue control flags and offers powerful filtering capabilities.

**Current Support Status:**

- ✅ Available in `find`, `list`, and `run` commands

**Supported Filtering Types:**

1. **Name-based filtering**: Target units/stacks by their directory name (exact match or glob patterns)
2. **Path-based filtering**: Target units/stacks by their file system path (relative, absolute, or glob patterns)
3. **Attribute-based filtering**: Target units by configuration attributes:
   - `type=unit` or `type=stack` - Filter by component type
   - `external=true` or `external=false` - Filter by whether the unit/stack is an external dependency (outside the current working directory)
   - `name=pattern` - Filter by name using glob patterns
4. **Negation filters**: Exclude units using the `!` prefix
5. **Filter intersection**: Combine filters using the `|` operator for results pruning
6. **Multiple filters**: Specify multiple `--filter` flags to union results

**Not Yet Implemented:**

- Git-based filtering (`[ref...ref]` syntax)
- Dependency/dependent traversal (`...` syntax)

#### `filter-flag` - How to provide feedback

Provide your feedback on the [Filter Flag RFC](https://github.com/gruntwork-io/terragrunt/issues/4060) GitHub issue.

#### `filter-flag` - Criteria for stabilization

To transition the `filter-flag` feature to a stable release, the following must be addressed, at a minimum:

- [x] Add support for name-based filtering
- [x] Add support for path-based filtering (relative, absolute, glob)
- [x] Add support for attribute-based filtering (type, external, name)
- [x] Add support for negation filters (!)
- [x] Add support for filter intersection (|)
- [x] Add support for multiple filters (union/OR semantics)
- [x] Integrate with the `find` command
- [x] Integrate with the `list` command
- [x] Integrate with the `run` command
- [ ] Add support for git-based filtering ([ref...ref] syntax)
- [ ] Add support for dependency/dependent traversal (... syntax)
- [ ] Add support for `--filters-file` flag
- [ ] Add support for `--filter-allow-destroy` flag
- [ ] Add support for `--filter-affected` shorthand
- [ ] Comprehensive integration testing across all commands
- [ ] Deprecate legacy queue control flags (queue-exclude-dir, queue-include-dir, etc.)

**Future Deprecations:**

When this experiment stabilizes, the following queue control flags will be deprecated in favor of the unified `--filter` flag:

- `--queue-exclude-dir`
- `--queue-excludes-file`
- `--queue-exclude-external`
- `--queue-include-dir`
- `--queue-include-external`
- `--queue-include-units-including`
- `--queue-strict-include`

The current plan is to continue to support the flags as aliases for particular `--filter` patterns.

## Completed Experiments

- [cli-redesign](#cli-redesign)
- [stacks](#stacks)
- [runner-pool](#runner-pool)
- [report](#report)
- [auto-provider-cache-dir](#auto-provider-cache-dir)

### `cli-redesign`

Support for the new Terragrunt CLI design.

#### `cli-redesign` - What it does

Enabled features from the CLI Redesign RFC.

This experiment flag is no longer needed, as the CLI Redesign is now the default.

#### `cli-redesign` - How to provide feedback

Now that the CLI Redesign experiment is complete, please provide feedback in the form of standard [GitHub issues](https://github.com/gruntwork-io/terragrunt/issues).

#### `cli-redesign` - Criteria for stabilization

To transition `cli-redesign` features to a stable the following have been completed:

- [x] Add support for `run` command.
  - [x] Add support for basic usage of the `run` command (e.g., `terragrunt run plan`, `terragrunt run -- plan -no-color`).
  - [x] Add support for the `--all` flag.
  - [x] Add support for the `--graph` flag.
- [x] Add support for `exec` command.
- [x] Rename legacy `--terragrunt-` prefixed flags so that they no longer need the prefix.
- [x] Add the `hcl` command, replacing commands like `hclfmt`, `hclvalidate` and `validate-inputs`.
- [x] Add OpenTofu commands as explicit shortcuts in the CLI instead of forwarding all unknown commands to OpenTofu/Terraform.
- [x] Add support for the `backend` command.
- [x] Add support for the `render` command.
- [x] Add support for the `info` command.
- [x] Add support for the `dag` command.
- [x] Add support for the `find` command.
  - [x] Add support for `find` without flags.
  - [x] Add support for `find` with colorful output.
  - [x] Add support for `find` with `--format=json` flag.
  - [x] Add support for `find` with stdout redirection detection.
  - [x] Add support for `find` with `--hidden` flag.
  - [x] Add support for `find` with `--sort=alpha` flag.
  - [x] Add support for `find` with `--sort=dag` flag.
  - [x] Add support for `find` with the `exclude` block used to exclude units from the search.
  - [x] Add integration with `symlinks` experiment to support finding units/stacks via symlinks.
  - [x] Add handling of broken configurations or configurations requiring authentication.
  - [x] Add integration test for `find` with `--sort=dag` flag on all the fixtures in the `test/fixtures` directory.
- [x] Add support for the `list` command.
  - [x] Add support for `list` without flags.
  - [x] Add support for `list` with colorful output.
  - [x] Add support for `list` with `--format=tree` flag.
  - [x] Add support for `list` with `--format=long` flag.
  - [x] Add support for `list` with stdout redirection detection.
  - [x] Add support for `list` with `--hidden` flag.
  - [x] Add support for `list` with `--sort=alpha` flag.
  - [x] Add support for `list` with `--sort=dag` flag.
  - [x] Add support for `list` with `--group-by=fs` flag.
  - [x] Add support for `list` with `--group-by=dag` flag.
  - [x] Add support for `list` with the `exclude` block used to exclude units from the search.
  - [x] Add integration with `symlinks` experiment to support listing units/stacks via symlinks.
  - [x] Add handling of broken configurations or configurations requiring authentication.
  - [x] Add integration test for `list` with `--sort=dag` flag on all the fixtures in the `test/fixtures` directory.

### stacks

Support for Terragrunt stacks.

#### What it does

Enable `stack` command to manage Terragrunt stacks.

#### stacks - Criteria for stabilization

To transition the `stacks` feature to a stable release, the following must be addressed:

- [x] Add support for `stack run *` command
- [x] Add support for `stack output` commands to extend stack-level operations.
- [x] Integration testing for recursive stack handling across typical workflows, ensuring smooth transitions during `plan`, `apply`, and `destroy` operations.
- [x] Confirm compatibility with parallelism flags (e.g., `--parallel`), especially for stacks with dependencies.
- [x] Ensure that error handling and failure recovery strategies work as intended across large and nested stacks.

### `runner-pool`

Proposes replacing Terragrunt's group-based execution with a dynamic runner pool that schedules Units as soon as dependencies are resolved.
This improves efficiency, reduces bottlenecks, and limits the impact of individual failures.

#### `runner-pool` - What it does

Allow usage of experimental runner pool implementation for units execution.

#### `runner-pool` - How to provide feedback

Provide your feedback on the [Runner Pool](https://github.com/gruntwork-io/terragrunt/issues/3629).

#### `runner-pool` - Criteria for stabilization

To transition the `runner-pool` feature to a stable release, the following must be addressed:

- [x] Use new discovery and queue packages to discover units.
- [x] Add support for including/excluding external units in the discovery process.
- [x] Add runner pool implementation to execute discovered units.
- [x] Add integration tests to track that the runner pool works in the same way as the current implementation.
- [x] Add performance tests to track that the runner pool implementation is faster than the current implementation.
- [x] Add support for fail fast behavior in the runner pool.
- [x] Improve the UI to queue to apply.
- [x] Add OpenTelemetry support to the runner pool.

### `report`

Support for Terragrunt Run Reports and Summaries.

#### `report` - What it does

Allows generation of run reports and summary displays. This experiment flag is no longer needed, as the report feature is now stable and available by default.

#### `report` - How to provide feedback

Now that the report experiment is complete, please provide feedback in the form of standard [GitHub issues](https://github.com/gruntwork-io/terragrunt/issues).

#### `report` - Criteria for stabilization

To transition the `report` feature to stable, the following have been completed:

- [x] Add support for generating reports (in CSV format by default).
- [x] Add support for displaying summaries of runs.
- [x] Add ability to disable summary display.
- [x] Add support for generating reports in JSON format.
- [x] Add comprehensive integration tests for the `report` experiment.
- [x] Finalize the design of run summaries and reports.

### `auto-provider-cache-dir`

Enable native OpenTofu provider caching by setting `TF_PLUGIN_CACHE_DIR` instead of using Terragrunt's internal provider cache server.

#### `auto-provider-cache-dir` - What it does

This experiment automatically configures OpenTofu to use its built-in provider caching mechanism by setting the `TF_PLUGIN_CACHE_DIR` environment variable. This approach leverages OpenTofu's native provider caching capabilities, which are more robust for concurrent operations in OpenTofu 1.10+.

This experiment flag is no longer needed, as the auto-provider-cache-dir feature is now enabled by default when using OpenTofu >= 1.10.

**Requirements:**

- OpenTofu version >= 1.10 is required
- Only works when using OpenTofu (not Terraform)

**Disabling the feature:**

You can disable the auto-provider-cache-dir feature using the `--no-auto-provider-cache-dir` flag:

```bash
terragrunt run --all apply --no-auto-provider-cache-dir
```

#### `auto-provider-cache-dir` - How to provide feedback

Now that the auto-provider-cache-dir experiment is complete, please provide feedback in the form of standard [GitHub issues](https://github.com/gruntwork-io/terragrunt/issues).

#### `auto-provider-cache-dir` - Criteria for stabilization

To transition the `auto-provider-cache-dir` feature to stable, the following have been completed:

- [x] Comprehensive testing to confirm the safety of concurrent runs using the same provider cache directory.
- [x] Performance comparison with the existing provider cache server approach.
- [x] Documentation and examples of best practices for usage.
- [x] Community feedback on real-world usage and any edge cases discovered.
