---
layout: collection-browser-doc
title: CLI options
category: reference
excerpt: >-
  Terragrunt forwards all arguments and options to Terraform. Learn more about CLI options in Terragrunt.
tags: ["CLI"]
order: 401
nav_title: Documentation
nav_title_link: /docs/
---

## CLI Options

Terragrunt forwards all arguments and options to Terraform. The only exceptions are `--version`, `terragrunt-info` and arguments that start with the prefix `--terragrunt-`. The currently available options are:

  - `--terragrunt-config`: A custom path to the `terragrunt.hcl` file. May also be specified via the `TERRAGRUNT_CONFIG` environment variable. The default path is `terragrunt.hcl` in the current directory (see [Configuration]({{site.baseurl}}/docs/getting-started/configuration/#configuration) for a slightly more nuanced explanation). This argument is not used with the `apply-all`, `destroy-all`, `output-all`, `validate-all`, and `plan-all` commands.

  - `--terragrunt-tfpath`: A custom path to the Terraform binary. May also be specified via the `TERRAGRUNT_TFPATH` environment variable. The default is `terraform` in a directory on your PATH.

  - `--terragrunt-no-auto-init`: Don’t automatically run `terraform init` when other commands are run (e.g. `terragrunt apply`). Useful if you want to pass custom arguments to `terraform init` that are specific to a user or execution environment, and therefore cannot be specified as `extra_arguments`. For example, `-plugin-dir`. You must run `terragrunt init` yourself in this case if needed. `terragrunt` will fail if it detects that `init` is needed, but auto init is disabled. See [Auto-Init]({{site.baseurl}}/docs/features/auto-init#auto-init)

  - `--terragrunt-no-auto-retry`: Don’t automatically retry commands which fail with transient errors. See [Auto-Retry]({{site.baseurl}}/docs/features/auto-retry#auto-retry)

  - `--terragrunt-non-interactive`: Don’t show interactive user prompts. This will default the answer for all prompts to 'yes'. Useful if you need to run Terragrunt in an automated setting (e.g. from a script). May also be specified with the [TF\_INPUT](https://www.terraform.io/docs/configuration/environment-variables.html#tf_input) environment variable.

  - `--terragrunt-working-dir`: Set the directory where Terragrunt should execute the `terraform` command. Default is the current working directory. Note that for the `apply-all`, `destroy-all`, `output-all`, `validate-all`, and `plan-all` commands, this parameter has a different meaning: Terragrunt will apply or destroy all the Terraform modules in the subfolders of the `terragrunt-working-dir`, running `terraform` in the root of each module it finds.

  - `--terragrunt-download-dir`: The path where to download Terraform code when using [remote Terraform configurations](https://blog.gruntwork.io/terragrunt-how-to-keep-your-terraform-code-dry-and-maintainable-f61ae06959d8). May also be specified via the `TERRAGRUNT_DOWNLOAD` environment variable. Default is `.terragrunt-cache` in the working directory. We recommend adding this folder to your `.gitignore`.

  - `--terragrunt-source`: Download Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder. May also be specified via the `TERRAGRUNT_SOURCE` environment variable. The source should use the same syntax as the [Terraform module source](https://www.terraform.io/docs/modules/sources.html) parameter. If you specify this argument for the `apply-all`, `destroy-all`, `output-all`, `validate-all`, or `plan-all` commands, Terragrunt will assume this is the local file path for all of your Terraform modules, and for each module processed by the `xxx-all` command, Terragrunt will automatically append the path of `source` parameter in each module to the `--terragrunt-source` parameter you passed in.

  - `--terragrunt-source-update`: Delete the contents of the temporary folder before downloading Terraform source code into it. Can also be enabled by setting the `TERRAGRUNT_SOURCE_UPDATE` environment variable to `true`.

  - `--terragrunt-ignore-dependency-errors`: `*-all` commands continue processing components even if a dependency fails

  - `--terragrunt-iam-role`: Assume the specified IAM role ARN before running Terraform or AWS commands. May also be specified via the `TERRAGRUNT_IAM_ROLE` environment variable. This is a convenient way to use Terragrunt and Terraform with multiple AWS accounts.

  - `--terragrunt-exclude-dir`: Unix-style glob of directories to exclude when running `*-all` commands. Modules under these directories will be excluded during execution of the commands. If a relative path is specified, it should be relative from `--terragrunt-working-dir`. Flag can be specified multiple times. This will only exclude the module, not its dependencies.

  - `--terragrunt-include-dir`: Unix-style glob of directories to include when running `*-all` commands. Only modules under these directories (and all dependent modules) will be included during execution of the commands. If a relative path is specified, it should be relative from `--terragrunt-working-dir`. Flag can be specified multiple times.

  - `--terragrunt-ignore-dependency-order`: Ignore the depedencies between modules when running `*-all` commands.

  - `--terragrunt-ignore-external-dependencies`: Dont attempt to include any external dependencies when running `*-all` commands

  - `--terragrunt-include-external-dependencies`: Include any external dependencies when running `*-all` without asking.
