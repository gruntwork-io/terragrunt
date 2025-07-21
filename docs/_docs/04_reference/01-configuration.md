---
layout: collection-browser-doc
title: Configuration
category: reference
categories_url: reference
excerpt: >-
  Learn how to configure Terragrunt using HCL files.
tags: ["config"]
order: 401
nav_title: Documentation
nav_title_link: /docs/
redirect_from:
  - /docs/getting-started/configuration/
  - /docs/etting-started/configuration/
slug: configuration
---

Terragrunt configuration is defined in [HCL](https://github.com/hashicorp/hcl) files. This uses the same HCL syntax as OpenTofu/Terraform itself.

Here's an example:

```hcl
# terragrunt.hcl

include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependencies {
  paths = ["../vpc", "../mysql", "../redis"]
}
```

The core of Terragrunt configuration is that of the [unit](/docs/getting-started/terminology#unit), which is canonically defined using `terragrunt.hcl` files.

Terragrunt also supports [JSON-serialized HCL](https://github.com/hashicorp/hcl/blob/hcl2/json/spec.md) defined in `terragrunt.hcl.json` files. Where `terragrunt.hcl` is mentioned in documentation, you can always use `terragrunt.hcl.json` instead.

> **Note:**
> Think carefully before using JSON configuration files for your Terragrunt configurations. The vast majority of Terragrunt users use HCL to configure Terragrunt, and you will alienate them by using JSON instead.
>
> JSON compatibility is largely support to make it easier to generate configurations from other tools, like `jq`.

When determining the configuration for a unit, Terragrunt figures out the path to its configuration file according to the following rules:

1. The value of the `--config` command-line option, if specified.

2. The value of the `TG_CONFIG` environment variable, if defined.

3. A `terragrunt.hcl` file in the current working directory, if it exists.

4. A `terragrunt.hcl.json` file in the current working directory, if it exists.

5. If none of these are found, exit with an error.

Refer to the following pages for a complete reference of supported features in the terragrunt configuration file:

- [Configuration Blocks and Attributes](/docs/reference/config-blocks-and-attributes)
- [Built-in Functions](/docs/reference/built-in-functions)

## Configuration parsing order

It is important to be aware of the terragrunt configuration parsing order when using features like [locals](/docs/reference/config-blocks-and-attributes/#locals) and [dependency outputs](/docs/features/stacks#passing-outputs-between-units), where you can reference attributes of other blocks in the config in your `inputs`. For example, because `locals` are evaluated before `dependency` blocks, you cannot bind outputs from `dependency` into `locals`. On the other hand, for the same reason, you can use `locals` in the `dependency` blocks.

Currently terragrunt parses the config in the following order:

1. `include` block

2. `locals` block

3. Evaluation of values for `iam_role`, `iam_assume_role_duration`, `iam_assume_role_session_name`, and `iam_web_identity_token` attributes, if defined

4. `dependencies` block

5. `dependency` blocks, including calling `terragrunt output` on the dependent units to retrieve the outputs

6. Everything else

7. The config referenced by `include`

8. A merge operation between the config referenced by `include` and the current config.

Blocks that are parsed earlier in the process will be made available for use in the parsing of later blocks. Similarly, you cannot use blocks that are parsed later earlier in the process (e.g you can't reference `dependency` in `locals`, `include`, or `dependencies` blocks).

Note that the parsing order is slightly different when using the `--all` flag of the [`run`](/docs/reference/cli-options/#run) command. When using the `--all` flag, Terragrunt parses the configuration twice. In the first pass, it follows the following parsing order:

1. `include` block of all configurations in the tree

2. `locals` block of all configurations in the tree

3. `dependency` blocks of all configurations in the tree, but does NOT retrieve the outputs

4. `terraform` block of all configurations in the tree

5. `dependencies` block of all configurations in the tree

The results of this pass are then used to build the dependency graph of the units in the stack. Once the graph is constructed, Terragrunt will loop through the units and run the specified command. It will then revert to the single configuration parsing order specified above for each unit as it runs the command.

This allows Terragrunt to avoid resolving `dependency` on units that haven't been applied yet when doing a clean deployment from scratch with `run --all apply`.

## Stacks

When multiple units, each with their own `terragrunt.hcl` file exist in child directories of a single parent directory, that parent directory becomes a [stack](/docs/getting-started/terminology#stack).

> **New to stacks?** For a comprehensive introduction to the concept, see our [Stacks](/docs/features/stacks) guide.

### What is a terragrunt.stack.hcl file?

A `terragrunt.stack.hcl` file is a **blueprint** that defines how to generate Terragrunt configuration programmatically.

It tells Terragrunt:

- What units to create.
- Where to get their configurations from.
- Where to place them in the directory structure.
- What values to pass to each unit.

### The Two Types of Blocks

#### `unit` blocks - Define Individual Infrastructure Components

- **Purpose**: Define a single, deployable piece of infrastructure.
- **Use case**: When you want to create a single piece of isolated infrastructure (e.g. a specific VPC, database, or application).
- **Result**: Generates a single `terragrunt.hcl` file in the specified path.

#### `stack` blocks - Define Reusable Infrastructure Patterns

- **Purpose**: Define a collection of related units that can be reused.
- **Use case**: When you have a common, multi-unit pattern (like "dev environment" or "three-tier web application") that you want to deploy multiple times.
- **Result**: Generates another `terragrunt.stack.hcl` file that can contain more units or stacks.

### Comparison: unit vs stack blocks

| Aspect | `unit` block | `stack` block |
|--------|-------------|---------------|
| **Purpose** | Define a single infrastructure component | Define a reusable collection of components |
| **When to use** | For specific, one-off infrastructure pieces | For patterns of infrastructure pieces that you want provisioned together |
| **Generated output** | A directory with a single `terragrunt.hcl` file | A directory with a `terragrunt.stack.hcl` file |

### The Complete Workflow

1. **Author**: Write a `terragrunt.stack.hcl` file with `unit` and/or `stack` blocks.
2. **Generate**: Run `terragrunt stack generate` to create the actual units*.
3. **Deploy**: Run `terragrunt stack run apply` to deploy all units**.

- Multiple commands (like `stack run` or `run --all`) automatically generate units from `terragrunt.stack.hcl` files for you.

** You can also just use `run --all apply` to deploy all units in the stack.

### Example: Simple Stack with Units

```hcl
# terragrunt.stack.hcl

unit "vpc" {
  source = "git::git@github.com:acme/infrastructure-catalog.git//units/vpc?ref=v0.0.1"
  path   = "vpc"
  values = {
    vpc_name = "main"
    cidr     = "10.0.0.0/16"
  }
}

unit "database" {
  source = "git::git@github.com:acme/infrastructure-catalog.git//units/database?ref=v0.0.1"
  path   = "database"
  values = {
    engine   = "postgres"
    version  = "13"

    vpc_path = "../vpc"
  }
}
```

Running `terragrunt stack generate` creates:

```tree
terragrunt.stack.hcl
.terragrunt-stack/
├── vpc/
│   ├── terragrunt.hcl
│   └── terragrunt.values.hcl
└── database/
    ├── terragrunt.hcl
    └── terragrunt.values.hcl
```

### Example: Nested Stack with Reusable Patterns

```hcl
# terragrunt.stack.hcl

stack "dev" {
  source = "git::git@github.com:acme/infrastructure-catalog.git//stacks/environment?ref=v0.0.1"
  path   = "dev"
  values = {
    environment = "development"
    cidr        = "10.0.0.0/16"
  }
}

stack "prod" {
  source = "git::git@github.com:acme/infrastructure-catalog.git//stacks/environment?ref=v0.0.1"
  path   = "prod"
  values = {
    environment = "production"
    cidr        = "10.1.0.0/16"
  }
}
```

The referenced stack might contain:

```hcl
# stacks/environment/terragrunt.stack.hcl

unit "vpc" {
  source = "git::git@github.com:acme/infrastructure-catalog.git//units/vpc?ref=v0.0.1"
  path   = "vpc"
  values = {
    vpc_name = values.environment
    cidr     = values.cidr
  }
}

unit "database" {
  source = "git::git@github.com:acme/infrastructure-catalog.git//units/database?ref=v0.0.1"
  path   = "database"
  values = {
    environment = values.environment

    vpc_path = "../vpc"
  }
}
```

For more information on these configuration blocks, see:

- [unit](/docs/reference/config-blocks-and-attributes/#unit)
- [stack](/docs/reference/config-blocks-and-attributes/#stack)
- [locals](/docs/reference/config-blocks-and-attributes/#locals)

## Formatting HCL files

You can rewrite the HCL files to a canonical format using the `hclfmt` command built into `terragrunt`. Similar to `tofu fmt`, this command applies a subset of [the OpenTofu/Terraform language style conventions](https://www.terraform.io/docs/configuration/style.html), along with other minor adjustments for readability.

By default, this command will recursively search for hcl files and format all of them for a given stack. Consider the following file structure:

```tree
root
├── root.hcl
├── prod
│   └── terragrunt.hcl
├── dev
│   └── terragrunt.hcl
└── qa
    ├── terragrunt.hcl
    └── services
        ├── services.hcl
        └── service01
            └── terragrunt.hcl
```

If you run `terragrunt hcl fmt` at the `root`, this will update:

- `root/root.hcl`

- `root/prod/terragrunt.hcl`

- `root/dev/terragrunt.hcl`

- `root/qa/terragrunt.hcl`

- `root/qa/services/services.hcl`

- `root/qa/services/service01/terragrunt.hcl`

You can set `--diff` option. `terragrunt hcl fmt --diff` will output the diff in a unified format which can be redirected to your favourite diff tool. `diff` utility must be presented in PATH.

Additionally, there's a flag `--check`. `terragrunt hcl fmt --check` will only verify if the files are correctly formatted **without rewriting** them. The command will return exit status 1 if any matching files are improperly formatted, or 0 if all matching `.hcl` files are correctly formatted.

You can exclude directories from the formatting process by using the `--exclude-dir` flag. For example, `terragrunt hcl fmt --exclude-dir=qa/services`.

If you want to format a single file, you can use the `--file` flag. For example, `terragrunt hcl fmt --file qa/services/services.hcl`.
