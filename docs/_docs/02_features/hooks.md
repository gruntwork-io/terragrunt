---
layout: collection-browser-doc
title: Before, After, and Error Hooks
category: features
categories_url: features
excerpt: Learn how to execute custom code before or after running OpenTofu/Terraform, or when errors occur.
tags: ["hooks"]
order: 240
nav_title: Documentation
nav_title_link: /docs/
---

## Before and After Hooks

_Before Hooks_ or _After Hooks_ are a feature of terragrunt that make it possible to define custom actions that will be called either before or after execution of the `tofu`/`terraform` command.

Here’s an example:

``` hcl
terraform {
  before_hook "before_hook" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Running OpenTofu"]
  }

  after_hook "after_hook" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Finished running OpenTofu"]
    run_on_error = true
  }
}
```

In this example configuration, whenever Terragrunt runs `tofu apply` or `tofu plan` (or the `terraform` equivalent), two things will happen:

- Before Terragrunt runs `tofu`/`terraform`, it will output `Running OpenTofu` to the console.
- After Terragrunt runs `tofu`/`terraform`, it will output `Finished running OpenTofu`, regardless of whether or not the
  command failed.

Any type of hook adds extra environment variables to the hook's run command:

- `TG_CTX_TF_PATH`
- `TG_CTX_COMMAND`
- `TG_CTX_HOOK_NAME`

For example:

``` hcl
terraform {
  before_hook "test_hook" {
    commands     = ["apply"]
    execute      = ["hook.sh"]
  }
}
```

`hook.sh` contains:

``` bash
#!/bin/sh

echo "TF_PATH=${TG_CTX_TF_PATH} COMMAND=${TG_CTX_COMMAND} HOOK_NAME=${TG_CTX_HOOK_NAME}"
```

In this example, whenever Terragrunt runs `tofu apply`/`terraform apply`, the `hook.sh` script will print "TF_PATH=tofu COMMAND=apply HOOK_NAME=test_hook"

You can have multiple before and after hooks. Each hook will execute in the order they are defined. For example:

``` hcl
terraform {
  before_hook "before_hook_1" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Will run OpenTofu"]
  }

  before_hook "before_hook_2" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Running OpenTofu"]
  }
}
```

This configuration will cause Terragrunt to output `Will run OpenTofu` and then `Running OpenTofu` before the call
to OpenTofu/Terraform.

You can learn more about all the various configuration options supported in [the reference docs for the terraform
block](/docs/reference/config-blocks-and-attributes/#terraform).

### Tflint hook

_Before Hooks_ or _After Hooks_ support natively _tflint_, a linter for OpenTofu/Terraform code. It will validate the
OpenTofu/Terraform code used by Terragrunt, and it's inputs.

Here's an example:

```hcl
terraform {
  before_hook "before_hook" {
    commands     = ["apply", "plan"]
    execute      = ["tflint"]
  }
}
```

The `.tflint.hcl` should exist in the same folder as `terragrunt.hcl` or one of it's parents. If Terragrunt can't find
a `.tflint.hcl` file, it won't execute tflint and return an error. All configurations should be in a `config` block in this
file, as per [Tflint's docs](https://github.com/terraform-linters/tflint/blob/master/docs/user-guide/config.md).

```hcl
plugin "aws" {
    enabled = true
    version = "0.21.0"
    source  = "github.com/terraform-linters/tflint-ruleset-aws"
}

config {
  module = true
}
```

#### Configuration

By default, is executed internal `tflint` which evaluate passed parameters. Any desired extra configuration should be added in the `.tflint.hcl` file.
It will work with a `.tflint.hcl` file in the current folder or any parent folder.
To utilize an alternative configuration file, use the `--config` flag with the path to the configuration file.

If there is a need to run `tflint` from the operating system directly, should be use the extra parameter `--terragrunt-external-tflint`.
Example:

```hcl
terraform {
    before_hook "tflint" {
    commands = ["apply", "plan"]
    execute = ["tflint" , "--terragrunt-external-tflint", "--minimum-failure-severity=error", "--config", "custom.tflint.hcl"]
  }
}
```

#### Authentication for tflint rulesets

<!-- markdownlint-disable MD036 -->
_Public rulesets_

`tflint` works without any authentication for public rulesets (hosted on public repositories).

_Private rulesets_

If you want to run a the `tflint` hook with custom rulesets defined in a private repository, you will need to export locally a valid `GITHUB_TOKEN` token.

#### Troubleshooting

__`flag provided but not defined: -act-as-bundled-plugin` error__

If you have an `.tflint.hcl` file that is empty, or uses the `terraform` ruleset without version or source constraint, it returns the following error:

```log
Failed to initialize plugins; Unrecognized remote plugin message: Incorrect Usage. flag provided but not defined: -act-as-bundled-plugin
```

To fix this, make sure that the configuration for the `terraform` ruleset, in the `.tflint.hcl` file contains a version constraint:

```hcl
plugin "terraform" {
    enabled = true
    version = "0.2.1"
    source  = "github.com/terraform-linters/tflint-ruleset-terraform"
}
```

## Error Hooks

_Error hooks_ are a special type of after hook that act as exception handlers. They allow you to specify a list of expressions that can be used to catch errors and run custom commands when those errors occur. Error hooks are executed after the before/after hooks.

Here is an example:

``` hcl
terraform {
  error_hook "import_resource" {
    commands  = ["apply"]
    execute   = ["echo", "Error Hook executed"]
    on_errors = [
      ".*",
    ]
  }
}
```
