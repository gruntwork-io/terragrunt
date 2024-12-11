---
layout: collection-browser-doc
title: Auto-init
category: features
categories_url: features
excerpt: Learn how Terragrunt makes it so that you don't have to explicitly call `init` when using it.
tags: ["CLI"]
order: 245
nav_title: Documentation
nav_title_link: /docs/
---

*Auto-Init* is a feature of Terragrunt that makes it so that `terragrunt init` does not need to be called explicitly before other terragrunt commands.

When Auto-Init is enabled (the default), terragrunt will automatically call [`tofu init`](https://opentofu.org/docs/cli/commands/init/)/[`terraform init`](https://www.terraform.io/docs/commands/init.html) before other commands (e.g. `terragrunt plan`) when terragrunt detects that any of the following are true:

- `init` has never been called.
- `source` needs to be downloaded.
- The `.terragrunt-init-required` file is in the downloaded module directory (`.terragrunt-cache/aaa/bbb/modules/<module>`).
- The modules or remote state used by a module have changed since the previous call to `init`.

As [mentioned]({{site.baseurl}}/docs/features/extra-arguments/#extra_arguments-for-init), `extra_arguments` can be configured to allow customization of the `terraform init` command.

Note that there might be cases where terragrunt does not properly detect that `terraform init` needs be called. In this case, OpenTofu/Terraform can fail. Running `terragrunt init` again in these circumstances can correct the issue.

## Disabling Auto-Init

In some cases, it might be desirable to disable Auto-Init.

For example, you might want to specify a different `-plugin-dir` option to `terraform init` (and don't want to have it set in `extra_arguments`).

To disable Auto-Init, use the `--terragrunt-no-auto-init` command line option or set the `TERRAGRUNT_NO_AUTO_INIT` environment variable to `true`.

Disabling Auto-Init means that you *must* explicitly call `terragrunt init` prior to any other Terragrunt commands for a particular configuration. If Auto-Init is disabled, and Terragrunt detects that `init` needs to be called, Terragrunt will throw an error.
