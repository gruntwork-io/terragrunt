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
  - [All OpenTofu/Terraform built-in commands](#all-opentofuterraform-built-in-commands)
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
  - [hclvalidate](#hclvalidate)
  - [aws-provider-patch](#aws-provider-patch)
  - [render-json](#render-json)
  - [output-module-groups](#output-module-groups)
  - [scaffold](#scaffold)
  - [catalog](#catalog)
  - [graph](#graph)
- [CLI options](#cli-options)
  - [terragrunt-config](#terragrunt-config)
  - [terragrunt-tfpath](#terragrunt-tfpath)
  - [terragrunt-no-auto-init](#terragrunt-no-auto-init)
  - [terragrunt-no-auto-approve](#terragrunt-no-auto-approve)
  - [terragrunt-no-auto-retry](#terragrunt-no-auto-retry)
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
  - [terragrunt-excludes-file](#terragrunt-excludes-file)
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
  - [terragrunt-no-color](#terragrunt-no-color)
  - [terragrunt-check](#terragrunt-check)
  - [terragrunt-diff](#terragrunt-diff)
  - [terragrunt-hclfmt-file](#terragrunt-hclfmt-file)
  - [terragrunt-hclvalidate-json](#terragrunt-hclvalidate-json)
  - [terragrunt-hclvalidate-show-config-path](#terragrunt-hclvalidate-show-config-path)
  - [terragrunt-override-attr](#terragrunt-override-attr)
  - [terragrunt-json-out](#terragrunt-json-out)
  - [terragrunt-json-disable-dependent-modules](#terragrunt-json-disable-dependent-modules)
  - [terragrunt-modules-that-include](#terragrunt-modules-that-include)
  - [terragrunt-fetch-dependency-output-from-state](#terragrunt-fetch-dependency-output-from-state)
  - [terragrunt-use-partial-parse-config-cache](#terragrunt-use-partial-parse-config-cache)
  - [terragrunt-include-module-prefix](#terragrunt-include-module-prefix)
  - [terragrunt-fail-on-state-bucket-creation](#terragrunt-fail-on-state-bucket-creation)
  - [terragrunt-disable-bucket-update](#terragrunt-disable-bucket-update)
  - [terragrunt-disable-command-validation](#terragrunt-disable-command-validation)
  - [terragrunt-json-log](#terragrunt-json-log)
  - [terragrunt-tf-logs-to-json](#terragrunt-tf-logs-to-json)
  - [terragrunt-provider-cache](#terragrunt-provider-cache)
  - [terragrunt-provider-cache-dir](#terragrunt-provider-cache-dir)
  - [terragrunt-provider-cache-hostname](#terragrunt-provider-cache-hostname)
  - [terragrunt-provider-cache-port](#terragrunt-provider-cache-port)
  - [terragrunt-provider-cache-token](#terragrunt-provider-cache-token)
  - [terragrunt-provider-cache-registry-names](#terragrunt-provider-cache-registry-names)
  - [terragrunt-out-dir](#terragrunt-out-dir)
  - [terragrunt-json-out-dir](#terragrunt-json-out-dir)

## CLI commands

Terragrunt supports the following CLI commands:

- [All OpenTofu/Terraform built-in commands](#all-opentofuterraform-built-in-commands)
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
- [hclvalidate](#hclvalidate)
- [aws-provider-patch](#aws-provider-patch)
- [render-json](#render-json)
- [output-module-groups](#output-module-groups)
- [scaffold](#scaffold)
- [catalog](#catalog)
- [graph](#graph)

### All OpenTofu/Terraform built-in commands

Terragrunt is an orchestration tool for OpenTofu/Terraform, so except for a few of the special commands defined in these docs,
Terragrunt forwards all other commands to OpenTofu/Terraform. For example, when you run `terragrunt apply`, Terragrunt executes
`tofu apply`/`terraform apply`.

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

Runs the provided OpenTofu/Terraform command against a `stack`, where a `stack` is a
tree of terragrunt modules. The command will recursively find terragrunt
modules in the current directory tree and run the OpenTofu/Terraform command in
dependency order (unless the command is destroy, in which case the command is
run in reverse dependency order).

Make sure to read [Execute OpenTofu/Terraform
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

**[WARNING] Using `run-all` with `plan` is currently broken for certain use cases**. If you have a stack of Terragrunt
modules with dependencies between them—either via `dependency` blocks or `terraform_remote_state` data sources—and
you've never deployed them, then `run-all plan` will fail as it will not be possible to resolve the `dependency` blocks
or `terraform_remote_state` data sources! Please [see here for more
information](https://github.com/gruntwork-io/terragrunt/issues/720#issuecomment-497888756).

**[NOTE]** Using `run-all` with `apply` or `destroy` silently adds the `-auto-approve` flag to the command line
arguments passed to OpenTofu/Terraform due to issues with shared `stdin` making individual approvals impossible. Please
[see here for more information](https://github.com/gruntwork-io/terragrunt/issues/386#issuecomment-358306268)

### plan-all (DEPRECATED: use run-all)

**DEPRECATED: Use `run-all plan` instead.**

Display the plans of a `stack` by running `terragrunt plan` in each subfolder. Make sure to read [Execute OpenTofu/Terraform
commands on multiple modules at once](/docs/features/execute-terraform-commands-on-multiple-modules-at-once/) for
context.

Example:

```bash
terragrunt run-all plan
```

This will recursively search the current working directory for any folders that contain Terragrunt modules and run
`plan` in each one, concurrently, while respecting ordering defined via
[`dependency`](/docs/reference/config-blocks-and-attributes/#dependency) and
[`dependencies`](/docs/reference/config-blocks-and-attributes/#dependencies) blocks.

**[WARNING] `run-all plan` is currently broken for certain use cases**. If you have a stack of Terragrunt modules with
dependencies between them—either via `dependency` blocks or `terraform_remote_state` data sources—and you've never
deployed them, then `run-all plan` will fail as it will not be possible to resolve the `dependency` blocks or
`terraform_remote_state` data sources! Please [see here for more
information](https://github.com/gruntwork-io/terragrunt/issues/720#issuecomment-497888756).

### apply-all (DEPRECATED: use run-all)

**DEPRECATED: Use `run-all apply` instead.**

Apply a `stack` by running `terragrunt apply` in each subfolder. Make sure to read [Execute OpenTofu/Terraform
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

**[NOTE]** Using `apply-all` silently adds the `-auto-approve` flag to the command line arguments passed to OpenTofu/Terraform
due to issues with shared `stdin` making individual approvals impossible. Please [see here for more
information](https://github.com/gruntwork-io/terragrunt/issues/386#issuecomment-358306268)

### output-all (DEPRECATED: use run-all)

**DEPRECATED: Use `run-all output` instead.**

Display the outputs of a `stack` by running `terragrunt output` in each subfolder. Make sure to read [Execute OpenTofu/Terraform
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

Destroy a `stack` by running `terragrunt destroy` in each subfolder. Make sure to read [Execute OpenTofu/Terraform
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

**[NOTE]** Using `destroy-all` silently adds the `-auto-approve` flag to the command line arguments passed to OpenTofu/Terraform
due to issues with shared `stdin` making individual approvals impossible. Please [see here for more
information](https://github.com/gruntwork-io/terragrunt/issues/386#issuecomment-358306268)

### validate-all (DEPRECATED: use run-all)

**DEPRECATED: Use `run-all validate` instead.**

Validate `stack` by running `terragrunt validate` in each subfolder. Make sure to read [Execute OpenTofu/Terraform
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
inputs (inputs that are not defined as an OpenTofu/Terraform variable in the
corresponding module) and undefined required inputs (required OpenTofu/Terraform
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

Be aware that other ways to pass variables to `tofu`/`terraform` are not checked by this command.

Additionally, there are **two modes** in which the `validate-inputs` command can be run: **relaxed** (default) and **strict**.

If you run the `validate-inputs` command without flags, relaxed mode will be enabled by default. In relaxed mode, any unused variables
that are passed, but not used by the underlying OpenTofu/Terraform configuration, will generate a warning, but not an error. Missing required variables will _always_ return an error, whether `validate-inputs` is running in relaxed or strict mode.

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

```text
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
and run the equivalent of `tofu fmt`/`terraform fmt` on them.

### hclvalidate

Find all hcl files from the configuration stack and validate them.

Example:

```bash
terragrunt hclvalidate
```

This will search all hcl files from the configuration stack in the current working directory and run the equivalent
of `tofu validate`/`terraform validate` on them.

For convenience in programmatically parsing these findings, you can also pass the `--terragrunt-hclvalidate-json` flag to output the results in JSON format.

Example:

```bash
terragrunt hclvalidate --terragrunt-hclvalidate-json
```

In addition, you can pass the `--terragrunt-hclvalidate-show-config-path` flag to only output paths of the invalid config files, delimited by newlines. This can be especially useful when combined with the [terragrunt-excludes-file](#terragrunt-excludes-file) flag.

Example:

```bash
terragrunt hclvalidate --terragrunt-hclvalidate-show-config-path
```

### aws-provider-patch

Overwrite settings on nested AWS providers to work around several OpenTofu/Terraform bugs. Due to
[issue #13018](https://github.com/hashicorp/terraform/issues/13018) and
[issue #26211](https://github.com/hashicorp/terraform/issues/26211), the `import` command may fail if your OpenTofu/Terraform
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

Both the `region` and `role_arn` parameters are set to dynamic values, which will trigger those OpenTofu/Terraform bugs. To work
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

1. Run `tofu init`/`terraform init` to download the code for all your modules into `.terraform/modules`.
1. Scan all the OpenTofu/Terraform code in `.terraform/modules`, find AWS `provider` blocks, and for each one, hard-code:
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

This should allow you to run `import` on the module and work around those OpenTofu/Terraform bugs. When you're done running
`import`, remember to delete your overridden code! E.g., Delete the `.terraform` or `.terragrunt-cache` folders.

### render-json

Render out the final interpreted `terragrunt.hcl` file (that is, with all the includes merged, dependencies
resolved/interpolated, function calls executed, etc) as json.

Example:

The following `terragrunt.hcl`:

```hcl
locals {
  aws_region = "us-east-1"
}

inputs = {
  aws_region = local.aws_region
}
```

Renders to the following `terragrunt_rendered.json`:

```json
{
  "locals": { "aws_region": "us-east-1" },
  "inputs": { "aws_region": "us-east-1" }
  // NOTE: other attributes are omitted for brevity
}
```

You can use the CLI option `--terragrunt-json-out` to configure where terragrunt renders out the json representation.

To generate json with metadata can be specified argument `--with-metadata` which will add metadata to the json output.

Example:

```json
{
  "inputs": {
    "aws_region": {
      "metadata": {
        "found_in_file": "/example/terragrunt.hcl"
      },
      "value": "us-east-1"
    }
  },
  "locals": {
    "aws_region": {
      "metadata": {
        "found_in_file": "/example/terragrunt.hcl"
      },
      "value": "us-east-1"
    }
  }
  // NOTE: other attributes are omitted for brevity
}
```

### output-module-groups

Output groups of modules ordered for apply (or destroy) as a list of list in JSON.

Example:

```bash
terragrunt output-module-groups <sub-command>
```

Optional sub-commands:

- apply (default)
- destroy

This will recursively search the current working directory for any folders that contain Terragrunt modules and build
the dependency graph based on [`dependency`](/docs/reference/config-blocks-and-attributes/#dependency) and
[`dependencies`](/docs/reference/config-blocks-and-attributes/#dependencies) blocks and output the graph as a JSON list of list (unless the sub-command is destroy, in which case the command will output the reverse dependency order).

This can be be useful in several scenarios, such as in CICD, when determining apply order or searching for all files to apply with CLI options
such as [`--terragrunt-modules-that-include`](#terragrunt-modules-that-include)

This may produce output such as:

```json
{
  "Group 1": ["stage/frontend-app"],
  "Group 2": ["stage/backend-app"],
  "Group 3": ["mgmt/bastion-host", "stage/search-app"],
  "Group 4": ["mgmt/kms-master-key", "stage/mysql", "stage/redis"],
  "Group 5": ["stage/vpc"],
  "Group 6": ["mgmt/vpc"]
}
```

### scaffold

Generate Terragrunt files from existing OpenTofu/Terraform modules.

More details in [scaffold section](https://terragrunt.gruntwork.io/docs/features/scaffold/).

### catalog

Launch the user interface for searching and managing your module catalog.

More details in [catalog section](https://terragrunt.gruntwork.io/docs/features/catalog/).

### graph

Run the provided OpenTofu/Terraform command against the graph of dependencies for the module in the current working directory. The graph consists of all modules that depend on the module in the current working directory via a `depends_on` or `dependencies` block, plus all the modules that depend on those modules, and all the modules that depend on those modules, and so on, recursively up the tree, up to the Git repository root, or the path specified via the optional `--terragrunt-graph-root` argument.

The Command will be executed following the order of dependencies: so it'll run on the module in the current working directory first, then on modules that depend on it directly, then on the modules that depend on those modules, and so on. Note that if the command is `destroy`, it will execute in the opposite order of the dependencies.

Example:
Having bellow dependencies:
[![dependency-graph](/assets/img/collections/documentation/dependency-graph.png){: width="80%" }]({{site.baseurl}}/assets/img/collections/documentation/dependency-graph.png)

Running `terragrunt graph apply` in `eks` module will lead to the following execution order:

```text
Group 1
- Module project/eks

Group 2
- Module project/services/eks-service-1
- Module project/services/eks-service-2

Group 3
- Module project/services/eks-service-2-v2
- Module project/services/eks-service-3
- Module project/services/eks-service-5

Group 4
- Module project/services/eks-service-3-v2
- Module project/services/eks-service-4

Group 5
- Module project/services/eks-service-3-v3
```

Notes:

- `lambda` modules aren't included in the graph, because they are not dependent on `eks` module.
- execution is from bottom up based on dependencies

Running `terragrunt graph destroy` in `eks` module will lead to the following execution order:

```text
Group 1
- Module project/services/eks-service-2-v2
- Module project/services/eks-service-3-v3
- Module project/services/eks-service-4
- Module project/services/eks-service-5

Group 2
- Module project/services/eks-service-3-v2

Group 3
- Module project/services/eks-service-3

Group 4
- Module project/services/eks-service-1
- Module project/services/eks-service-2

Group 5
- Module project/eks
```

Notes:

- execution is in reverse order, first are destroyed "top" modules and in the end `eks`
- `lambda` modules aren't affected at all

Running `terragrunt graph apply` in `services/eks-service-3`:

```text
Group 1
- Module project/services/eks-service-3

Group 2
- Module project/services/eks-service-3-v2
- Module project/services/eks-service-4

Group 3
- Module project/services/eks-service-3-v3

```

Notes:

- in execution are included only services dependent from `eks-service-3`

Running `terragrunt graph destroy` in `services/eks-service-3`:

```text
Group 1
- Module project/services/eks-service-3-v3
- Module project/services/eks-service-4

Group 2
- Module project/services/eks-service-3-v2

Group 3
- Module project/services/eks-service-3
```

Notes:

- destroy will be executed only on subset of services dependent from `eks-service-3`

## CLI options

Terragrunt forwards all options to OpenTofu/Terraform. The only exceptions are `--version` and arguments that start with the
prefix `--terragrunt-` (e.g., `--terragrunt-config`). The currently available options are:

- [CLI commands](#cli-commands)
  - [All OpenTofu/Terraform built-in commands](#all-opentofuterraform-built-in-commands)
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
  - [output-module-groups](#output-module-groups)
  - [scaffold](#scaffold)
  - [catalog](#catalog)
  - [graph](#graph)
- [CLI options](#cli-options)
  - [terragrunt-config](#terragrunt-config)
  - [terragrunt-tfpath](#terragrunt-tfpath)
  - [terragrunt-no-auto-init](#terragrunt-no-auto-init)
  - [terragrunt-no-auto-approve](#terragrunt-no-auto-approve)
  - [terragrunt-no-auto-retry](#terragrunt-no-auto-retry)
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
  - [terragrunt-excludes-file](#terragrunt-excludes-file)
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
  - [terragrunt-no-color](#terragrunt-no-color)
  - [terragrunt-check](#terragrunt-check)
  - [terragrunt-diff](#terragrunt-diff)
  - [terragrunt-hclfmt-file](#terragrunt-hclfmt-file)
  - [terragrunt-hclvalidate-json](#terragrunt-hclvalidate-json)
  - [terragrunt-hclvalidate-show-config-path](#terragrunt-hclvalidate-show-config-path)
  - [terragrunt-override-attr](#terragrunt-override-attr)
  - [terragrunt-json-out](#terragrunt-json-out)
  - [terragrunt-json-disable-dependent-modules](#terragrunt-json-disable-dependent-modules)
  - [terragrunt-modules-that-include](#terragrunt-modules-that-include)
  - [terragrunt-fetch-dependency-output-from-state](#terragrunt-fetch-dependency-output-from-state)
  - [terragrunt-use-partial-parse-config-cache](#terragrunt-use-partial-parse-config-cache)
  - [terragrunt-include-module-prefix](#terragrunt-include-module-prefix)
  - [terragrunt-fail-on-state-bucket-creation](#terragrunt-fail-on-state-bucket-creation)
  - [terragrunt-disable-bucket-update](#terragrunt-disable-bucket-update)
  - [terragrunt-disable-command-validation](#terragrunt-disable-command-validation)
  - [terragrunt-json-log](#terragrunt-json-log)
  - [terragrunt-tf-logs-to-json](#terragrunt-tf-logs-to-json)
  - [terragrunt-provider-cache](#terragrunt-provider-cache)
  - [terragrunt-provider-cache-dir](#terragrunt-provider-cache-dir)
  - [terragrunt-provider-cache-hostname](#terragrunt-provider-cache-hostname)
  - [terragrunt-provider-cache-port](#terragrunt-provider-cache-port)
  - [terragrunt-provider-cache-token](#terragrunt-provider-cache-token)
  - [terragrunt-provider-cache-registry-names](#terragrunt-provider-cache-registry-names)
  - [terragrunt-out-dir](#terragrunt-out-dir)
  - [terragrunt-json-out-dir](#terragrunt-json-out-dir)

### terragrunt-config

<!-- markdownlint-disable MD033 -->

**CLI Arg**: `--terragrunt-config`<br/>
**Environment Variable**: `TERRAGRUNT_CONFIG`<br/>
**Requires an argument**: `--terragrunt-config /path/to/terragrunt.hcl`<br/>

A custom path to the `terragrunt.hcl` or `terragrunt.hcl.json` file. The
default path is `terragrunt.hcl` (preferred) or `terragrunt.hcl.json` in the current directory (see
[Configuration]({{site.baseurl}}/docs/getting-started/configuration/#configuration) for a slightly more nuanced
explanation). This argument is not used with the `run-all` commands.

### terragrunt-tfpath

**CLI Arg**: `--terragrunt-tfpath`<br/>
**Environment Variable**: `TERRAGRUNT_TFPATH`<br/>
**Requires an argument**: `--terragrunt-tfpath /path/to/terraform-binary`<br/>

A custom path to the OpenTofu/Terraform binary. The default is `tofu` in a directory on your PATH.

**NOTE**: This will override the `terraform` binary that is used by `terragrunt` in all instances, including
`dependency` lookups. This setting will also override any [terraform_binary]({{site.baseurl}}/docs/reference/config-blocks-and-attributes/#terraform_binary)
configuration values specified in the `terragrunt.hcl` config for both the top level, and dependency lookups.

### terragrunt-no-auto-init

**CLI Arg**: `--terragrunt-no-auto-init`<br/>
**Environment Variable**: `TERRAGRUNT_NO_AUTO_INIT` (set to `true`)<br/>
_(Prior to Terragrunt v0.48.6, this environment variable was called `TERRAGRUNT_AUTO_INIT` (set to `false`), and is still available for backwards compatibility)_

When passed in, don't automatically run `terraform init` when other commands are run (e.g. `terragrunt apply`). Useful
if you want to pass custom arguments to `terraform init` that are specific to a user or execution environment, and
therefore cannot be specified as `extra_arguments`. For example, `-plugin-dir`. You must run `terragrunt init`
yourself in this case if needed. `terragrunt` will fail if it detects that `init` is needed, but auto init is
disabled. See [Auto-Init]({{site.baseurl}}/docs/features/auto-init#auto-init)

### terragrunt-no-auto-approve

**CLI Arg**: `--terragrunt-no-auto-approve`<br/>
**Environment Variable**: `TERRAGRUNT_NO_AUTO_APPROVE` (set to `true`)<br/>
_(Prior to Terragrunt v0.48.6, this environment variable was called `TERRAGRUNT_AUTO_APPROVE` (set to `false`), and is still available for backwards compatibility)_
**Commands**:

- [run-all](#run-all)

When passed in, Terragrunt will no longer automatically append `-auto-approve` to the underlying OpenTofu/Terraform commands run
with `run-all`. Note that due to the interactive prompts, this flag will also **automatically assume
`--terragrunt-parallelism 1`**.

### terragrunt-no-auto-retry

**CLI Arg**: `--terragrunt-no-auto-retry`<br/>
**Environment Variable**: `TERRAGRUNT_NO_AUTO_RETRY` (set to `true`)<br/>
_(Prior to Terragrunt v0.48.6, this environment variable was called `TERRAGRUNT_AUTO_RETRY` (set to `false`), and is still available for backwards compatibility)_

When passed in, don't automatically retry commands which fail with transient errors. See
[Auto-Retry]({{site.baseurl}}/docs/features/auto-retry#auto-retry)

### terragrunt-non-interactive

**CLI Arg**: `--terragrunt-non-interactive`<br/>
**Environment Variable**: `TERRAGRUNT_NON_INTERACTIVE` (set to `true`)<br/>
_(Prior to Terragrunt v0.48.6, this environment variable was called `TF_INPUT` (set to `false`), and is still available for backwards compatibility. NOTE: [TF_INPUT](https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_input) is native to OpenTofu/Terraform!)_

When passed in, don't show interactive user prompts. This will default the answer for all Terragrunt (not OpenTofu/Terraform) prompts to `yes` except for
the listed cases below. This is useful if you need to run Terragrunt in an automated setting (e.g. from a script). May
also be specified with the [TF_INPUT](https://www.terraform.io/docs/configuration/environment-variables.html#tf_input) environment variable.

This setting will default to `no` for the following cases:

- Prompts related to pulling in external dependencies. You can force include external dependencies using the
  [--terragrunt-include-external-dependencies](#terragrunt-include-external-dependencies) option.

Note that this does not impact the behavior of OpenTofu/Terraform commands invoked by Terragrunt.

e.g.

```bash
terragrunt --terragrunt-non-interactive apply -auto-approve
```

Is how you would make Terragrunt apply without any user prompts from Terragrunt or OpenTofu/Terraform.

### terragrunt-working-dir

**CLI Arg**: `--terragrunt-working-dir`<br/>
**Environment Variable**: `TERRAGRUNT_WORKING_DIR`<br/>
**Requires an argument**: `--terragrunt-working-dir /path/to/working-directory`<br/>

Set the directory where Terragrunt should execute the `terraform` command. Default is the current working directory.
Note that for the `run-all` commands, this parameter has a different meaning: Terragrunt will apply or destroy all the
OpenTofu/Terraform modules in the subfolders of the `terragrunt-working-dir`, running `terraform` in the root of each module it
finds.

### terragrunt-download-dir

**CLI Arg**: `--terragrunt-download-dir`<br/>
**Environment Variable**: `TERRAGRUNT_DOWNLOAD`<br/>
**Requires an argument**: `--terragrunt-download-dir /path/to/dir-to-download-terraform-code`<br/>

The path where to download OpenTofu/Terraform code when using [remote OpenTofu/Terraform
configurations](https://blog.gruntwork.io/terragrunt-how-to-keep-your-terraform-code-dry-and-maintainable-f61ae06959d8).
Default is `.terragrunt-cache` in the working directory. We recommend adding this folder to your `.gitignore`.

### terragrunt-source

**CLI Arg**: `--terragrunt-source`<br/>
**Environment Variable**: `TERRAGRUNT_SOURCE`<br/>
**Requires an argument**: `--terragrunt-source /path/to/local-terraform-code`<br/>

Download OpenTofu/Terraform configurations from the specified source into a temporary folder, and run OpenTofu/Terraform in that temporary
folder. The source should use the same syntax as the [OpenTofu/Terraform module
source](https://www.terraform.io/docs/modules/sources.html) parameter. If you specify this argument for the `run-all`
commands, Terragrunt will assume this is the local file path for all of your OpenTofu/Terraform modules, and for each module
processed by the `run-all` command, Terragrunt will automatically append the path of `source` parameter in each module
to the `--terragrunt-source` parameter you passed in.

### terragrunt-source-map

**CLI Arg**: `--terragrunt-source-map`<br/>
**Environment Variable**: `TERRAGRUNT_SOURCE_MAP` (encoded as comma separated value, e.g., `source1=dest1,source2=dest2`)<br/>
**Requires an argument**: `--terragrunt-source-map git::ssh://github.com=/path/to/local-terraform-code`<br/>

Can be supplied multiple times: `--terragrunt-source-map source1=dest1 --terragrunt-source-map source2=dest2`

The `--terragrunt-source-map source=dest` param replaces any `source` URL (including the source URL of a config pulled
in with `dependency` blocks) that has root `source` with `dest`.

For example:

```bash
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
**Environment Variable**: `TERRAGRUNT_SOURCE_UPDATE` (set to `true`)<br/>

When passed in, delete the contents of the temporary folder before downloading OpenTofu/Terraform source code into it.

### terragrunt-ignore-dependency-errors

**CLI Arg**: `--terragrunt-ignore-dependency-errors`<br/>

When passed in, the `*-all` commands continue processing components even if a dependency fails

### terragrunt-iam-role

**CLI Arg**: `--terragrunt-iam-role`<br/>
**Environment Variable**: `TERRAGRUNT_IAM_ROLE`<br/>
**Requires an argument**: `--terragrunt-iam-role "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"`<br/>

Assume the specified IAM role ARN before running OpenTofu/Terraform or AWS commands. This is a convenient way to use Terragrunt
and OpenTofu/Terraform with multiple AWS accounts.

### terragrunt-iam-assume-role-duration

**CLI Arg**: `--terragrunt-iam-assume-role-duration`<br/>
**Environment Variable**: `TERRAGRUNT_IAM_ASSUME_ROLE_DURATION`<br/>
**Requires an argument**: `--terragrunt-iam-assume-role-duration 3600`<br/>

Uses the specified duration as the session duration (in seconds) for the STS session which assumes the role defined in `--terragrunt-iam-role`.

### terragrunt-iam-assume-role-session-name

**CLI Arg**: `--terragrunt-iam-assume-role-session-name`<br/>
**Environment Variable**: `TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME`<br/>
**Requires an argument**: `--terragrunt-iam-assume-role-session-name "terragrunt-iam-role-session-name"`<br/>

Used as the session name for the STS session which assumes the role defined in `--terragrunt-iam-role`.

### terragrunt-excludes-file

**CLI Arg**: `--terragrunt-excludes-file`<br/>
**Environment Variable**: `TERRAGRUNT_EXCLUDES_FILE`<br/>
**Requires an argument**: `--terragrunt-excludes-file /path/to/file`<br/>

Path to a file with a list of directories that need to be excluded when running *-all commands, by default `.terragrunt-excludes`. Modules under these directories will be
excluded during execution of the commands. If a relative path is specified, it should be relative from
[--terragrunt-working-dir](#terragrunt-working-dir). This will only exclude the module, not its dependencies.

This flag has been designed to integrate nicely with the `hclvalidate` command, which can return a list of invalid files delimited by newlines when passed the `--terragrunt-hclvalidate-show-config-path` flag. To integrate the two, you can run something like the following using bash process substitution:

```bash
terragrunt run-all plan --terragrunt-excludes-file <(terragrunt hclvalidate --terragrunt-hclvalidate-show-config-path)
```

### terragrunt-exclude-dir

**CLI Arg**: `--terragrunt-exclude-dir`<br/>
**Environment Variable**: `TERRAGRUNT_EXCLUDE_DIR`<br/>
**Requires an argument**: `--terragrunt-exclude-dir /path/to/dirs/to/exclude*`<br/>

Can be supplied multiple times: `--terragrunt-exclude-dir /path/to/dirs/to/exclude --terragrunt-exclude-dir /another/path/to/dirs/to/exclude`

Unix-style glob of directories to exclude when running `*-all` commands. Modules under these directories will be
excluded during execution of the commands. If a relative path is specified, it should be relative from
[--terragrunt-working-dir](#terragrunt-working-dir). Flag can be specified multiple times. This will only exclude the
module, not its dependencies.

### terragrunt-include-dir

**CLI Arg**: `--terragrunt-include-dir`<br/>
**Requires an argument**: `--terragrunt-include-dir /path/to/dirs/to/include*`<br/>

Can be supplied multiple times: `--terragrunt-include-dir /path/to/dirs/to/include --terragrunt-include-dir /another/path/to/dirs/to/include`

Unix-style glob of directories to include when running `*-all` commands. Only modules under these directories (and all
dependent modules) will be included during execution of the commands. If a relative path is specified, it should be
relative from `--terragrunt-working-dir`. Flag can be specified multiple times.

### terragrunt-strict-include

**CLI Arg**: `--terragrunt-strict-include`<br/>

When passed in, only modules under the directories passed in with [--terragrunt-include-dir](#terragrunt-include-dir)
will be included. All dependencies of the included directories will be excluded if they are not in the included
directories. If no [--terragrunt-include-dir](#terragrunt-include-dir) flags are included, terragrunt will not include
any modules during the execution of the commands.

### terragrunt-strict-validate

**CLI Arg**: `--terragrunt-strict-validate`<br/>

When passed in, and running `terragrunt validate-inputs`, enables strict mode for the `validate-inputs` command. When strict mode is enabled, an error will be returned if any variables required by the underlying OpenTofu/Terraform configuration are not passed in, OR if any unused variables are passed in. By default, `terragrunt validate-inputs` runs in relaxed mode. In relaxed mode, an error is only returned when a variable required by the underlying OpenTofu/Terraform configuration is not passed in.

### terragrunt-ignore-dependency-order

**CLI Arg**: `--terragrunt-ignore-dependency-order`<br/>

When passed in, ignore the depedencies between modules when running `*-all` commands.

### terragrunt-ignore-external-dependencies

**CLI Arg**: `--terragrunt-ignore-external-dependencies`<br/>

When passed in, don't attempt to include any external dependencies when running `*-all` commands. Note that an external
dependency is a dependency that is outside the current terragrunt working directory, and is not respective to the
included directories with `terragrunt-include-dir`.

### terragrunt-include-external-dependencies

**CLI Arg**: `--terragrunt-include-external-dependencies`<br/>
**Environment Variable**: `TERRAGRUNT_INCLUDE_EXTERNAL_DEPENDENCIES`<br/>

When passed in, include any external dependencies when running `*-all` without asking. Note that an external
dependency is a dependency that is outside the current terragrunt working directory, and is not respective to the
included directories with `terragrunt-include-dir`.

### terragrunt-parallelism

**CLI Arg**: `--terragrunt-parallelism`<br/>
**Environment Variable**: `TERRAGRUNT_PARALLELISM`<br/>

When passed in, limit the number of modules that are run concurrently to this number during \*-all commands.
The exception is the `terraform init` command, which is always executed sequentially if the [terraform plugin cache](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache) is used. This is because the terraform plugin cache is not guaranteed to be concurrency safe.

### terragrunt-debug

**CLI Arg**: `--terragrunt-debug`<br/>
**Environment Variable**: `TERRAGRUNT_DEBUG`<br/>

When passed in, Terragrunt will create a tfvars file that can be used to invoke the terraform module in the same way
that Terragrunt invokes the module, so that you can debug issues with the terragrunt config. See
[Debugging]({{site.baseurl}}/docs/features/debugging) for some additional details.

### terragrunt-log-level

**CLI Arg**: `--terragrunt-log-level`<br/>
**Environment Variable**: `TERRAGRUNT_LOG_LEVEL`<br/>
**Requires an argument**: `--terragrunt-log-level <LOG_LEVEL>`<br/>

When passed it, sets logging level for terragrunt. All supported levels are:

- `panic`
- `fatal`
- `error`
- `warn`
- `info` (this is the default)
- `debug`
- `trace`

### terragrunt-no-color

**CLI Arg**: `--terragrunt-no-color`<br/>
**Environment Variable**: `TERRAGRUNT_NO_COLOR`<br/>

If specified, Terragrunt output won't contain any color.

NOTE: This option does not disable OpenTofu/Terraform output colors. Use the OpenTofu/Terraform [`-no-color`](https://developer.hashicorp.com/terraform/cli/commands/plan#no-color) argument.

### terragrunt-check

**CLI Arg**: `--terragrunt-check`<br/>
**Environment Variable**: `TERRAGRUNT_CHECK` (set to `true`)<br/>
**Commands**:

- [hclfmt](#hclfmt)

When passed in, run `hclfmt` in check only mode instead of actively overwriting the files. This will cause the
command to exit with exit code 1 if there are any files that are not formatted.

### terragrunt-diff

**CLI Arg**: `--terragrunt-diff`<br/>
**Environment Variable**: `TERRAGRUNT_DIFF` (set to `true`)<br/>
**Commands**:

- [hclfmt](#hclfmt)

When passed in, running `hclfmt` will print diff between original and modified file versions.

### terragrunt-hclfmt-file

**CLI Arg**: `--terragrunt-hclfmt-file`<br/>
**Requires an argument**: `--terragrunt-hclfmt-file /path/to/terragrunt.hcl`<br/>
**Commands**:

- [hclfmt](#hclfmt)

When passed in, run `hclfmt` only on specified hcl file.

### terragrunt-hclvalidate-json

**CLI Arg**: `--terragrunt-hclvalidate-json`<br/>
**Environment Variable**: `TERRAGRUNT_HCLVALIDATE_JSON` (set to `true`)<br/>
**Commands**:

- [hclvalidate](#hclvalidate)

When passed in, render the output in the JSON format.

### terragrunt-hclvalidate-show-config-path

**CLI Arg**: `--terragrunt-hclvalidate-show-config-path`<br/>
**Environment Variable**: `TERRAGRUNT_HCLVALIDATE_INVALID` (set to `true`)<br/>
**Commands**:

- [hclvalidate](#hclvalidate)

When passed in, output a list of files with invalid configuration.

### terragrunt-override-attr

**CLI Arg**: `--terragrunt-override-attr`<br/>
**Requires an argument**: `--terragrunt-override-attr ATTR=VALUE`<br/>

Override the attribute named `ATTR` with the value `VALUE` in a `provider` block as part of the [aws-provider-patch
command](#aws-provider-patch). May be specified multiple times. Also, `ATTR` can specify attributes within a nested
block by specifying `<BLOCK>.<ATTR>`, where `<BLOCK>` is the block name: e.g., `assume_role.role` arn will override the
`role_arn` attribute of the `assume_role { ... }` block.

### terragrunt-json-out

**CLI Arg**: `--terragrunt-json-out`<br/>
**Requires an argument**: `--terragrunt-json-out /path/to/terragrunt_rendered.json`<br/>
**Commands**:

- [render-json](#render-json)

When passed in, render the json representation in this file.

### terragrunt-json-disable-dependent-modules

**CLI Arg**: `--terragrunt-json-disable-dependent-modules`<br/>
**Requires an argument**: `--terragrunt-json-disable-dependent-modules`<br/>
**Commands**:

- [render-json](#render-json)

When the `--terragrunt-json-disable-dependent-modules` flag is included in the command, the process of identifying dependent modules will be disabled during JSON rendering.
This lead to a faster rendering process, but the output will not include any dependent modules.

### terragrunt-modules-that-include

**CLI Arg**: `--terragrunt-modules-that-include`<br/>
**Requires an argument**: `--terragrunt-modules-that-include /path/to/included-terragrunt.hcl`<br/>
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

```tree
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

```hcl
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

**CLI Arg**: `--terragrunt-fetch-dependency-output-from-state`<br/>
**Environment Variable**: `TERRAGRUNT_FETCH_DEPENDENCY_OUTPUT_FROM_STATE` (set to `true`)<br/>

When using many dependencies, this option can speed up the dependency processing by fetching dependency output directly
from the state file instead of init dependencies and running terraform on them.
NOTE: This is an experimental feature, use with caution.
Currently only AWS S3 backend is supported.

### terragrunt-use-partial-parse-config-cache

**CLI Arg**: `--terragrunt-use-partial-parse-config-cache`<br/>
**Environment Variable**: `TERRAGRUNT_USE_PARTIAL_PARSE_CONFIG_CACHE` (set to `true`)<br/>

This flag can be used to drastically decrease time required for parsing Terragrunt files. The effect will only show if a lot of similar includes are expected such as the root terragrunt.hcl include.
NOTE: This is an experimental feature, use with caution.

The reason you might want to use this flag is that Terragrunt frequently only needs to perform a partial parse of Terragrunt configurations.

This is the case for scenarios like:

- Building the Directed Acyclic Graph (DAG) during a `run-all` command where only the `dependency` blocks need to be evaluated to determine run order.
- Parsing the `terraform` block to determine state configurations for fetching `dependency` outputs.
- Determining whether Terragrunt execution behavior has to change like for `prevent_destroy` or `skip` flags in configuration.

These configurations are generally safe to cache, but due to the nature of HCL being a dynamic configuration language, there are some edge cases where caching these can lead to incorrect behavior.

Once this flag has been tested thoroughly, we will consider making it the default behavior.

### terragrunt-include-module-prefix

**CLI Arg**: `--terragrunt-include-module-prefix`<br/>
**Environment Variable**: `TERRAGRUNT_INCLUDE_MODULE_PREFIX` (set to `true`)<br/>

When this flag is set output from OpenTofu/Terraform sub-commands is prefixed with module path.

### terragrunt-fail-on-state-bucket-creation

**CLI Arg**: `--terragrunt-fail-on-state-bucket-creation`<br/>
**Environment Variable**: `TERRAGRUNT_FAIL_ON_STATE_BUCKET_CREATION` (set to `true`)<br/>

When this flag is set, Terragrunt will fail and exit if it is necessary to create the remote state bucket.

### terragrunt-disable-bucket-update

**CLI Arg**: `--terragrunt-disable-bucket-update`<br/>
**Environment Variable**: `TERRAGRUNT_DISABLE_BUCKET_UPDATE` (set to `true`)<br/>

When this flag is set, Terragrunt does not update the remote state bucket, which is useful to set if the state bucket is managed by a third party.

### terragrunt-disable-command-validation

**CLI Arg**: `--terragrunt-disable-command-validation`<br/>
**Environment Variable**: `TERRAGRUNT_DISABLE_COMMAND_VALIDATION` (set to `true`)<br/>

When this flag is set, Terragrunt will not validate the terraform command, which can be useful when need to use non-existent commands in hooks.

### terragrunt-json-log

**CLI Arg**: `--terragrunt-json-log`<br/>
**Environment Variable**: `TERRAGRUNT_JSON_LOG` (set to `true`)<br/>

When this flag is set, Terragrunt will output its logs in JSON format.

### terragrunt-tf-logs-to-json

**CLI Arg**: `--terragrunt-tf-logs-to-json`<br/>
**Environment Variable**: `TERRAGRUNT_TF_JSON_LOG` (set to `true`)<br/>

When this flag is set, Terragrunt will wrap OpenTofu/Terraform `stdout` and `stderr` in JSON log messages. Works only with `--terragrunt-json-log` flag.

### terragrunt-provider-cache

**CLI Arg**: `--terragrunt-provider-cache`<br/>
**Environment Variable**: `TERRAGRUNT_PROVIDER_CACHE`<br/>
**Commands**:

- [run-all](#run-all)

Enables Terragrunt's provider caching. This forces OpenTofu/Terraform to make provider requests through the Terragrunt Provider Cache server. Make sure to read [Provider Caching](https://terragrunt.gruntwork.io/docs/features/provider-cache/) for context.

### terragrunt-provider-cache-dir

**CLI Arg**: `--terragrunt-provider-cache-dir`<br/>
**Environment Variable**: `TERRAGRUNT_PROVIDER_CACHE_DIR`<br/>
**Commands**:

- [run-all](#run-all)

The path to the Terragrunt provider cache directory. By default, `terragrunt/providers` folder in the user cache directory: `$HOME/.cache` on Unix systems, `$HOME/Library/Caches` on Darwin, `%LocalAppData%` on Windows. The file structure of the cache directory is identical to the OpenTofu/Terraform [plugin_cache_dir](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache) directory. Make sure to read [Provider Caching](https://terragrunt.gruntwork.io/docs/features/provider-cache/) for context.

### terragrunt-provider-cache-hostname

**CLI Arg**: `--terragrunt-provider-cache-hostname`<br/>
**Environment Variable**: `TERRAGRUNT_PROVIDER_CACHE_HOSTNAME`<br/>
**Commands**:

- [run-all](#run-all)

The hostname of the Terragrunt Provider Cache server. By default, 'localhost'. Make sure to read [Provider Caching](https://terragrunt.gruntwork.io/docs/features/provider-cache/) for context.

### terragrunt-provider-cache-port

**CLI Arg**: `--terragrunt-provider-cache-port`<br/>
**Environment Variable**: `TERRAGRUNT_PROVIDER_CACHE_PORT`<br/>
**Commands**:

- [run-all](#run-all)

The port of the Terragrunt Provider Cache server. By default, assigned automatically. Make sure to read [Provider Caching](https://terragrunt.gruntwork.io/docs/features/provider-cache/) for context.

### terragrunt-provider-cache-token

**CLI Arg**: `--terragrunt-provider-cache-token`<br/>
**Environment Variable**: `TERRAGRUNT_PROVIDER_CACHE_TOKEN`<br/>
**Commands**:

- [run-all](#run-all)

The Token for authentication on the Terragrunt Provider Cache server. By default, assigned automatically. Make sure to read [Provider Caching](https://terragrunt.gruntwork.io/docs/features/provider-cache/) for context.

### terragrunt-provider-cache-registry-names

**CLI Arg**: `--terragrunt-provider-cache-registry-names`<br/>
**Environment Variable**: `TERRAGRUNT_PROVIDER_CACHE_REGISTRY_NAMES`<br/>
**Commands**:

- [run-all](#run-all)

The list of remote registries to cached by Terragrunt Provider Cache server. By default, 'registry.terraform.io', 'registry.opentofu.org'. Make sure to read [Provider Caching](https://terragrunt.gruntwork.io/docs/features/provider-cache/) for context.

### terragrunt-out-dir

**CLI Arg**: `--terragrunt-out-dir`<br/>
**Environment Variable**: `TERRAGRUNT_OUT_DIR`<br/>
**Commands**:

- [run-all](#run-all)

Specify the plan output directory for the `*-all` commands. Useful to save plans between runs in a single place.

### terragrunt-json-out-dir

**CLI Arg**: `--terragrunt-json-out-dir`<br/>
**Environment Variable**: `TERRAGRUNT_JSON_OUT_DIR`<br/>
**Commands**:

- [run-all](#run-all)

Specify the output directory for the `*-all` commands to store plans in JSON format. Useful to read plans programmatically.

### terragrunt-auth-provider-cmd

**CLI Arg**: `--terragrunt-auth-provider-cmd`<br/>
**Environment Variable**: `TERRAGRUNT_AUTH_PROVIDER_CMD`<br/>
**Requires an argument**: `--terragrunt-auth-provider-cmd "command [arguments]"`<br/>

The command and arguments used to obtain authentication credentials dynamically. If specified, Terragrunt runs this command for every working directory before running the underlying IAC for a `terragrunt.hcl` file.

The output must be valid JSON of the following schema:

```json
{
  "awsCredentials": {
    "ACCESS_KEY_ID": "",
    "SECRET_ACCESS_KEY": "",
    "SESSION_TOKEN": ""
  },
  "envs": {
    "ANY_KEY": ""
  }
}
```

This allows Terragrunt to acquire different credentials at runtime without changing any `terragrunt.hcl` configuration. You can use this flag to set arbitrary credentials for continuous integration, authentication with providers other than AWS and more.

As long as the standard output of the command passed to `terragrunt-auth-provider-cmd` results in JSON matching the schema above, corresponding environment variables will be set before Terragrunt begins IAC execution for a `terragrunt.hcl` file.

The simplest approach to leverage this flag is to write a script that fetches desired credentials, and emits them to STDOUT in the JSON format listed above:

```bash
#!/usr/bin/env bash

echo -n '{"envs": {"KEY": "a secret"}}'
```

You can use any technology you'd like, however, as long as Terragrunt can execute it. The expected pattern for using this flag is to populate the values dynamically using a secret store, etc.

Note that more specific configurations (e.g. `awsCredentials`) take precedence over less specific configurations (e.g. `envs`).

If you would like to set credentials for AWS with this method, you are encouraged to use `awsCredentials` instead of `envs`, as these keys will be validated to conform to the officially supported environment variables expected by the AWS SDK.

Other credential configurations will be supported in the future, but until then, if your provider authenticates via environment variables, you can use the `envs` field to fetch credentials dynamically from a secret store, etc before Terragrunt executes any IAC.
