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
  - [render-json](#render-json)

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

**[NOTE]** Using `run-all` with `apply` or `destroy` silently adds the `-auto-approve` flag to the command line
arguments passed to Terraform due to issues with shared `stdin` making individual approvals impossible. Please
[see here for more information](https://github.com/gruntwork-io/terragrunt/issues/386#issuecomment-358306268)




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

**[NOTE]** Using `apply-all` silently adds the `-auto-approve` flag to the command line arguments passed to Terraform
due to issues with shared `stdin` making individual approvals impossible. Please [see here for more
information](https://github.com/gruntwork-io/terragrunt/issues/386#issuecomment-358306268)


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

**[NOTE]** Using `destroy-all` silently adds the `-auto-approve` flag to the command line arguments passed to Terraform
due to issues with shared `stdin` making individual approvals impossible. Please [see here for more
information](https://github.com/gruntwork-io/terragrunt/issues/386#issuecomment-358306268)


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

Additionally, there are <b>two modes</b> in which the `validate-inputs` command can be run: <b>relaxed</b> (default) and <b>strict</b>.

If you run the `validate-inputs` command without flags, relaxed mode will be enabled by default. In relaxed mode, any unused variables
that are passed, but not used by the underlying Terraform configuration, will generate a warning, but not an error. Missing required variables will <em>always</em> return an error, whether `validate-inputs` is running in relaxed or strict mode.

To enable strict mode, you can pass the `--terragrunt-strict-validate` flag like so:

```bash
> terragrunt validate-inputs --terragrunt-strict-validate
```

When running in strict mode, `validate-inputs` will return an error if there are unused inputs.

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

Recursively find hcl files and rewrite them into a canonical format.

Example:

```bash
terragrunt hclfmt
```

This will recursively search the current working directory for any folders that contain Terragrunt configuration files
and run the equivalent of `terraform fmt` on them.


### aws-provider-patch

Overwrite settings on nested AWS providers to work around several Terraform bugs. Due to
[issue #13018](https://github.com/hashicorp/terraform/issues/13018) and
[issue #26211](https://github.com/hashicorp/terraform/issues/26211), the `import` command may fail if your Terraform
code uses a module that has a `provider` block nested within it that sets any of its attributes to computed values.
This command is a hacky attempt at working around this problem by allowing you to temporarily hard-code those
attributes so `import` can work.

You specify which attributes to hard-code using the [`--terragrunt-override-attr`](#terragrunt-override-attr) option,
passing it `ATTR=VALUE`, where `ATTR` is the attribute name and `VALUE` is the new value. `VALUE` is assumed to be a
json encoded string, which means that you must have quotes (e.g., `--terragrunt-override-attr 'region="eu-west-1"'`).
Additionally, note that `ATTR` can specify attributes within a nested block by specifying `<BLOCK>.<ATTR>`, where
`<BLOCK>` is the block name.

For example, let's say you had a `provider` block in a module that looked like this:

```hcl
provider "aws" {
  region              = var.aws_region
  allowed_account_ids = var.allowed_account_ids
  assume_role {
    role_arn = var.role_arn
  }
}
```

Both the `region` and `role_arn` parameters are set to dynamic values, which will trigger those Terraform bugs. To work
around it, run the following command:

```bash
# NOTE: The single quotes around the args is to allow you to pass through the " character in the args via bash quoting
# rules.
terragrunt aws-provider-patch \
  --terragrunt-override-attr 'region="eu-west-1"' \
  --terragrunt-override-attr 'assume_role.role_arn=""' \
  --terragrunt-override-attr 'allowed_account_ids=["00000000"]'
```

When you run the command above, Terragrunt will:

1. Run `terraform init` to download the code for all your modules into `.terraform/modules`.
1. Scan all the Terraform code in `.terraform/modules`, find AWS `provider` blocks, and for each one, hard-code:
    1. The `region` param to `"eu-west-1"`.
    1. The `role_arn` within the `assume_role` block to `""`.
    1. The `allowed_account_ids` param to `["0000000"]`.

The result will look like this:

```hcl
provider "aws" {
  region              = "eu-west-1"
  allowed_account_ids = ["0000000"]
  assume_role {
    role_arn = ""
  }
}
```

This should allow you to run `import` on the module and work around those Terraform bugs. When you're done running
`import`, remember to delete your overridden code! E.g., Delete the `.terraform` or `.terragrunt-cache` folders.


### render-json

Render out the final interpreted `terragrunt.hcl` file (that is, with all the includes merged, dependencies
resolved/interpolated, function calls executed, etc) as json.

Example:

_terragrunt.hcl_
```hcl
locals {
  aws_region = "us-east-1"
}

inputs = {
  aws_region = local.aws_region
}
```

_terragrunt_rendered.json_
```json
{
  "locals": {"aws_region": "us-east-1"},
  "inputs": {"aws_region": "us-east-1"},
  // NOTE: other attributes are omitted for brevity
}
```

You can use the CLI option `--terragrunt-json-out` to configure where terragrunt renders out the json representation.


## CLI options

Terragrunt forwards all options to Terraform. The only exceptions are `--version` and arguments that start with the
prefix `--terragrunt-` (e.g., `--terragrunt-config`). The currently available options are:

- [terragrunt-config](#terragrunt-config)
- [terragrunt-tfpath](#terragrunt-tfpath)
- [terragrunt-no-auto-init](#terragrunt-no-auto-init)
- [terragrunt-no-auto-retry](#terragrunt-no-auto-retry)
- [terragrunt-no-auto-approve](#terragrunt-no-auto-approve)
- [terragrunt-non-interactive](#terragrunt-non-interactive)
- [terragrunt-working-dir](#terragrunt-working-dir)
- [terragrunt-download-dir](#terragrunt-download-dir)
- [terragrunt-source](#terragrunt-source)
- [terragrunt-source-map](#terragrunt-source-map)
- [terragrunt-source-update](#terragrunt-source-update)
- [terragrunt-ignore-dependency-errors](#terragrunt-ignore-dependency-errors)
- [terragrunt-iam-role](#terragrunt-iam-role)
- [terragrunt-iam-assume-role-duration](#terragrunt-iam-assume-role-duration)
- [terragrunt-iam-assume-role-session-name](#terragrunt-iam-assume-role-session-name)
- [terragrunt-exclude-dir](#terragrunt-exclude-dir)
- [terragrunt-include-dir](#terragrunt-include-dir)
- [terragrunt-strict-include](#terragrunt-strict-include)
- [terragrunt-strict-validate](#terragrunt-strict-validate)
- [terragrunt-ignore-dependency-order](#terragrunt-ignore-dependency-order)
- [terragrunt-ignore-external-dependencies](#terragrunt-ignore-external-dependencies)
- [terragrunt-include-external-dependencies](#terragrunt-include-external-dependencies)
- [terragrunt-parallelism](#terragrunt-parallelism)
- [terragrunt-debug](#terragrunt-debug)
- [terragrunt-log-level](#terragrunt-log-level)
- [terragrunt-check](#terragrunt-check)
- [terragrunt-hclfmt-file](#terragrunt-hclfmt-file)
- [terragrunt-override-attr](#terragrunt-override-attr)
- [terragrunt-json-out](#terragrunt-json-out)
- [terragrunt-modules-that-include](#terragrunt-modules-that-include)
- [terragrunt-fetch-dependency-output-from-state](#terragrunt-fetch-dependency-output-from-state)

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

**NOTE**: This will override the `terraform` binary that is used by `terragrunt` in all instances, including
`dependency` lookups. This setting will also override any [terraform_binary]({{site.baseurl}}/docs/reference/config-blocks-and-attributes/#terraform_binary)
configuration values specified in the `terragrunt.hcl` config for both the top level, and dependency lookups.


### terragrunt-no-auto-init

**CLI Arg**: `--terragrunt-no-auto-init`<br/>
**Environment Variable**: `TERRAGRUNT_AUTO_INIT` (set to `false`)

When passed in, don't automatically run `terraform init` when other commands are run (e.g. `terragrunt apply`). Useful
if you want to pass custom arguments to `terraform init` that are specific to a user or execution environment, and
therefore cannot be specified as `extra_arguments`. For example, `-plugin-dir`. You must run `terragrunt init`
yourself in this case if needed. `terragrunt` will fail if it detects that `init` is needed, but auto init is
disabled. See [Auto-Init]({{site.baseurl}}/docs/features/auto-init#auto-init)


### terragrunt-no-auto-approve

**CLI Arg**: `--terragrunt-no-auto-approve`<br/>
**Environment Variable**: `TERRAGRUNT_AUTO_APPROVE` (set to `false`)
**Commands**:
- [run-all](#run-all)

When passed in, Terragrunt will no longer automatically append `-auto-approve` to the underlying Terraform commands run
with `run-all`. Note that due to the interactive prompts, this flag will also **automatically assume
`--terragrunt-parallelism 1`**.


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


### terragrunt-source-map

**CLI Arg**: `--terragrunt-source-map`<br/>
**Environment Variable**: `TERRAGRUNT_SOURCE_MAP` (encoded as comma separated value, e.g., `source1=dest1,source2=dest2`)<br/>
**Requires an argument**: `--terragrunt-source-map git::ssh://github.com=/path/to/local-terraform-code`

Can be supplied multiple times: `--terragrunt-source-map source1=dest1 --terragrunt-source-map source2=dest2`

The `--terragrunt-source-map source=dest` param replaces any `source` URL (including the source URL of a config pulled
in with `dependency` blocks) that has root `source` with `dest`.

For example:

```
terragrunt apply --terragrunt-source-map github.com/org/modules.git=/local/path/to/modules
```

The above would replace `terraform { source = "github.com/org/modules.git//xxx" }` with `terraform { source = /local/path/to/modules//xxx }` regardless of
whether you were running `apply`, or `run-all`, or using a `dependency`.

**NOTE**: This setting is ignored if you pass in `--terragrunt-source`.

Note that this only performs literal matches on the URL portion. For example, a map key of
`ssh://git@github.com/gruntwork-io/terragrunt.git` will only match terragrunt configurations with source `source =
"ssh://git@github.com/gruntwork-io/terragrunt.git//xxx"` and not sources of the form `source =
"git::ssh://git@github.com/gruntwork-io/terragrunt.git//xxx"`. The latter requires a map key of
`git::ssh://git@github.com/gruntwork-io/terragrunt.git`.



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


### terragrunt-iam-assume-role-duration

**CLI Arg**: `--terragrunt-iam-assume-role-duration`<br/>
**Environment Variable**: `TERRAGRUNT_IAM_ASSUME_ROLE_DURATION`<br/>
**Requires an argument**: `--terragrunt-iam-assume-role-duration 3600`

Uses the specified duration as the session duration (in seconds) for the STS session which assumes the role defined in `--terragrunt-iam-role`.


### terragrunt-iam-assume-role-session-name

**CLI Arg**: `--terragrunt-iam-assume-role-session-name`<br/>
**Environment Variable**: `TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME`<br/>
**Requires an argument**: `--terragrunt-iam-assume-role-session-name "terragrunt-iam-role-session-name"`

Used as the session name for the STS session which assumes the role defined in `--terragrunt-iam-role`.


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
directories. If no [--terragrunt-include-dir](#terragrunt-include-dir) flags are included, terragrunt will not include
any modules during the execution of the commands.

### terragrunt-strict-validate

**CLI Arg**: `--terragrunt-strict-validate`

When passed in, and running `terragrunt validate-inputs`, enables strict mode for the `validate-inputs` command. When strict mode is enabled, an error will be returned if any variables required by the underlying Terraform configuration are not passed in, OR if any unused variables are passed in. By default, `terragrunt validate-inputs` runs in relaxed mode. In relaxed mode, an error is only returned when a variable required by the underlying Terraform configuration is not passed in.

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
**Environment Variable**: `TERRAGRUNT_INCLUDE_EXTERNAL_DEPENDENCIES`

When passed in, include any external dependencies when running `*-all` without asking. Note that an external
dependency is a dependency that is outside the current terragrunt working directory, and is not respective to the
included directories with `terragrunt-include-dir`.


### terragrunt-parallelism

**CLI Arg**: `--terragrunt-parallelism`<br/>
**Environment Variable**: `TERRAGRUNT_PARALLELISM`

When passed in, limit the number of modules that are run concurrently to this number during *-all commands.


### terragrunt-debug

**CLI Arg**: `--terragrunt-debug`<br/>
**Environment Variable**: `TERRAGRUNT_DEBUG`

When passed in, Terragrunt will create a tfvars file that can be used to invoke the terraform module in the same way
that Terragrunt invokes the module, so that you can debug issues with the terragrunt config. See
[Debugging]({{site.baseurl}}/docs/features/debugging) for some additional details.


### terragrunt-log-level

**CLI Arg**: `--terragrunt-log-level`<br/>
**Environment Variable**: `TERRAGRUNT_LOG_LEVEL`
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
**Commands**:
- [hclfmt](#hclfmt)

When passed in, run `hclfmt` in check only mode instead of actively overwriting the files. This will cause the
command to exit with exit code 1 if there are any files that are not formatted.


### terragrunt-hclfmt-file

**CLI Arg**: `--terragrunt-hclfmt-file`
**Requires an argument**: `--terragrunt-hclfmt-file /path/to/terragrunt.hcl`
**Commands**:
- [hclfmt](#hclfmt)

When passed in, run `hclfmt` only on specified hcl file.


### terragrunt-override-attr

**CLI Arg**: `--terragrunt-override-attr`
**Requires an argument**: `--terragrunt-override-attr ATTR=VALUE`

Override the attribute named `ATTR` with the value `VALUE` in a `provider` block as part of the [aws-provider-patch
command](#aws-provider-patch). May be specified multiple times. Also, `ATTR` can specify attributes within a nested
block by specifying `<BLOCK>.<ATTR>`, where `<BLOCK>` is the block name: e.g., `assume_role.role` arn will override the
`role_arn` attribute of the `assume_role { ... }` block.

### terragrunt-json-out

**CLI Arg**: `--terragrunt-json-out`
**Requires an argument**: `--terragrunt-json-out /path/to/terragrunt_rendered.json`
**Commands**:
- [render-json](#render-json)

When passed in, render the json representation in this file.


### terragrunt-modules-that-include

**CLI Arg**: `--terragrunt-modules-that-include`
**Requires an argument**: `--terragrunt-modules-that-include /path/to/included-terragrunt.hcl`
**Commands**:
- [run-all](#run-all)
- [plan-all (DEPRECATED: use run-all)](#plan-all-deprecated-use-run-all)
- [apply-all (DEPRECATED: use run-all)](#apply-all-deprecated-use-run-all)
- [output-all (DEPRECATED: use run-all)](#output-all-deprecated-use-run-all)
- [destroy-all (DEPRECATED: use run-all)](#destroy-all-deprecated-use-run-all)
- [validate-all (DEPRECATED: use run-all)](#validate-all-deprecated-use-run-all)

When passed in, `run-all` will only run the command against Terragrunt modules that include the specified file.

This applies to the set of modules that are identified based on all the existing criteria for deciding which modules to
include. For example, consider the following folder structure:

```
.
├── _envcommon
│   └── data-stores
│       └── aurora.hcl
├── dev
│   └── us-west-2
│       └── dev
│           ├── data-stores
│           │   └── aurora
│           │       └── terragrunt.hcl
│           └── networking
│               └── vpc
│                   └── terragrunt.hcl
└── stage
    └── us-west-2
        └── stage
            ├── data-stores
            │   └── aurora
            │       └── terragrunt.hcl
            └── networking
                └── vpc
                    └── terragrunt.hcl
```

Suppose that both `dev/us-west-2/dev/data-stores/aurora/terragrunt.hcl` and
`stage/us-west-2/stage/data-stores/aurora/terragrunt.hcl` had the following contents:

```
include "envcommon" {
  path = "../../../../../_envcommon/data-stores/aurora.hcl"
}
```

If you run the command `run-all init --terragrunt-modules-that-include ../_envcommon/data-stores/aurora.hcl` from the
`dev` folder, only `dev/us-west-2/dev/data-stores/aurora` will be run; not `stage/us-west-2/stage/data-stores/aurora`.
This is because `run-all` by default restricts the modules to only those that are direct descendents of the current
folder you are running from. If you also pass in `--terragrunt-include-dir ../stage`, then it will now include
`stage/us-west-2/stage/data-stores/aurora` because now the `stage` folder is in consideration.

In other words, Terragrunt will always first find all the modules that should be included before applying this filter,
and then will apply this filter on the set of modules that it found.

You can pass this argument in multiple times to provide a list of include files to consider. When multiple files are
passed in, the set will be the union of modules that includes at least one of the files in the list.

NOTE: When using relative paths, the paths are relative to the working directory. This is either the current working
directory, or any path passed in to [terragrunt-working-dir](#terragrunt-working-dir).

### terragrunt-fetch-dependency-output-from-state

**CLI Arg**: `--terragrunt-fetch-dependency-output-from-state`
**Environment Variable**: `TERRAGRUNT_FETCH_DEPENDENCY_OUTPUT_FROM_STATE` (set to `true`)
When using many dependencies, this option can speed up the dependency processing by fetching dependency output directly
from the state file instead of init dependencies and running terraform on them.
NOTE: This is an experimental feature, use with caution.
Currently only AWS S3 backend is supported.
