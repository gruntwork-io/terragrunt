---
layout: collection-browser-doc
title: Stacks
category: features
categories_url: features
excerpt: Learn how to work with multiple units at once in a stack.
tags: ["DRY", "Unit", "Modules", "Use cases", "CLI", "Stack"]
order: 202
nav_title: Documentation
nav_title_link: /docs/
redirect_from:
  - /docs/features/execute-terraform-commands-on-multiple-units-at-once/
  - /docs/features/execute-terraform-commands-on-multiple-modules-at-once/
slug: stacks
---

- [Motivation](#motivation)
- [Stacks to the rescue](#stacks-to-the-rescue)
- [The `run --all` command](#the-run---all-command)
- [The `stack run` command](#the-stack-run-command)
- [Passing outputs between units](#passing-outputs-between-units)
  - [Unapplied dependency and mock outputs](#unapplied-dependency-and-mock-outputs)
- [Stack outputs](#stack-outputs)
- [Dependencies between units](#dependencies-between-units)
- [Visualizing the DAG](#visualizing-the-dag)
- [Testing multiple units locally](#testing-multiple-units-locally)
- [Limiting run parallelism](#limiting-run-parallelism)
- [Saving OpenTofu/Terraform plan output](#saving-opentofuterraform-plan-output)
- [Nested Stacks](#nested-stacks)
- [Using Local State with Stacks](#using-local-state-with-stacks)
- [Learning more](#learning-more)

## Motivation

Let's say your infrastructure is defined across multiple OpenTofu/Terraform root modules:

```tree
root
├── backend-app
│   └── main.tf
├── frontend-app
│   └── main.tf
├── mysql
│   └── main.tf
├── valkey
│   └── main.tf
└── vpc
    └── main.tf
```

There is one unit to deploy a frontend-app, another to deploy a backend-app, another for the MySQL database, and so on.

To deploy such an environment, you’d have to manually run `tofu`/`terraform` `apply` in each root module, wait for it to complete, and then run `tofu apply`/`terraform apply` in the next root module. Moreover, you have to make sure you manually run `tofu`/`terraform` `apply` in the _right_ root module each time. The order in which they are applied can be important, especially if one root module depends on another.

How do you avoid this tedious, error-prone and time-consuming process?

## Stacks to the rescue

Terragrunt provides special tooling for operating on sets of units at once. Sets of units in Terragrunt are called [stacks](/docs/getting-started/terminology/#stack).

To work with stacks, you author [`terragrunt.stack.hcl` files](/docs/reference/configuration/#stacks) to define stacks, then use [Stack commands](/docs/reference/cli-options/#stack-commands) to generate and interact with those generated stacks.

For example, the configuration for a stack that defines the units above might look like this:

```hcl
# terragrunt.stack.hcl

unit "backend_app" {
  source = "git::git@github.com:acme/infrastructure-catalog.git//units/backend-app?ref=v0.0.1"
  path   = "backend-app"
}

unit "frontend_app" {
  source = "git::git@github.com:acme/infrastructure-catalog.git//units/frontend-app?ref=v0.0.1"
  path   = "frontend-app"
}

unit "mysql" {
  source = "git::git@github.com:acme/infrastructure-catalog.git//units/mysql?ref=v0.0.1"
  path   = "mysql"
}

unit "valkey" {
  source = "git::git@github.com:acme/infrastructure-catalog.git//units/valkey?ref=v0.0.1"
  path   = "valkey"
}

unit "vpc" {
  source = "git::git@github.com:acme/infrastructure-catalog.git//units/vpc?ref=v0.0.1"
  path   = "vpc"
}
```

And use commands like `stack run apply` to deploy the stack.

```bash
terragrunt stack run apply
```

Using `terragrunt.stack.hcl` files to define infrastructure is a bit more advanced than defining units directly in a repository, so learning how to work with stacks without using `terragrunt.stack.hcl` files will help you understand how to author effective `terragrunt.stack.hcl` files as well. At the end of the day, the core functionality of a stack is the same whether you are using `terragrunt.stack.hcl` files or not, they merely provide a more concise way to define a stack, and generate units on demand.

## The `run --all` command

To make it possible to concurrently deploy all these OpenTofu/Terraform root modules concurrently with as little change to your existing infrastructure as possible, first convert the OpenTofu/Terraform root modules into units. This is done simply by adding an empty `terragrunt.hcl` file within each root module folder.

```tree
root
├── backend-app
│   ├── main.tf
│   └── terragrunt.hcl
├── frontend-app
│   ├── main.tf
│   └── terragrunt.hcl
├── mysql
│   ├── main.tf
│   └── terragrunt.hcl
├── valkey
│   ├── main.tf
│   └── terragrunt.hcl
└── vpc
    ├── main.tf
    └── terragrunt.hcl
```

Because you've created a directory of units, you've also implicitly created a stack!

Now, you can go into the `root` folder and deploy all the units within it by using the `run --all` command with `apply`:

```bash
terragrunt run --all apply
```

When you run this command, Terragrunt will recursively discover all the units under the current working directory, and run `terragrunt apply` on each of those units concurrently\*.

Similarly, to undeploy all the OpenTofu/Terraform units, you can use the `run --all` command with `destroy`:

```bash
terragrunt run --all destroy
```

To see the currently applied outputs of all of the units, you can use the `run --all` command with `output`:

```bash
terragrunt run --all output
```

Finally, if you make some changes to your project, you could evaluate the impact by using `run --all` command with `plan`:

Note: It is important to realize that you could get errors running `run --all plan` if you have dependencies between your
projects and some of those dependencies haven't been applied yet.

_Ex: If unit A depends on unit B and unit B hasn't been applied yet, then run --all plan will show the plan for B, but exit with an error when trying to show the plan for A._

```bash
terragrunt run --all plan
```

\* Note that the units _might_ run concurrently, but some units can be blocked from running until their dependencies are run.

If your units have dependencies between them, for example, you can't deploy the backend-app until MySQL and valkey are deployed. You'll need to express those dependencies in your Terragrunt configuration as explained in the next section.

Additional note: If your units have dependencies between them, and you run a `terragrunt run --all destroy` command, Terragrunt will destroy all the units under the current working directory, _as well as each of the unit dependencies_ (that is, units you depend on via `dependencies` and `dependency` blocks)! If you wish to use exclude dependencies from being destroyed, add the `--queue-exclude-external` flag, or use the `--exclude-dir` once for each directory you wish to exclude.

## The `stack run` command

The `stack run` command is the equivalent of the `run --all` command for a single stack defined using a `terragrunt.stack.hcl` file.

When you run `stack run`, under the hood, Terragrunt will simply:

1. Recursively generate all stacks that are defined via the `stack` blocks of the `terragrunt.stack.hcl` file in the current directory (and all the stacks they generate).
2. Generate all units that are defined via the `unit` blocks all the `terragrunt.stack.hcl` files in the current directory (and all the stacks generated in step 1).
3. Perform a `run --all` in the current directory.

As such, the following is functionally equivalent to running `stack run <a-command>`:

```bash
terragrunt stack generate
terragrunt run --all <a-command>
```

This manually generates all the relevant units, using the `stack generate` command, then explicitly running `run --all <a-command>` on those units.

## Passing outputs between units

Consider the following file structure:

```tree
root
├── backend-app
│   ├── main.tf
│   └── terragrunt.hcl
├── mysql
│   ├── main.tf
│   └── terragrunt.hcl
├── valkey
│   ├── main.tf
│   └── terragrunt.hcl
└── vpc
    ├── main.tf
    └── terragrunt.hcl
```

Suppose that you wanted to pass in the VPC ID of the VPC that is created from the `vpc` unit in the folder structure above to the `mysql` unit as an input variable. Or that you wanted to pass in the subnet IDs of the private subnet that is allocated as part of the `vpc` unit.

You can use the `dependency` block to extract those outputs and use them as `inputs` to the `mysql` unit.

For example, suppose the `vpc` unit outputs the ID under the output named `vpc_id`. To access that output, you would specify in `mysql/terragrunt.hcl`:

```hcl
dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
```

When you apply this unit, the output will be read from the `vpc` unit and passed in as an input to the `mysql` unit right before calling `tofu apply`/`terraform apply`.

You can also specify multiple `dependency` blocks to access the outputs of multiple units.

For example, in the above folder structure, you might want to reference the `domain` output of the `valkey` and `mysql` units for use as `inputs` in the `backend-app` unit. To access those outputs, you would specify the following in `backend-app/terragrunt.hcl`:

```hcl
dependency "mysql" {
  config_path = "../mysql"
}

dependency "valkey" {
  config_path = "../valkey"
}

inputs = {
  mysql_url = dependency.mysql.outputs.domain
  valkey_url = dependency.valkey.outputs.domain
}
```

Note that each `dependency` block results in a relevant status in the Terragrunt [DAG](/docs/getting-started/terminology/#directed-acyclic-graph-dag). This means that when you run `run --all apply` on a config that has `dependency` blocks, Terragrunt will not attempt to deploy the config until all the units referenced in `dependency` blocks have been applied. So for the above example, the order for the `run --all apply` command would be:

1. Deploy the VPC

2. Deploy MySQL and valkey in parallel

3. Deploy the backend-app

If any of the units failed to deploy, then Terragrunt will not attempt to deploy the units that depend on them.

**Note**: Not all blocks are able to access outputs passed by `dependency` blocks. See the section on [Configuration parsing order]({{site.baseurl}}/docs/reference/configuration/#configuration-parsing-order) for more information.

### Unapplied dependency and mock outputs

Terragrunt will return an error if the unit referenced in a `dependency` block has not been applied yet. This is because you cannot actually fetch outputs out of an unapplied unit, even if there are no resources being created in the unit.

This is most problematic when running commands that do not modify state (e.g `run --all plan` and `run --all validate`) on a completely new setup where no infrastructure has been deployed. You won't be able to `plan` or `validate` a unit if you can't determine the `inputs`. If the unit depends on the outputs of another unit that hasn't been applied yet, you won't be able to compute the `inputs` unless the dependencies are all applied.

Of course, in real life usage, you typically need the ability to run `run --all validate` or `run --all plan` on a completely new set of infrastructure.

To address this, you can provide mock outputs to use when a unit hasn't been applied yet. This is configured using the `mock_outputs` attribute on the `dependency` block and it corresponds to a map that will be injected in place of the actual dependency outputs if the target config hasn't been applied yet.

Using a mock output is typically the best solution here, as you typically don't actually care that an _accurate_ value is used for a given value at this stage, just that it will plan successfully. When you actually apply the unit, that's when you want to be sure that a real value is used.

For example, in the previous scenario with a `mysql` unit and `vpc` unit, suppose you wanted to mock a value for the `vpc_id` during a `run --all validate` for the `mysql` unit.

You can specify that in `mysql/terragrunt.hcl`:

```hcl
dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    vpc_id = "mock-vpc-id"
  }
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
```

You can now run `validate` on this config before the `vpc` unit is applied because Terragrunt will use the map `{vpc_id = "mock-vpc-id"}` as the `outputs` attribute on the dependency instead of erroring out.

What if you wanted to restrict this behavior to only the `validate` command? For example, you might not want to use the defaults for a `plan` operation because the plan doesn't give you any indication of what is actually going to be created.

You can use the `mock_outputs_allowed_terraform_commands` attribute to indicate that the `mock_outputs` should only be used when running those OpenTofu/Terraform commands. So to restrict the `mock_outputs` to only when `validate` is being run, you can modify the above `terragrunt.hcl` file to:

```hcl
dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    vpc_id = "temporary-dummy-id"
  }

  mock_outputs_allowed_terraform_commands = ["validate"]
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
```

Note that indicating `validate` means that the `mock_outputs` will be used either with `validate` or with `run --all validate`.

You can also use `skip_outputs` on the `dependency` block to specify the dependency without pulling in the outputs:

```hcl
dependency "vpc" {
  config_path = "../vpc"

  skip_outputs = true
}
```

When `skip_outputs` is used with `mock_outputs`, mocked outputs will be returned without attempting to load outputs from OpenTofu/Terraform.

This can be useful when you disable backend initialization (`remote_state.disable_init`) in CI for example.

```hcl
dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    vpc_id = "temporary-dummy-id"
  }

  skip_outputs = true
}
```

You can also use `mock_outputs_merge_strategy_with_state` on the `dependency` block to merge mocked outputs and real outputs:

```hcl
dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    vpc_id     = "temporary-dummy-id"
    new_output = "temporary-dummy-value"
  }

  mock_outputs_merge_strategy_with_state = "shallow"
}
```

If real outputs only contain `vpc_id`, `dependency.outputs` will contain a real value for `vpc_id` and a mocked value for `new_output`.

## Stack outputs

When defining a stack using a `terragrunt.stack.hcl` file, you also have the ability to interact with the aggregated outputs of all the units in the stack.

To do this, use the [`stack output`](/docs/reference/cli-options/#stack-output) command (not the [`stack run output`](/docs/reference/cli-options/#stack-run) command).

```bash
$ terragrunt stack output
backend_app = {
  domain = "backend-app.example.com"
}
frontend_app = {
  domain = "frontend-app.example.com"
}
mysql = {
  endpoint = "terraform-20250504140737772400000001.abcdefghijkl.us-east-1.rds.amazonaws.com"
}
valkey = {
  endpoint = "serverless-valkey-01.amazonaws.com"
}
vpc = {
  vpc_id = "vpc-1234567890"
}
```

This will return a single aggregated value for all the outputs of all the units in the stack.

## Dependencies between units

You can also specify dependencies without accessing any of the outputs of units. Consider the following file structure:

```tree
root
├── backend-app
│   ├── main.tf
│   └── terragrunt.hcl
├── frontend-app
│   ├── main.tf
│   └── terragrunt.hcl
├── mysql
│   ├── main.tf
│   └── terragrunt.hcl
├── valkey
│   ├── main.tf
│   └── terragrunt.hcl
└── vpc
    ├── main.tf
    └── terragrunt.hcl
```

Let's assume you have the following dependencies between OpenTofu/Terraform units:

- `backend-app` depends on `mysql`, `valkey`, and `vpc`

- `frontend-app` depends on `backend-app` and `vpc`

- `mysql` depends on `vpc`

- `valkey` depends on `vpc`

- `vpc` has no dependencies

You can express these dependencies in your `terragrunt.hcl` config files using a `dependencies` block. For example, in `backend-app/terragrunt.hcl` you would specify:

``` hcl
dependencies {
  paths = ["../vpc", "../mysql", "../valkey"]
}
```

Similarly, in `frontend-app/terragrunt.hcl`, you would specify:

``` hcl
dependencies {
  paths = ["../vpc", "../backend-app"]
}
```

Once you've specified these dependencies in each `terragrunt.hcl` file, Terragrunt will be able to perform updates respecting the [DAG](/docs/getting-started/terminology/#directed-acyclic-graph-dag) of dependencies.

For the example at the start of this section, the order of runs for the `run --all apply` command would be:

1. Deploy the VPC

2. Deploy MySQL and valkey in parallel

3. Deploy the backend-app

4. Deploy the frontend-app

Any error encountered in an individual unit during a `run --all` command will prevent Terragrunt from proceeding with the deployment of any dependent units.

To check all of your dependencies and validate the code in them, you can use the `run --all validate` command.

**Note:** During `destroy` runs, Terragrunt will try to find all dependent units and show a confirmation prompt with a list of detected dependencies. This is because Terragrunt knows that once resources in a dependency is destroyed, any commands run on dependent units may fail. For example, if `destroy` was called on the `valkey` unit, you'll be asked for confirmation, as the `backend-app` depends on `valkey`. You can avoid the prompt by using `--non-interactive`.

## Visualizing the DAG

To visualize the dependency graph you can use the `dag graph` command (similar to the `terraform graph` command).

The graph is output in DOT format. The typical program used to render this file format is GraphViz, but many web services are available that can do this as well.

```bash
terragrunt dag graph | dot -Tsvg > graph.svg
```

The example above generates the following graph:

![terragrunt dag graph]({{site.baseurl}}/assets/img/collections/documentation/graph.png)

Note that this graph shows the dependency relationship in the direction of the arrow, with the tip pointing to the dependency (e.g. `frontend-app` depends on `backend-app`).

For most commands, Terragrunt will run in the opposite direction, however (e.g. `backend-app` would be applied before `frontend-app`).

The exception to this rule is during the `destroy` (and `plan -destroy`) command, where Terragrunt will run in the direction of the arrow (e.g. `frontend-app` would be destroyed before `backend-app`).

## Testing multiple units locally

If you are using Terragrunt to download [remote OpenTofu/Terraform modules]({{site.baseurl}}/docs/features/units/#remote-opentofuterraform-modules) and all of your units have the `source` parameter set to a Git URL, but you want to test with a local checkout of the code, you can use the `--source` parameter to override that value:

```bash
terragrunt run --all plan --source /source/modules
```

If you set the `--source` parameter, the `run --all` command will assume that parameter is pointing to a folder on your local file system that has a local checkout of all of your OpenTofu/Terraform modules.

For each unit that is being processed via a `run --all` command, Terragrunt will:

1. Read in the `source` parameter in that unit's `terragrunt.hcl` file.
2. Parse out the path (the portion after the double-slash).
3. Append the path to the `--source` parameter to create the final local path for that unit.

For example, consider the following `terragrunt.hcl` file:

``` hcl
terraform {
  source = "git::git@github.com:acme/infrastructure-modules.git//networking/vpc?ref=v0.0.1"
}
```

Running the following:

```bash
terragrunt run --all apply --source /source/infrastructure-modules
```

Will result in a unit with the configuration for the source above being resolved to `/source/infrastructure-modules//networking/vpc`.

## Limiting run parallelism

By default, Terragrunt will not impose a limit on the number of units it executes when it traverses the dependency graph,
meaning that if it finds 5 units without dependencies, it'll run OpenTofu/Terraform 5 times in parallel, once in each unit.

Sometimes, this can create a problem if there are a lot of units in the dependency graph, like hitting a rate limit on a
cloud provider.

To limit the maximum number of unit executions at any given time use the `--parallelism [number]` flag

```sh
terragrunt run --all apply --parallelism 4
```

## Saving OpenTofu/Terraform plan output

A powerful feature of OpenTofu/Terraform is the ability to [save the result of a plan as a binary or JSON file using the -out flag](https://opentofu.org/docs/cli/commands/plan/).

Terragrunt provides special tooling in `run --all` execution in order to ensure that the saved plan for a `run --all` against a stack has
a corresponding entry for each unit in the stack in a directory structure that mirrors the stack structure.

To save plan against a stack, use the `--out-dir` flag (or `TG_OUT_DIR` environment variable) as demonstrated below:

```sh
$ terragrunt run --all plan --out-dir /tmp/tfplan
$ tree /tmp/tfplan
/tmp/tfplan
├── app1
│   └── tfplan.tfplan
├── app2
│   └── tfplan.tfplan
├── app3
│   └── tfplan.tfplan
└── project-2
    └── project-2-app1
        └── tfplan.tfplan
$ terragrunt run --all apply --out-dir /tmp/tfplan
```

For planning a destroy operation, use the following commands:

```sh
terragrunt run --all --out-dir /tmp/tfplan -- plan -destroy
terragrunt run --all apply --out-dir /tmp/tfplan
```

To save plan in json format use `--json-out-dir` flag (or `TG_JSON_OUT_DIR` environment variable):

```sh
$ terragrunt run --all plan --json-out-dir /tmp/json
$ tree /tmp/json
/tmp/json
├── app1
│   └── tfplan.json
├── app2
│   └── tfplan.json
├── app3
│   └── tfplan.json
└── project-2
    └── project-2-app1
        └── tfplan.json

# combine binary and json plans
$ terragrunt run --all plan --out-dir /tmp/all --json-out-dir /tmp/all
$ tree /tmp/all
/tmp/all
├── app1
│   ├── tfplan.json
│   └── tfplan.tfplan
├── app2
│   ├── tfplan.json
│   └── tfplan.tfplan
├── app3
│   ├── tfplan.json
│   └── tfplan.tfplan
└── project-2
    └── project-2-app1
        ├── tfplan.json
        └── tfplan.tfplan
```

To recap:

- The plan for each unit in a stack is saved in the same hierarchy as the unit structure.
- The file name for plan binaries are `tfplan.tfplan` and `tfplan.json` for plan JSON.
- JSON plan files can't be used with `terragrunt run --all apply` command, only binary plan files can be used.
- Output directories can be combined which will lead to saving both binary and JSON plans.

## Nested Stacks

Note that you can also have nested stacks.

For example, consider the following file structure:

```tree
root
├── us-east-1
│   ├── app
│   │   └── terragrunt.hcl
│   └── db
│       └── terragrunt.hcl
└── us-west-2
    ├── app
    │   └── terragrunt.hcl
    └── db
        └── terragrunt.hcl
```

In this example, there's the `root` stack, that contains all the infrastructure you've defined so far,
and there's also the `us-east-1` and `us-west-2` stacks, that contain the infrastructure for the `app` and `db` units in those regions.

You can run `run --all` commands at any depth of the stack to run the units in that stack and all of its children.

For example, to run all the units in the `us-east-1` stack, you can run:

```sh
cd root/us-east-1
terragrunt run --all apply
```

Terragrunt will only include the units in the `us-east-1` stack and its children in the queue of units to run (unless external dependencies are pulled in, as discussed in the [run --all command](#the-run---all-command) section).

Generally speaking, this is the primary tool Terragrunt users use to control the blast radius of their changes. For the most part, it is the current working directory that determines the blast radius of a `run --all` command.

In addition to using your working directory to control what's included in a [run queue](/docs/getting-started/terminology/#runner-queue), you can also use flags like [--queue-include-dir](/docs/reference/cli-options/#queue-include-dir) and [--queue-exclude-dir](/docs/reference/cli-options/#queue-exclude-dir) to explicitly control what's included in a run queue within a stack, or outside of it.

There are more flags that control the behavior of the `run --all` command, which you can find in the [CLI Options](/docs/reference/cli-options) section.

## Using Local State with Stacks

When using Terragrunt Stacks, you might want to use local state files instead of remote state for development, testing, or specific use cases. However, this presents a challenge because the generated `.terragrunt-stack` directory can be safely deleted and regenerated using `terragrunt stack clean && terragrunt stack generate`, which would normally cause local state files to be lost.

To solve this problem, you can configure your stack to store state files outside of the `.terragrunt-stack` directory, in a persistent location that survives stack regeneration.

### Configuration

Here's how to configure local state that persists across stack regeneration:

**1. Create a `root.hcl` file with local backend configuration:**

```hcl
# root.hcl
remote_state {
  backend = "local"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }

  config = {
    path = "${get_parent_terragrunt_dir()}/.terragrunt-local-state/${path_relative_to_include()}/tofu.tfstate"
  }
}
```

**2. Create your stack definition:**

```hcl
# live/terragrunt.stack.hcl
unit "vpc" {
  source = "${find_in_parent_folders("units/vpc")}"
  path   = "vpc"
}

unit "database" {
  source = "${find_in_parent_folders("units/database")}"
  path   = "database"
}

unit "app" {
  source = "${find_in_parent_folders("units/app")}"
  path   = "app"
}
```

**3. Configure your units to include the root configuration:**

```hcl
# units/vpc/terragrunt.hcl
include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "."
}
```

**4. Add a `.gitignore` file to exclude state files from version control:**

```gitignore
# .gitignore
.terragrunt-local-state
```

**Important:** Local state files should never be committed to version control as they may contain sensitive information and can cause conflicts when multiple developers work on the same infrastructure.

### How It Works

The key insight is using `path_relative_to_include()` in the state path configuration. This function returns the relative path from each unit to the `root.hcl` file, creating unique state file paths like:

```text
.terragrunt-local-state/live/.terragrunt-stack/vpc/tofu.tfstate
.terragrunt-local-state/live/.terragrunt-stack/database/tofu.tfstate
.terragrunt-local-state/live/.terragrunt-stack/app/tofu.tfstate
```

Since these state files are stored in `.terragrunt-local-state/` (outside of `.terragrunt-stack/`), they persist when you run:

```bash
terragrunt stack clean && terragrunt stack generate
```

### Directory Structure

After running the stack, your directory structure will look like this:

```tree
.
├── root.hcl
├── .gitignore                        # Excludes .terragrunt-local-state
├── .terragrunt-local-state/          # Persistent state files (ignored by git)
│   └── live/
│       └── .terragrunt-stack/
│           ├── vpc/
│           │   └── tofu.tfstate
│           ├── database/
│           │   └── tofu.tfstate
│           └── app/
│               └── tofu.tfstate
├── live/
│   ├── terragrunt.stack.hcl
│   └── .terragrunt-stack/            # Generated stack (can be deleted)
│       ├── vpc/
│       │   ├── terragrunt.hcl
│       │   └── main.tf
│       ├── database/
│       │   ├── terragrunt.hcl
│       │   └── main.tf
│       └── app/
│           ├── terragrunt.hcl
│           └── main.tf
└── units/                            # Reusable unit definitions
    ├── vpc/
    ├── database/
    └── app/
```

### Benefits

This approach provides several advantages:

- **State Persistence**: State files survive stack regeneration
- **Isolation**: Each unit gets its own state file
- **Consistency**: Directory structure mirrors the stack layout
- **Flexibility**: You can switch between local and remote state easily by changing the backend configuration

### Example Workflow

```bash
# Initial setup
terragrunt stack generate
terragrunt stack run apply

# Later, regenerate the stack without losing state
terragrunt stack clean
terragrunt stack generate

# Verify existing resources are recognized
terragrunt stack run plan  # Should show "No changes"
```

This pattern is particularly useful for development environments, testing scenarios, or when you need to maintain local state for compliance or security reasons while still benefiting from Terragrunt's stack management capabilities.

## Learning more

If you'd like more advanced examples on stacks, check out the [terragrunt-infrastructure-catalog-example repository](https://github.com/gruntwork-io/terragrunt-infrastructure-catalog-example/tree/main/examples/terragrunt/stacks). These have full-featured examples of stacks that deploy real, stateful infrastructure in an AWS account.
