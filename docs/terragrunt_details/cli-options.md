---
title: CLI Options
layout: single
author_profile: true
sidebar:
  nav: "docs"
---

## CLI Options

Terragrunt forwards all arguments and options to Terraform. The only exceptions are `--version` and arguments that
start with the prefix `--terragrunt-`. The currently available options are:

* `--terragrunt-config`: A custom path to the `terraform.tfvars` file. May also be specified via the `TERRAGRUNT_CONFIG`
  environment variable. The default path is `terraform.tfvars` in the current directory (see
  [Configuration](#configuration) for a slightly more nuanced explanation). This argument is not
  used with the `apply-all`, `destroy-all`, `output-all`, `validate-all`, and `plan-all` commands.

* `--terragrunt-tfpath`: A custom path to the Terraform binary. May also be specified via the `TERRAGRUNT_TFPATH`
  environment variable. The default is `terraform` in a directory on your PATH.

* `--terragrunt-no-auto-init`: Don't automatically run `terraform init` when other commands are run (e.g. `terragrunt apply`).
  Useful if you want to pass custom arguments to `terraform init` that are specific to a user or execution environment,
  and therefore cannot be specified as `extra_arguments`.  For example, `-plugin-dir`.
  You must run `terragrunt init` yourself in this case if needed.
  `terragrunt` will fail if it detects that `init` is needed, but auto init is disabled.
  See [Auto-Init](#auto-init)

* `--terragrunt-non-interactive`: Don't show interactive user prompts. This will default the answer for all prompts to
  'yes'. Useful if you need to run Terragrunt in an automated setting (e.g. from a script).

* `--terragrunt-working-dir`: Set the directory where Terragrunt should execute the `terraform` command. Default is the
  current working directory. Note that for the `apply-all`, `destroy-all`, `output-all`, `validate-all`, and `plan-all`
  commands, this parameter has a different meaning: Terragrunt will apply or destroy all the Terraform modules in the
  subfolders of the `terragrunt-working-dir`, running `terraform` in the root of each module it finds.

* `--terragrunt-source`: Download Terraform configurations from the specified source into a temporary folder, and run
  Terraform in that temporary folder. May also be specified via the `TERRAGRUNT_SOURCE` environment variable. The
  source should use the same syntax as the [Terraform module source](https://www.terraform.io/docs/modules/sources.html)
  parameter. If you specify this argument for the `apply-all`, `destroy-all`, `output-all`, `validate-all`, or `plan-all`
  commands, Terragrunt will assume this is the local file path for all of your Terraform modules, and for each module
  processed by the `xxx-all` command, Terragrunt will automatically append the path of `source` parameter in each
  module to the `--terragrunt-source` parameter you passed in.

* `--terragrunt-source-update`: Delete the contents of the temporary folder before downloading Terraform source code
  into it.

* `--terragrunt-ignore-dependency-errors`: `*-all` commands continue processing components even if a dependency fails

* `--terragrunt-iam-role`: Assume the specified IAM role ARN before running Terraform or AWS commands. May also be
  specified via the `TERRAGRUNT_IAM_ROLE` environment variable. This is a convenient way to use Terragrunt and
  Terraform with multiple AWS accounts.
