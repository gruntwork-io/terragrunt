---
layout: collection-browser-doc
title: CLI Redesign
category: migrate
categories_url: migrate
excerpt: Migration guide to adopt changes from RFC 3445
tags: ["migration", "community"]
order: 503
nav_title: Documentation
nav_title_link: /docs/
slug: cli-redesign
---

## Background

As part of the redesign in [#3445](https://github.com/gruntwork-io/terragrunt/issues/3445), several CLI adjustments have been made to improve user experience and consistency. This guide will help you understand the changes and how to adapt to them.

Note that this guide is being written while work is in progress (WIP), so some of the changes may not be fully implemented yet. We'll do our best to keep this guide up to date as the changes are finalized.

The high-level changes made as a part of that RFC that require migration for current users are as follows:

- The `terragrunt-` prefix has been removed from all flags.
- All environment variables have had their prefixes renamed to `TG_` instead of `TERRAGRUNT_`.
- A new `run` command has been introduced to the CLI that also handles the responsibilities of deprecated `run-all` and `graph` commands.
- A new `backend` command has been introduced to support users in interacting with backends.
- Infrequently used commands have been reorganized into a structure that makes it easier to find them.
- **WIP:** Arguments are no longer sent to `tofu` / `terraform` by default.

## Migration guide

All of the changes you need to make to adopt to this new CLI design involve changing how you invoke Terragrunt.

### Remove `terragrunt-` prefix from flags

If you are currently using flags that are prefixed with `terragrunt-`, you will need to stop using that flag, and use a differently named one instead (usually the same exact flag with `terragrunt-` removed from the beginning, but not always).

For example, if you are using the `--terragrunt-non-interactive` flag, you will need to switch to the [`--non-interactive`](/docs/reference/cli/#non-interactive) flag instead.

Before:

```bash
terragrunt plan --terragrunt-non-interactive
```

After:

```bash
terragrunt plan --non-interactive
```

Sometimes, the flag change might be slightly more involved than simply removing the `terragrunt-` prefix.

For example, if you are using the `--terragrunt-debug` flag, you will need to switch to the [`--inputs-debug`](/docs/reference/cli/#inputs-debug) flag instead.

Before:

```bash
terragrunt plan --terragrunt-debug
```

After:

```bash
terragrunt plan --inputs-debug
```

You can find the new flag names in the [CLI reference](/docs/reference/cli/) (including the deprecated flags they replace).

### Update environment variables

If you are currently using environment variables to configure Terragrunt, you will need to stop using that environment variable, and use a differently named one instead (usually the same exact environment variable with `TERRAGRUNT_` replaced with `TG_`, but not always).

For example, if you are using the `TERRAGRUNT_NON_INTERACTIVE` environment variable, you will need to switch to the [`TG_NON_INTERACTIVE`](/docs/reference/cli/#non-interactive) environment variable instead.

Before:

```bash
export TERRAGRUNT_NON_INTERACTIVE=true
```

After:

```bash
export TG_NON_INTERACTIVE=true
```

Sometimes, the environment variable change might be slightly more involved than simply replacing `TERRAGRUNT_` with `TG_`.

For example, if you are using the `TERRAGRUNT_DEBUG` environment variable, you will need to switch to the [`TG_DEBUG_INPUTS`](/docs/reference/cli/#inputs-debug) environment variable instead.

Before:

```bash
export TERRAGRUNT_DEBUG=true
```

After:

```bash
export TG_DEBUG_INPUTS=true
```

You can find the new environment variable names in the [CLI reference](/docs/reference/cli/) (including the deprecated environment variables they replace).

### Use the new `run` command

The `run` command has been introduced to the CLI to handle the responsibility currently held by the default command in Terragrunt.

If you want to tell Terragrunt that what you are running is a command in the orchestrated IaC tool (OpenTofu/Terraform), you can use the `run` command to explicitly indicate this.

For example, if you are currently using the `terragrunt` command to run `plan`, you can switch to the `run plan` command instead.

Before:

```bash
terragrunt plan
```

After:

```bash
terragrunt run plan
```

Note that certain shortcuts will be supported out of the box, such as `terragrunt plan`, so you can continue to use most `run` commands without the `run` keyword, as you were doing before.

For example, the following command will continue to work as expected:

```bash
terragrunt plan
```

The commands that will not receive shortcuts are OpenTofu/Terraform commands that are not recommended for usage with Terragrunt, or have a conflict with the Terragrunt CLI API.

For example, the `workspace` command will not receive a shortcut, as you are encouraged not to use workspaces when working with Terragrunt. Terragrunt manages state isolation for you, so you don't need to use workspaces.

If you would like to explicitly run a command that does not have a shortcut, you can use the `run` command to do so:

```bash
terragrunt run workspace ls
```

Similarly, commands like `graph` won't be supported as a shortcut, as `graph` is a now deprecated command in the Terragrunt CLI. Supporting it as a shortcut would be misleading, so you can use the `run` command to run it explicitly:

```bash
terragrunt run graph
```

In addition to allowing for explicit invocation of OpenTofu/Terraform instead of using shortcuts, the `run` command also takes on the responsibilities of the now deprecated `run-all` and `graph` commands using flags.

For example, if you are currently using the `terragrunt run-all` command, you can switch to the `run` command with the `--all` flag instead.

Before:

```bash
terragrunt run-all plan
```

After:

```bash
terragrunt run --all plan
```
