---
layout: collection-browser-doc
title: CLI options
category: reference
categories_url: reference
excerpt: >-
  Learn about all CLI arguments and options you can use with Terragrunt.
tags: ["CLI"]
order: 401
nav_title: Documentation
nav_title_link: /docs/
---

This page documents the CLI commands and options available with Terragrunt:

  - [CLI commands](#cli-commands)
  - [CLI options](#cli-options)




## CLI commands

Terragrunt supports the following CLI commands:

  - [All Terraform built-in commands](#all-terraform-built-in-commands)
  - [run-all](#run-all)
  - [plan-all (DEPRECATED: use run-all)](#plan-all-deprecated-use-run-all)
  - [apply-all (DEPRECATED: use run-all)](#apply-all-deprecated-use-run-all)
  - [output-all (DEPRECATED: use run-all)](#output-all-deprecated-use-run-all)
  - [destroy-all (DEPRECATED: use run-all)](#destroy-all-deprecated-use-run-all)
  - [validate-all (DEPRECATED: use run-all)](#validate-all-deprecated-use-run-all)
  - [terragrunt-info](#terragrunt-info)
  - [validate-inputs](#validate-inputs)
  - [graph-dependencies](#graph-dependencies)
  - [hclfmt](#hclfmt)
  - [aws-provider-patch](#aws-provider-patch)

### All Terraform built-in commands

Terragrunt is a thin wrapper for Terraform, so except for a few of the special commands defined in these docs,
Terragrunt forwards all other commands to Terraform. For example, when you run `terragrunt apply`, Terragrunt executes
`terraform apply`.

Examples:

```bash
terragrunt plan
terragrunt apply
terragrunt output
terragrunt destroy
# etc
```

Run `terraform --help` to get the full list.


### run-all

Runs the provided terraform command against a 'stack', where a 'stack' is a
tree of terragrunt modules. The command will recursively find terragrunt
modules in the current directory tree and run the terraform command in
dependency order (unless the command is destroy, in which case the command is
run in reverse dependency order).

Make sure to read [Execute Terraform
commands on multiple modules at once](/docs/features/execute-terraform-commands-on-multiple-modules-at-once/) for
context.

Example:

```bash
terragrunt run-all apply
```

This will recursively search the current working directory for any folders that contain Terragrunt modules and run
`apply` in each one, concurrently, while respecting ordering defined via
[`dependency`](/docs/reference/config-blocks-and-attributes/#dependency) and
[`dependencies`](/docs/reference/config-blocks-and-attributes/#dependencies) blocks.

**[WARNING] Using `run-all` with `plan` is currently broken for certain use cases**. If you have a stack of Terragrunt modules with
dependencies between them—either via `dependency` blocks or `terraform_remote_state` data sources—and you've never
deployed them, then `plan-all` will fail as it will not be possible to resolve the `dependency` blocks or
`terraform_remote_state` data sources! Please [see here for more
information](https://github.com/gruntwork-io/terragrunt/issues/720#issuecomment-497888756).




### plan-all (DEPRECATED: use run-all)

**DEPRECATED: Use `run-all plan` instead.**

Display the plans of a 'stack' by running 'terragrunt plan' in each subfolder. Make sure to read [Execute Terraform
commands on multiple modules at once](/docs/features/execute-terraform-commands-on-multiple-modules-at-once/) for
context.

Example:

```bash
terragrunt plan-all
```

This will recursively search the current working directory for any folders that contain Terragrunt modules and run
`plan` in each one, concurrently, while respecting ordering defined via
[`dependency`](/docs/reference/config-blocks-and-attributes/#dependency) and
[`dependencies`](/docs/reference/config-blocks-and-attributes/#dependencies) blocks.

**[WARNING] `plan-all` is currently broken for certain use cases**. If you have a stack of Terragrunt modules with
dependencies between them—either via `dependency` blocks or `terraform_remote_state` data sources—and you've never
deployed them, then `plan-all` will fail as it will not be possible to resolve the `dependency` blocks or
`terraform_remote_state` data sources! Please [see here for more
information](https://github.com/gruntwork-io/terragrunt/issues/720#issuecomment-497888756).


### apply-all (DEPRECATED: use run-all)

**DEPRECATED: Use `run-all apply` instead.**

Apply a 'stack' by running 'terragrunt apply' in each subfolder. Make sure to read [Execute Terraform
commands on multiple modules at once](/docs/features/execute-terraform-commands-on-multiple-modules-at-once/) for
context.

Example:

```bash
terragrunt apply-all
```

This will recursively search the current working directory for any folders that contain Terragrunt modules and run
`apply` in each one, concurrently, while respecting ordering defined via
[`dependency`](/docs/reference/config-blocks-and-attributes/#dependency) and
[`dependencies`](/docs/reference/config-blocks-and-attributes/#dependencies) blocks.

### output-all (DEPRECATED: use run-all)

**DEPRECATED: Use `run-all output` instead.**

Display the outputs of a 'stack' by running 'terragrunt output' in each subfolder. Make sure to read [Execute Terraform
commands on multiple modules at once](/docs/features/execute-terraform-commands-on-multiple-modules-at-once/) for
context.

Example:

```bash
terragrunt output-all
```

This will recursively search the current working directory for any folders that contain Terragrunt modules and run
`output` in each one, concurrently, while respecting ordering defined via
[`dependency`](/docs/reference/config-blocks-and-attributes/#dependency) and
[`dependencies`](/docs/reference/config-blocks-and-attributes/#dependencies) blocks.

**[WARNING] `output-all` is currently broken for certain use cases**. If you have a stack of Terragrunt modules with
dependencies between them—either via `dependency` blocks or `terraform_remote_state` data sources—and you've never
deployed them, then `output-all` will fail as it will not be possible to resolve the `dependency` blocks or
`terraform_remote_state` data sources! Please [see here for more
information](https://github.com/gruntwork-io/terragrunt/issues/720#issuecomment-497888756).

### destroy-all (DEPRECATED: use run-all)

**DEPRECATED: Use `run-all destroy` instead.**

Destroy a 'stack' by running 'terragrunt destroy' in each subfolder. Make sure to read [Execute Terraform
commands on multiple modules at once](/docs/features/execute-terraform-commands-on-multiple-modules-at-once/) for
context.

Example:

```bash
terragrunt destroy-all
```

This will recursively search the current working directory for any folders that contain Terragrunt modules and run
`destroy` in each one, concurrently, while respecting ordering defined via
[`dependency`](/docs/reference/config-blocks-and-attributes/#dependency) and
[`dependencies`](/docs/reference/config-blocks-and-attributes/#dependencies) blocks.

### validate-all (DEPRECATED: use run-all)

**DEPRECATED: Use `run-all validate` instead.**

Validate 'stack' by running 'terragrunt validate' in each subfolder. Make sure to read [Execute Terraform
commands on multiple modules at once](/docs/features/execute-terraform-commands-on-multiple-modules-at-once/) for
context.

Example:

```bash
terragrunt validate-all
```

This will recursively search the current working directory for any folders that contain Terragrunt modules and run
`validate` in each one, concurrently, while respecting ordering defined via
[`dependency`](/docs/reference/config-blocks-and-attributes/#dependency) and
[`dependencies`](/docs/reference/config-blocks-and-attributes/#dependencies) blocks.

### terragrunt-info

Emits limited terragrunt state on `stdout` in a JSON format and exits.

Example:

```bash
terragrunt terragrunt-info
```

Might produce output such as:

```json
{
  "ConfigPath": "/example/path/terragrunt.hcl",
  "DownloadDir": "/example/path/.terragrunt-cache",
  "IamRole": "",
  "TerraformBinary": "terraform",
  "TerraformCommand": "terragrunt-info",
  "WorkingDir": "/example/path"
}
```

### validate-inputs

Emits information about the input variables that are configured with the given
terragrunt configuration. Specifically, this command will print out unused
inputs (inputs that are not defined as a terraform variable in the
corresponding module) and undefined required inputs (required terraform
variables that are not currently being passed in).

Example:

```bash
> terragrunt validate-inputs
The following inputs passed in by terragrunt are unused:

    - foo
    - bar


The following required inputs are missing:

    - baz

```

Note that this only checks for variables passed in in the following ways:

- Configured `inputs` attribute.

- var files defined on `terraform.extra_arguments` blocks using `required_var_files` and `optional_var_files`.

- `-var-file` and `-var` CLI arguments defined on `terraform.extra_arguments` using `arguments`.

- `-var-file` and `-var` CLI arguments passed to terragrunt.

- Automatically loaded var files (`terraform.tfvars`, `terraform.tfvars.json`, `*.auto.tfvars`, `*.auto.tfvars.json`)

- `TF_VAR` environment variables defined on `terraform.extra_arguments` blocks.

- `TF_VAR` environment variables defined in the environment.

Be aware that other ways to pass variables to `terraform` are not checked by this command.

This command will exit with an error if terragrunt detects any unused inputs or undefined required inputs.


### graph-dependencies

Prints the terragrunt dependency graph, in DOT format, to `stdout`. You can generate charts from DOT format using tools
such as [GraphViz](http://www.graphviz.org/).

Example:

```bash
terragrunt graph-dependencies
```

This will recursively search the current working directory for any folders that contain Terragrunt modules and build
the dependency graph based on [`dependency`](/docs/reference/config-blocks-and-attributes/#dependency) and
[`dependencies`](/docs/reference/config-blocks-and-attributes/#dependencies) blocks. This may produce output such as:

```
digraph {
	"mgmt/bastion-host" ;
	"mgmt/bastion-host" -> "mgmt/vpc";
	"mgmt/bastion-host" -> "mgmt/kms-master-key";
	"mgmt/kms-master-key" ;
	"mgmt/vpc" ;
	"stage/backend-app" ;
	"stage/backend-app" -> "stage/vpc";
	"stage/backend-app" -> "mgmt/bastion-host";
	"stage/backend-app" -> "stage/mysql";
	"stage/backend-app" -> "stage/search-app";
	"stage/frontend-app" ;
	"stage/frontend-app" -> "stage/vpc";
	"stage/frontend-app" -> "mgmt/bastion-host";
	"stage/frontend-app" -> "stage/backend-app";
	"stage/mysql" ;
	"stage/mysql" -> "stage/vpc";
	"stage/redis" ;
	"stage/redis" -> "stage/vpc";
	"stage/search-app" ;
	"stage/search-app" -> "stage/vpc";
	"stage/search-app" -> "stage/redis";
	"stage/vpc" ;
	"stage/vpc" -> "mgmt/vpc";
}
```

### hclfmt

Recursively find `terragrunt.hcl` files and rewrite them into a canonical format.

Example:

```bash
terragrunt hclfmt
```

This will recursively search the current working directory for any folders that contain Terragrunt configuration files
(`terragrunt.hcl`) and run the equivalent of `terraform fmt` on them.


### aws-provider-patch

Overwrite settings on nested AWS providers to work around several Terraform bugs. Due to
[issue #13018](https://github.com/hashicorp/terraform/issues/13018) and
[issue #26211](https://github.com/hashicorp/terraform/issues/26211), the `import` command may fail if your Terraform
code uses a module that has a `provider` block nested within it that sets any of its attributes to computed values.
This command is a hacky attempt at working around this problem by allowing you to temporarily hard-code those
attributes so `import` can work.

You specify which attributes to hard-code using the [`--terragrunt-override-attr`](#terragrunt-override-attr) option,
passing it `ATTR=VALUE`, where `ATTR` is the attribute name and `VALUE` is the new value. Note that `ATTR` can specify
attributes within a nested block by specifying `<BLOCK>.<ATTR>`, where `<BLOCK>` is the block name. For example, let's
say you had a `provider` block in a module that looked like this:

```hcl
provider "aws" {
  region = var.aws_region
  assume_role {
    role_arn = var.role_arn
  }
}
```

Both the `region` and `role_arn` parameters are set to dynamic values, which will trigger those Terraform bugs. To work
around it, run the following command:

```bash
terragrunt aws-provider-patch \
  --terragrunt-override-attr region=eu-west-1 \
  --terragrunt-override-attr assume_role.role_arn=""
```

When you run the command above, Terragrunt will:

1. Run `terraform init` to download the code for all your modules into `.terraform/modules`.
1. Scan all the Terraform code in `.terraform/modules`, find AWS `provider` blocks, and for each one, hard-code:
    1. The `region` param to `"eu-west-1"`.
    1. The `role_arn` within the `assume_role` block to `""`.

The result will look like this:

```hcl
provider "aws" {
  region = "eu-west-1"
  assume_role {
    role_arn = ""
  }
}
```

This should allow you to run `import` on the module and work around those Terraform bugs. When you're done running
`import`, remember to delete your overridden code! E.g., Delete the `.terraform` or `.terragrunt-cache` folders.




## CLI options

Terragrunt forwards all options to Terraform. The only exceptions are `--version` and arguments that start with the
prefix `--terragrunt-` (e.g., `--terragrunt-config`). The currently available options are:

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
- [terragrunt-strict-include](#terragrunt-strict-include)
- [terragrunt-ignore-dependency-order](#terragrunt-ignore-dependency-order)
- [terragrunt-ignore-external-dependencies](#terragrunt-ignore-external-dependencies)
- [terragrunt-include-external-dependencies](#terragrunt-include-external-dependencies)
- [terragrunt-parallelism](#terragrunt-parallelism)
- [terragrunt-debug](#terragrunt-debug)
- [terragrunt-check](#terragrunt-check)
- [terragrunt-hclfmt-file](#terragrunt-hclfmt-file)
- [terragrunt-override-attr](#terragrunt-override-attr)


### terragrunt-config

**CLI Arg**: `--terragrunt-config`<br/>
**Environment Variable**: `TERRAGRUNT_CONFIG`<br/>
**Requires an argument**: `--terragrunt-config /path/to/terragrunt.hcl`

A custom path to the `terragrunt.hcl` or `terragrunt.hcl.json` file. The
default path is `terragrunt.hcl` (preferred) or `terragrunt.hcl.json` in the current directory (see
[Configuration]({{site.baseurl}}/docs/getting-started/configuration/#configuration) for a slightly more nuanced
explanation). This argument is not used with the `apply-all`, `destroy-all`, `output-all`, `validate-all`, and
`plan-all` commands.


### terragrunt-tfpath

**CLI Arg**: `--terragrunt-tfpath`<br/>
**Environment Variable**: `TERRAGRUNT_TFPATH`<br/>
**Requires an argument**: `--terragrunt-tfpath /path/to/terraform-binary`

A custom path to the Terraform binary. The default is `terraform` in a directory on your PATH.


### terragrunt-no-auto-init

**CLI Arg**: `--terragrunt-no-auto-init`<br/>
**Environment Variable**: `TERRAGRUNT_AUTO_INIT` (set to `false`)

When passed in, don't automatically run `terraform init` when other commands are run (e.g. `terragrunt apply`). Useful
if you want to pass custom arguments to `terraform init` that are specific to a user or execution environment, and
therefore cannot be specified as `extra_arguments`. For example, `-plugin-dir`. You must run `terragrunt init`
yourself in this case if needed. `terragrunt` will fail if it detects that `init` is needed, but auto init is
disabled. See [Auto-Init]({{site.baseurl}}/docs/features/auto-init#auto-init)


### terragrunt-no-auto-retry

**CLI Arg**: `--terragrunt-no-auto-retry`<br/>
**Environment Variable**: `TERRAGRUNT_AUTO_RETRY` (set to `false`)

When passed in, don't automatically retry commands which fail with transient errors. See
[Auto-Retry]({{site.baseurl}}/docs/features/auto-retry#auto-retry)


### terragrunt-non-interactive

**CLI Arg**: `--terragrunt-non-interactive`<br/>
**Environment Variable**: `TF_INPUT` (set to `false`)

When passed in, don't show interactive user prompts. This will default the answer for all prompts to `yes` except for
the listed cases below. This is useful if you need to run Terragrunt in an automated setting (e.g. from a script). May
also be specified with the [TF\_INPUT](https://www.terraform.io/docs/configuration/environment-variables.html#tf_input)
environment variable.

This setting will default to `no` for the following cases:

- Prompts related to pulling in external dependencies. You can force include external dependencies using the
  [--terragrunt-include-external-dependencies](#terragrunt-include-external-dependencies) option.


### terragrunt-working-dir

**CLI Arg**: `--terragrunt-working-dir`<br/>
**Environment Variable**: `TERRAGRUNT_WORKING_DIR`<br/>
**Requires an argument**: `--terragrunt-working-dir /path/to/working-directory`

Set the directory where Terragrunt should execute the `terraform` command. Default is the current working directory.
Note that for the `apply-all`, `destroy-all`, `output-all`, `validate-all`, and `plan-all` commands, this parameter has
a different meaning: Terragrunt will apply or destroy all the Terraform modules in the subfolders of the
`terragrunt-working-dir`, running `terraform` in the root of each module it finds.


### terragrunt-download-dir

**CLI Arg**: `--terragrunt-download-dir`<br/>
**Environment Variable**: `TERRAGRUNT_DOWNLOAD`<br/>
**Requires an argument**: `--terragrunt-download-dir /path/to/dir-to-download-terraform-code`

The path where to download Terraform code when using [remote Terraform
configurations](https://blog.gruntwork.io/terragrunt-how-to-keep-your-terraform-code-dry-and-maintainable-f61ae06959d8).
Default is `.terragrunt-cache` in the working directory. We recommend adding this folder to your `.gitignore`.


### terragrunt-source

**CLI Arg**: `--terragrunt-source`<br/>
**Environment Variable**: `TERRAGRUNT_SOURCE`<br/>
**Requires an argument**: `--terragrunt-source /path/to/local-terraform-code`

Download Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary
folder. The source should use the same syntax as the [Terraform module
source](https://www.terraform.io/docs/modules/sources.html) parameter. If you specify this argument for the `apply-all`,
`destroy-all`, `output-all`, `validate-all`, or `plan-all` commands, Terragrunt will assume this is the local file path
for all of your Terraform modules, and for each module processed by the `xxx-all` command, Terragrunt will automatically
append the path of `source` parameter in each module to the `--terragrunt-source` parameter you passed in.


### terragrunt-source-update

**CLI Arg**: `--terragrunt-source-update`<br/>
**Environment Variable**: `TERRAGRUNT_SOURCE_UPDATE` (set to `true`)

When passed in, delete the contents of the temporary folder before downloading Terraform source code into it.


### terragrunt-ignore-dependency-errors

**CLI Arg**: `--terragrunt-ignore-dependency-errors`

When passed in, the `*-all` commands continue processing components even if a dependency fails


### terragrunt-iam-role

**CLI Arg**: `--terragrunt-iam-role`<br/>
**Environment Variable**: `TERRAGRUNT_IAM_ROLE`<br/>
**Requires an argument**: `--terragrunt-iam-role "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"`

Assume the specified IAM role ARN before running Terraform or AWS commands. This is a convenient way to use Terragrunt
and Terraform with multiple AWS accounts.


### terragrunt-exclude-dir

**CLI Arg**: `--terragrunt-exclude-dir`<br/>
**Requires an argument**: `--terragrunt-exclude-dir /path/to/dirs/to/exclude*`

Can be supplied multiple times: `--terragrunt-exclude-dir /path/to/dirs/to/exclude --terragrunt-exclude-dir /another/path/to/dirs/to/exclude`

Unix-style glob of directories to exclude when running `*-all` commands. Modules under these directories will be
excluded during execution of the commands. If a relative path is specified, it should be relative from
[--terragrunt-working-dir](#terragrunt-working-dir). Flag can be specified multiple times. This will only exclude the
module, not its dependencies.


### terragrunt-include-dir

**CLI Arg**: `--terragrunt-include-dir`<br/>
**Requires an argument**: `--terragrunt-include-dir /path/to/dirs/to/include*`

Can be supplied multiple times: `--terragrunt-include-dir /path/to/dirs/to/include --terragrunt-include-dir /another/path/to/dirs/to/include`

Unix-style glob of directories to include when running `*-all` commands. Only modules under these directories (and all
dependent modules) will be included during execution of the commands. If a relative path is specified, it should be
relative from `--terragrunt-working-dir`. Flag can be specified multiple times.


### terragrunt-strict-include

**CLI Arg**: `--terragrunt-strict-include`

When passed in, only modules under the directories passed in with [--terragrunt-include-dir](#terragrunt-include-dir)
will be included. All dependencies of the included directories will be excluded if they are not in the included
directories.


### terragrunt-ignore-dependency-order

**CLI Arg**: `--terragrunt-ignore-dependency-order`

When passed in, ignore the depedencies between modules when running `*-all` commands.


### terragrunt-ignore-external-dependencies

**CLI Arg**: `--terragrunt-ignore-external-dependencies`

When passed in, don't attempt to include any external dependencies when running `*-all` commands. Note that an external
dependency is a dependency that is outside the current terragrunt working directory, and is not respective to the
included directories with `terragrunt-include-dir`.


### terragrunt-include-external-dependencies

**CLI Arg**: `--terragrunt-include-external-dependencies`

When passed in, include any external dependencies when running `*-all` without asking. Note that an external
dependency is a dependency that is outside the current terragrunt working directory, and is not respective to the
included directories with `terragrunt-include-dir`.


### terragrunt-parallelism

**CLI Arg**: `--terragrunt-parallelism`<br/>
**Environment Variable**: `TERRAGRUNT_PARALLELISM`

When passed in, limit the number of modules that are run concurrently to this number during *-all commands.



### terragrunt-debug

**CLI Arg**: `--terragrunt-debug`

When passed in, Terragrunt will create a tfvars file that can be used to invoke the terraform module in the same way
that Terragrunt invokes the module, so that you can debug issues with the terragrunt config. See
[Debugging]({{site.baseurl}}/docs/features/debugging) for some additional details.


### terragrunt-log-level

**CLI Arg**: `--terragrunt-log-level`<br/>
**Requires an argument**: `--terragrunt-log-level <LOG_LEVEL>`

When passed it, sets logging level for terragrunt. All supported levels are:

* panic
* fatal
* error
* warn
* info (this is the default)
* debug
* trace



### terragrunt-check

**CLI Arg**: `--terragrunt-check`<br/>
**Environment Variable**: `TERRAGRUNT_CHECK` (set to `true`)

When passed in, run `hclfmt` in check only mode instead of actively overwriting the files. This will cause the
command to exit with exit code 1 if there are any files that are not formatted.


### terragrunt-hclfmt-file

**CLI Arg**: `--terragrunt-hclfmt-file`
**Requires an argument**: `--terragrunt-hclfmt-file /path/to/terragrunt.hcl`

When passed in, run `hclfmt` only on specified `*/terragrunt.hcl` file.


### terragrunt-override-attr

**CLI Arg**: `--terragrunt-override-attr`
**Requires an argument**: `--terragrunt-override-attr ATTR=VALUE`

Override the attribute named `ATTR` with the value `VALUE` in a `provider` block as part of the [aws-provider-patch
command](#aws-provider-patch). May be specified multiple times. Also, `ATTR` can specify attributes within a nested
block by specifying `<BLOCK>.<ATTR>`, where `<BLOCK>` is the block name: e.g., `assume_role.role` arn will override the
`role_arn` attribute of the `assume_role { ... }` block.
