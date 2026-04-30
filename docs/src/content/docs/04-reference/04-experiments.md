---
title: Experiments
description: Opt-in to experimental features before they're stable.
slug: reference/experiments
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

The simplest way to enable experiment mode is to set the [experiment-mode](/reference/experiments) flag.

This will enable experiment mode for all Terragrunt commands, for all experiments (note that this isn't generally recommended, unless you are following Terragrunt development closely and are prepared for the possibility of breaking changes).

```bash
terragrunt plan --experiment-mode
```

You can also use the environment variable, which can be more useful in CI/CD pipelines:

```bash
TG_EXPERIMENT_MODE='true' terragrunt plan
```

Instead of enabling experiment mode, you can also enable specific experiments by setting the [experiment](/reference/experiments)
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

## Ongoing Experiments

### `azure-backend`

The `azure-backend` experiment enables native Azure Storage (azurerm) as a remote state backend. When enabled, Terragrunt can manage Azure Blob Storage for state files, including automatic creation of storage accounts and containers via the `remote_state` block with `backend = "azurerm"`.

**What it does:**

- Adds `azurerm` as a supported backend type in the `remote_state` block
- Supports automatic storage account and container creation (`create_storage_account_if_not_exists`)
- Supports multiple Azure authentication methods (Azure AD, Managed Identity, Service Principal, SAS token)
- Integrates with the `backend bootstrap` and `backend delete` CLI commands

**How to enable:**

```bash
terragrunt --experiment azure-backend plan
```

Or via environment variable:

```bash
TG_EXPERIMENT='azure-backend' terragrunt plan
```

**Feedback:** File issues on the [Terragrunt GitHub repository](https://github.com/gruntwork-io/terragrunt) with the `azure-backend` label.

**Stabilization criteria:** The experiment will be considered stable after sufficient community testing across different Azure environments, authentication methods, and edge cases (sovereign clouds, private endpoints, cross-subscription access).
