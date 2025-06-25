---
layout: collection-browser-doc
title: Experiments
category: reference
categories_url: reference
excerpt: >-
  Opt-in to experimental features before they're stable.
tags: ["CLI"]
order: 406
nav_title: Documentation
nav_title_link: /docs/
slug: experiments
redirect_from:
    - /docs/reference/experiment-mode/
---

Terragrunt supports operating in a mode referred to as "Experiment Mode".

Experiment Mode is a set of controls that can be enabled to opt-in to experimental features before they're stable.
These features are subject to change and may be removed or altered at any time.
They generally provide early access to new features or changes that are being considered for inclusion in future releases.

Those experiments will be documented here so that you know the following:

1. What the experiment is.
2. What the experiment does.
3. How to provide feedback on the experiment.
4. What criteria must be met for the experiment to be considered stable.

Sometimes, the criteria for an experiment to be considered stable is unknown, as there may not be a clear path to stabilization. In that case, this will be noted in the experiment documentation, and collaboration with the community will be encouraged to help determine the future of the experiment.

## Controlling Experiment Mode

The simplest way to enable experiment mode is to set the [experiment-mode](/docs/reference/cli-options/#experiment-mode) flag.

This will enable experiment mode for all Terragrunt commands, for all experiments (note that this isn't generally recommended, unless you are following Terragrunt development closely and are prepared for the possibility of breaking changes).

```bash
terragrunt plan --experiment-mode
```

You can also use the environment variable, which can be more useful in CI/CD pipelines:

```bash
TG_EXPERIMENT_MODE='true' terragrunt plan
```

Instead of enabling experiment mode, you can also enable specific experiments by setting the [experiment](/docs/reference/cli-options/#experiment)
flag to a value that's specific to a experiment.
This can allow you to experiment with a specific unstable feature that you think might be useful to you.

```bash
terragrunt plan --experiment symlinks
```

Again, you can also use the environment variable, which can be more useful in CI/CD pipelines:

```bash
TG_EXPERIMENT='symlinks' terragrunt plan
```

You can also enable multiple experiments at once with a comma delimited list.

**TODO**: Will add an example here once there's more than one officially supported experiment. The existing experiments are scattered throughout configuration, so they need to be pulled into this system first.

## Active Experiments

The following experiments are available:

- [symlinks](#symlinks)
- [cas](#cas)
- [report](#report)
- [runner-pool](#runner-pool)
- [auto-provider-cache-dir](#auto-provider-cache-dir)

### `symlinks`

Support symlink resolution for Terragrunt units.

#### `symlinks` - What it does

By default, Terragrunt will ignore symlinks when determining which units it should run. By enabling this experiment, Terragrunt will resolve symlinks and add them to the list of units being run.

#### `symlinks` - How to provide feedback

Provide your feedback on the [Experiment: Symlinks](https://github.com/gruntwork-io/terragrunt/discussions/3671) discussion.

#### `symlinks` - Criteria for stabilization

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

### `report`

Support for Terragrunt Run Reports and Summaries.

#### `report` - What it does

Allow usage of experimental run report generation, and summary displays.

#### `report` - How to provide feedback

Provide your feedback on the [Run Summary RFC](https://github.com/gruntwork-io/terragrunt/issues/3628).

#### `report` - Criteria for stabilization

To transition the `report` feature to a stable release, the following must be addressed:

- [x] Add support for generating reports (in CSV format by default).
- [x] Add support for displaying summaries of runs.
- [x] Add ability to disable summary display.
- [x] Add support for generating reports in JSON format.
- [x] Add comprehensive integration tests for the `report` experiment.
- [x] Finalize the design of run summaries and reports.

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
- [ ] Add support for including/excluding external units in the discovery process.
- [ ] Add runner pool implementation to execute discovered units.
- [ ] Add integration tests to track that the runner pool works in the same way as the current implementation.
- [ ] Add performance tests to track that the runner pool implementation is faster than the current implementation.
- [ ] Add support for fail fast behavior in the runner pool.
- [ ] Improve the UI to queue to apply.
- [ ] Add OpenTelemetry support to the runner pool.

### `auto-provider-cache-dir`

Enable native OpenTofu provider caching by setting `TF_PLUGIN_CACHE_DIR` instead of using Terragrunt's internal provider cache server.

#### `auto-provider-cache-dir` - What it does

When enabled, this experiment automatically configures OpenTofu to use its built-in provider caching mechanism by setting the `TF_PLUGIN_CACHE_DIR` environment variable. This approach leverages OpenTofu's native provider caching capabilities, which are more robust for concurrent operations in OpenTofu 1.10+.

**Requirements:**

- OpenTofu version >= 1.10 is required
- Only works when using OpenTofu (not Terraform)
- If the requirements are not met, the experiment silently does nothing

**Usage:**

```bash
terragrunt run --all apply --experiment auto-provider-cache-dir
```

Or with environment variables:

```bash
TG_EXPERIMENT='auto-provider-cache-dir' \
terragrunt run --all apply
```

**Disabling the feature:**

Even when the experiment is enabled, you can still disable the auto-provider-cache-dir feature for specific runs using the `--no-auto-provider-cache-dir` flag:

```bash
terragrunt run --all apply --experiment auto-provider-cache-dir --no-auto-provider-cache-dir
```

This will be most important post-stabilization, when the feature is enabled by default.

#### `auto-provider-cache-dir` - How to provide feedback

Please provide feedback through [GitHub issues](https://github.com/gruntwork-io/terragrunt/issues) with the `experiment: auto-provider-cache-dir` label.

#### `auto-provider-cache-dir` - Criteria for stabilization

To transition the `auto-provider-cache-dir` feature to a stable release, the following must be addressed:

- [ ] Comprehensive testing to confirm the safety of concurrent runs using the same provider cache directory.
- [ ] Performance comparison with the existing provider cache server approach.
- [ ] Documentation and examples of best practices for usage.
- [ ] Community feedback on real-world usage and any edge cases discovered.

Note that the current plan for stabilization is to have the feature be enabled by default, and to allow users to opt-out if they need to, or use the provider cache server if they want to do something more advanced, like store their provider cache in a different filesystem.

## Completed Experiments

- [cli-redesign](#cli-redesign)
- [stacks](#stacks)

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

### `stacks`

Support for Terragrunt stacks.

#### `stacks` - What it does

Enable `stack` command to manage Terragrunt stacks.

#### `stacks` - Criteria for stabilization

To transition the `stacks` feature to a stable release, the following must be addressed:

- [x] Add support for `stack run *` command
- [x] Add support for `stack output` commands to extend stack-level operations.
- [x] Add support for stack "values".
- [x] Add support for recursive stacks.
- [x] Integration testing for recursive stack handling across typical workflows, ensuring smooth transitions during `plan`, `apply`, and `destroy` operations.
- [x] Confirm compatibility with parallelism flags (e.g., `--parallel`), especially for stacks with dependencies.
- [x] Ensure that error handling and failure recovery strategies work as intended across large and nested stacks.
