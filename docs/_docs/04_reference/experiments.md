---
layout: collection-browser-doc
title: Experiments
category: reference
categories_url: reference
excerpt: >-
  Opt-in to experimental features before they're stable.
tags: ["CLI"]
order: 405
nav_title: Documentation
nav_title_link: /docs/
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

### symlinks

Support symlink resolution for Terragrunt units.

#### What it does

By default, Terragrunt will ignore symlinks when determining which units it should run. By enabling this experiment, Terragrunt will resolve symlinks and add them to the list of units being run.

#### How to provide feedback

Provide your feedback on the [Experiment: Symlinks](https://github.com/gruntwork-io/terragrunt/discussions/3671) discussion.

#### Criteria for stabilization

To stabilize this feature, the following need to be resolved, at a minimum:

- [ ] Ensure that symlink support continues to work for users referencing symlinks in flags. See [#3622](https://github.com/gruntwork-io/terragrunt/issues/3622).
  - [ ] Add integration tests for all filesystem flags to confirm support with symlinks (or document the fact that they cannot be supported).
- [ ] Ensure that MacOS integration tests still work. See [#3616](https://github.com/gruntwork-io/terragrunt/issues/3616).
  - [ ] Add integration tests for MacOS in CI.

### stacks

Support for Terragrunt stacks.

#### What it does

Enable `stack` command to manage Terragrunt stacks.

#### How to provide feedback

Share your experience with the `stack` command in the [Stacks](https://github.com/gruntwork-io/terragrunt/issues/3313) RFC.
Feedback is crucial for ensuring the feature meets real-world use cases. Please include:

- Any bugs or issues encountered (including logs or stack traces if possible).
- Suggestions for additional improvements or enhancements.

#### Criteria for stabilization

To transition the `stacks` feature to a stable release, the following must be addressed:

- [ ] Add support for `stack run *` and `stack output` commands to extend stack-level operations.
- [ ] Integration testing for recursive stack handling across typical workflows, ensuring smooth transitions during `plan`, `apply`, and `destroy` operations.
- [ ] Confirm compatibility with parallelism flags (e.g., `--parallel`), especially for stacks with dependencies.
- [ ] Ensure that error handling and failure recovery strategies work as intended across large and nested stacks.
