---
title: Auto-Init
layout: single
author_profile: true
sidebar:
  nav: "docs"
---

_Auto-Init_ is a feature of terragrunt that makes it so that `terragrunt init` does not need to be called explicitly before other terragrunt commands.

When Auto-Init is enabled (the default), terragrunt will automatically call [`terraform init`](https://www.terraform.io/docs/commands/init.html)
during other commands (e.g. `terragrunt plan`) when terragrunt detects that
* `terraform init` has never been called, or
* source needs to be downloaded, or
* the modules or remote state used by the module have changed since the previous call to `terraform init`.

As mentioned [above](#extra_arguments-for-init), `extra_arguments` can be configured
to allow customization of the `terraform init` command.

Note that there might be cases where terragrunt does not properly detect that `terraform init` needs be called.
In this case, terraform would fail.  Just run `terragrunt init` to correct this situation.

For some use cases, it might be desirable to disable Auto-Init.
For example, if each user wants to specify a different `-plugin-dir` option to `terraform init` (and therefore it cannot be put in `extra_arguments`).

To disable Auto-Init, use the `--terragrunt-no-auto-init` command line option or set the `TERRAGRUNT_AUTO_INIT` environment variable to `false`.

Disabling Auto-Init means that you _must_ explicitly call `terragrunt init` prior to any other terragrunt commands for a particular configuration.

If Auto-Init is disabled, and terragrunt detects that `terraform init` needs to be called, then terragrunt will fail.
