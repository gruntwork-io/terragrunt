---
layout: collection-browser-doc
title: CLI Redesign
category: migrate
categories_url: migrate
excerpt: Migration guide to adopt changes from RFC 3445
tags: ["migration", "community"]
order: 603
nav_title: Documentation
nav_title_link: /docs/
slug: cli-redesign
---

## Background

As part of the redesign in [#3445](https://github.com/gruntwork-io/terragrunt/issues/3445), several CLI adjustments have been made to improve user experience and consistency. This guide will help you understand the changes and how to adapt to them.

Note that this guide is being written while deprecations are in effect, so some of the changes may not be breaking yet. We'll do our best to keep this guide up to date as the changes are finalized. To opt in to breaking changes early, you can use the relevant [strict control](/docs/reference/strict-mode/).

The high-level changes made as a part of that RFC that require migration for current users are as follows:

- The `terragrunt-` prefix has been removed from all flags.
- All environment variables have had their prefixes renamed to `TG_` instead of `TERRAGRUNT_`.
- A new `run` command has been introduced to the CLI that also handles the responsibilities of deprecated `run-all` and `graph` commands.
- A new `backend` command has been introduced to support users in interacting with backends.
- Infrequently used commands have been reorganized into a structure that makes it easier to find them.
- Arguments are no longer sent to `tofu` / `terraform` by default.

## Migration guide

All of the changes you need to make to adopt to this new CLI design involve changing how you invoke Terragrunt from the command line.

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

For example, if you are using the `TERRAGRUNT_DEBUG` environment variable, you will need to switch to the [`TG_INPUTS_DEBUG`](/docs/reference/cli/#inputs-debug) environment variable instead.

Before:

```bash
export TERRAGRUNT_DEBUG=true
```

After:

```bash
export TG_INPUTS_DEBUG=true
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

For example, the `workspace` command will not receive a shortcut, as you are encouraged not to use workspaces when working with Terragrunt. Terragrunt manages state isolation for you, so you don't need to use them.

If you would like to explicitly run a command that does not have a shortcut, you can use the `run` command to do so:

```bash
terragrunt run workspace ls
```

Similarly, commands like `graph` won't be supported as a shortcut, as `graph` is a now deprecated command in the Terragrunt CLI. Supporting it as a shortcut would be misleading, so you can use the `run` command to run it explicitly:

```bash
terragrunt run graph
```

You might want to explicitly indicate that the flag you are using is one for OpenTofu/Terraform, and not a Terragrunt flag. To do this, you can use the `--` argument to explicitly separate the Terragrunt flags from the OpenTofu/Terraform flags:

```bash
terragrunt run  -- apply -auto-approve
```

This usually isn't necessary, except when combining a complicated series of flags and arguments, which can be difficult to parse for the CLI.

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

### Take advantage of the new `exec` command

Previously, Terragrunt only supported orchestrating the `tofu` and `terraform` binaries as the main program being executed when Terragrunt was invoked.

With the introduction of the new [exec](/docs/reference/cli-options/#exec) command, this is no longer the case. You can now orchestrate any program you want, and integrate it with Terragrunt's ability to fetch outputs, download OpenTofu/Terraform modules, set `inputs`, and more.

For example, if you want to use Terragrunt to list the contents of an AWS S3 bucket, you can do the following:

```bash
terragrunt exec -- bash -c 'aws s3 ls s3://$TF_VAR_bucket_name'
```

With the following `terragrunt.hcl` file:

```hcl
inputs = {
  bucket_name = "my-bucket"
}
```

Terragrunt will load the `inputs` for the unit, and make them available as `TF_VAR_` prefixed environment variables for the executed command.

This offers a flexible way to integrate Terragrunt with other tools, besides just OpenTofu/Terraform for simple operational tasks.

### Use the new `backend` capabilities

Previously, Terragrunt would automatically provision any backend resources defined in the [remote_state](/docs/reference/config-blocks-and-attributes/#remote_state) block of a `terragrunt.hcl` file.

This was a source of confusion for many users, as it was potentially performing additional actions that users did not intend without asking for it.

As part of the CLI Redesign, Terragrunt now supports a dedicated [backend command](/docs/reference/cli-options/#backend-commands) to handle processes involved with interacting with OpenTofu/Terraform backends.

This includes the ability to bootstrap (provision) backend resources, migrate state between backend state files, and delete backend state files.

In addition, the `--backend-bootstrap` flag has been introduced, which preserves the legacy behavior of automatically provisioning backend resources. As this flag requires explicit opt in, you will want to explicitly set this flag (or the corresponding environment variable `TG_BACKEND_BOOTSTRAP` to `true`) if you want to continue to have Terragrunt automatically provision backend resources.

Before:

```bash
terragrunt plan
```

After:

```bash
terragrunt plan --backend-bootstrap
```

### Use the new `find` and `list` commands

The [find](/docs/reference/cli-options/#find) and [list](/docs/reference/cli-options/#list) commands have been introduced to help you discover configurations in your Terragrunt projects.

The `find` command is useful when you want to perform programmatic discovery of a Terragrunt unit or configuration of that unit, and the `list` command is useful when you want to get a high-level overview of the Terragrunt units and configurations in your project.

If you are currently using the `output-module-groups` command, you can switch to the `find --dag --json` command to get a more fine grained outlook on the nature of your Terragrunt configurations. You can also use the `list --dag --tree` command to get a better overview of how your units interact in the Directed Acyclic Graph (DAG) of Terragrunt units.

Before:

```bash
terragrunt output-module-groups
{
  "Group 1": [
    "/absolute/path/to/vpc"
  ],
  "Group 2": [
    "/absolute/path/to/db"
  ],
  "Group 3": [
    "/absolute/path/to/ec2"
  ]
}
```

After:

```bash
terragrunt find --dag --json
[
  {
    "type": "unit",
    "path": "vpc"
  },
  {
    "type": "unit",
    "path": "db"
  },
  {
    "type": "unit",
    "path": "ec2"
  }
]
```

```bash
terragrunt list --dag --tree
.
╰── vpc
    ├── db
    │   ╰── ec2
    ╰── ec2
```

You might be wondering why these commands no longer reference "Groups". That's because the concurrency model of Terragrunt will change in a future release (see [#3629](https://github.com/gruntwork-io/terragrunt/issues/3629)), and at that point, Terragrunt will no longer run units in "Groups" of units, but instead run each unit when it is free to run. If you are currently relying on the `output-module-groups` to programmatically determine when units can run, you'll want to switch to using the `find --dag --json --dependencies` command to get a detailed breakdown of dependencies between units, and use that information to determine when units can run.

```bash
terragrunt find --dag --json --dependencies
[
  {
    "type": "unit",
    "path": "vpc"
  },
  {
    "type": "unit",
    "path": "db",
    "dependencies": [
      "vpc"
    ]
  },
  {
    "type": "unit",
    "path": "ec2",
    "dependencies": [
      "vpc",
      "db"
    ]
  }
]
```

### Use the newly renamed commands

Aside from the adjustments listed above, you'll also want to replace usage of deprecated commands with their newly renamed counterparts.

- `hclfmt` (use `hcl fmt` instead)
- `hclvalidate` (use `hcl validate` instead)
- `validate-inputs` (use `hcl validate --inputs` instead)
- `terragrunt-info` (use `info print` instead)
- `render-json` (use `render --json -w` instead)
- `graph-dependencies` (use `dag graph` instead)
- `run-all` (use `run --all` instead)
- `graph` (use `run --graph` instead)
