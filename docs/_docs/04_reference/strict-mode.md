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

## Controlling Strict Mode

The simplest way to enable strict mode is to set the `TERRAGRUNT_STRICT_MODE` environment variable to `true`.

This will enable strict mode for all Terragrunt commands, for all strict mode controls.

```bash
$ terragrunt plan-all
15:26:08.585 WARN   The `plan-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all plan` instead.
```

```bash
$ TERRAGRUNT_STRICT_MODE='true' terragrunt plan-all
15:26:23.685 ERROR  The `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead.
```

Instead of setting this environment variable, you can also enable strict mode for specific controls by setting the `TERRAGRUNT_STRICT_CONTROL`
environment variable to a value that's specific to a particular strict control.
This can allow you to gradually increase your confidence in the future compatibility of your Terragrunt usage.

```bash
$ TERRAGRUNT_STRICT_CONTROL='apply-all' terragrunt plan-all
15:26:08.585 WARN   The `plan-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all plan` instead.
```

```bash
$ TERRAGRUNT_STRICT_CONTROL='plan-all' terragrunt plan-all
15:26:23.685 ERROR  The `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead.
```

You can also enable multiple strict controls at once with a comma delimited list.

```bash
$ TERRAGRUNT_STRICT_CONTROL='plan-all,apply-all' bash -c 'terragrunt plan-all; terragrunt apply-all'
15:26:46.521 ERROR  The `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead.
15:26:46.521 ERROR  Unable to determine underlying exit code, so Terragrunt will exit with error code 1
15:26:46.564 ERROR  The `apply-all` command is no longer supported. Use `terragrunt run-all apply` instead.
15:26:46.564 ERROR  Unable to determine underlying exit code, so Terragrunt will exit with error code 1
```

## Strict Mode Controls

The following strict mode controls are available:

- [spin-up](#spin-up)
- [tear-down](#tear-down)
- [plan-all](#plan-all)
- [apply-all](#apply-all)
- [destroy-all](#destroy-all)
- [output-all](#output-all)
- [validate-all](#validate-all)

### spin-up

Throw an error when using the `spin-up` command.

**Reason**: The `spin-up` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead.

### tear-down

Throw an error when using the `tear-down` command.

**Reason**: The `tear-down` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead.

### plan-all

Throw an error when using the `plan-all` command.

**Reason**: The `plan-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all plan` instead.

### apply-all

Throw an error when using the `apply-all` command.

**Reason**: The `apply-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead.

### destroy-all

Throw an error when using the `destroy-all` command.

**Reason**: The `destroy-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead.

### output-all

Throw an error when using the `output-all` command.

**Reason**: The `output-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all output` instead.

### validate-all

Throw an error when using the `validate-all` command.

**Reason**: The `validate-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all validate` instead.
