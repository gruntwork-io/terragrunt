---
title: Strict Controls
description: Opt-in to strict controls to avoid deprecated features and ensure your code is future-proof.
slug: reference/strict-controls
sidebar:
  order: 3
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

The simplest way to enable strict mode is to set the [strict-mode](/reference/strict-controls) flag.

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

Instead of enabling strict mode like this, you can also enable specific strict controls by setting the [strict-control](/reference/strict-controls)
flag to a value that's specific to a particular strict control.
This can allow you to gradually increase your confidence in the future compatibility of your Terragrunt usage.

```bash
$ terragrunt run-all plan --strict-control cli-redesign
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

You can also enable multiple strict controls at once when using the environment variable by using a comma-delimited list.

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

## Control Categories

Certain strict controls are grouped into categories to make it easier to enable multiple strict controls at once.

These categories change over time, so you might want to use the specific strict controls if you want to ensure that only certain controls are enabled.

### deprecated-commands

Throw an error when using the deprecated commands.

**Controls**:

- [default-command](/reference/strict-controls/active#default-command)
- [cli-redesign](/reference/strict-controls/active#cli-redesign)

**Note**: The individual `*-all` commands (`plan-all`, `apply-all`, `destroy-all`, `output-all`, `validate-all`, `spin-up`, `tear-down`) have been removed from Terragrunt and are no longer available as strict controls. Use `terragrunt run --all` for the modern equivalent.

### deprecated-flags

Throw an error when using the deprecated flags.

**Controls**:

- [queue-exclude-external](/reference/strict-controls/active#queue-exclude-external)
- [no-destroy-dependencies-check](/reference/strict-controls/active#no-destroy-dependencies-check)
- [deprecated-hidden-flag](/reference/strict-controls/active#deprecated-hidden-flag)
- [queue-strict-include](/reference/strict-controls/active#queue-strict-include)
- [units-that-include](/reference/strict-controls/active#units-that-include)
- [disable-command-validation](/reference/strict-controls/active#disable-command-validation)
- [disable-dependent-modules](/reference/strict-controls/active#disable-dependent-modules)

### deprecated-env-vars

Throw an error when using the deprecated environment variables.

**Controls**:

- [terragrunt-prefix-env-vars](/reference/strict-controls/active#terragrunt-prefix-env-vars)
