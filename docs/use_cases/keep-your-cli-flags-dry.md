---
title: Keep your CLI flags DRY
layout: single
author_profile: true
sidebar:
  nav: "keep-your-cli-flags-dry"
---

## Motivation

Sometimes you may need to pass extra CLI arguments every time you run certain `terraform` commands. For example, you
may want to set the `lock-timeout` setting to 20 minutes for all commands that may modify remote state so that
Terraform will keep trying to acquire a lock for up to 20 minutes if someone else already has the lock rather than
immediately exiting with an error.

You can configure Terragrunt to pass specific CLI arguments for specific commands using an `extra_arguments` block
in your `terraform.tfvars` file:

```json
terragrunt = {
  terraform {
    # Force Terraform to keep trying to acquire a lock for
    # up to 20 minutes if someone else already has the lock
    extra_arguments "retry_lock" {
      commands = [
        "init",
        "apply",
        "refresh",
        "import",
        "plan",
        "taint",
        "untaint"
      ]

      arguments = [
        "-lock-timeout=20m"
      ]
    }
  }
}
```

Each `extra_arguments` block includes an arbitrary name (in the example above, `retry_lock`), a list of `commands` to
which the extra arguments should be add, a list of `arguments` or `required_var_files` or `optional_var_files` to add.
With the configuration above, when you run `terragrunt apply`, Terragrunt will call Terraform as follows:

When available, it is preferable to use interpolation functions such as
[get_terraform_commands_that_need_locking](#get_terraform_commands_that_need_locking) and
[get_terraform_commands_that_need_vars](#get_terraform_commands_that_need_vars)
since they provide the complete list of terraform commands that make use of the desired parameter:

```json
terragrunt = {
  terraform {
    # Force Terraform to keep trying to acquire a lock for up to 20 minutes if someone else already has the lock
    extra_arguments "retry_lock" {
      commands  = ["${get_terraform_commands_that_need_locking()}"]
      arguments = ["-lock-timeout=20m"]
    }
  }
}
```

```
> terragrunt apply

terraform apply -lock-timeout=20m
```


## Multiple extra_arguments blocks

You can specify one or more `extra_arguments` blocks. The `arguments` in each block will be applied any time you call
`terragrunt` with one of the commands in the `commands` list. If more than one `extra_arguments` block matches a
command, the arguments will be added in the order of of appearance in the configuration. For example, in addition to
lock settings, you may also want to pass custom `-var-file` arguments to several commands:

```json
terragrunt = {
  terraform {
    # Force Terraform to keep trying to acquire a lock for
    # up to 20 minutes if someone else already has the lock
    extra_arguments "retry_lock" {
      commands = [
        "init",
        "apply",
        "refresh",
        "import",
        "plan",
        "taint",
        "untaint"
      ]

      arguments = [
        "-lock-timeout=20m"
      ]
    }

    # Pass custom var files to Terraform
    extra_arguments "custom_vars" {
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]

      arguments = [
        "-var", "foo=bar",
        "-var", "region=us-west-1"
      ]
    }
  }
}
```

With the configuration above, when you run `terragrunt apply`, Terragrunt will call Terraform as follows:

```
> terragrunt apply

terraform apply -lock-timeout=20m -var foo=bar -var region=us-west-1
```

## `extra_arguments` for `init`

Extra arguments for the `init` command have some additional behavior and constraints.

In addition to being appended to the `terraform init` command that is run when you explicitly run `terragrunt init`,
`extra_arguments` for `init` will also be appended to the `init` commands that are automatically
run during other commands (see [Auto-Init](#auto-init)).

You must _not_ specify the `-from-module` option (aka. the `SOURCE` argument for terraform < 0.10.0) or the `DIR` argument
in the `extra_arguments` for `init`.  This option and argument will be provided automatically by terragrunt.

Here's an example of configuring `extra_arguments` for `init` in an environment in which terraform plugins are manually installed,
rather than relying on terraform to automatically download them.

```json
terragrunt = {
  terraform = {
    ...

    extra_arguments "init_args" {
      commands = [
        "init"
      ]

      arguments = [
        "-get-plugins=false",
        "-plugin-dir=/my/terraform/plugin/dir",
      ]
    }
  }
}
```


## Required and optional var-files

One common usage of extra_arguments is to include tfvars files. instead of using arguments, it is simpler to use either `required_var_files`
or `optional_var_files`. Both options require only to provide the list of file to include. The only difference is that `required_var_files`
will add the extra argument `-var-file=<your file>` for each file specified and if they don't exist, terraform will complain. Using
`optional_var_files` instead, terragrunt will only add the `-var-file=<your file>` for existing files. This allows many conditional
configurations based on environment variables as you can see in the following example:

```
/my/tf
├── terraform.tfvars
├── prod.tfvars
├── us-west-2.tfvars
├── backend-app
│   ├── main.tf
│   ├── dev.tfvars
│   └── terraform.tfvars
├── frontend-app
│   ├── main.tf
│   ├── us-east-1.tfvars
│   └── terraform.tfvars
```

```json
terragrunt = {
  terraform {
    extra_arguments "conditional_vars" {
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]

      required_var_files = [
        "${get_parent_tfvars_dir()}/terraform.tfvars"
      ]

      optional_var_files = [
        "${get_parent_tfvars_dir()}/${get_env("TF_VAR_env", "dev")}.tfvars",
        "${get_parent_tfvars_dir()}/${get_env("TF_VAR_region", "us-east-1")}.tfvars",
        "${get_tfvars_dir()}/${get_env("TF_VAR_env", "dev")}.tfvars",
        "${get_tfvars_dir()}/${get_env("TF_VAR_region", "us-east-1")}.tfvars"
      ]
    }
  }
```

See the [get_tfvars_dir()](#get_tfvars_dir) and [get_parent_tfvars_dir()](#get_parent_tfvars_dir) documentation for more details.

_Note that terragrunt cannot interpolate terraform variables (${var.xxx}) in the terragrunt configuration,
your variables have to be defined through TF_VAR_xxx environment variable to be referred by terragrunt._

With the configuration above, when you run `terragrunt apply-all`, Terragrunt will call Terraform as follows:

```
> terragrunt apply-all
[backend-app]  terraform apply -var-file=/my/tf/terraform.tfvars -var-file=/my/tf/backend-app/dev.tfvars
[frontend-app] terraform apply -var-file=/my/tf/terraform.tfvars -var-file=/my/tf/frontend-app/us-east-1.tfvars

> TF_VAR_env=prod terragrunt apply-all
[backend-app]  terraform apply -var-file=/my/tf/terraform.tfvars -var-file=/my/tf/prod.tfvars
[frontend-app] terraform apply -var-file=/my/tf/terraform.tfvars -var-file=/my/tf/prod.tfvars -var-file=/my/tf/frontend-app/us-east-1.tfvars

> TF_VAR_env=prod TF_VAR_region=us-west-2 terragrunt apply-all
[backend-app]  terraform apply -var-file=/my/tf/terraform.tfvars -var-file=/my/tf/prod.tfvars -var-file=/my/tf/us-west-2.tfvars
[frontend-app] terraform apply -var-file=/my/tf/terraform.tfvars -var-file=/my/tf/prod.tfvars -var-file=/my/tf/us-west-2.tfvars
```

## Handling whitespace

The list of arguments cannot include whitespaces, so if you need to pass command line arguments that include
spaces (e.g. `-var bucket=example.bucket.name`), then each of the arguments will need to be a separate item in the
`arguments` list:

```json
terragrunt = {
  terraform {
    extra_arguments "bucket" {
      arguments = [
        "-var", "bucket=example.bucket.name",
      ]
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]
    }
  }
}
```

With the configuration above, when you run `terragrunt apply`, Terragrunt will call Terraform as follows:

```
> terragrunt apply

terraform apply -var bucket=example.bucket.name
```
