---
layout: collection-browser-doc
title: Strict Mode
category: reference
categories_url: reference
excerpt: >-
  Opt-in to strict mode to avoid deprecated features and ensure your code is future-proof.
tags: ["CLI"]
order: 404
nav_title: Documentation
nav_title_link: /docs/
---

Terragrunt supports operating in a mode referred to as "Strict Mode".

Strict Mode is a set of controls that can be enabled to ensure that your Terragrunt usage is future-proof
by making deprecated features throw errors instead of warnings. This can be useful when you want to ensure
that your Terragrunt code is up-to-date with the latest conventions to avoid breaking changes in
future versions of Terragrunt.

Whenever possible, Terragrunt will initially provide you with a warning when you use a deprecated feature, without throwing an error.
However, in Strict Mode, these warnings will be converted to errors, which will cause the Terragrunt command to fail.

A good practice for using strict controls is to enable Strict Mode in your CI/CD pipelines for lower environments
to catch any deprecated features early on. This allows you to fix them before they become a problem
in production in a future Terragrunt release.

If you are unsure about the impact of enabling strict controls, you can enable them for specific controls to
gradually increase your confidence in the future compatibility of your Terragrunt usage.

## Controlling Strict Mode

The simplest way to enable strict mode is to set the [strict-mode](/docs/reference/cli-options/#strict-mode) flag.

This will enable strict mode for all Terragrunt commands, for all strict mode controls.

```bash
$ terragrunt plan-all
15:26:08.585 WARN   The `plan-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all plan` instead.
```

```bash
$ terragrunt --strict-mode plan-all
15:26:23.685 ERROR  The `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead.
```

You can also use the environment variable, which can be more useful in CI/CD pipelines:

```bash
$ TG_STRICT_MODE='true' terragrunt plan-all
15:26:23.685 ERROR  The `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead.
```

Instead of enabling strict mode like this, you can also enable specific strict controls by setting the [strict-control](/docs/reference/cli-options/#strict-control)
flag to a value that's specific to a particular strict control.
This can allow you to gradually increase your confidence in the future compatibility of your Terragrunt usage.

```bash
$ terragrunt plan-all --strict-control deprecated-commands
15:26:08.585 WARN   The `plan-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all plan` instead.
```

```bash
$ terragrunt plan-all --strict-control deprecated-commands
15:26:23.685 ERROR  The `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead.
```

Again, you can also use the environment variable, which might be more useful in CI/CD pipelines:

```bash
$ TG_STRICT_CONTROL='deprecated-commands' terragrunt plan-all
15:26:23.685 ERROR  The `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead.
```

You can enable multiple strict controls at once:

```bash
$ terragrunt plan-all --strict-control plan-all --strict-control deprecated-commands
15:26:23.685 ERROR  The `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead.
15:26:46.521 ERROR  Unable to determine underlying exit code, so Terragrunt will exit with error code 1
```

```bash
$ terragrunt apply-all --strict-control plan-all --strict-control deprecated-commands
15:26:46.564 ERROR  The `apply-all` command is no longer supported. Use `terragrunt run-all apply` instead.
15:26:46.564 ERROR  Unable to determine underlying exit code, so Terragrunt will exit with error code 1
```

You can also enable multiple strict controls at once when using the environment variable by using a comma delimited list.

```bash
$ TG_STRICT_CONTROL='deprecated-commands' bash -c 'terragrunt plan-all; terragrunt apply-all'
15:26:46.521 ERROR  The `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead.
15:26:46.521 ERROR  Unable to determine underlying exit code, so Terragrunt will exit with error code 1
15:26:46.564 ERROR  The `apply-all` command is no longer supported. Use `terragrunt run-all apply` instead.
15:26:46.564 ERROR  Unable to determine underlying exit code, so Terragrunt will exit with error code 1
```

## Strict Mode Controls

The following strict mode controls are available:

- [deprecated-flags](#deprecated-flags)
- [deprecated-env-vars](#deprecated-env-vars)
- [deprecated-commands](#deprecated-commands)
- [deprecated-default-command](#deprecated-default-command)
- [dependencies-inputs](#dependencies-inputs)
- [root-terragrunt-hcl](#root-terragrunt-hcl)

### deprecated-flags

Throw an error when using the deprecated flags.

**Example**: `--terragrunt-non-interactive` flag is deprecated and will be removed in a future version. Use `--non-interactive` instead.

### deprecated-env-vars

Throw an error when using the deprecated environment variables.

**Example**: `--TERRAGRUNT_LOG_LEVEL` env var is deprecated and will be removed in a future version. Use `--TG_LOG_LEVEL=info` instead.

### deprecated-commands

Throw an error when using the deprecated commandes.

**Example**: `plan-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all plan` instead.

### deprecated-default-command

Throw an error when using the deprecated default command.

**Example**: `plan` command is deprecated and will be removed in a future version. Use `terragrunt run -- plan` instead.

### dependencies-inputs

Throw an error when reading of dependency inputs.

**Reason**: Enabling the `dependencies-inputs` option prevents the recursive parsing of Terragrunt inputs, leading to optimized performance during dependency resolution.

### root-terragrunt-hcl

Throw an error when users try to reference a root `terragrunt.hcl` file using `find_in_parent_folders`.

This control will also try to find other scenarios where users may be using `terragrunt.hcl` as the root configuration, including when using commands like `scaffold` and `catalog`, which can generate a `terragrunt.hcl` file expecting a `terragrunt.hcl` file at the root of the project. Enabling this flag adjusts the defaults for those commands so that they expect a recommended `root.hcl` file by default, and will throw an error if a `terragrunt.hcl` file is explicitly set.

**Reason**: Using a root `terragrunt.hcl` file was previously the recommended pattern to use with Terragrunt, but that is no longer the case. For more information see [Migrating from root `terragrunt.hcl`](/docs/migrate/migrating-from-root-terragrunt-hcl/).
