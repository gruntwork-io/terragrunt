---
title: Interpolation syntax
layout: single
author_profile: true
sidebar:
  nav: "interpolation-syntax"
---

Terragrunt allows you to use [Terraform interpolation syntax](https://www.terraform.io/docs/configuration/interpolation.html)
(`${...}`) to call specific Terragrunt built-in functions. Note that Terragrunt built-in functions **only** work within a 
`terragrunt = { ... }` block. Terraform does NOT process interpolations in `.tfvars` files.


## find_in_parent_folders

`find_in_parent_folders()` searches up the directory tree from the current `.tfvars` file and returns the relative path
to to the first `terraform.tfvars` in a parent folder or exit with an error if no such file is found. This is
primarily useful in an `include` block to automatically find the path to a parent `.tfvars` file:

```json
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }
}
```

The function takes an optional `name` parameter that allows you to specify a different filename to search for:

```json
terragrunt = {
  include {
    path = "${find_in_parent_folders("some-other-file-name.tfvars")}"
  }
}
```

You can also pass an optional second `fallback` parameter which causes the function to return the fallback value 
(instead of exiting with an error) if the file in the `name` parameter cannot be found:

```json
terragrunt = {
  include {
    path = "${find_in_parent_folders("some-other-file-name.tfvars", "fallback.tfvars")}"
  }
}
```


## path_relative_to_include

`path_relative_to_include()` returns the relative path between the current `.tfvars` file and the `path` specified in
its `include` block. For example, consider the following folder structure:

```
├── terraform.tfvars
└── prod
    └── mysql
        └── terraform.tfvars
└── stage
    └── mysql
        └── terraform.tfvars
```

Imagine `prod/mysql/terraform.tfvars` and `stage/mysql/terraform.tfvars` include all settings from the root
`terraform.tfvars` file:

```json
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }
}
```

The root `terraform.tfvars` can use the `path_relative_to_include()` in its `remote_state` configuration to ensure
each child stores its remote state at a different `key`:

```json
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      bucket = "my-terraform-bucket"
      region = "us-east-1"
      key    = "${path_relative_to_include()}/terraform.tfstate"
    }
  }
}
```

The resulting `key` will be `prod/mysql/terraform.tfstate` for the prod `mysql` module and
`stage/mysql/terraform.tfstate` for the stage `mysql` module.


## path_relative_from_include

`path_relative_from_include()` returns the relative path between the `path` specified in its `include` block and the current
`.tfvars` file (it is the counterpart of `path_relative_to_include()`). For example, consider the following folder structure:

```
├── sources
|  ├── mysql
|  |  └── *.tf
|  └── secrets
|     └── mysql
|         └── *.tf
└── terragrunt
  └── common.tfvars
  ├── mysql
  |  └── terraform.tfvars
  ├── secrets
  |  └── mysql
  |     └── terraform.tfvars
  └── terraform.tfvars
```

Imagine `terragrunt/mysql/terraform.tfvars` and `terragrunt/secrets/mysql/terraform.tfvars` include all settings from the root
`terraform.tfvars` file:

```json
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }
}
```

The root `terraform.tfvars` can use the `path_relative_from_include()` in combination with `path_relative_to_include()` in its `source` configuration to retrieve the relative terraform source code from the terragrunt configuration file:

```json
terragrunt = {
  terraform {
    source = "${path_relative_from_include()}/../sources//${path_relative_to_include()}"
  }
  ...
}
```

The resulting `source` will be `../../sources//mysql` for `mysql` module and `../../../sources//secrets/mysql` for `secrets/mysql` module.

Another use case would be to add extra argument to include the common.tfvars file for all subdirectories:

```json
terragrunt = {
  terraform = {
    ...

    extra_arguments "common_var" {
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]

      arguments = [
        "-var-file=${get_tfvars_dir()}/${path_relative_from_include()}/common.tfvars",
      ]
    }
  }
}
```

This allows proper retrieval of the `common.tfvars` from whatever the level of subdirectories we have.


## get_env

`get_env(NAME, DEFAULT)` returns the value of the environment variable named `NAME` or `DEFAULT` if that environment
variable is not set. Example:

```json
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      bucket = "${get_env("BUCKET", "my-terraform-bucket")}"
    }
  }
}
```

Note that [Terraform will read environment
variables](https://www.terraform.io/docs/configuration/environment-variables.html#tf_var_name) that start with the
prefix `TF_VAR_`, so one way to share the a variable named `foo` between Terraform and Terragrunt is to set its value
as the environment variable `TF_VAR_foo` and to read that value in using this `get_env()` built-in function.


## get_tfvars_dir

`get_tfvars_dir()` returns the directory where the Terragrunt configuration file (by default, `terraform.tfvars`) lives.
This is useful when you need to use relative paths with [remote Terraform
configurations](#remote-terraform-configurations) and you want those paths relative to your Terragrunt configuration
file and not relative to the temporary directory where Terragrunt downloads the code.

For example, imagine you have the following file structure:

```
/terraform-code
├── common.tfvars
├── frontend-app
│   └── terraform.tfvars
```

Inside of `/terraform-code/frontend-app/terraform.tfvars` you might try to write code that looks like this:

```json
terragrunt = {
  terraform {
    source = "git::git@github.com:foo/modules.git//frontend-app?ref=v0.0.3"

    extra_arguments "custom_vars" {
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]

      arguments = [
        "-var-file=../common.tfvars", # Note: This relative path will NOT work correctly!
        "-var-file=terraform.tfvars"
      ]
    }
  }
}
```

Note how the `source` parameter is set, so Terragrunt will download the `frontend-app` code from the `modules` repo
into a temporary folder and run `terraform` in that temporary folder. Note also that there is an `extra_arguments`
block that is trying to allow the `frontend-app` to read some shared variables from a `common.tfvars` file.
Unfortunately, the relative path (`../common.tfvars`) won't work, as it will be relative to the temporary folder!
Moreover, you can't use an absolute path, or the code won't work on any of your teammates' computers.

To make the relative path work, you need to use `get_tfvars_dir()` to combine the path with the folder where
the `.tfvars` file lives:

```json
terragrunt = {
  terraform {
    source = "git::git@github.com:foo/modules.git//frontend-app?ref=v0.0.3"

    extra_arguments "custom_vars" {
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]

      # With the get_tfvars_dir() function, you can use relative paths!
      arguments = [
        "-var-file=${get_tfvars_dir()}/../common.tfvars",
        "-var-file=terraform.tfvars"
      ]
    }
  }
}
```

For the example above, this path will resolve to `/terraform-code/frontend-app/../common.tfvars`, which is exactly
what you want.


## get_parent_tfvars_dir

`get_parent_tfvars_dir()` returns the absolute directory where the Terragrunt parent configuration file (by default, `terraform.tfvars`) lives.
This is useful when you need to use relative paths with [remote Terraform configurations](#remote-terraform-configurations) and you want
those paths relative to your parent Terragrunt configuration file and not relative to the temporary directory where Terragrunt downloads
the code.

This function is very similar to [get_tfvars_dir()](#get_tfvars_dir) except it returns the root instead of the leaf of your terragrunt
configuration folder.

```
/terraform-code
├── terraform.tfvars
├── common.tfvars
├── app1
│   └── terraform.tfvars
├── tests
│   ├── app2
│   |   └── terraform.tfvars
│   └── app3
│       └── terraform.tfvars
```

```json
terragrunt = {
  terraform {
    extra_arguments "common_vars" {
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]

      arguments = [
        "-var-file=${get_parent_tfvars_dir()}/common.tfvars"
      ]
    }
  }
}
```

The common.tfvars located in the terraform root folder will be included by all applications, whatever their relative location to the root.

## get_terraform_commands_that_need_vars

`get_terraform_commands_that_need_vars()`

Returns the list of terraform commands that accept -var and -var-file parameters. This function is used when defining [extra_arguments](#keep-your-cli-flags-dry).

```
terragrunt = {
  terraform = {
    ...

    extra_arguments "common_var" {
      commands  = ["${get_terraform_commands_that_need_vars()}"]
      arguments = ["-var-file=${get_aws_account_id()}.tfvars"]
    }
  }
}
```

## get_terraform_commands_that_need_input

`get_terraform_commands_that_need_input()`

Returns the list of terraform commands that accept -input=(true or false) parameter. This function is used when defining [extra_arguments](#keep-your-cli-flags-dry).

```json
terragrunt = {
  terraform {
    # Force Terraform to not ask for input value if some variables are undefined.
    extra_arguments "disable_input" {
      commands  = ["${get_terraform_commands_that_need_input()}"]
      arguments = ["-input=false"]
    }
  }
}
```

## get_terraform_commands_that_need_locking

`get_terraform_commands_that_need_locking()`

Returns the list of terraform commands that accept -lock-timeout parameter. This function is used when defining [extra_arguments](#keep-your-cli-flags-dry).

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

_Note: Functions that return a list of values must be used in a single declaration like:_

```json
commands = ["${get_terraform_commands_that_need_vars()}"]

# which result in:
commands = ["apply", "console", "destroy", "import", "plan", "push", "refresh"]

# We do not recommend using them in string composition like:
commands = "Some text ${get_terraform_commands_that_need_locking()}"

# which result in something useless like:
commands = "Some text [apply destroy import init plan refresh taint untaint]"
```


## get_aws_account_id

`get_aws_account_id()` returns the AWS account id associated with the current set of credentials. Example:

```json
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      bucket = "mycompany-${get_aws_account_id()}"
    }
  }
}
```

This allows uniqueness of the storage bucket per AWS account (since bucket name must be globally unique).

It is also possible to configure variables specifically based on the account used:

```
terragrunt = {
  terraform = {
    ...

    extra_arguments "common_var" {
      commands = ["${get_terraform_commands_that_need_vars()}"]
      arguments = ["-var-file=${get_aws_account_id()}.tfvars"]
    }
  }
}
```
