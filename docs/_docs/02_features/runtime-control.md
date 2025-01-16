---
layout: collection-browser-doc
title: Feature Flags, Errors and Excludes
category: features
categories_url: features
excerpt: Learn how Terragrunt allows for runtime control using feature flags, error handling, and excludes.
tags: ["CLI"]
order: 250
nav_title: Documentation
nav_title_link: /docs/
redirect_from:
    - /docs/features/auto-retry/
---

Sometimes, you need to have Terragrunt behave differently at runtime due to specific context that you have in your environment.

The following configuration blocks have been designed to work together in concert to provide you a great deal of flexibility in how Terragrunt behaves at runtime:

- [Feature Flags](#feature-flags)
- [Errors](#errors)
- [Excludes](#excludes)

## Feature Flags

Defined using the [feature](/docs/reference/config-blocks-and-attributes/#feature) configuration block, Terragrunt allows for the control of specific features at runtime using feature flags.

For example:

```hcl
feature "s3_version" {
  default = "v1.0.0"
}

terraform {
  source = "git::git@github.com:acme/infrastructure-modules.git//storage/s3?ref=${feature.s3_version.value}"
}
```

The configuration above allows you to set a default version for the `s3_version` feature flag, controlling the tag used for fetching the `s3` module from the `infrastructure-modules` repository.

At runtime, you can override the default value by doing one of the following:

```bash
terragrunt apply --feature s3_version=v1.1.0
```

Or by setting the corresponding environment variable:

```bash
export TERRAGRUNT_FEATURE="s3_version=v1.1.0"
terragrunt apply
```

This can be a useful way to opt-in to new features or to test changes in your infrastructure.

Setting a different version of an OpenTofu/Terraform module in a lower environment can be useful for testing changes before rolling them out to production. Users will always use the default version unless they explicitly set a different value.

## Errors

Defined using the [errors](/docs/reference/config-blocks-and-attributes/#errors) configuration block, Terragrunt allows for fine-grained control of errors at runtime.

For example:

```hcl
errors {
    # Retry block for transient errors
    retry "transient_errors" {
        retryable_errors = [".*Error: transient network issue.*"]
        max_attempts = 3
        sleep_interval_sec = 5
    }

    # Ignore block for known safe-to-ignore errors
    ignore "known_safe_errors" {
        ignorable_errors = [
            ".*Error: safe warning.*",
            "!.*Error: do not ignore.*"
        ]
        message = "Ignoring safe warning errors"
        signals = {
            alert_team = false
        }
    }
}
```

This configuration allows for control over how Terragrunt hanldles errors at runtime.

In the example above, Terragrunt will retry up to three times with a five-second pause between each retry for any error that matches the regex `.*Error: transient network issue.*`.

It will also ignore any error that matches the regex `.*Error: safe warning.*`, but will not ignore any error that matches the regex `.*Error: do not ignore.*`.

When it ignores an error that it can safely ignore, it will output the message `Ignoring safe warning errors`, and will generate a file named `error-signals.json` in the working directory with the following content:

```json
{
    "alert_team": false
}
```

You can learn more about how this configuration block works in the documentation linked above, but for now, what's important to know is that it allows you to determine what Terragrunt should do when it encounters an error at runtime.

Note that these configurations can also be adjusted dynamically. You can use a combination of feature flags and errors to control how Terragrunt behaves at runtime.

Say, for example, a developer was trying to roll out a new version of your module that is known to be potentially flaky. You want to integrate your new module update with the rest of your team, but you don't want to break runs that aren't ready for the new module.

You could use a feature flag to control introduction of that new module, and an error block to ignore any errors that you know are safe to ignore.

```hcl
feature "enable_flaky_module" {
  default = false
}

locals {
  version = feature.enable_flaky_module.value ? "v1.0.0" : "v1.1.0"
}

terraform {
  source = "git::git@github.com:acme/infrastructure-modules.git//storage/s3?ref=${local.version}"
}

errors {
    # Ignore errors when set
    ignore "flaky_module_errors" {
        ignorable_errors = feature.enable_flaky_module.value ? [
            ".*Error: flaky module error.*"
        ] : []
        message = "Ignoring flaky module error"
        signals = {
            send_notification = true
        }
    }
}
```

In this example, the `enable_flaky_module` feature flag sets _both_ the version of the module, and the error handling behavior for the unit that consumes it. This would allow the developer to integrate the unit configuration update with the rest of the codebase, enable the flag that introduces the module update in a lower environment, and then ignore any errors that are known to be safe to ignore.

This pattern allows for greater speed of integration with larger codebases, and can be a useful tool for managing risk in your infrastructure.

## Excludes

Defined using the [exclude](/docs/reference/config-blocks-and-attributes/#exclude) configuration block, Terragrunt allows for the exclusion of specific units at runtime.

For example:

```hcl
locals {
  day_of_week = formatdate("EEE", timestamp())
  ban_deploy  = contains(["Fri", "Sat", "Sun"], local.day_of_week)
}

exclude {
    if = local.ban_deploy
    actions = ["apply", "destroy"]
}
```

In this example, the `exclude` block will prevent the `apply` command from running in a given unit on Fridays, Saturdays, and Sundays, as all good DevOps engineers know that deploying that close to a weekend is a recipe for disaster.

While a toy example, this demonstrates how you can use the `exclude` block to use dynamic information at runtime to control the [run queue](/docs/getting-started/terminology/#runner-queue).

You can use this block to prevent certain units from running in certain environments, or to prevent certain commands from running in certain units.

Note that, just like with the other blocks mentioned so far, you can use a combination of configurations mentioned here to ensure that Terragrunt behaves exactly as you need it to at runtime.

### Exclusion from the Run Queue

The `exclude` block will only exclude the unit from the run queue, which is only relevant in the context of a `run-all` command.

A user could still explicitly navigate to the unit directory and run the command manually.

If you would like to explicitly prevent a command from being run, even if a user was to navigate to the unit directory and run the command manually, you can use a combination of the `exclude` block and a `before_hook` block to prevent the command from running.

For example:

```hcl
locals {
  day_of_week = formatdate("EEE", timestamp())
  ban_deploy  = contains(["Fri", "Sat", "Sun"], local.day_of_week)
}

exclude {
    if = local.ban_deploy
    actions = ["apply", "destroy"]
}

terraform {
  before_hook "prevent_deploy" {
    commands = ["apply", "destroy"]
    execute  = local.ban_deploy ? ["bash", "-c", "echo 'Deploying on weekends is not allowed. Go home.' && exit 1"] : []
  }
}
```

Note that this will result in an exit code of 1, rather than merely excluding the unit from the run queue, which is slightly different behavior.
