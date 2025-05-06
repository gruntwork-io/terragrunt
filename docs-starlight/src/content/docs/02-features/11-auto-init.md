---
title: Auto-init
description: Learn how Terragrunt makes it so that you don't have to explicitly call `init` when using it.
slug: docs/features/auto-init
sidebar:
  order: 11
---

*Auto-Init* is a feature of Terragrunt that makes it so that `terragrunt init` does not need to be called explicitly before other terragrunt commands.

When Auto-Init is enabled (the default), terragrunt will automatically call [`tofu init`](https://opentofu.org/docs/cli/commands/init/)/[`terraform init`](https://www.terraform.io/docs/commands/init.html) before other commands (e.g. `terragrunt plan`) when terragrunt detects that any of the following are true:

- `init` has never been called.
- `source` needs to be downloaded.
- The `.terragrunt-init-required` file is in the downloaded module directory (`.terragrunt-cache/aaa/bbb/modules/<module>`).
- The modules or remote state used by a module have changed since the previous call to `init`.

As [mentioned](/docs/features/extra-arguments/#extra_arguments-for-init), `extra_arguments` can be configured to allow customization of the `tofu init` command.

Note that there might be cases where Terragrunt does not detect that `tofu init` needs to be called. In such cases, OpenTofu/Terraform may fail, and re-running `terragrunt init` can resolve the issue.

## Disabling Auto-Init

In some cases, it might be desirable to disable Auto-Init.

For example, you might want to specify a different `-plugin-dir` option to `tofu init` (and don't want to have it set in `extra_arguments`).

To disable Auto-Init, use the `--no-auto-init` command line option or set the `TG_NO_AUTO_INIT` environment variable to `true`.

Disabling Auto-Init requires you to explicitly run `terragrunt init` before executing any other Terragrunt commands for that configuration. If Auto-Init is disabled and Terragrunt detects that `init` should have been run, it will throw an error.
