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

- [terragrunt-config](#terragrunt-config)
- [terragrunt-tfpath](#terragrunt-tfpath)
- [terragrunt-no-auto-init](#terragrunt-no-auto-init)
- [terragrunt-no-auto-retry](#terragrunt-no-auto-retry)
- [terragrunt-non-interactive](#terragrunt-non-interactive)
- [terragrunt-working-dir](#terragrunt-working-dir)
- [terragrunt-download-dir](#terragrunt-download-dir)
- [terragrunt-source](#terragrunt-source)
- [terragrunt-source-update](#terragrunt-source-update)
- [terragrunt-ignore-dependency-errors](#terragrunt-ignore-dependency-errors)
- [terragrunt-iam-role](#terragrunt-iam-role)
- [terragrunt-exclude-dir](#terragrunt-exclude-dir)
- [terragrunt-include-dir](#terragrunt-include-dir)
- [terragrunt-ignore-dependency-order](#terragrunt-ignore-dependency-order)
- [terragrunt-ignore-external-dependencies](#terragrunt-ignore-external-dependencies)
- [terragrunt-include-external-dependencies](#terragrunt-include-external-dependencies)
- [terragrunt-check](#terragrunt-check)


### terragrunt-config

CLI Arg: `--terragrunt-config`

Requires an argument: `--terragrunt-config /path/to/terragrunt.hcl`

A custom path to the `terragrunt.hcl` file. May also be specified via the `TERRAGRUNT_CONFIG` environment variable. The
default path is `terragrunt.hcl` in the current directory (see
[Configuration]({{site.baseurl}}/docs/getting-started/configuration/#configuration) for a slightly more nuanced
explanation). This argument is not used with the `apply-all`, `destroy-all`, `output-all`, `validate-all`, and
`plan-all` commands.


### terragrunt-tfpath

CLI Arg: `--terragrunt-tfpath`

Requires an argument: `--terragrunt-tfpath /path/to/terraform-binary`

A custom path to the Terraform binary. May also be specified via the `TERRAGRUNT_TFPATH` environment variable. The
default is `terraform` in a directory on your PATH.


### terragrunt-no-auto-init

CLI Arg: `--terragrunt-no-auto-init`

When passed in, don't automatically run `terraform init` when other commands are run (e.g. `terragrunt apply`). Useful
if you want to pass custom arguments to `terraform init` that are specific to a user or execution environment, and
therefore cannot be specified as `extra_arguments`. For example, `-plugin-dir`. You must run `terragrunt init`
yourself in this case if needed. `terragrunt` will fail if it detects that `init` is needed, but auto init is
disabled. See [Auto-Init]({{site.baseurl}}/docs/features/auto-init#auto-init)


### terragrunt-no-auto-retry

CLI Arg: `--terragrunt-no-auto-retry`

When passed in, don't automatically retry commands which fail with transient errors. See
[Auto-Retry]({{site.baseurl}}/docs/features/auto-retry#auto-retry)


### terragrunt-non-interactive

CLI Arg: `--terragrunt-non-interactive`

When passed in, don't show interactive user prompts. This will default the answer for all prompts to 'yes'. Useful if
you need to run Terragrunt in an automated setting (e.g. from a script). May also be specified with the
[TF\_INPUT](https://www.terraform.io/docs/configuration/environment-variables.html#tf_input) environment variable.


### terragrunt-working-dir

CLI Arg: `--terragrunt-working-dir`

Requires an argument: `--terragrunt-working-dir /path/to/working-directory`

Set the directory where Terragrunt should execute the `terraform` command. Default is the current working directory.
Note that for the `apply-all`, `destroy-all`, `output-all`, `validate-all`, and `plan-all` commands, this parameter has
a different meaning: Terragrunt will apply or destroy all the Terraform modules in the subfolders of the
`terragrunt-working-dir`, running `terraform` in the root of each module it finds.


### terragrunt-download-dir

CLI Arg: `--terragrunt-download-dir`

Requires an argument: `--terragrunt-download-dir /path/to/dir-to-download-terraform-code

The path where to download Terraform code when using [remote Terraform
configurations](https://blog.gruntwork.io/terragrunt-how-to-keep-your-terraform-code-dry-and-maintainable-f61ae06959d8).
May also be specified via the `TERRAGRUNT_DOWNLOAD` environment variable. Default is `.terragrunt-cache` in the working
directory. We recommend adding this folder to your `.gitignore`.


### terragrunt-source

CLI Arg: `--terragrunt-source`

Requires an argument: `--terragrunt-source /path/to/local-terraform-code

Download Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary
folder. May also be specified via the `TERRAGRUNT_SOURCE` environment variable. The source should use the same syntax as
the [Terraform module source](https://www.terraform.io/docs/modules/sources.html) parameter. If you specify this
argument for the `apply-all`, `destroy-all`, `output-all`, `validate-all`, or `plan-all` commands, Terragrunt will
assume this is the local file path for all of your Terraform modules, and for each module processed by the `xxx-all`
command, Terragrunt will automatically append the path of `source` parameter in each module to the `--terragrunt-source`
parameter you passed in.


### terragrunt-source-update

CLI Arg: `--terragrunt-source-update`

When passed in, delete the contents of the temporary folder before downloading Terraform source code into it. Can also
be enabled by setting the `TERRAGRUNT_SOURCE_UPDATE` environment variable to `true`.


### terragrunt-ignore-dependency-errors

CLI Arg: `--terragrunt-ignore-dependency-errors`

When passed in, the `*-all` commands continue processing components even if a dependency fails


### terragrunt-iam-role

CLI Arg: `--terragrunt-iam-role`

Requires an argument: `--terragrunt-iam-role "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"`

Assume the specified IAM role ARN before running Terraform or AWS commands. May also be specified via the
`TERRAGRUNT_IAM_ROLE` environment variable. This is a convenient way to use Terragrunt and Terraform with multiple AWS
accounts.


### terragrunt-exclude-dir

CLI Arg: `--terragrunt-exclude-dir`

Requires an argument: `--terragrunt-exclude-dir /path/to/dirs/to/exclude*`

Can be supplied multiple times: `--terragrunt-exclude-dir /path/to/dirs/to/exclude --terragrunt-exclude-dir /another/path/to/dirs/to/exclude`

Unix-style glob of directories to exclude when running `*-all` commands. Modules under these directories will be
excluded during execution of the commands. If a relative path is specified, it should be relative from
[--terragrunt-working-dir](#terragrunt-working-dir). Flag can be specified multiple times. This will only exclude the
module, not its dependencies.


### terragrunt-include-dir

CLI Arg: `--terragrunt-include-dir`

Requires an argument: `--terragrunt-include-dir /path/to/dirs/to/include*`

Can be supplied multiple times: `--terragrunt-include-dir /path/to/dirs/to/include --terragrunt-include-dir /another/path/to/dirs/to/include`

Unix-style glob of directories to include when running `*-all` commands. Only modules under these directories (and all
dependent modules) will be included during execution of the commands. If a relative path is specified, it should be
relative from `--terragrunt-working-dir`. Flag can be specified multiple times.


### terragrunt-ignore-dependency-order

CLI Arg: `--terragrunt-ignore-dependency-order`

When passed in, ignore the depedencies between modules when running `*-all` commands.


### terragrunt-ignore-external-dependencies

CLI Arg: `--terragrunt-ignore-external-dependencies`

When passed in, don't attempt to include any external dependencies when running `*-all` commands


### terragrunt-include-external-dependencies

CLI Arg: `--terragrunt-include-external-dependencies`

When passed in, include any external dependencies when running `*-all` without asking.


### terragrunt-check

CLI Arg: `--terragrunt-check`

When passed in, run `hclfmt` in check only mode instead of actively overwriting the files. This will cause the
command to exit with exit code 1 if there are any files that are not formatted.
