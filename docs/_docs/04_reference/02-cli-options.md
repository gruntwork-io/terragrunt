---
layout: collection-browser-doc
title: CLI options
category: reference
categories_url: reference
excerpt: >-
  Learn about all CLI arguments and options you can use with Terragrunt.
tags: ["CLI"]
order: 402
nav_title: Documentation
nav_title_link: /docs/
slug: cli-options
---

## Commands

The main commands available in Terragrunt are:

- [Main commands](#main-commands)
  - [OpenTofu shortcuts](#opentofu-shortcuts)
  - [run](#run)
  - [exec](#exec)
  - [run-all](#run-all)
  - [graph](#graph)

The commands relevant to managing a Terragrunt stack are:

- [Stack commands](#stack-commands)
  - [stack generate](#stack-generate)
  - [stack run](#stack-run)

The commands relevant to managing an IaC catalog are:

- [Catalog commands](#catalog-commands)
  - [catalog](#catalog)
  - [scaffold](#scaffold)

The commands used for managing Terragrunt configuration itself are:

- [Configuration commands](#configuration-commands)
  - [graph-dependencies](#graph-dependencies)
  - [hclfmt](#hclfmt)
  - [hclvalidate](#hclvalidate)
  - [output-module-groups](#output-module-groups)
  - [render-json](#render-json)
  - [info](#terragrunt-info)
  - [validate-inputs](#validate-inputs)

### Main commands

#### OpenTofu shortcuts

Terragrunt is an orchestration tool for OpenTofu/Terraform, so with a couple exceptions, you can generally use it as a drop-in replacement for OpenTofu/Terraform. Terragrunt has a shortcut for most OpenTofu commands, you can usually just replace `tofu` or `terraform` with `terragrunt` and it will do what you expect.

For example:

```bash
# This will run `tofu/terraform apply` for you.
terragrunt apply
```

The list of shortcuts Terragrunt supports are:

- `apply`
- `destroy`
- `force-unlock`
- `import`
- `init`
- `output`
- `plan`
- `refresh`
- `show`
- `state`
- `test`
- `validate`

If you want to run a command that doesn't have a shortcut in Terragrunt, you can use the [`run`](#run) command.

#### run

**[NOTE] The `run` command is experimental, usage requires the [`--experiment cli-redesign` flag](/docs/reference/experiments/#cli-redesign).**

Run the provided OpenTofu/Terraform command against the unit in the current working directory.

Example:

```bash
terragrunt run plan
```

Note that the `run` command is a more explicit way to run OpenTofu/Terraform commands, and it provides some flexible options that are not available with the shortcut commands.

The `run` command also supports the following flags that can be used to drive runs in multiple units:

- [`--all`](#all): Run the provided OpenTofu/Terraform command against all units in the current stack. This is equivalent to the deprecated `run-all` command.
- [`--graph`](#graph): Run the provided OpenTofu/Terraform command against the graph of dependencies for the unit in the current working directory. This is equivalent to the deprecated `graph` command.

You may, at times, need to explicitly separate the flags used for Terragrunt from those used for OpenTofu/Terraform. In those circumstances, you can use the argument `--` to separate the Terragrunt flags from the OpenTofu/Terraform flags.

Example:

```bash
terragrunt run -- plan -no-color
```

#### exec

**[NOTE] The `exec` command is experimental, usage requires the [`--experiment cli-redesign` flag](/docs/reference/experiments/#cli-redesign).**

Execute an arbitrary command orchestrated by Terragrunt.

In contrast to the `run` command, which will always invoke OpenTofu/Terraform, the `exec` command allows for execution of any arbitrary command via Terragrunt.

This can be useful, as it allows you full control over the process that is being orchestrated by Terragrunt, while taking advantage of Terragrunt's features such as dependency resolution, inputs, and more.

Example:

```bash
terragrunt exec -- echo "Hello, Terragrunt!"
```

When using `exec`, you will have almost the exact same context that you have when using `run`, including inputs.

Example:

```hcl
inputs = {
  message = "Hello, Terragrunt!"
}
```

```bash
$ terragrunt exec -- env | grep 'TF_VAR_message'
TF_VAR_message=Hello, Terragrunt!
```

#### run-all

Runs the provided OpenTofu/Terraform command against a [stack](/docs/getting-started/terminology/#stack).
The command will recursively find terragrunt [units](/docs/getting-started/terminology/#unit) in the current directory
tree and run the OpenTofu/Terraform command in dependency order (unless the command is destroy,
in which case the command is run in reverse dependency order).

Make sure to read about the [stacks feature](/docs/features/stacks) for context.

Example:

```bash
terragrunt run-all apply
```

This will recursively search the current working directory for any folders that contain Terragrunt units and run
`apply` in each one, concurrently, while respecting ordering defined via
[`dependency`](/docs/reference/config-blocks-and-attributes/#dependency) and
[`dependencies`](/docs/reference/config-blocks-and-attributes/#dependencies) blocks.

**[WARNING] Do not set [TF_PLUGIN_CACHE_DIR](https://opentofu.org/docs/cli/config/config-file/#provider-plugin-cache) when using `run-all`**

Instead take advantage of the built-in [Provider Cache Server](/docs/features/provider-cache-server/) that
mitigates some of the limitations of using the OpenTofu/Terraform Provider Plugin Cache directly.

We are [working with the OpenTofu team to improve this behavior](https://github.com/opentofu/opentofu/issues/1483) so that you don't have to worry about this in the future.

**[NOTE] Use `run-all` with care if you have unapplied dependencies**.

If you have a stack of Terragrunt units with dependencies between them—either via `dependency` blocks
and you've never deployed them, then commands like `run-all plan` will fail,
as it will not be possible to resolve outputs of `dependency` blocks without applying first.

The solution for this is to take advantage of [mock outputs in dependency blocks](/docs/reference/config-blocks-and-attributes/#dependency).

**[NOTE]** Using `run-all` with `apply` or `destroy` silently adds the `-auto-approve` flag to the command line
arguments passed to OpenTofu/Terraform due to issues with shared `stdin` making individual approvals impossible.

**[NOTE]** Using the OpenTofu/Terraform [-detailed-exitcode](https://opentofu.org/docs/cli/commands/plan/#other-options)
flag with the `run-all` command results in an aggregate exit code being returned, rather than the exit code of any particular unit.

The algorithm for determining the aggregate exit code is as follows:

- If any unit throws a 1, Terragrunt will throw a 1.
- If any unit throws a 2, but nothing throws a 1, Terragrunt will throw a 2.
- If nothing throws a non-zero, Terragrunt will throw a 0.

#### graph

Run the provided OpenTofu/Terraform command against the graph of dependencies for the unit in the current working directory. The graph consists of all units that depend on the unit in the current working directory via a `dependency` or `dependencies` blocks, plus all the units that depend on those units, and all the units that depend on those units, and so on, recursively up the tree, up to the Git repository root, or the path specified via the optional `--graph-root` argument.

The Command will be executed following the order of dependencies: so it'll run on the unit in the current working directory first, then on units that depend on it directly, then on the units that depend on those units, and so on. Note that if the command is `destroy`, it will execute in the opposite order of the dependencies.

Example:
Having below dependencies:
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

- `lambda` units aren't included in the graph, because they are not dependent on `eks` unit.
- execution is from bottom up based on dependencies

Running `terragrunt graph destroy` in `eks` unit will lead to the following execution order:

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

- execution is in reverse order, first are destroyed "top" units and in the end `eks`
- `lambda` units aren't affected at all

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

### Stack commands

The `terragrunt stack` commands provide an interface for managing collections of Terragrunt units defined in `terragrunt.stack.hcl` files.
These commands simplify the process of handling multiple infrastructure units by grouping them into a "stack", reducing code duplication and streamlining operations across environments.

**[NOTE] `stack` commands are experimental, usage requires the [`--experiment stacks` flag](/docs/reference/experiments/#stacks).**

#### stack generate

The `stack generate` command is used to generate a stack of `terragrunt.hcl` files based on the configuration provided in the `terragrunt.stack.hcl` file.

Given the following `terragrunt.stack.hcl` configuration:

```hcl
locals {
  version = "v0.68.4"
}

unit "app1" {
  source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=${local.version}"
  path   = "app1"
}

unit "app2" {
  source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=${local.version}"
  path   = "app2"
}

```

Executing generate:

```bash
terragrunt stack generate
```

Will create the following directory structure:

```tree
.stack/
├── app1/
│   └── terragrunt.hcl
└── app2/
    └── terragrunt.hcl
```

#### stack run

The `stack run *` command allows users to execute IaC commands across all units defined in a `terragrunt.stack.hcl` file.
This feature facilitates efficient orchestration of operations on multiple units, simplifying workflows for managing complex infrastructure stacks.

**Examples:**

Run a plan on each unit:

```bash
terragrunt stack run plan
```

Apply changes for each unit:

```bash
terragrunt stack run apply
```

Destroy all units:

```bash
terragrunt stack run destroy
```

**Note:**

Before executing the specified command, the `terragrunt stack run *` command will automatically generate the stack by creating
the `.terragrunt-stack` directory using the `terragrunt.stack.hcl` configuration file.
This ensures that all units are up-to-date before running the requested operation.

### info

Emits limited terragrunt state on `stdout` in a JSON format and exits.

Example:

```bash
terragrunt info
```

Might produce output such as:

```json
{
  "ConfigPath": "/example/path/terragrunt.hcl",
  "DownloadDir": "/example/path/.cache",
  "IamRole": "",
  "TerraformBinary": "terraform",
  "TerraformCommand": "info",
  "WorkingDir": "/example/path"
}
```

### Catalog commands

#### catalog

Launch the user interface for searching and managing your module catalog.

More details in [catalog section](https://terragrunt.gruntwork.io/docs/features/catalog/).

#### scaffold

Generate Terragrunt files from existing OpenTofu/Terraform modules.

More details in [scaffold section](https://terragrunt.gruntwork.io/docs/features/scaffold/).

### Configuration commands

#### graph-dependencies

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

#### hclfmt

Recursively find hcl files and rewrite them into a canonical format.

Example:

```bash
terragrunt hclfmt
```

This will recursively search the current working directory for any folders that contain Terragrunt configuration files
and run the equivalent of `tofu fmt`/`terraform fmt` on them.

#### hclvalidate

Find all hcl files from the configuration stack and validate them.

Example:

```bash
terragrunt hclvalidate
```

This will search all hcl files from the configuration stack in the current working directory and run the equivalent
of `tofu validate`/`terraform validate` on them.

For convenience in programmatically parsing these findings, you can also pass the `--json` flag to output the results in JSON format.

Example:

```bash
terragrunt hclvalidate --json
```

In addition, you can pass the `--show-config-path` flag to only output paths of the invalid config files, delimited by newlines. This can be especially useful when combined with the [queue-excludes-file](#queue-excludes-file) flag.

Example:

```bash
terragrunt hclvalidate --show-config-path
```

#### output-module-groups

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
such as [`--units-that-include`](#units-that-include)

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

#### render-json

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

You can use the CLI option `--out` to configure where terragrunt renders out the json representation.

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

#### terragrunt-info

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

#### validate-inputs

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

To enable strict mode, you can pass the `--strict-validate` flag like so:

```bash
> terragrunt validate-inputs --strict-validate
```

When running in strict mode, `validate-inputs` will return an error if there are unused inputs.

This command will exit with an error if terragrunt detects any unused inputs or undefined required inputs.

## Flags

- [all](#all)
- [auth-provider-cmd](#auth-provider-cmd)
- [config](#config)
- [tf-path](#tf-path)
- [no-auto-init](#no-auto-init)
- [no-auto-approve](#no-auto-approve)
- [no-auto-retry](#no-auto-retry)
- [non-interactive](#non-interactive)
- [working-dir](#working-dir)
- [download-dir](#download-dir)
- [source](#source)
- [source-map](#source-map)
- [source-update](#source-update)
- [iam-assume-role](#iam-assume-role)
- [iam-assume-role-duration](#iam-assume-role-duration)
- [iam-assume-role-session-name](#iam-assume-role-session-name)
- [iam-assume-role-web-identity-token](#iam-assume-role-web-identity-token)
- [queue-ignore-errors](#queue-ignore-errors)
- [queue-excludes-file](#queue-excludes-file)
- [queue-exclude-dir](#queue-exclude-dir)
- [queue-include-dir](#queue-include-dir)
- [queue-strict-include](#queue-strict-include)
- [strict-validate](#strict-validate)
- [queue-ignore-dag-order](#queue-ignore-dag-order)
- [queue-exclude-external](#queue-exclude-external)
- [queue-include-external](#queue-include-external)
- [parallelism](#parallelism)
- [inputs-debug](#inputs-debug)
- [log-level](#log-level)
- [log-format](#log-format)
- [log-custom-format](#log-custom-format)
- [log-disable](#log-disable)
- [log-show-abs-paths](#log-show-abs-paths)
- [no-color](#no-color)
- [check](#check)
- [diff](#diff)
- [hclfmt-file](#hclfmt-file)
- [hclfmt-exclude-dir](#hclfmt-exclude-dir)
- [hclfmt-stdin](#hclfmt-stdin)
- [hclvalidate-json](#hclvalidate-json)
- [hclvalidate-show-config-path](#hclvalidate-show-config-path)
- [out](#out)
- [disable-dependent-modules](#disable-dependent-modules)
- [units-that-include](#units-that-include)
- [dependency-fetch-output-from-state](#dependency-fetch-output-from-state)
- [use-partial-parse-config-cache](#use-partial-parse-config-cache)
- [backend-require-bootstrap](#backend-require-bootstrap)
- [disable-bucket-update](#disable-bucket-update)
- [disable-command-validation](#disable-command-validation)
- [provider-cache](#provider-cache)
- [provider-cache-dir](#provider-cache-dir)
- [provider-cache-hostname](#provider-cache-hostname)
- [provider-cache-port](#provider-cache-port)
- [provider-cache-token](#provider-cache-token)
- [provider-cache-registry-names](#provider-cache-registry-names)
- [out-dir](#out-dir)
- [json-out-dir](#json-out-dir)
- [tf-forward-stdout](#tf-forward-stdout)
- [no-destroy-dependencies-check](#no-destroy-dependencies-check)
- [feature](#feature)
- [experiment](#experiment)
- [experiment-mode](#experiment-mode)
- [strict-control](#strict-control)
- [strict-mode](#strict-mode)
- [in-download-dir](#in-download-dir)

### all

<!-- markdownlint-disable MD033 -->

**CLI Arg**: `--all`<br/>
**Environment Variable**: `TG_ALL` (set to `true`)<br/>

Run the provided OpenTofu/Terraform command against all units in the current stack. This is equivalent to the deprecated `run-all` command.

See [Stacks](/docs/features/stacks/) for more information.

### auth-provider-cmd

**CLI Arg**: `--auth-provider-cmd`<br/>
**CLI Arg Alias**: `--terragrunt-auth-provider-cmd` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_AUTH_PROVIDER_CMD`<br/>
**Environment Variable Alias**: `TERRAGRUNT_AUTH_PROVIDER_CMD` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--auth-provider-cmd "command [arguments]"`<br/>

The command and arguments used to obtain authentication credentials dynamically. If specified, Terragrunt runs this command whenever it might need authentication. This includes HCL parsing, where it might be useful to authenticate with a cloud provider _before_ running HCL functions like [`get_aws_account_id`](/docs/reference/built-in-functions/#get_aws_account_id) where authentication has to already have taken place. It can also be useful for HCL functions like [`run_cmd`](/docs/reference/built-in-functions/#run_cmd) where it may be useful to be authenticated before calling the function.

The output must be valid JSON of the following schema:

```json
{
  "awsCredentials": {
    "ACCESS_KEY_ID": "",
    "SECRET_ACCESS_KEY": "",
    "SESSION_TOKEN": ""
  },
  "awsRole": {
    "roleARN": "",
    "sessionName": "",
    "duration": 0,
    "webIdentityToken": ""
  },
  "envs": {
    "ANY_KEY": ""
  }
}
```

This allows Terragrunt to acquire different credentials at runtime without changing any `terragrunt.hcl` configuration. You can use this flag to set arbitrary credentials for continuous integration, authentication with providers other than AWS and more.

As long as the standard output of the command passed to `auth-provider-cmd` results in JSON matching the schema above, corresponding environment variables will be set (and/or roles assumed) before Terragrunt begins parsing an `terragrunt.hcl` file or running an OpenTofu/Terraform command.

The simplest approach to leverage this flag is to write a script that fetches desired credentials, and emits them to STDOUT in the JSON format listed above:

```bash
#!/usr/bin/env bash

echo -n '{"envs": {"KEY": "a secret"}}'
```

You can use any technology for the authentication provider you'd like, however, as long as Terragrunt can execute it. The expected pattern for using this flag is to author a script/program that will dynamically fetch secret values from a secret store, etc. then emit them to STDOUT for consumption by Terragrunt.

Note that more specific configurations (e.g. `awsCredentials`) take precedence over less specific configurations (e.g. `envs`).

If you would like to set credentials for AWS with this method, you are encouraged to use `awsCredentials` instead of `envs`, as these keys will be validated to conform to the officially supported environment variables expected by the AWS SDK.

Similarly, if you would like Terragrunt to assume an AWS role on your behalf, you are encouraged to use the `awsRole` configuration instead of `envs`.

Other credential configurations will be supported in the future, but until then, if your provider authenticates via environment variables, you can use the `envs` field to fetch credentials dynamically from a secret store, etc before Terragrunt executes any IAC.

**Note**: The `awsRole` configuration is only used when the `awsCredentials` configuration is not present. If both are present, the `awsCredentials` configuration will take precedence.

### config

**CLI Arg**: `--config`<br/>
**CLI Arg Alias**: `--terragrunt-config` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_CONFIG`<br/>
**Environment Variable Alias**: `TERRAGRUNT_CONFIG` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--config /path/to/terragrunt.hcl`<br/>

A custom path to the `terragrunt.hcl` or `terragrunt.hcl.json` file. The
default path is `terragrunt.hcl` (preferred) or `terragrunt.hcl.json` in the current directory (see
[Configuration]({{site.baseurl}}/docs/getting-started/configuration/#configuration) for a slightly more nuanced
explanation). This argument is not used with the `run-all` commands.

### tf-path

**CLI Arg**: `--tf-path`<br/>
**CLI Arg Alias**: `--terragrunt-tfpath` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_TF_PATH`<br/>
**Environment Variable Alias**: `TERRAGRUNT_TFPATH` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--tf-path /path/to/tofu-or-terraform-binary`<br/>

An explicit path to the `tofu` or `terraform` binary you wish to have Terragrunt use.

Note that if you _only_ have `terraform` installed, and available in your PATH, Terragrunt will automatically use that binary.

If you have _both_ `terraform` and `tofu` installed, and you want to use `terraform`, you can set the `TG_TF_PATH` to `terraform`.

If you have _multiple_ versions of `tofu` and/or `terraform` available, or you have a custom wrapper for `tofu` or `terraform`, you can set the `TG_TF_PATH` to the absolute path of the executable you want to use.

**NOTE**: This will override the `terraform` binary that is used by `terragrunt` in all instances, including
`dependency` lookups. This setting will also override any [terraform_binary]({{site.baseurl}}/docs/reference/config-blocks-and-attributes/#terraform_binary)
configuration values specified in the `terragrunt.hcl` config for both the top level, and dependency lookups.

### no-auto-init

**CLI Arg**: `--no-auto-init`<br/>
**CLI Arg Alias**: `--terragrunt-no-auto-init` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_NO_AUTO_INIT` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_NO_AUTO_INIT` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
_(Prior to Terragrunt v0.48.6, this environment variable was called `TERRAGRUNT_AUTO_INIT` (set to `false`), and is still available for backwards compatibility)_

When passed in, don't automatically run `terraform init` when other commands are run (e.g. `terragrunt apply`). Useful
if you want to pass custom arguments to `terraform init` that are specific to a user or execution environment, and
therefore cannot be specified as `extra_arguments`. For example, `-plugin-dir`. You must run `terragrunt init`
yourself in this case if needed. `terragrunt` will fail if it detects that `init` is needed, but auto init is
disabled. See [Auto-Init]({{site.baseurl}}/docs/features/auto-init#auto-init)

### no-auto-approve

**CLI Arg**: `--no-auto-approve`<br/>
**CLI Arg Alias**: `--terragrunt-no-auto-approve` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_NO_AUTO_APPROVE` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_NO_AUTO_APPROVE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
_(Prior to Terragrunt v0.48.6, this environment variable was called `TERRAGRUNT_AUTO_APPROVE` (set to `false`), and is still available for backwards compatibility)_
**Commands**:

- [run-all](#run-all)

When passed in, Terragrunt will no longer automatically append `-auto-approve` to the underlying OpenTofu/Terraform commands run
with `run-all`. Note that due to the interactive prompts, this flag will also **automatically assume
`--parallelism 1`**.

### no-auto-retry

**CLI Arg**: `--no-auto-retry`<br/>
**CLI Arg Alias**: `--terragrunt-no-auto-retry` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_NO_AUTO_RETRY` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_NO_AUTO_RETRY` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
_(Prior to Terragrunt v0.48.6, this environment variable was called `TERRAGRUNT_AUTO_RETRY` (set to `false`), and is still available for backwards compatibility)_

When passed in, don't automatically retry commands which fail with transient errors. See
[Feature Flags, Errors and Excludes]({{site.baseurl}}/docs/features/runtime-control#errors)

### non-interactive

**CLI Arg**: `--non-interactive`<br/>
**CLI Arg Alias**: `--terragrunt-non-interactive` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_NON_INTERACTIVE` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_NON_INTERACTIVE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
_(Prior to Terragrunt v0.48.6, this environment variable was called `TF_INPUT` (set to `false`), and is still available for backwards compatibility. NOTE: [TF_INPUT](https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_input) is native to OpenTofu/Terraform!)_

When passed in, don't show interactive user prompts. This will default the answer for all Terragrunt (not OpenTofu/Terraform) prompts to `yes` except for
the listed cases below. This is useful if you need to run Terragrunt in an automated setting (e.g. from a script). May
also be specified with the [TF_INPUT](https://www.terraform.io/docs/configuration/environment-variables.html#tf_input) environment variable.

This setting will default to `no` for the following cases:

- Prompts related to pulling in external dependencies. You can force include external dependencies using the
  [--queue-include-external](#queue-include-external) option.

Note that this does not impact the behavior of OpenTofu/Terraform commands invoked by Terragrunt.

e.g.

```bash
terragrunt --non-interactive apply -auto-approve
```

Is how you would make Terragrunt apply without any user prompts from Terragrunt or OpenTofu/Terraform.

### working-dir

**CLI Arg**: `--working-dir`<br/>
**CLI Arg Alias**: `--terragrunt-working-dir` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_WORKING_DIR`<br/>
**Environment Variable Alias**: `TERRAGRUNT_WORKING_DIR` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--working-dir /path/to/working-directory`<br/>

Set the directory where Terragrunt should execute the `terraform` command. Default is the current working directory.
Note that for the `run-all` commands, this parameter has a different meaning: Terragrunt will apply or destroy all the
OpenTofu/Terraform modules in the subfolders of the `working-dir`, running `terraform` in the root of each module it
finds.

### download-dir

**CLI Arg**: `--download-dir`<br/>
**CLI Arg Alias**: `--terragrunt-download-dir` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_DOWNLOAD_DIR`<br/>
**Environment Variable Alias**: `TERRAGRUNT_DOWNLOAD` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--download-dir /path/to/dir-to-download-terraform-code`<br/>

The path where to download OpenTofu/Terraform code when using [remote OpenTofu/Terraform
configurations](https://blog.gruntwork.io/terragrunt-how-to-keep-your-terraform-code-dry-and-maintainable-f61ae06959d8).
Default is `.terragrunt-cache` in the working directory. We recommend adding this folder to your `.gitignore`.

### source

**CLI Arg**: `--source`<br/>
**CLI Arg Alias**: `--terragrunt-source` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_SOURCE`<br/>
**Environment Variable Alias**: `TERRAGRUNT_SOURCE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--source /path/to/local-terraform-code`<br/>

Download OpenTofu/Terraform configurations from the specified source into a temporary folder, and run OpenTofu/Terraform in that temporary
folder. The source should use the same syntax as the [OpenTofu/Terraform module
source](https://www.terraform.io/docs/modules/sources.html) parameter. If you specify this argument for the `run-all`
commands, Terragrunt will assume this is the local file path for all of your OpenTofu/Terraform modules, and for each module
processed by the `run-all` command, Terragrunt will automatically append the path of `source` parameter in each module
to the `--source` parameter you passed in.

### source-map

**CLI Arg**: `--source-map`<br/>
**CLI Arg Alias**: `--terragrunt-source-map` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_SOURCE_MAP` (encoded as comma separated value, e.g., `source1=dest1,source2=dest2`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_SOURCE_MAP` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--source-map git::ssh://github.com=/path/to/local-terraform-code`<br/>

Can be supplied multiple times: `--source-map source1=dest1 --source-map source2=dest2`

The `--source-map source=dest` param replaces any `source` URL (including the source URL of a config pulled
in with `dependency` blocks) that has root `source` with `dest`.

For example:

```bash
terragrunt apply --source-map github.com/org/modules.git=/local/path/to/modules
```

The above would replace `terraform { source = "github.com/org/modules.git//xxx" }` with `terraform { source = /local/path/to/modules//xxx }` regardless of
whether you were running `apply`, or `run-all`, or using a `dependency`.

**NOTE**: This setting is ignored if you pass in `--source`.

Note that this only performs literal matches on the URL portion. For example, a map key of
`ssh://git@github.com/gruntwork-io/terragrunt.git` will only match terragrunt configurations with source `source =
"ssh://git@github.com/gruntwork-io/terragrunt.git//xxx"` and not sources of the form `source =
"git::ssh://git@github.com/gruntwork-io/terragrunt.git//xxx"`. The latter requires a map key of
`git::ssh://git@github.com/gruntwork-io/terragrunt.git`.

### source-update

**CLI Arg**: `--source-update`<br/>
**CLI Arg Alias**: `--terragrunt-source-update` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_SOURCE_UPDATE` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_SOURCE_UPDATE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

When passed in, delete the contents of the temporary folder before downloading OpenTofu/Terraform source code into it.

### iam-assume-role

**CLI Arg**: `--iam-assume-role`<br/>
**CLI Arg Alias**: `--terragrunt-iam-role` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_IAM_ASSUME_ROLE`<br/>
**Environment Variable Alias**: `TERRAGRUNT_IAM_ROLE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--iam-assume-role "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"`<br/>

Assume the specified IAM role ARN before running OpenTofu/Terraform or AWS commands. This is a convenient way to use Terragrunt
and OpenTofu/Terraform with multiple AWS accounts.

When using this option, AWS authentication takes place right before an OpenTofu/Terraform run. This takes place after `terragrunt.hcl` files are fully parsed, so HCL functions like [`get_aws_account_id`](/docs/reference/built-in-functions/#get_aws_account_id) and [`run_cmd`](/docs/reference/built-in-functions/#run_cmd) will not run after assuming the role. If you need roles to be assumed prior to parsing Terragrunt configurations, use [`auth-provider-cmd`](#auth-provider-cmd) instead.

### iam-assume-role-duration

**CLI Arg**: `--iam-assume-role-duration`<br/>
**CLI Arg Alias**: `--terragrunt-iam-assume-role-duration` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_IAM_ASSUME_ROLE_DURATION`<br/>
**Environment Variable Alias**: `TERRAGRUNT_IAM_ASSUME_ROLE_DURATION` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--iam-assume-role-duration 3600`<br/>

Uses the specified duration as the session duration (in seconds) for the STS session which assumes the role defined in `--iam-assume-role`.

### iam-assume-role-session-name

**CLI Arg**: `--iam-assume-role-session-name`<br/>
**CLI Arg Alias**: `--terragrunt-iam-assume-role-session-name` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_IAM_ASSUME_ROLE_SESSION_NAME`<br/>
**Environment Variable Alias**: `TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--iam-assume-role-session-name "terragrunt-iam-role-session-name"`<br/>

Used as the session name for the STS session which assumes the role defined in `--iam-assume-role`.

### iam-assume-role-web-identity-token

**CLI Arg**: `--iam-assume-role-web-identity-token`<br/>
**CLI Arg Alias**: `--terragrunt-iam-web-identity-token` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN`<br/>
**Environment Variable Alias**: `TERRAGRUNT_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--iam-assume-role-web-identity-token [/path/to/web-identity-token | web-identity-token-value]`<br/>

Used as the web identity token for assuming a role temporarily using the AWS Security Token Service (STS) with the [AssumeRoleWithWebIdentity](https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRoleWithWebIdentity.html) API.

### queue-ignore-errors

**CLI Arg**: `--queue-ignore-errors`<br/>
**CLI Arg Alias**: `--terragrunt-ignore-dependency-errors` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_QUEUE_IGNORE_ERRORS`<br/>
**Environment Variable Alias**: `TERRAGRUNT_IGNORE_DEPENDENCY_ERRORS` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

When passed in, the `*-all` commands continue processing components even if a dependency fails

### queue-excludes-file

**CLI Arg**: `--queue-excludes-file`<br/>
**CLI Arg Alias**: `--terragrunt-excludes-file` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_QUEUE_EXCLUDES_FILE`<br/>
**Environment Variable Alias**: `TERRAGRUNT_EXCLUDES_FILE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--queue-excludes-file /path/to/file`<br/>

Path to a file with a list of directories that need to be excluded when running *-all commands, by default `.terragrunt-excludes`. Modules under these directories will be
excluded during execution of the commands. If a relative path is specified, it should be relative from
[--working-dir](#working-dir). This will only exclude the module, not its dependencies.

This flag has been designed to integrate nicely with the `hclvalidate` command, which can return a list of invalid files delimited by newlines when passed the `--show-config-path` flag. To integrate the two, you can run something like the following using bash process substitution:

```bash
terragrunt run-all plan --queue-excludes-file <(terragrunt hclvalidate --show-config-path)
```

### queue-exclude-dir

**CLI Arg**: `--queue-exclude-dir`<br/>
**CLI Arg Alias**: `--terragrunt-exclude-dir` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_QUEUE_EXCLUDE_DIR`<br/>
**Environment Variable Alias**: `TERRAGRUNT_EXCLUDE_DIR` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--queue-exclude-dir /path/to/dirs/to/exclude*`<br/>

Can be supplied multiple times: `--queue-exclude-dir /path/to/dirs/to/exclude --queue-exclude-dir /another/path/to/dirs/to/exclude`

Unix-style glob of directories to exclude when running `*-all` commands. Modules under these directories will be
excluded during execution of the commands. If a relative path is specified, it should be relative from
[--working-dir](#working-dir). Flag can be specified multiple times. This will only exclude the
module, not its dependencies.

Please note that the glob curly braces expansion is not taken in account using environment variable unlike of its equivalent as a parameter on the command line.
You should consider using `TG_QUEUE_EXCLUDE_DIR="foo/module,bar/module"` instead of `TG_QUEUE_EXCLUDE_DIR="{foo,bar}/module"`.

### queue-include-dir

**CLI Arg**: `--queue-include-dir`<br/>
**CLI Arg Alias**: `--terragrunt-include-dir` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_QUEUE_INCLUDE_DIR`<br/>
**Environment Variable Alias**: `TERRAGRUNT_INCLUDE_DIR` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--queue-include-dir /path/to/dirs/to/include*`<br/>

Can be supplied multiple times: `--queue-include-dir /path/to/dirs/to/include --queue-include-dir /another/path/to/dirs/to/include`

Unix-style glob of directories to include when running `*-all` commands. Only modules under these directories (and all
dependent modules) will be included during execution of the commands. If a relative path is specified, it should be
relative from `--working-dir`. Flag can be specified multiple times.

Please note that the glob curly braces expansion is not taken in account using environment variable unlike of its equivalent as a parameter on the command line.
You should consider using `TG_QUEUE_INCLUDE_DIR="foo/module,bar/module"` instead of `TG_QUEUE_INCLUDE_DIR="{foo,bar}/module"`.

### queue-strict-include

**CLI Arg**: `--queue-strict-include`<br/>
**CLI Arg Alias**: `--terragrunt-strict-include` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_QUEUE_STRICT_INCLUDE`<br/>
**Environment Variable Alias**: `TERRAGRUNT_STRICT_INCLUDE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

When passed in, only modules under the directories passed in with [--queue-include-dir](#queue-include-dir)
will be included. All dependencies of the included directories will be excluded if they are not in the included
directories. If no [--queue-include-dir](#queue-include-dir) flags are included, terragrunt will not include
any modules during the execution of the commands.

### queue-ignore-dag-order

**CLI Arg**: `--queue-ignore-dag-order`<br/>
**CLI Arg Alias**: `--terragrunt-ignore-dependency-order` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_QUEUE_IGNORE_DAG_ORDER`<br/>
**Environment Variable Alias**: `TERRAGRUNT_IGNORE_DEPENDENCY_ORDER` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

When passed in, ignore the dependencies between units when running `*-all` commands.

### queue-exclude-external

**CLI Arg**: `--queue-exclude-external`<br/>
**CLI Arg Alias**: `--terragrunt-ignore-external-dependencies` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_QUEUE_EXCLUDE_EXTERNAL`<br/>
**Environment Variable Alias**: `TERRAGRUNT_IGNORE_EXTERNAL_DEPENDENCIES` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

When passed in, don't attempt to include any external dependencies when running `*-all` commands. Note that an external
dependency is a dependency that is outside the current terragrunt working directory, and is not respective to the
included directories with `queue-include-dir`.

### queue-include-external

**CLI Arg**: `--queue-include-external`<br/>
**CLI Arg Alias**: `--terragrunt-include-external-dependencies` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_QUEUE_INCLUDE_EXTERNAL`<br/>
**Environment Variable Alias**: `TERRAGRUNT_INCLUDE_EXTERNAL_DEPENDENCIES` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

When passed in, include any external dependencies when running `*-all` without asking. Note that an external
dependency is a dependency that is outside the current terragrunt working directory, and is not respective to the
included directories with `queue-include-dir`.

### strict-validate

**CLI Arg**: `--strict-validate`<br/>
**CLI Arg Alias**: `--terragrunt-strict-validate` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_STRICT_VALIDATE`<br/>
**Environment Variable Alias**: `TERRAGRUNT_STRICT_VALIDATE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

When passed in, and running `terragrunt validate-inputs`, enables strict mode for the `validate-inputs` command. When strict mode is enabled, an error will be returned if any variables required by the underlying OpenTofu/Terraform configuration are not passed in, OR if any unused variables are passed in. By default, `terragrunt validate-inputs` runs in relaxed mode. In relaxed mode, an error is only returned when a variable required by the underlying OpenTofu/Terraform configuration is not passed in.

### parallelism

**CLI Arg**: `--parallelism`<br/>
**CLI Arg Alias**: `--terragrunt-parallelism` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_PARALLELISM`<br/>
**Environment Variable Alias**: `TERRAGRUNT_PARALLELISM` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

When passed in, limit the number of units that are run concurrently to this number during \*-all commands.
The exception is the `terraform init` command, which is always executed sequentially if the [OpenTofu provider plugin cache](https://opentofu.org/docs/cli/config/config-file/#provider-plugin-cache) is used. This is because the provider plugin cache is not guaranteed to be concurrency safe when used in isolation.

To safely access provider cache concurrently, enable the [Provider Cache Server](https://terragrunt.gruntwork.io/docs/features/provider-cache-server/).

### inputs-debug

**CLI Arg**: `--inputs-debug`<br/>
**CLI Arg Alias**: `--terragrunt-debug` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_DEBUG_INPUTS`<br/>
**Environment Variable Alias**: `TERRAGRUNT_DEBUG` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

When passed in, Terragrunt will create a tfvars file that can be used to invoke the terraform module in the same way
that Terragrunt invokes the module, so that you can debug issues with the terragrunt config. See
[Debugging]({{site.baseurl}}/docs/features/debugging) for additional details.

### log-level

**CLI Arg**: `--log-level`<br/>
**CLI Arg Alias**: `--terragrunt-log-level` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_LOG_LEVEL`<br/>
**Environment Variable Alias**: `TERRAGRUNT_LOG_LEVEL` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--log-level <LOG_LEVEL>`<br/>

When passed it, sets logging level for terragrunt. All supported levels are:

- `stderr`
- `stdout`
- `error`
- `warn`
- `info` (this is the default)
- `debug`
- `trace`

Where the first two control the logging of Terraform/OpenTofu output.

### log-format

**CLI Arg**: `--log-format`<br/>
**CLI Arg Alias**: `--terragrunt-log-format` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_LOG_FORMAT`<br/>
**Environment Variable Alias**: `TERRAGRUNT_LOG_FORMAT` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--log-format <LOG_FORMAT>`<br/>

There are four log format presets:

- `pretty` (this is the default)
- `bare` (old Terragrunt logging, pre-[v0.67.0](https://github.com/gruntwork-io/terragrunt/tree/v0.67.0))
- `json`
- `key-value`

### log-custom-format

**CLI Arg**: `--log-custom-format`<br/>
**CLI Arg Alias**: `--terragrunt-log-custom-format` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_LOG_CUSTOM_FORMAT`<br/>
**Environment Variable Alias**: `TERRAGRUNT_LOG_CUSTOM_FORMAT` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--log-custom-format <LOG_CUSTOM_FORMAT>`<br/>

This allows you to customize logging however you like.

Make sure to read [Custom Log Format](https://terragrunt.gruntwork.io/docs/features/log-formatting) for syntax details.

### log-disable

**CLI Arg**: `--log-disable`<br/>
**CLI Arg Alias**: `--terragrunt-log-disable` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_LOG_DISABLE`<br/>
**Environment Variable Alias**: `TERRAGRUNT_LOG_DISABLE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

Disable logging. This flag also enables [tf-forward-stdout](#tf-forward-stdout).

### log-show-abs-paths

**CLI Arg**: `--log-show-abs-paths`<br/>
**CLI Arg Alias**: `--terragrunt-log-show-abs-paths` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_LOG_SHOW_ABS_PATHS`<br/>
**Environment Variable Alias**: `TERRAGRUNT_LOG_SHOW_ABS_PATHS` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

If specified, Terragrunt paths in logs will be absolute. By default, the paths are relative to the working directory.

### no-color

**CLI Arg**: `--no-color`<br/>
**CLI Arg Alias**: `--terragrunt-no-color` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_NO_COLOR`<br/>
**Environment Variable Alias**: `TERRAGRUNT_NO_COLOR` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

If specified, Terragrunt output won't contain any color.

NOTE: This option also disables OpenTofu/Terraform output colors by propagating the OpenTofu/Terraform [`-no-color`](https://developer.hashicorp.com/terraform/cli/commands/plan#no-color) argument.

### check

**CLI Arg**: `--check`<br/>
**CLI Arg Alias**: `--terragrunt-check` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_HCLFMT_CHECK` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_CHECK` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [hclfmt](#hclfmt)

When passed in, run `hclfmt` in check only mode instead of actively overwriting the files. This will cause the
command to exit with exit code 1 if there are any files that are not formatted.

### diff

**CLI Arg**: `--diff`<br/>
**CLI Arg Alias**: `--terragrunt-diff` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_HCLFMT_DIFF` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_DIFF` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [hclfmt](#hclfmt)

When passed in, running `hclfmt` will print diff between original and modified file versions.

### hclfmt-file

**CLI Arg**: `--file`<br/>
**CLI Arg Alias**: `--terragrunt-hclfmt-file` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_HCLFMT_FILE` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_HCLFMT_FILE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--file /path/to/terragrunt.hcl`<br/>
**Commands**:

- [hclfmt](#hclfmt)

When passed in, run `hclfmt` only on the specified file.

### hclfmt-exclude-dir

**CLI Arg**: `--exclude-dir`<br/>
**CLI Arg Alias**: `--terragrunt-hclfmt-exclude-dir` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_HCLFMT_EXCLUDE_DIR`<br/>
**Environment Variable Alias**: `TERRAGRUNT_HCLFMT_EXCLUDE_DIR` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--exclude-dir /path/to/dir`<br/>
**Commands**:

- [hclfmt](#hclfmt)

Can be supplied multiple times: `--exclude-dir .back --exclude-dir .archive`<br/>
When passed in, `hclfmt` will ignore files in the specified directories.

### hclfmt-stdin

**CLI Arg**: `--stdin`<br/>
**CLI Arg Alias**: `--terragrunt-hclfmt-stdin` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_HCLFMT_STDIN` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_HCLFMT_STDIN` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [hclfmt](#hclfmt)

When passed in, run `hclfmt` only on hcl passed to `stdin`, result is printed to `stdout`.

### hclvalidate-json

**CLI Arg**: `--json`<br/>
**CLI Arg Alias**: `--terragrunt-hclvalidate-json` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_HCLVALIDATE_JSON` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_HCLVALIDATE_JSON` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [hclvalidate](#hclvalidate)

When passed in, render the output in the JSON format.

### hclvalidate-show-config-path

**CLI Arg**: `--show-config-path`<br/>
**CLI Arg Alias**: `--terragrunt-hclvalidate-show-config-path` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_HCLVALIDATE_SHOW_CONFIG_PATH` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_HCLVALIDATE_SHOW_CONFIG_PATH` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [hclvalidate](#hclvalidate)

When passed in, output a list of files with invalid configuration.

### disable-dependent-modules

**CLI Arg**: `--disable-dependent-modules`<br/>
**CLI Arg Alias**: `--terragrunt-json-disable-dependent-modules` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_RENDER_JSON_DISABLE_DEPENDENT_MODULES` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_JSON_DISABLE_DEPENDENT_MODULES` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [render-json](#render-json)

When `--disable-dependent-modules` is set, the process of identifying dependent modules will be disabled during JSON rendering.
This lead to a faster rendering process, but the output will not include any dependent units.

### out

**CLI Arg**: `--out`<br/>
**CLI Arg Alias**: `--terragrunt-json-out` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_RENDER_JSON_OUT` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_JSON_OUT` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--out /path/to/terragrunt_rendered.json`<br/>
**Commands**:

- [render-json](#render-json)

When passed in, render the json representation in this file.

### units-that-include

**CLI Arg**: `--units-that-include`<br/>
**CLI Arg Alias**: `--terragrunt-modules-that-include` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_UNITS_THAT_INCLUDE`<br/>
**Environment Variable Alias**: `TERRAGRUNT_MODULES_THAT_INCLUDE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Requires an argument**: `--units-that-include /path/to/included-terragrunt.hcl`<br/>
**Commands**:

- [run](#run)
- [run-all](#run-all)

When passed in, `run-all` will only run the command against Terragrunt modules that include the specified file.

This applies to the set of modules that are identified based on all the existing criteria for deciding which modules to
include. For example, consider the following folder structure:

```tree
.
├── _envcommon
│   └── data-stores
│       └── aurora.hcl
├── dev
│   └── us-west-2
│       └── dev
│           ├── data-stores
│           │   └── aurora
│           │       └── terragrunt.hcl
│           └── networking
│               └── vpc
│                   └── terragrunt.hcl
└── stage
    └── us-west-2
        └── stage
            ├── data-stores
            │   └── aurora
            │       └── terragrunt.hcl
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

If you run the command `run-all init --units-that-include ../_envcommon/data-stores/aurora.hcl` from the
`dev` folder, only `dev/us-west-2/dev/data-stores/aurora` will be run; not `stage/us-west-2/stage/data-stores/aurora`.
This is because `run-all` by default restricts the modules to only those that are direct descendents of the current
folder you are running from. If you also pass in `--queue-include-dir ../stage`, then it will now include
`stage/us-west-2/stage/data-stores/aurora` because now the `stage` folder is in consideration.

In other words, Terragrunt will always first find all the modules that should be included before applying this filter,
and then will apply this filter on the set of modules that it found.

You can pass this argument in multiple times to provide a list of include files to consider. When multiple files are
passed in, the set will be the union of modules that includes at least one of the files in the list.

**NOTE**: When using relative paths, the paths are relative to the working directory. This is either the current working
directory, or any path passed in to [working-dir](#working-dir).

**TIP**: This flag is functionally covered by the `--terragrunt-queue-include-units-reading` flag, but is more explicitly
only for the `include` configuration block.

### terragrunt-queue-include-units-reading

**CLI Arg**: `--terragrunt-queue-include-units-reading`<br/>
**Environment Variable**: `TERRAGRUNT_QUEUE_INCLUDE_UNITS_READING`<br/>
**CLI Arg Alias**: `` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable Alias**: `` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [run](#run)
- [run-all](#run-all)

This flag works very similarly to the `--units-that-include` flag, but instead of looking only for included configurations,
it also looks for configurations that read a given file.

When passed in, the `*-all` commands will include all units (modules) that read a given file into the queue. This is useful
when you want to trigger an update on all units that read or include a given file using HCL functions in their configurations.

Consider the following folder structure:

```tree
.
├── reading-shared-hcl
│   └── terragrunt.hcl
├── also-reading-shared-hcl
│   └── terragrunt.hcl
├── not-reading-shared-hcl
│   └── terragrunt.hcl
└── shared.hcl
```

Suppose that `reading-shared-hcl` and `also-reading-shared-hcl` both read `shared.hcl` in their configurations, like so:

```hcl
locals {
 shared = read_terragrunt_config(find_in_parent_folders("shared.hcl"))
}
```

If you run the command `run-all init --terragrunt-queue-include-units-reading shared.hcl` from the root folder, both
`reading-shared-hcl` and `also-reading-shared-hcl` will be run; not `not-reading-shared-hcl`.

This is because the `read_terragrunt_config` HCL function has a special hook that allows Terragrunt to track that it has
read the file `shared.hcl`. This hook is used by all native HCL functions that Terragrunt supports which read files.

Note, however, that there are certain scenarios where Terragrunt may not be able to track that a file has been read this way.

For example, you may be using a bash script to read a file via `run_cmd`, or reading the file via OpenTofu code. To support these
use-cases, the [mark_as_read](/docs/reference/built-in-functions/#mark_as_read) function can be used to manually mark a file as read.

That would look something like this:

```hcl
locals {
  filename = mark_as_read("file-read-by-tofu.txt")
}

inputs = {
  filename = local.filename
}
```

**⚠️**: Due to the way that Terragrunt parses configurations during a `run-all`, functions will only properly mark files as read
if they are used in the `locals` block. Reading a file directly in the `inputs` block will not mark the file as read, as the `inputs`
block is not evaluated until _after_ the queue has been populated with units to run.

### dependency-fetch-output-from-state

**CLI Arg**: `--dependency-fetch-output-from-state`<br/>
**CLI Arg Alias**: `--terragrunt-fetch-dependency-output-from-state` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_DEPENDENCY_FETCH_OUTPUT_FROM_STATE` (set to `true`)<br/>
**Environment Variable Alias**:  `TERRAGRUNT_FETCH_DEPENDENCY_OUTPUT_FROM_STATE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

When using many dependencies, this option can speed up the dependency processing by fetching dependency output directly
from the state file instead of using `tofu/terraform output` to fetch them.
NOTE: This is an experimental feature, use with caution.
Currently only AWS S3 backend is supported.

### use-partial-parse-config-cache

**CLI Arg**: `--use-partial-parse-config-cache`<br/>
**CLI Arg Alias**: `--terragrunt-use-partial-parse-config-cache` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_USE_PARTIAL_PARSE_CONFIG_CACHE` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_USE_PARTIAL_PARSE_CONFIG_CACHE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

This flag can be used to drastically decrease time required for parsing Terragrunt files. The effect will only show if a lot of similar includes are expected such as the root terragrunt configuration (e.g. `root.hcl`) include.
NOTE: This is an experimental feature, use with caution.

The reason you might want to use this flag is that Terragrunt frequently only needs to perform a partial parse of Terragrunt configurations.

This is the case for scenarios like:

- Building the Directed Acyclic Graph (DAG) during a `run-all` command where only the `dependency` blocks need to be evaluated to determine run order.
- Parsing the `terraform` block to determine state configurations for fetching `dependency` outputs.
- Determining whether Terragrunt execution behavior has to change like for `prevent_destroy` or `skip` flags in configuration.

These configurations are generally safe to cache, but due to the nature of HCL being a dynamic configuration language, there are some edge cases where caching these can lead to incorrect behavior.

Once this flag has been tested thoroughly, we will consider making it the default behavior.

### backend-require-bootstrap

**CLI Arg**: `--backend-require-bootstrap`<br/>
**CLI Arg Alias**: `--terragrunt-fail-on-state-bucket-creation` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_BACKEND_REQUIRE_BOOTSTRAP` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_FAIL_ON_STATE_BUCKET_CREATION` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

When this flag is set, Terragrunt will fail and exit if it is necessary to create the remote state bucket.

### disable-bucket-update

**CLI Arg**: `--disable-bucket-update`<br/>
**CLI Arg Alias**: `--terragrunt-disable-bucket-update` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_DISABLE_BUCKET_UPDATE` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_DISABLE_BUCKET_UPDATE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

When this flag is set, Terragrunt does not update the remote state bucket, which is useful to set if the state bucket is managed by a third party.

### disable-command-validation

**CLI Arg**: `--disable-command-validation`<br/>
**CLI Arg Alias**: `--terragrunt-disable-command-validation` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_DISABLE_COMMAND_VALIDATION` (set to `true`)<br/>
**Environment Variable Alias**: `TERRAGRUNT_DISABLE_COMMAND_VALIDATION` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

When this flag is set, Terragrunt will not validate the terraform command, which can be useful when need to use non-existent commands in hooks.

### provider-cache

**CLI Arg**: `--provider-cache`<br/>
**CLI Arg Alias**: `--terragrunt-provider-cache` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_PROVIDER_CACHE`<br/>
**Environment Variable Alias**: `TERRAGRUNT_PROVIDER_CACHE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [run-all](#run-all)

Enables Terragrunt's provider caching. This forces OpenTofu/Terraform to make provider requests through the Terragrunt Provider Cache server. Make sure to read [Provider Cache Server](https://terragrunt.gruntwork.io/docs/features/provider-cache-server) for context.

### provider-cache-dir

**CLI Arg**: `--provider-cache-dir`<br/>
**CLI Arg Alias**: `--terragrunt-provider-cache-dir` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_PROVIDER_CACHE_DIR`<br/>
**Environment Variable Alias**: `TERRAGRUNT_PROVIDER_CACHE_DIR` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [run-all](#run-all)

The path to the Terragrunt provider cache directory. By default, `terragrunt/providers` folder in the user cache directory: `$HOME/.cache` on Unix systems, `$HOME/Library/Caches` on Darwin, `%LocalAppData%` on Windows. The file structure of the cache directory is identical to the OpenTofu/Terraform [plugin_cache_dir](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache) directory. Make sure to read [Provider Cache Server](https://terragrunt.gruntwork.io/docs/features/provider-cache-server) for context.

### provider-cache-hostname

**CLI Arg**: `--provider-cache-hostname`<br/>
**CLI Arg Alias**: `--terragrunt-provider-cache-hostname` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_PROVIDER_CACHE_HOSTNAME`<br/>
**Environment Variable Alias**: `TERRAGRUNT_PROVIDER_CACHE_HOSTNAME` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [run-all](#run-all)

The hostname of the Terragrunt Provider Cache server. By default, 'localhost'. Make sure to read [Provider Cache Server](https://terragrunt.gruntwork.io/docs/features/provider-cache-server) for context.

### provider-cache-port

**CLI Arg**: `--provider-cache-port`<br/>
**CLI Arg Alias**: `--terragrunt-provider-cache-port` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_PROVIDER_CACHE_PORT`<br/>
**Environment Variable Alias**: `TERRAGRUNT_PROVIDER_CACHE_PORT` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [run-all](#run-all)

The port of the Terragrunt Provider Cache server. By default, assigned automatically. Make sure to read [Provider Cache Server](https://terragrunt.gruntwork.io/docs/features/provider-cache-server) for context.

### provider-cache-token

**CLI Arg**: `--provider-cache-token`<br/>
**CLI Arg Alias**: `--terragrunt-provider-cache-token` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_PROVIDER_CACHE_TOKEN`<br/>
**Environment Variable Alias**: `TERRAGRUNT_PROVIDER_CACHE_TOKEN` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [run-all](#run-all)

The Token for authentication on the Terragrunt Provider Cache server. By default, assigned automatically. Make sure to read [Provider Cache Server](https://terragrunt.gruntwork.io/docs/features/provider-cache-server) for context.

### provider-cache-registry-names

**CLI Arg**: `--provider-cache-registry-names`<br/>
**CLI Arg Alias**: `--terragrunt-provider-cache-registry-names` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_PROVIDER_CACHE_REGISTRY_NAMES`<br/>
**Environment Variable Alias**: `TERRAGRUNT_PROVIDER_CACHE_REGISTRY_NAMES` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [run-all](#run-all)

The list of remote registries to cached by Terragrunt Provider Cache server. By default, 'registry.terraform.io', 'registry.opentofu.org'. Make sure to read [Provider Cache Server](https://terragrunt.gruntwork.io/docs/features/provider-cache-server) for context.

### out-dir

**CLI Arg**: `--out-dir`<br/>
**CLI Arg Alias**: `--terragrunt-out-dir` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_OUT_DIR`<br/>
**Environment Variable Alias**: `TERRAGRUNT_OUT_DIR` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [run-all](#run-all)

Specify the plan output directory for the `*-all` commands. Useful to save plans between runs in a single place.

### json-out-dir

**CLI Arg**: `--json-out-dir`<br/>
**CLI Arg Alias**: `--terragrunt-json-out-dir` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_JSON_OUT_DIR`<br/>
**Environment Variable Alias**: `TERRAGRUNT_JSON_OUT_DIR` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Commands**:

- [run-all](#run-all)

Specify the output directory for the `*-all` commands to store plans in JSON format. Useful to read plans programmatically.

### tf-forward-stdout

**CLI Arg**: `--tf-forward-stdout`<br/>
**CLI Arg Alias**: `--terragrunt-forward-tf-stdout` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_TF_FORWARD_STDOUT`<br/>
**Environment Variable Alias**: `TERRAGRUNT_FORWARD_TF_STDOUT` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

If specified, the output of Terraform/OpenTofu commands will be printed as is. By default, all logs, except when using the `output` command or `-json` flags, are integrated into the Terragrunt log.

The example of what the log looks like without the `--tf-forward-stdout` flag specified:

```bash
14:19:25.081 INFO   [app] Running command: tofu plan -input=false
14:19:25.174 STDOUT [app] tofu: OpenTofu used the selected providers to generate the following execution
14:19:25.174 STDOUT [app] tofu: plan. Resource actions are indicated with the following symbols:
14:19:25.174 STDOUT [app] tofu:   + create
14:19:25.174 STDOUT [app] tofu: OpenTofu will perform the following actions:
```

The example of what the log looks like with the `--tf-forward-stdout` flag specified:

```bash
14:19:25.081 INFO   [app] Running command: tofu plan -input=false

OpenTofu used the selected providers to generate the following execution
plan. Resource actions are indicated with the following symbols:
  + create

OpenTofu will perform the following actions:
```

### no-destroy-dependencies-check

**CLI Arg**: `--no-destroy-dependencies-check`<br/>
**CLI Arg Alias**: `--terragrunt-no-destroy-dependencies-check` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable**: `TG_NO_DESTROY_DEPENDENCIES_CHECK`<br/>
**Environment Variable Alias**: `TERRAGRUNT_NO_DESTROY_DEPENDENCIES_CHECK` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

If specified, Terragrunt will not check dependent units when running the `destroy` command.

By default, Terragrunt checks dependent units when running `destroy` command to provide a warning that other units may be not work correctly if their dependency is destroyed.

### feature

**CLI Arg**: `--feature`<br/>
**Environment Variable**: `TG_FEATURE`<br/>
**Environment Variable Alias**: `TERRAGRUNT_FEATURE` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

Feature flags in Terragrunt allow users to dynamically control configuration behavior through CLI arguments or environment variables.

These flags enable a more flexible and controlled deployment process, particularly in monorepo contexts with interdependent infrastructure units.

Example HCL flags definition:

```hcl
feature "string_feature_flag" {
  default = "test"
}

feature "int_feature_flag" {
  default = 777
}

feature "bool_feature_flag" {
  default = false
}

terraform {
  before_hook "conditional_command" {
    commands = ["apply", "plan", "destroy"]
    execute  = feature.bool_feature_flag.value ? ["sh", "-c", "echo running conditional bool_feature_flag"] : [ "sh", "-c", "exit", "0" ]
  }
}

inputs = {
  string_feature_flag = feature.string_feature_flag.value
  int_feature_flag = feature.int_feature_flag.value
}

```

Setting a feature flag through the CLI:

```bash
terragrunt --feature int_feature_flag=123 --feature bool_feature_flag=true --feature string_feature_flag=app1 apply
```

Setting feature flags through environment variables:

```bash
export TERRAGRUNT_FEATURE=int_feature_flag=123,bool_feature_flag=true,string_feature_flag=app1
terragrunt apply
```

### experiment

**CLI Arg**: `--experiment`<br/>
**Environment Variable**: `TG_EXPERIMENT`<br/>

Enable experimental features in Terragrunt before they're stable.

For more information, see the [Experiments](/docs/reference/experiments) documentation.

### experiment-mode

**CLI Arg**: `--experiment-mode`<br/>
**Environment Variable**: `TG_EXPERIMENT_MODE`<br/>

Enable all experimental features in Terragrunt before they're stable.

For more information, see the [Experiments](/docs/reference/experiments) documentation.

### feature

**CLI Arg**: `--feature`<br/>
**Environment Variable**: `TERRAGRUNT_FEATURE`<br/>

Feature flags in Terragrunt allow users to dynamically control configuration behavior through CLI arguments or environment variables.

These flags enable a more flexible and controlled deployment process, particularly in monorepo contexts with interdependent infrastructure units.

Example HCL flags definition:

```hcl
feature "string_feature_flag" {
  default = "test"
}

feature "int_feature_flag" {
  default = 777
}

feature "bool_feature_flag" {
  default = false
}

terraform {
  before_hook "conditional_command" {
    commands = ["apply", "plan", "destroy"]
    execute  = feature.bool_feature_flag.value ? ["sh", "-c", "echo running conditional bool_feature_flag"] : [ "sh", "-c", "exit", "0" ]
  }
}

inputs = {
  string_feature_flag = feature.string_feature_flag.value
  int_feature_flag = feature.int_feature_flag.value
}

```

Setting a feature flag through the CLI:

```bash
terragrunt --feature int_feature_flag=123 --feature bool_feature_flag=true --feature string_feature_flag=app1 apply
```

Setting feature flags through environment variables:

```bash
export TERRAGRUNT_FEATURE=int_feature_flag=123,bool_feature_flag=true,string_feature_flag=app1
terragrunt apply
```

### strict-control

**CLI Arg**: `--strict-control`<br/>
**Environment Variable**: `TERRAGRUNT_STRICT_CONTROL`<br/>

Enable strict controls that opt-in future breaking changes in Terragrunt.

For more information, see the [Strict Mode](/docs/reference/strict-mode) documentation.

### strict-mode

**CLI Arg**: `--strict-mode`<br/>
**Environment Variable**: `TERRAGRUNT_STRICT_MODE`<br/>

Enable all strict controls that opt-in future breaking changes in Terragrunt.

For more information, see the [Strict Mode](/docs/reference/strict-mode) documentation.

### in-download-dir

**CLI Arg**: `--in-download-dir`<br/>
**Environment Variable**: `TG_IN_DOWNLOAD_DIR`<br/>
**Commands**:

- [exec](#exec)

Execute the provided command in the download directory.

## Deprecated

### Deprecated Commands

The following are deprecated commands that are no longer recommended for use. They are still available for backwards compatibility, but will be removed in a future release.

- [Deprecated Commands](#deprecated-commands)
  - [plan-all (DEPRECATED: use run-all)](#plan-all)
  - [apply-all (DEPRECATED: use run-all)](#apply-all)
  - [output-all (DEPRECATED: use run-all)](#output-all)
  - [destroy-all (DEPRECATED: use run-all)](#destroy-all)
  - [validate-all (DEPRECATED: use run-all)](#validate-all)

#### plan-all

**DEPRECATED: Use `run-all plan` instead.**

Display the plans of a `stack` by running `terragrunt plan` in each subfolder. Make sure to read [Execute OpenTofu/Terraform
commands on multiple modules at once](/docs/features/stacks) for
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

#### apply-all

**DEPRECATED: Use `run-all apply` instead.**

Apply a `stack` by running `terragrunt apply` in each subfolder. Make sure to read [Execute OpenTofu/Terraform
commands on multiple modules at once](/docs/features/stacks) for
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

#### output-all

**DEPRECATED: Use `run-all output` instead.**

Display the outputs of a `stack` by running `terragrunt output` in each subfolder. Make sure to read [Execute OpenTofu/Terraform
commands on multiple modules at once](/docs/features/stacks) for
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

#### destroy-all

**DEPRECATED: Use `run-all destroy` instead.**

Destroy a `stack` by running `terragrunt destroy` in each subfolder. Make sure to read [Execute OpenTofu/Terraform
commands on multiple modules at once](/docs/features/stacks) for
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

#### validate-all

**DEPRECATED: Use `run-all validate` instead.**

Validate `stack` by running `terragrunt validate` in each subfolder. Make sure to read [Execute OpenTofu/Terraform
commands on multiple modules at once](/docs/features/stacks) for
context.

Example:

```bash
terragrunt validate-all
```

This will recursively search the current working directory for any folders that contain Terragrunt modules and run
`validate` in each one, concurrently, while respecting ordering defined via
[`dependency`](/docs/reference/config-blocks-and-attributes/#dependency) and
[`dependencies`](/docs/reference/config-blocks-and-attributes/#dependencies) blocks.

### Deprecated Flags

The following are deprecated flags that are no longer recommended for use. They are still available for backwards compatibility, but will be removed in a future release.

- [Deprecated Flags](#deprecated-flags)
  - [include-module-prefix](#terragrunt-include-module-prefix)
  - [json-log](#terragrunt-json-log)
  - [tf-logs-to-json](#terragrunt-tf-logs-to-json) (DEPRECATED: use [log-format](#log-format))
  - [disable-log-formatting](#terragrunt-disable-log-formatting) (DEPRECATED: use [log-format](#log-format))

#### terragrunt-include-module-prefix

**DEPRECATED: Since this behavior has become the default, this flag has been removed. In order to get raw Terraform/OpenTofu output, use [tf-forward-stdout](#tf-forward-stdout).**

**CLI Arg**: `--terragrunt-include-module-prefix`<br/>
**Environment Variable**: `TERRAGRUNT_INCLUDE_MODULE_PREFIX` (set to `true`)<br/>

When this flag is set output from OpenTofu/Terraform sub-commands is prefixed with module path.

#### terragrunt-json-log

**DEPRECATED: Use [log-format](#log-format).**

**CLI Arg**: `--terragrunt-json-log`<br/>
**Environment Variable**: `TERRAGRUNT_JSON_LOG` (set to `true`)<br/>

When this flag is set, Terragrunt will output its logs in JSON format.

#### terragrunt-tf-logs-to-json

**DEPRECATED: Use [log-format](#log-format).**

**OpenTofu/Terraform `stdout` and `stderr` are wrapped in JSON by default when using the `--log-format json` flag if the `--terragrunt-tf-forward-stdout` flag is not specified.**

**In other words, the behavior when using the deprecated `--json-log --terragrunt-tf-logs-to-json` flags is now equivalent to `--log-format json` and the previous behavior with the `--terragrunt-json-log` is now equivalent to `--log-format json --terragrunt-tf-forward-stdout`.**

**CLI Arg**: `--tf-logs-to-json`<br/>
**Environment Variable**: `TERRAGRUNT_TF_JSON_LOG` (set to `true`)<br/>

When this flag is set, Terragrunt will wrap OpenTofu/Terraform `stdout` and `stderr` in JSON log messages. Works only with `--json-log` flag.

#### terragrunt-disable-log-formatting

**DEPRECATED: Use [log-format](#log-format).**

**CLI Arg**: `--terragrunt-disable-log-formatting`<br/>
**Environment Variable**: `TERRAGRUNT_DISABLE_LOG_FORMATTING`<br/>
**CLI Arg Alias**: `` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>
**Environment Variable Alias**: `` (deprecated: [See migration guide](/docs/migrate/cli-redesign/))<br/>

If specified, logs will be displayed in key/value format. By default, logs are formatted in a human readable format.

The example of what the log looks like without the `--terragrunt-disable-log-formatting` flag specified:

```bash
14:19:25.081 INFO   [app] Running command: tofu plan -input=false
14:19:25.174 STDOUT [app] tofu: OpenTofu used the selected providers to generate the following execution
14:19:25.174 STDOUT [app] tofu: plan. Resource actions are indicated with the following symbols:
14:19:25.174 STDOUT [app] tofu:   + create
14:19:25.174 STDOUT [app] tofu: OpenTofu will perform the following actions:
```

The example of what the log looks like with the `--tf-forward-stdout` flag specified:

```bash
time=2024-08-23T11:47:18+03:00 level=info prefix=app msg=Running command: tofu plan -input=false
time=2024-08-23T11:47:18+03:00 level=stdout prefix=app binary=tofu msg=OpenTofu used the selected providers to generate the following execution
time=2024-08-23T11:47:18+03:00 level=stdout prefix=app binary=tofu msg=plan. Resource actions are indicated with the following symbols:
time=2024-08-23T11:47:18+03:00 level=stdout prefix=app binary=tofu msg=  + create
time=2024-08-23T11:47:18+03:00 level=stdout prefix=app binary=tofu msg=OpenTofu will perform the following actions:
```
