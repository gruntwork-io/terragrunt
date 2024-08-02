---
layout: collection-browser-doc
title: Built-in functions
category: reference
categories_url: reference
excerpt: Terragrunt allows you to use built-in functions anywhere in `terragrunt.hcl`, just like OpenTofu/Terraform.
tags: ["functions"]
order: 402
nav_title: Documentation
nav_title_link: /docs/
---

Terragrunt allows you to use built-in functions anywhere in `terragrunt.hcl`, just like OpenTofu/Terraform\! The functions currently available are:

- [OpenTofu/Terraform built-in functions](#opentofuterraform-built-in-functions)
- [find\_in\_parent\_folders](#find_in_parent_folders)
- [path\_relative\_to\_include](#path_relative_to_include)
- [path\_relative\_from\_include](#path_relative_from_include)
- [get\_env](#get_env)
- [get\_platform](#get_platform)
- [get\_repo\_root](#get_repo_root)
- [get\_path\_from\_repo\_root](#get_path_from_repo_root)
- [get\_path\_to\_repo\_root](#get_path_to_repo_root)
- [get\_terragrunt\_dir](#get_terragrunt_dir)
- [get\_working\_dir](#get_working_dir)
- [get\_parent\_terragrunt\_dir](#get_parent_terragrunt_dir)
- [get\_original\_terragrunt\_dir](#get_original_terragrunt_dir)
- [get\_terraform\_commands\_that\_need\_vars](#get_terraform_commands_that_need_vars)
- [get\_terraform\_commands\_that\_need\_input](#get_terraform_commands_that_need_input)
- [get\_terraform\_commands\_that\_need\_locking](#get_terraform_commands_that_need_locking)
- [get\_terraform\_commands\_that\_need\_parallelism](#get_terraform_commands_that_need_parallelism)
- [get\_aws\_account\_id](#get_aws_account_id)
- [get\_aws\_caller\_identity\_arn](#get_aws_caller_identity_arn)
- [get\_terraform\_command](#get_terraform_command)
- [get\_terraform\_cli\_args](#get_terraform_cli_args)
- [get\_default\_retryable\_errors](#get_default_retryable_errors)
- [get\_aws\_caller\_identity\_user\_id](#get_aws_caller_identity_user_id)
- [run\_cmd](#run_cmd)
- [read\_terragrunt\_config](#read_terragrunt_config)
- [sops\_decrypt\_file](#sops_decrypt_file)
- [get\_terragrunt\_source\_cli\_flag](#get_terragrunt_source_cli_flag)
- [read\_tfvars\_file](#read_tfvars_file)

## OpenTofu/Terraform built-in functions

All [OpenTofu/Terraform built-in functions](https://opentofu.org/docs/language/functions/) are supported in Terragrunt config files:

```hcl
terraform {
  source = "../modules/${basename(get_terragrunt_dir())}"
}

remote_state {
  backend = "s3"
  config = {
    bucket = trimspace("   my-tofu-bucket     ")
    region = join("-", ["us", "east", "1"])
    key    = format("%s/tofu.tfstate", path_relative_to_include())
  }
}
```

Note: Any `file*` functions (`file`, `fileexists`, `filebase64`, etc) are relative to the directory containing the `terragrunt.hcl` file they’re used in.

Given the following structure:

```tree
└── terragrunt
  └── common.tfvars
  ├── assets
  |  └── mysql
  |     └── assets.txt
  └── terragrunt.hcl
```

Then `assets.txt` could be read with the following function call:

```hcl
file("assets/mysql/assets.txt")
```

## find_in_parent_folders

`find_in_parent_folders()` searches up the directory tree from the current `terragrunt.hcl` file and returns the absolute path to the first `terragrunt.hcl` in a parent folder or exit with an error if no such file is found. This is primarily useful in an `include` block to automatically find the path to a parent `terragrunt.hcl` file:

```hcl
include "root" {
  path = find_in_parent_folders()
}
```

The function takes an optional `name` parameter that allows you to specify a different path to search for:

```hcl
include "root" {
  path = find_in_parent_folders("some-other-file-name.hcl")
}
```

```hcl
include "root" {
  path = find_in_parent_folders("some-folder")
}
```

You can also pass an optional second `fallback` parameter which causes the function to return the fallback value (instead of exiting with an error) if the file in the `name` parameter cannot be found:

```hcl
include "root" {
  path = find_in_parent_folders("some-other-file-name.hcl", "fallback.hcl")
}
```

Note that this function searches relative to the child `terragrunt.hcl` file when called from a parent config. For
example, if you had the following folder structure:

```tree
    ├── terragrunt.hcl
    └── prod
        ├── env.hcl
        └── mysql
            └── terragrunt.hcl
```

And the root `terragrunt.hcl` contained the following:

```hcl
locals {
  env_vars = read_terragrunt_config(find_in_parent_folders("env.hcl"))
}
```

The `find_in_parent_folders` will search from the **child `terragrunt.hcl`** (`prod/mysql/terragrunt.hcl`) config,
finding the `env.hcl` file in the `prod` directory.

## path_relative_to_include

`path_relative_to_include()` returns the relative path between the current `terragrunt.hcl` file and the `path` specified in its `include` block. For example, consider the following folder structure:

```tree
├── terragrunt.hcl
├── prod
|   └── mysql
|       └── terragrunt.hcl
└── stage
    └── mysql
        └── terragrunt.hcl
```

Imagine `prod/mysql/terragrunt.hcl` and `stage/mysql/terragrunt.hcl` include all settings from the root `terragrunt.hcl` file:

```hcl
include "root" {
  path = find_in_parent_folders()
}
```

The root `terragrunt.hcl` can use the `path_relative_to_include()` in its `remote_state` configuration to ensure each child stores its remote state at a different `key`:

```hcl
remote_state {
  backend = "s3"
  config = {
    bucket = "my-tofu-bucket"
    region = "us-east-1"
    key    = "${path_relative_to_include()}/tofu.tfstate"
  }
}
```

The resulting `key` will be `prod/mysql/tofu.tfstate` for the prod `mysql` module and `stage/mysql/tofu.tfstate` for the stage `mysql` module.

If you have `include` blocks, this function requires a `name` parameter when used in the child config to specify which
`include` block to base the relative path on.

Example:

```hcl
include "root" {
  path = find_in_parent_folders()
}
include "region" {
  path = find_in_parent_folders("region.hcl")
}

terraform {
  source = "../modules/${path_relative_to_include("root")}"
}
```

## path_relative_from_include

`path_relative_from_include()` returns the relative path between the `path` specified in its `include` block and the current `terragrunt.hcl` file (it is the counterpart of `path_relative_to_include()`). For example, consider the following folder structure:

```tree
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
```

Imagine `terragrunt/mysql/terragrunt.hcl` and `terragrunt/secrets/mysql/terragrunt.hcl` include all settings from the root `terragrunt.hcl` file:

```hcl
include "root" {
  path = find_in_parent_folders()
}
```

The root `terragrunt.hcl` can use the `path_relative_from_include()` in combination with `path_relative_to_include()` in its `source` configuration to retrieve the relative OpenTofu/Terraform source code from the terragrunt configuration file:

```hcl
terraform {
  source = "${path_relative_from_include()}/../sources//${path_relative_to_include()}"
}
```

The resulting `source` will be `../../sources//mysql` for `mysql` module and `../../../sources//secrets/mysql` for `secrets/mysql` module.

Another use case would be to add extra argument to include the `common.tfvars` file for all subdirectories:

```hcl
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

If you have `include` blocks, this function requires a `name` parameter when used in the child config to specify which
`include` block to base the relative path on.

Example:

```hcl
include "root" {
  path = find_in_parent_folders()
}
include "region" {
  path = find_in_parent_folders("region.hcl")
}

terraform {
  source = "../modules/${path_relative_from_include("root")}"
}
```

## get_env

`get_env(NAME)` return the value of variable named `NAME` or throws exceptions if that variable is not set. Example:

```hcl
remote_state {
  backend = "s3"
  config = {
    bucket = get_env("BUCKET")
  }
}
```

`get_env(NAME, DEFAULT)` returns the value of the environment variable named `NAME` or `DEFAULT` if that environment variable is not set. Example:

```hcl
remote_state {
  backend = "s3"
  config = {
    bucket = get_env("BUCKET", "my-tofu-bucket")
  }
}
```

Note that [OpenTofu/Terraform will read environment variables](https://opentofu.org/docs/cli/config/environment-variables/#tf_var_name) that start with the prefix `TF_VAR_`, so one way to share a variable named `foo` between OpenTofu/Terraform and Terragrunt is to set its value as the environment variable `TF_VAR_foo` and to read that value in using this `get_env()` built-in function.

## get_platform

`get_platform()` returns the current Operating System. Example:

```hcl
inputs = {
  platform = get_platform()
}
```

This function can also be used in a comparison to evaluate what to do based on the current operating system. Example:

```hcl
output "platform" {
  value = var.platform == "darwin" ? "(value for MacOS)" : "(value for other OS's)"
}
```

Some of the returned values can be:

- `darwin`
- `freebsd`
- `linux`
- `windows`

## get_repo_root

`get_repo_root()` returns the absolute path to the root of the Git repository:

```hcl
inputs {
  very_important_config = "${get_repo_root()}/config/strawberries.conf"
}
```

This function will error if the file is not located in a Git repository.

## get_path_from_repo_root

`get_path_from_repo_root()` returns the path from the root of the Git repository to the current directory:

```hcl
remote_state {
  backend = "s3"

  config = {
    bucket         = "tofu"
    dynamodb_table = "tofu"
    encrypt        = true
    key            = "${get_path_from_repo_root()}/tofu.tfstate"
    session_name   = "tofu"
    region         = "us-east-1"
  }
}
```

This function will error if the file is not located in a Git repository.

## get_path_to_repo_root

`get_path_to_repo_root()` returns the relative path to the root of the Git repository:

```hcl
terraform {
  source = "${get_path_to_repo_root()}//modules/example"
}
```

This function will error if the file is not located in a Git repository.

## get_terragrunt_dir

`get_terragrunt_dir()` returns the directory where the Terragrunt configuration file (by default `terragrunt.hcl`) lives. This is useful when you need to use relative paths with [remote OpenTofu/Terraform configurations]({{site.baseurl}}/docs/features/keep-your-terraform-code-dry/#remote-opentofu-terraform-configurations) and you want those paths relative to your Terragrunt configuration file and not relative to the temporary directory where Terragrunt downloads the code.

For example, imagine you have the following file structure:

```tree
├── common.tfvars
├── frontend-app
│   └── terragrunt.hcl
```

Inside of `/tofu-code/frontend-app/terragrunt.hcl` you might try to write code that looks like this:

```hcl
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

Note how the `source` parameter is set, so Terragrunt will download the `frontend-app` code from the `modules` repo into a temporary folder and run `tofu`/`terraform` in that temporary folder. Note also that there is an `extra_arguments` block that is trying to allow the `frontend-app` to read some shared variables from a `common.tfvars` file. Unfortunately, the relative path (`../common.tfvars`) won’t work, as it will be relative to the temporary folder\! Moreover, you can’t use an absolute path, or the code won’t work on any of your teammates' computers.

To make the relative path work, you need to use `get_terragrunt_dir()` to combine the path with the folder where the `terragrunt.hcl` file lives:

```hcl
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

## get_working_dir

`get_working_dir()` returns the absolute path where Terragrunt runs OpenTofu/Terraform commands. This is useful when you need to manage substitutions of vars inside a \*.tfvars file located right inside terragrunt's tmp dir.

## get_parent_terragrunt_dir

`get_parent_terragrunt_dir()` returns the absolute directory where the Terragrunt parent configuration file (by default `terragrunt.hcl`) lives. This is useful when you need to use relative paths with [remote OpenTofu/Terraform configurations]({{site.baseurl}}/docs/features/keep-your-terraform-code-dry/#remote-opentofu-terraform-configurations) and you want those paths relative to your parent Terragrunt configuration file and not relative to the temporary directory where Terragrunt downloads the code.

This function is very similar to [get_terragrunt_dir()](#get_terragrunt_dir) except it returns the root instead of the leaf of your terragrunt configuration folder.

```tree
├── terragrunt.hcl
├── common.tfvars
├── app1
│   └── terragrunt.hcl
├── tests
│   ├── app2
│   |   └── terragrunt.hcl
│   └── app3
│       └── terragrunt.hcl
```

```hcl
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

The common.tfvars located in the root folder will be included by all applications, whatever their relative location to the root.

If you have `include` blocks, this function requires a `name` parameter when used in the child config to specify which
`include` block to base the parent dir on.

Example:

```hcl
include "root" {
  path = find_in_parent_folders()
}
include "region" {
  path = find_in_parent_folders("region.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir("root")}/modules/vpc"
}
```

## get_original_terragrunt_dir

`get_original_terragrunt_dir()` returns the directory where the original Terragrunt configuration file (by default
`terragrunt.hcl`) lives. This is primarily useful when one Terragrunt config is being read from another: e.g., if
`/tofu-code/terragrunt.hcl` calls `read_terragrunt_config("/foo/bar.hcl")`, and within `bar.hcl`, you call
`get_original_terragrunt_dir()`, you'll get back `/tofu-code`.

## get_terraform_commands_that_need_vars

`get_terraform_commands_that_need_vars()` returns the list of OpenTofu/Terraform commands that accept `-var` and `-var-file` parameters. This function is used when defining [extra_arguments]({{site.baseurl}}/docs/features/keep-your-cli-flags-dry/#multiple-extra_arguments-blocks).

```hcl
terraform {
  extra_arguments "common_var" {
    commands  = get_terraform_commands_that_need_vars()
    arguments = ["-var-file=${get_aws_account_id()}.tfvars"]
  }
}
```

## get_terraform_commands_that_need_input

`get_terraform_commands_that_need_input()` returns the list of OpenTofu/Terraform commands that accept the `-input=(true or false)` parameter. This function is used when defining [extra_arguments]({{site.baseurl}}/docs/features/keep-your-cli-flags-dry/#multiple-extra_arguments-blocks).

```hcl
terraform {
  # Force OpenTofu/Terraform to not ask for input value if some variables are undefined.
  extra_arguments "disable_input" {
    commands  = get_terraform_commands_that_need_input()
    arguments = ["-input=false"]
  }
}
```

## get_terraform_commands_that_need_locking

`get_terraform_commands_that_need_locking()` returns the list of terraform commands that accept the `-lock-timeout` parameter. This function is used when defining [extra_arguments]({{site.baseurl}}/docs/features/keep-your-cli-flags-dry/#multiple-extra_arguments-blocks).

```hcl
terraform {
  # Force OpenTofu/Terraform to keep trying to acquire a lock for up to 20 minutes if someone else already has the lock
  extra_arguments "retry_lock" {
    commands  = get_terraform_commands_that_need_locking()
    arguments = ["-lock-timeout=20m"]
  }
}
```

## get_terraform_commands_that_need_parallelism

`get_terraform_commands_that_need_parallelism()` returns the list of terraform commands that accept the `-parallelism` parameter. This function is used when defining [extra_arguments]({{site.baseurl}}/docs/features/keep-your-cli-flags-dry/#multiple-extra_arguments-blocks).

```hcl
terraform {
  # Force OpenTofu/Terraform to run with reduced parallelism
  extra_arguments "parallelism" {
    commands  = get_terraform_commands_that_need_parallelism()
    arguments = ["-parallelism=5"]
  }
}
```

## get_aws_account_id

`get_aws_account_id()` returns the AWS account id associated with the current set of credentials. Example:

```hcl
remote_state {
  backend = "s3"
  config = {
    bucket = "mycompany-${get_aws_account_id()}"
  }
}
```

**Note:** value returned by `get_aws_account_id()` can change during parsing of HCL code, for example after evaluation of `iam_role` attribute.

## get_aws_caller_identity_arn

`get_aws_caller_identity_arn()` returns the ARN of the AWS identity associated with the current set of credentials. Example:

```hcl
inputs = {
  caller_arn = get_aws_caller_identity_arn()
}
```

**Note:** value returned by `get_aws_caller_identity_arn()` can change during parsing of HCL code, for example after evaluation of `iam_role` attribute.

## get_terraform_command

`get_terraform_command()` returns the current terraform command in execution. Example:

```hcl
inputs = {
  current_command = get_terraform_command()
}
```

## get_terraform_cli_args

`get_terraform_cli_args()` returns cli args for the current terraform command in execution. Example:

```hcl
inputs = {
  current_cli_args = get_terraform_cli_args()
}
```

## get_default_retryable_errors

`get_default_retryable_errors()` returns default retryabled errors. Example:

```hcl
retryable_errors = concat(get_default_retryable_errors(), ["my custom error"])
```

## get_aws_caller_identity_user_id

`get_aws_caller_identity_user_id()` returns the UserId of the AWS identity associated with the current set of credentials. Example:

```hcl
inputs = {
  caller_user_id = get_aws_caller_identity_user_id()
}
```

This allows uniqueness of the storage bucket per AWS account (since bucket name must be globally unique).

It is also possible to configure variables specifically based on the account used:

```hcl
terraform {
  extra_arguments "common_var" {
    commands = get_terraform_commands_that_need_vars()
    arguments = ["-var-file=${get_aws_account_id()}.tfvars"]
  }
}
```

**Note:** value returned by `get_aws_caller_identity_user_id()` can change during parsing of HCL code, for example after evaluation of `iam_role` attribute.

## run_cmd

`run_cmd(command, arg1, arg2…​)` runs a shell command and returns the stdout as the result of the interpolation. The command is executed at the same folder as the `terragrunt.hcl` file. This is useful whenever you want to dynamically fill in arbitrary information in your Terragrunt configuration.

As an example, you could write a script that determines the bucket and DynamoDB table name based on the AWS account, instead of hardcoding the name of every account:

```hcl
remote_state {
  backend = "s3"
  config = {
    bucket         = run_cmd("./get_names.sh", "bucket")
    dynamodb_table = run_cmd("./get_names.sh", "dynamodb")
  }
}
```

If the command you are running has the potential to output sensitive values, you may wish to redact the output from appearing in the terminal. To do so, use the special `--terragrunt-quiet` argument which must be passed as one of the first arguments to `run_cmd()`:

```hcl
super_secret_value = run_cmd("--terragrunt-quiet", "./decrypt_secret.sh", "foo")
```

**Note:** This will prevent terragrunt from displaying the output from the command in its output. However, the value could still be displayed in the OpenTofu/Terraform output if OpenTofu/Terraform does not treat it as a [sensitive value](https://www.terraform.io/docs/configuration/outputs.html#sensitive-suppressing-values-in-cli-output).

Invocations of `run_cmd` are cached based on directory and executed command, so cached values are re-used later, rather than executed multiple times. Here's an example:

```hcl
locals {
  uuid = run_cmd("echo", "uuid1",  uuid())
  uuid2 = run_cmd("echo", "uuid2", uuid())
  uuid3 = run_cmd("echo", "uuid3", uuid())
  potato = run_cmd("echo", "potato")
  potato2 = run_cmd("echo", "potato")
  carrot = run_cmd("echo", "carrot")
}
inputs = {
  potato3 = run_cmd("echo", "potato")
  uuid3 = run_cmd("echo", "uuid3", uuid())
  uuid4 = run_cmd("echo", "uuid4", uuid())
  carrot2 = run_cmd("echo", "carrot")
}
```

Output:

```bash
$ terragrunt init
uuid1 b48379e1-924d-2403-8789-c72d50be964c
uuid1 9f3a8398-b11f-5314-7783-dad176ee487d
uuid1 649ac501-e5db-c935-1499-c59fb7a75625
uuid2 2d65972b-3fa9-181f-64fe-dcd574d944d0
uuid3 e345de60-9cfa-0455-79b7-af0d053a15a5
potato
uuid3 7f90a4ed-96e3-1dd8-5fee-91b8c8e07650
uuid2 8638fe79-c589-bebd-2a2a-3e6b96f7fc34
uuid3 310d0447-f0a6-3f67-efda-e6b1521fa1fb
uuid4 f8e80cc6-1892-8db7-bd63-6089fef00c01
uuid2 289ff371-8021-54c6-2254-72de9d11392a
uuid3 baa19863-1d99-e0ef-11f2-ede830d1c58a
carrot
```

**Notes:**

- Output contains only once `carrot` and `potato`, because other invocations got cached, caching works for all sections
- Output contains multiple times `uuid1` and `uuid2` because during HCL evaluation each `run_cmd` in `locals` is evaluated multiple times and random argument generated from `uuid()` save cached value under different key each time
- Output contains multiple times `uuid3`, +1 more output comparing to `uuid1` and `uuid2` - because `uuid3` is declared in locals and inputs which add one more evaluation
- Output contains only once `uuid4` since it is declared only once in `inputs`, `inputs` is not evaluated twice

You can modify this caching behavior to ignore the existing directory if you know the command you are running is not dependent on the current directory path. To do so, use the special `--terragrunt-global-cache` argument which must be passed as one of the first arguments to `run_cmd()` (and can be combined with `--terragrunt-quiet` in any order):

```hcl
value = run_cmd("--terragrunt-global-cache", "--terragrunt-quiet", "/usr/local/bin/get-account-map")
```

## read_terragrunt_config

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

## sops_decrypt_file

`sops_decrypt_file(file_path)` decrypts a yaml, json, ini, env or "raw text" file encrypted with `sops`.

[sops](https://github.com/getsops/sops) is an editor of encrypted files that supports YAML, JSON, ENV, INI and
BINARY formats and encrypts with AWS KMS, GCP KMS, Azure Key Vault, Hashicorp Vault and PGP.

This allows static secrets to be stored encrypted within your Terragrunt repository.

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

If you absolutely need to fallback to a default value you can make use of the OpenTofu/Terraform `try` function:

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

## get_terragrunt_source_cli_flag

`get_terragrunt_source_cli_flag()` returns the value passed in via the CLI `--terragrunt-source` or an environment variable `TERRAGRUNT_SOURCE`. Note that this will return an empty string when either of those values are not provided.

This is useful for constructing before and after hooks, or TF flags that only apply to local development (e.g., setting up debug flags, or adjusting the `iam_role` parameter).

Some example use cases are:

- Setting debug logging when doing local development.
- Adjusting the kubernetes provider configuration so that it targets minikube instead of real clusters.
- Providing special mocks pulled in from the local dev source (e.g., something like `mock_outputs = jsondecode(file("${get_terragrunt_source_cli_arg()}/dependency_mocks/vpc.json"))`).

## read_tfvars_file

`read_tfvars_file(file_path)` reads a `.tfvars` or `.tfvars.json` file and returns a map of the variables defined in it.

This is useful for reading variables from a `.tfvars` file and merging them into the inputs or to use them in a `locals` block:

```hcl

locals {
  inputs_from_tfvars = jsondecode(read_tfvars_file("common.tfvars"))
}

inputs = merge(
  local.inputs_from_tfvars,
  {
    # additional inputs
  }
)
```

Another example:

```hcl

locals {
  backend = jsondecode(read_tfvars_file("backend.tfvars"))
}

remote_state {
  backend = "s3"
  config = {
    bucket         = "${get_env("TG_BUCKET_PREFIX", "tf-bucket")}-${get_aws_account_id()}"
    key            = "${path_relative_to_include()}/terraform-${local.aws_region}.tfstate"
    region         = local.backend.region
  }
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
}
```
