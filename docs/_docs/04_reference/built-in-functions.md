---
layout: collection-browser-doc
title: Built-in functions
category: reference
categories_url: reference
excerpt: Terragrunt allows you to use built-in functions anywhere in `terragrunt.hcl`, just like Terraform.
tags: ["functions"]
order: 402
nav_title: Documentation
nav_title_link: /docs/
---

Terragrunt allows you to use built-in functions anywhere in `terragrunt.hcl`, just like Terraform\! The functions currently available are:

  - [All Terraform built-in functions](#terraform-built-in-functions)

  - [find\_in\_parent\_folders()](#find_in_parent_folders)

  - [path\_relative\_to\_include()](#path_relative_to_include)

  - [path\_relative\_from\_include()](#path_relative_from_include)

  - [get\_env(NAME, DEFAULT)](#get_env)

  - [get\_platform()](#get_platform)

  - [get\_terragrunt\_dir()](#get_terragrunt_dir)

  - [get\_parent\_terragrunt\_dir()](#get_parent_terragrunt_dir)

  - [get\_terraform\_commands\_that\_need\_vars()](#get_terraform_commands_that_need_vars)

  - [get\_terraform\_commands\_that\_need\_input()](#get_terraform_commands_that_need_input)

  - [get\_terraform\_commands\_that\_need\_locking()](#get_terraform_commands_that_need_locking)

  - [get\_terraform\_commands\_that\_need\_parallelism()](#get_terraform_commands_that_need_parallelism)

  - [get\_terraform\_command()](#get_terraform_command)

  - [get\_terraform\_cli\_args()](#get_terraform_cli_args)

  - [get\_aws\_account\_id()](#get_aws_account_id)

  - [get\_aws\_caller\_identity\_arn()](#get_aws_caller_identity_arn)

  - [get\_aws\_caller\_identity\_user\_id()](#get_aws_caller_identity_user_id)

  - [run\_cmd()](#run_cmd)

  - [read\_terragrunt\_config()](#read_terragrunt_config)

  - [sops\_decrypt\_file()](#sops_decrypt_file)

  - [get\_terragrunt\_source\_cli\_flag()](#get_terragrunt_source_cli_flag)

## Terraform built-in functions

All [Terraform built-in functions](https://www.terraform.io/docs/configuration/functions.html) are supported in Terragrunt config files:

``` hcl
terraform {
  source = "../modules/${basename(get_terragrunt_dir())}"
}

remote_state {
  backend = "s3"
  config = {
    bucket = trimspace("   my-terraform-bucket     ")
    region = join("-", ["us", "east", "1"])
    key    = format("%s/terraform.tfstate", path_relative_to_include())
  }
}
```

Note: Any `file*` functions (`file`, `fileexists`, `filebase64`, etc) are relative to the directory containing the `terragrunt.hcl` file they’re used in.

Given the following structure:

    └── terragrunt
      └── common.tfvars
      ├── assets
      |  └── mysql
      |     └── assets.txt
      └── terragrunt.hcl

Then `assets.txt` could be read with the following function call:

``` hcl
file("assets/mysql/assets.txt")
```

## find\_in\_parent\_folders

`find_in_parent_folders()` searches up the directory tree from the current `terragrunt.hcl` file and returns the absolute path to the first `terragrunt.hcl` in a parent folder or exit with an error if no such file is found. This is primarily useful in an `include` block to automatically find the path to a parent `terragrunt.hcl` file:

``` hcl
include {
  path = find_in_parent_folders()
}
```

The function takes an optional `name` parameter that allows you to specify a different filename to search for:

``` hcl
include {
  path = find_in_parent_folders("some-other-file-name.hcl")
}
```

You can also pass an optional second `fallback` parameter which causes the function to return the fallback value (instead of exiting with an error) if the file in the `name` parameter cannot be found:

``` hcl
include {
  path = find_in_parent_folders("some-other-file-name.hcl", "fallback.hcl")
}
```

Note that this function searches relative to the child `terragrunt.hcl` file when called from a parent config. For
example, if you had the following folder structure:

    ├── terragrunt.hcl
    └── prod
        ├── env.hcl
        └── mysql
            └── terragrunt.hcl

And the root `terragrunt.hcl` contained the following:

    locals {
      env_vars = read_terragrunt_config(find_in_parent_folders("env.hcl"))
    }

The `find_in_parent_folders` will search from the __child `terragrunt.hcl`__ (`prod/mysql/terragrunt.hcl`) config,
finding the `env.hcl` file in the `prod` directory.


## path\_relative\_to\_include

`path_relative_to_include()` returns the relative path between the current `terragrunt.hcl` file and the `path` specified in its `include` block. For example, consider the following folder structure:

    ├── terragrunt.hcl
    └── prod
        └── mysql
            └── terragrunt.hcl
    └── stage
        └── mysql
            └── terragrunt.hcl

Imagine `prod/mysql/terragrunt.hcl` and `stage/mysql/terragrunt.hcl` include all settings from the root `terragrunt.hcl` file:

``` hcl
include {
  path = find_in_parent_folders()
}
```

The root `terragrunt.hcl` can use the `path_relative_to_include()` in its `remote_state` configuration to ensure each child stores its remote state at a different `key`:

``` hcl
remote_state {
  backend = "s3"
  config = {
    bucket = "my-terraform-bucket"
    region = "us-east-1"
    key    = "${path_relative_to_include()}/terraform.tfstate"
  }
}
```

The resulting `key` will be `prod/mysql/terraform.tfstate` for the prod `mysql` module and `stage/mysql/terraform.tfstate` for the stage `mysql` module.

## path\_relative\_from\_include

`path_relative_from_include()` returns the relative path between the `path` specified in its `include` block and the current `terragrunt.hcl` file (it is the counterpart of `path_relative_to_include()`). For example, consider the following folder structure:

    ├── sources
    |  ├── mysql
    |  |  └── \*.tf
    |  └── secrets
    |     └── mysql
    |         └── \*.tf
    └── terragrunt
      └── common.tfvars
      ├── mysql
      |  └── terragrunt.hcl
      ├── secrets
      |  └── mysql
      |     └── terragrunt.hcl
      └── terragrunt.hcl

Imagine `terragrunt/mysql/terragrunt.hcl` and `terragrunt/secrets/mysql/terragrunt.hcl` include all settings from the root `terragrunt.hcl` file:

``` hcl
include {
  path = find_in_parent_folders()
}
```

The root `terragrunt.hcl` can use the `path_relative_from_include()` in combination with `path_relative_to_include()` in its `source` configuration to retrieve the relative terraform source code from the terragrunt configuration file:

``` hcl
terraform {
  source = "${path_relative_from_include()}/../sources//${path_relative_to_include()}"
}
```

The resulting `source` will be `../../sources//mysql` for `mysql` module and `../../../sources//secrets/mysql` for `secrets/mysql` module.

Another use case would be to add extra argument to include the `common.tfvars` file for all subdirectories:

``` hcl
  terraform {
    extra_arguments "common_var" {
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]

      arguments = [
        "-var-file=${get_terragrunt_dir()}/${path_relative_from_include()}/common.tfvars",
      ]
    }
  }
```

This allows proper retrieval of the `common.tfvars` from whatever the level of subdirectories we have.

## get\_env

`get_env(NAME)` return the value of variable named `NAME` or throws exceptions if that variable is not set. Example:

``` hcl
remote_state {
  backend = "s3"
  config = {
    bucket = get_env("BUCKET")
  }
}
```

`get_env(NAME, DEFAULT)` returns the value of the environment variable named `NAME` or `DEFAULT` if that environment variable is not set. Example:

``` hcl
remote_state {
  backend = "s3"
  config = {
    bucket = get_env("BUCKET", "my-terraform-bucket")
  }
}
```

Note that [Terraform will read environment variables](https://www.terraform.io/docs/configuration/environment-variables.html#tf_var_name) that start with the prefix `TF_VAR_`, so one way to share a variable named `foo` between Terraform and Terragrunt is to set its value as the environment variable `TF_VAR_foo` and to read that value in using this `get_env()` built-in function.

## get\_platform

`get_platform()` returns the current Operating System. Example:

``` hcl
inputs = {
  platform = get_platform()
}
```

This function can also be used in a comparison to evaluate what to do based on the current operating system. Example:
``` hcl
output "platform" {
  value = var.platform == "darwin" ? "(value for MacOS)" : "(value for other OS's)"
}
```

Some of the returned values can be:
```
darwin
freebsd
linux
windows
```

## get\_terragrunt\_dir

`get_terragrunt_dir()` returns the directory where the Terragrunt configuration file (by default `terragrunt.hcl`) lives. This is useful when you need to use relative paths with [remote Terraform configurations]({{site.baseurl}}/docs/features/keep-your-terraform-code-dry/#remote-terraform-configurations) and you want those paths relative to your Terragrunt configuration file and not relative to the temporary directory where Terragrunt downloads the code.

For example, imagine you have the following file structure:

    /terraform-code
    ├── common.tfvars
    ├── frontend-app
    │   └── terragrunt.hcl

Inside of `/terraform-code/frontend-app/terragrunt.hcl` you might try to write code that looks like this:

``` hcl
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
      "-var-file=../common.tfvars" # Note: This relative path will NOT work correctly!
    ]
  }
}
```

Note how the `source` parameter is set, so Terragrunt will download the `frontend-app` code from the `modules` repo into a temporary folder and run `terraform` in that temporary folder. Note also that there is an `extra_arguments` block that is trying to allow the `frontend-app` to read some shared variables from a `common.tfvars` file. Unfortunately, the relative path (`../common.tfvars`) won’t work, as it will be relative to the temporary folder\! Moreover, you can’t use an absolute path, or the code won’t work on any of your teammates' computers.

To make the relative path work, you need to use `get_terragrunt_dir()` to combine the path with the folder where the `terragrunt.hcl` file lives:

``` hcl
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

    # With the get_terragrunt_dir() function, you can use relative paths!
    arguments = [
      "-var-file=${get_terragrunt_dir()}/../common.tfvars"
    ]
  }
}
```

For the example above, this path will resolve to `/terraform-code/frontend-app/../common.tfvars`, which is exactly what you want.

## get\_parent\_terragrunt\_dir

`get_parent_terragrunt_dir()` returns the absolute directory where the Terragrunt parent configuration file (by default `terragrunt.hcl`) lives. This is useful when you need to use relative paths with [remote Terraform configurations]({{site.baseurl}}/docs/features/keep-your-terraform-code-dry/#remote-terraform-configurations) and you want those paths relative to your parent Terragrunt configuration file and not relative to the temporary directory where Terragrunt downloads the code.

This function is very similar to [get\_terragrunt\_dir()](#get_terragrunt_dir) except it returns the root instead of the leaf of your terragrunt configuration folder.

    /terraform-code
    ├── terragrunt.hcl
    ├── common.tfvars
    ├── app1
    │   └── terragrunt.hcl
    ├── tests
    │   ├── app2
    │   |   └── terragrunt.hcl
    │   └── app3
    │       └── terragrunt.hcl

``` hcl
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
      "-var-file=${get_parent_terragrunt_dir()}/common.tfvars"
    ]
  }
}
```

The common.tfvars located in the terraform root folder will be included by all applications, whatever their relative location to the root.

## get\_terraform\_commands\_that\_need\_vars

`get_terraform_commands_that_need_vars()` returns the list of terraform commands that accept `-var` and `-var-file` parameters. This function is used when defining [extra\_arguments]({{site.baseurl}}/docs/features/keep-your-cli-flags-dry/#multiple-extra_arguments-blocks).

``` hcl
terraform {
  extra_arguments "common_var" {
    commands  = get_terraform_commands_that_need_vars()
    arguments = ["-var-file=${get_aws_account_id()}.tfvars"]
  }
}
```

## get\_terraform\_commands\_that\_need\_input

`get_terraform_commands_that_need_input()` returns the list of terraform commands that accept the `-input=(true or false)` parameter. This function is used when defining [extra\_arguments]({{site.baseurl}}/docs/features/keep-your-cli-flags-dry/#multiple-extra_arguments-blocks).

``` hcl
terraform {
  # Force Terraform to not ask for input value if some variables are undefined.
  extra_arguments "disable_input" {
    commands  = get_terraform_commands_that_need_input()
    arguments = ["-input=false"]
  }
}
```

## get\_terraform\_commands\_that\_need\_locking

`get_terraform_commands_that_need_locking()` returns the list of terraform commands that accept the `-lock-timeout` parameter. This function is used when defining [extra\_arguments]({{site.baseurl}}/docs/features/keep-your-cli-flags-dry/#multiple-extra_arguments-blocks).

``` hcl
terraform {
  # Force Terraform to keep trying to acquire a lock for up to 20 minutes if someone else already has the lock
  extra_arguments "retry_lock" {
    commands  = get_terraform_commands_that_need_locking()
    arguments = ["-lock-timeout=20m"]
  }
}
```

## get\_terraform\_commands\_that\_need\_parallelism

`get_terraform_commands_that_need_parallelism()` returns the list of terraform commands that accept the `-parallelism` parameter. This function is used when defining [extra\_arguments]({{site.baseurl}}/docs/features/keep-your-cli-flags-dry/#multiple-extra_arguments-blocks).

``` hcl
terraform {
  # Force Terraform to run with reduced parallelism
  extra_arguments "parallelism" {
    commands  = get_terraform_commands_that_need_parallelism()
    arguments = ["-parallelism=5"]
  }
}
```

## get\_aws\_account\_id

`get_aws_account_id()` returns the AWS account id associated with the current set of credentials. Example:

``` hcl
remote_state {
  backend = "s3"
  config = {
    bucket = "mycompany-${get_aws_account_id()}"
  }
}
```

## get\_aws\_caller\_identity\_arn

`get_aws_caller_identity_arn()` returns the ARN of the AWS identity associated with the current set of credentials. Example:

``` hcl
inputs = {
  caller_arn = get_aws_caller_identity_arn()
}
```

## get\_terraform\_command

`get_terraform_command()` returns the current terraform command in execution. Example:

``` hcl
inputs = {
  current_command = get_terraform_command()
}
```

## get\_terraform\_cli\_args

`get_terraform_cli_args()` returns cli args for the current terraform command in execution. Example:

``` hcl
inputs = {
  current_cli_args = get_terraform_cli_args()
}
```

## get\_aws\_caller\_identity\_user\_id

`get_aws_caller_identity_user_id()` returns the UserId of the AWS identity associated with the current set of credentials. Example:

``` hcl
inputs = {
  caller_user_id = get_aws_caller_identity_user_id()
}
```

This allows uniqueness of the storage bucket per AWS account (since bucket name must be globally unique).

It is also possible to configure variables specifically based on the account used:

``` hcl
terraform {
  extra_arguments "common_var" {
    commands = get_terraform_commands_that_need_vars()
    arguments = ["-var-file=${get_aws_account_id()}.tfvars"]
  }
}
```

## run\_cmd

`run_cmd(command, arg1, arg2…​)` runs a shell command and returns the stdout as the result of the interpolation. The command is executed at the same folder as the `terragrunt.hcl` file. This is useful whenever you want to dynamically fill in arbitrary information in your Terragrunt configuration.

As an example, you could write a script that determines the bucket and DynamoDB table name based on the AWS account, instead of hardcoding the name of every account:

``` hcl
remote_state {
  backend = "s3"
  config = {
    bucket         = run_cmd("./get_names.sh", "bucket")
    dynamodb_table = run_cmd("./get_names.sh", "dynamodb")
  }
}
```

If the command you are running has the potential to output sensitive values, you may wish to redact the output from appearing in the terminal. To do so, use the special `--terragrunt-quiet` argument which must be passed as the first argument to `run_cmd()`:

``` hcl
super_secret_value = run_cmd("--terragrunt-quiet", "./decrypt_secret.sh", "foo")
```

**Note:** This will prevent terragrunt from displaying the output from the command in its output. However, the value could still be displayed in the Terraform output if Terraform does not treat it as a [sensitive value](https://www.terraform.io/docs/configuration/outputs.html#sensitive-suppressing-values-in-cli-output).


## read\_terragrunt\_config

`read_terragrunt_config(config_path, [default_val])` parses the terragrunt config at the given path and serializes the
result into a map that can be used to reference the values of the parsed config. This function will expose all blocks
and attributes of a terragrunt config.

For example, suppose you had a config file called `common.hcl` that contains common input variables:

```hcl
inputs = {
  stack_name = "staging"
  account_id = "1234567890"
}
```

You can read these inputs in another config by using `read_terragrunt_config`, and merge them into the inputs:

```hcl
locals {
  common_vars = read_terragrunt_config(find_in_parent_folders("common.hcl"))
}

inputs = merge(
  local.common_vars.inputs,
  {
    # additional inputs
  }
)
```

This function also takes in an optional second parameter which will be returned if the file does not exist:

```hcl
locals {
  common_vars = read_terragrunt_config(find_in_parent_folders("i-dont-exist.hcl", "i-dont-exist.hcl"), {inputs = {}})
}

inputs = merge(
  local.common_vars.inputs, # This will be {}
  {
    # additional inputs
  }
)
```

Note that this function will also render `dependency` blocks. That is, the parsed config will make the outputs of the
`dependency` blocks available. For example, suppose you had the following config in a file called `common_deps.hcl`:

```hcl
dependency "vpc" {
  config_path = "${get_terragrunt_dir()}/../vpc"
}
```

You can access the outputs of the vpc dependency through the parsed outputs of `read_terragrunt_config`:

```hcl
locals {
  common_deps = read_terragrunt_config(find_in_parent_folders("common_deps.hcl"))
}

inputs = {
  vpc_id = local.common_deps.dependency.vpc.outputs.vpc_id
}
```

## sops\_decrypt\_file

`sops_decrypt_file(file_path)` decrypts a yaml or json file encrypted with `sops`.

[sops](https://github.com/mozilla/sops) is an editor of encrypted files that supports YAML, JSON, ENV, INI and
BINARY formats and encrypts with AWS KMS, GCP KMS, Azure Key Vault and PGP.

This allows static secrets to be stored encrypted within your Terragrunt repository.

Only YAML and JSON formats are supported by `sops_decrypt_file`

For example, suppose you have some static secrets required to bootstrap your
infrastructure in `secrets.yaml`, you can decrypt and merge them into the inputs
by using `sops_decrypt_file`:

```hcl
locals {
  secret_vars = yamldecode(sops_decrypt_file(find_in_parent_folders("secrets.yaml")))
}

inputs = merge(
  local.secret_vars,
  {
    # additional inputs
  }
)
```

If you absolutely need to fallback to a default value you can make use of the Terraform `try` function:

```hcl
locals {
  secret_vars = try(jsondecode(sops_decrypt_file(find_in_parent_folders("no-secrets-here.json"))), {})
}

inputs = merge(
  local.secret_vars, # This will be {}
  {
    # additional inputs
  }
)
```

## get\_terragrunt\_source\_cli\_flag

`get_terragrunt_source_cli_flag()` returns the value passed in via the CLI `--terragrunt-source` or an environment variable `TERRAGRUNT_SOURCE`. Note that this will return an empty string when either of those values are not provided.

This is useful for constructing before and after hooks, or TF flags that only apply to local development (e.g., setting up debug flags, or adjusting the `iam_role` parameter).

Some example use cases are:

- Setting debug logging when doing local development.
- Adjusting the kubernetes provider configuration so that it targets minikube instead of real clusters.
- Providing special mocks pulled in from the local dev source (e.g., something like `mock_outputs = jsondecode(file("${get_terragrunt_source_cli_arg()}/dependency_mocks/vpc.json"))`).
