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
TERRAGRUNT_EXPERIMENT_MODE='true' terragrunt plan
```

Instead of enabling experiment mode, you can also enable specific experiments by setting the [experiment](/docs/reference/cli-options/#experiment)
flag to a value that's specific to a experiment.
This can allow you to experiment with a specific unstable feature that you think might be useful to you.

```bash
terragrunt plan --experiment symlinks
```

Again, you can also use the environment variable, which can be more useful in CI/CD pipelines:

```bash
TERRAGRUNT_EXPERIMENT='symlinks' terragrunt plan
```

You can also enable multiple experiments at once with a comma delimited list.

**TODO**: Will add an example here once there's more than one officially supported experiment. The existing experiments are scattered throughout configuration, so they need to be pulled into this system first.

## Active Experiments

The following strict mode controls are available:

- [symlinks](#symlinks)
- [stacks](#stacks)
- [cli-redesign](#cli-redesign)

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

### `stacks`

Support for Terragrunt stacks.

#### `stacks` - What it does

Enable `stack` command to manage Terragrunt stacks.

#### `stacks` - How to provide feedback

Share your experience with the `stack` command in the [Stacks](https://github.com/gruntwork-io/terragrunt/issues/3313) RFC.
Feedback is crucial for ensuring the feature meets real-world use cases. Please include:

- Any bugs or issues encountered (including logs or stack traces if possible).
- Suggestions for additional improvements or enhancements.

#### `stacks` - Criteria for stabilization

To transition the `stacks` feature to a stable release, the following must be addressed:

- [x] Add support for `stack run *` command
- [x] Add support for `stack output` commands to extend stack-level operations.
- [x] Add support for stack "values".
- [x] Add support for recursive stacks.
- [x] Integration testing for recursive stack handling across typical workflows, ensuring smooth transitions during `plan`, `apply`, and `destroy` operations.
- [x] Confirm compatibility with parallelism flags (e.g., `--parallel`), especially for stacks with dependencies.
- [ ] Ensure that error handling and failure recovery strategies work as intended across large and nested stacks.

### `cli-redesign`

Support for the new Terragrunt CLI design.

#### `cli-redesign` - What it does

Enable features from the CLI Redesign RFC.

This includes:

- Addition of the `run` command.
- Addition of the `exec` command.

#### `cli-redesign` - How to provide feedback

Share your experience with these features in the [CLI Redesign](https://github.com/gruntwork-io/terragrunt/issues/3445) RFC.
Feedback is crucial for ensuring the feature meets real-world use cases. Please include:

- Any bugs or issues encountered (including logs or stack traces if possible).
- Suggestions for additional improvements or enhancements.

#### `cli-redesign` - Criteria for stabilization

To transition `cli-redesign` features to a stable release, the following must be addressed:

- [x] Add support for `run` command.
  - [x] Add support for basic usage of the `run` command (e.g., `terragrunt run plan`, `terragrunt run -- plan -no-color`).
  - [x] Add support for the `--all` flag.
  - [x] Add support for the `--graph` flag.
- [x] Add support for `exec` command.
- [x] Rename legacy `--terragrunt-` prefixed flags so that they no longer need the prefix.
- [ ] Add the `hcl` command, replacing commands like `hclfmt`, `hclvalidate` and `validate-inputs`.
- [x] Add OpenTofu commands as explicit shortcuts in the CLI instead of forwarding all unknown commands to OpenTofu/Terraform.
- [ ] Add support for the `backend` command.
- [ ] Add support for the `render` command.
- [ ] Add support for the `info` command.
- [ ] Add support for the `dag` command.
- [ ] Add support for the `find` command.
  - [x] Add support for `find` without flags.
  - [x] Add support for `find` with colorful output.
  - [x] Add support for `find` with `--format=json` flag.
  - [x] Add support for `find` with stdout redirection detection.
  - [x] Add support for `find` with `--hidden` flag.
  - [x] Add support for `find` with `--sort=alpha` flag.
  - [ ] Add support for `find` with `--sort=dag` flag.
  - [ ] Add support for `find` with the `exclude` block used to exclude units from the search.
  - [ ] Add integration with `symlinks` experiment to support finding units/stacks via symlinks.
- [ ] Add support for the `list` command.

### `cas`

Support for Terragrunt Content Addressable Storage (CAS).

#### `cas` - What it does

Allow Terragrunt to store and retrieve state files from a Content Addressable Storage (CAS) system.

At the moment, the CAS is only used to speed up catalog cloning, but in the future, it can be used to store more content.

#### `cas` - How to provide feedback

Share your experience with this feature in the [CAS](https://github.com/gruntwork-io/terragrunt/discussions/3939) Feedback GitHub Discussion.
Feedback is crucial for ensuring the feature meets real-world use cases. Please include:

- Any bugs or issues encountered (including logs or stack traces if possible).
- Suggestions for additional improvements or enhancements.

#### `cas` - Criteria for stabilization

To transition the `cas` feature to a stable release, the following must be addressed:

- [x] Add support for storing and retrieving catalog repositories from the CAS.
- [ ] Add support for storing and retrieving OpenTofu/Terraform modules from the CAS.
- [ ] Add support for storing and retrieving Unit/Stack configurations from the CAS.
