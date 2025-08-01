---
layout: collection-browser-doc
title: Strict Mode
category: reference
categories_url: reference
excerpt: >-
  Opt-in to strict mode to avoid deprecated features and ensure your code is future-proof.
tags: ["CLI"]
order: 405
nav_title: Documentation
nav_title_link: /docs/
slug: strict-mode
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

For example:

```bash
$ terragrunt run-all plan --strict-control deprecated-commands
15:26:23.685 ERROR  The `run-all plan` command is no longer supported. Use `terragrunt run --all plan` instead.
```

## Controlling Strict Mode

The simplest way to enable strict mode is to set the [strict-mode](/docs/reference/cli-options/#strict-mode) flag.

This will enable strict mode for all Terragrunt commands, for all strict mode controls.

```bash
$ terragrunt run-all plan
15:26:08.585 WARN   The `run-all plan` command is deprecated and will be removed in a future version. Use `terragrunt run --all plan` instead.
```

```bash
$ terragrunt --strict-mode run-all plan
15:26:23.685 ERROR  The `run-all plan` command is no longer supported. Use `terragrunt run --all plan` instead.
```

You can also use the environment variable, which can be more useful in CI/CD pipelines:

```bash
$ TG_STRICT_MODE='true' terragrunt run-all plan
15:26:23.685 ERROR  The `run-all plan` command is no longer supported. Use `terragrunt run --all plan` instead.
```

Instead of enabling strict mode like this, you can also enable specific strict controls by setting the [strict-control](/docs/reference/cli-options/#strict-control)
flag to a value to a particular strict control.

This can allow you to gradually increase your confidence in the future compatibility of your Terragrunt usage.

```bash
$ terragrunt run-all plan
15:26:08.585 WARN   The `run-all plan` command is deprecated and will be removed in a future version. Use `terragrunt run --all plan` instead.
```

```bash
$ terragrunt run-all plan --strict-control cli-redesign
15:26:23.685 ERROR  The `run-all plan` command is no longer supported. Use `terragrunt run --all plan` instead.
```

Again, you can also use the environment variable, which might be more useful in CI/CD pipelines:

```bash
$ TG_STRICT_CONTROL='cli-redesign' terragrunt run-all plan
15:26:23.685 ERROR  The `run-all plan` command is no longer supported. Use `terragrunt run --all plan` instead.
```

You can enable multiple strict controls at once:

```bash
$ terragrunt run-all plan --strict-control cli-redesign --strict-control default-command
15:26:23.685 ERROR  The `run-all plan` command is no longer supported. Use `terragrunt run --all plan` instead.
15:26:46.521 ERROR  Unable to determine underlying exit code, so Terragrunt will exit with error code 1
```

```bash
$ terragrunt run-all apply --strict-control cli-redesign --strict-control default-command
15:26:46.564 ERROR  The `run-all apply` command is no longer supported. Use `terragrunt run --all apply` instead.
15:26:46.564 ERROR  Unable to determine underlying exit code, so Terragrunt will exit with error code 1
```

You can also enable multiple strict controls at once when using the environment variable by using a comma delimited list.

```bash
$ TG_STRICT_CONTROL='cli-redesign,default-command' bash -c 'terragrunt run-all plan; terragrunt run-all apply'
15:26:46.521 ERROR  The `run-all plan` command is no longer supported. Use `terragrunt run --all plan` instead.
15:26:46.521 ERROR  Unable to determine underlying exit code, so Terragrunt will exit with error code 1
15:26:46.564 ERROR  The `run-all apply` command is no longer supported. Use `terragrunt run --all apply` instead.
15:26:46.564 ERROR  Unable to determine underlying exit code, so Terragrunt will exit with error code 1
```

You can also use [control categories](#control-categories) to enable certain categories of strict controls.

```bash
$ terragrunt run-all plan --strict-control deprecated-commands
15:26:23.685 ERROR  The `run-all plan` command is no longer supported. Use `terragrunt run --all plan` instead.
```

## Strict Mode Controls

The following strict mode controls are available:

- [Controlling Strict Mode](#controlling-strict-mode)
- [Strict Mode Controls](#strict-mode-controls)
  - [spin-up](#spin-up)
  - [tear-down](#tear-down)
  - [plan-all](#plan-all)
  - [apply-all](#apply-all)
  - [destroy-all](#destroy-all)
  - [output-all](#output-all)
  - [validate-all](#validate-all)
  - [skip-dependencies-inputs](#skip-dependencies-inputs)
  - [require-explicit-bootstrap](#require-explicit-bootstrap)
  - [root-terragrunt-hcl](#root-terragrunt-hcl)
  - [terragrunt-prefix-flags](#terragrunt-prefix-flags)
  - [terragrunt-prefix-env-vars](#terragrunt-prefix-env-vars)
  - [default-command](#default-command)
  - [cli-redesign](#cli-redesign)
  - [bare-include](#bare-include)
- [Control Categories](#control-categories)
  - [deprecated-commands](#deprecated-commands)
  - [deprecated-flags](#deprecated-flags)
  - [deprecated-env-vars](#deprecated-env-vars)
  - [deprecated-configs](#deprecated-configs)
  - [legacy-all](#legacy-all)

### skip-dependencies-inputs

Disable reading of dependency inputs to enhance dependency resolution performance by preventing recursively parsing Terragrunt inputs from dependencies.

Skipping dependency inputs is a performance optimization. For more details on performance optimizations, their tradeoffs, and other performance tips, read the dedicated [Performance documentation](/docs/troubleshooting/performance).

**Reason**: Enabling the `skip-dependencies-inputs` option prevents the recursive parsing of Terragrunt inputs, leading to optimized performance during dependency resolution.

### require-explicit-bootstrap

Require explicit usage of `--backend-bootstrap` to automatically bootstrap backend resources.

### root-terragrunt-hcl

Throw an error when users try to reference a root `terragrunt.hcl` file using `find_in_parent_folders`.

This control will also try to find other scenarios where users may be using `terragrunt.hcl` as the root configuration, including when using commands like `scaffold` and `catalog`, which can generate a `terragrunt.hcl` file expecting a `terragrunt.hcl` file at the root of the project. Enabling this flag adjusts the defaults for those commands so that they expect a recommended `root.hcl` file by default, and will throw an error if a `terragrunt.hcl` file is explicitly set.

**Reason**: Using a root `terragrunt.hcl` file was previously the recommended pattern to use with Terragrunt, but that is no longer the case. For more information see [Migrating from root `terragrunt.hcl`](/docs/migrate/migrating-from-root-terragrunt-hcl/).

### terragrunt-prefix-flags

Throw an error when using the `--terragrunt-` prefix for flags.

**Reason**: This is no longer necessary, due to the work in RFC [#3445](https://github.com/gruntwork-io/terragrunt/issues/3445).
**Example**: The `--terragrunt-non-interactive` flag is deprecated and will be removed in a future version. Use `--non-interactive` instead.

### terragrunt-prefix-env-vars

Throw an error when using the `TERRAGRUNT_` prefix for environment variables.

**Reason**: This prefix has been renamed to `TG_` to shorten the prefix, due to the work in RFC [#3445](https://github.com/gruntwork-io/terragrunt/issues/3445).
**Example**: The `TERRAGRUNT_LOG_LEVEL` env var is deprecated and will be removed in a future version. Use `TG_LOG_LEVEL=info` instead.

### default-command

Throw an error when using the Terragrunt default command.

**Reason**: Terragrunt now supports a special `run` command that can be used to explicitly forward commands to OpenTofu/Terraform when no shortcut exists in the Terragrunt CLI.
**Example**: The default command is deprecated and will be removed in a future version. Use `terragrunt run` instead.

### cli-redesign

Throw an error when using commands that were deprecated as part of the CLI redesign.

**Commands**:

- [run-all](/docs/reference/cli-options/#run-all)
- [graph](/docs/reference/cli-options/#graph)
- [graph-dependencies](/docs/reference/cli-options/#graph-dependencies)
- [hclfmt](/docs/reference/cli-options/#hclfmt)
- [hclvalidate](/docs/reference/cli-options/#hclvalidate)
- [output-module-groups](/docs/reference/cli-options/#output-module-groups)
- [render-json](/docs/reference/cli-options/#render-json)
- [terragrunt-info](/docs/reference/cli-options/#terragrunt-info)
- [validate-inputs](/docs/reference/cli-options/#validate-inputs)

**Reason**: These commands have been deprecated in favor of more consistent and intuitive commands as part of the CLI redesign. For more information, see the [CLI Redesign Migration Guide](/docs/migrate/cli-redesign/).

### bare-include

Throw an error when using a bare include.

**Reason**: Backwards compatibility for supporting bare includes results in a performance penalty for Terragrunt, and deprecating support provides a significant performance improvement. For more information, see the [Bare Include Migration Guide](/docs/migrate/bare-include/).

## Control Categories

Certain strict controls are grouped into categories to make it easier to enable multiple strict controls at once.

These categories change over time, so you might want to use the specific strict controls if you want to ensure that only certain controls are enabled.

### deprecated-commands

Throw an error when using the deprecated commands.

**Controls**:

- [default-command](#default-command)
- [cli-redesign](#cli-redesign)

### deprecated-flags

Throw an error when using the deprecated flags.

**Controls**:

- [terragrunt-prefix-flags](#terragrunt-prefix-flags)

### deprecated-env-vars

Throw an error when using the deprecated environment variables.

**Controls**:

- [terragrunt-prefix-env-vars](#terragrunt-prefix-env-vars)

### deprecated-configs

Throw an error when using the deprecated Terragrunt configuration.

**Controls**:

- [skip-dependencies-inputs](#skip-dependencies-inputs)

## Completed Controls

The following strict controls have been completed and are no longer needed:

- [legacy-all](#legacy-all)
- [spin-up](#spin-up)
- [tear-down](#tear-down)
- [plan-all](#plan-all)
- [apply-all](#apply-all)
- [destroy-all](#destroy-all)
- [output-all](#output-all)
- [validate-all](#validate-all)

### legacy-all

**Status**: Completed - The legacy `*-all` commands have been removed from Terragrunt.

This control was previously used to throw an error when using any of the legacy commands that were replaced by `run-all`. These commands have now been completely removed from Terragrunt as part of the deprecation schedule.

**Previously controlled commands** (now removed):

- `plan-all` - Use `terragrunt run --all plan` instead
- `apply-all` - Use `terragrunt run --all apply` instead
- `destroy-all` - Use `terragrunt run --all destroy` instead
- `output-all` - Use `terragrunt run --all output` instead
- `validate-all` - Use `terragrunt run --all validate` instead
- `spin-up` - Use `terragrunt run --all apply` instead
- `tear-down` - Use `terragrunt run --all destroy` instead

**Note**: The `run-all` command itself is still deprecated and will be removed in a future version. Use `terragrunt run --all` for the most future-proof syntax.

### spin-up

**Status**: Completed - This command has been completely removed from Terragrunt.

**Reason**: The `spin-up` command was deprecated and has now been removed as part of the deprecation schedule. Use `terragrunt run --all apply` instead.

### tear-down

**Status**: Completed - This command has been completely removed from Terragrunt.

**Reason**: The `tear-down` command was deprecated and has now been removed as part of the deprecation schedule. Use `terragrunt run --all destroy` instead.

### plan-all

**Status**: Completed - This command has been completely removed from Terragrunt.

**Reason**: The `plan-all` command was deprecated and has now been removed as part of the deprecation schedule. Use `terragrunt run --all plan` instead.

### apply-all

**Status**: Completed - This command has been completely removed from Terragrunt.

**Reason**: The `apply-all` command was deprecated and has now been removed as part of the deprecation schedule. Use `terragrunt run --all apply` instead.

### destroy-all

**Status**: Completed - This command has been completely removed from Terragrunt.

**Reason**: The `destroy-all` command was deprecated and has now been removed as part of the deprecation schedule. Use `terragrunt run --all destroy` instead.

### output-all

**Status**: Completed - This command has been completely removed from Terragrunt.

**Reason**: The `output-all` command was deprecated and has now been removed as part of the deprecation schedule. Use `terragrunt run --all output` instead.

### validate-all

**Status**: Completed - This command has been completely removed from Terragrunt.

**Reason**: The `validate-all` command was deprecated and has now been removed as part of the deprecation schedule. Use `terragrunt run --all validate` instead.
