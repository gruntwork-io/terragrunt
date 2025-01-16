---
layout: collection-browser-doc
title: Includes
category: features
categories_url: features
excerpt: Learn how to reuse partial Terragrunt configurations to DRY up your configurations.
tags: ["DRY", "Use cases", "include"]
order: 203
nav_title: Documentation
nav_title_link: /docs/
redirect_from:
    - /docs/features/keep-your-terragrunt-architecture-dry/
---

- [Motivation](#motivation)
- [Using multiple includes](#using-multiple-includes)
- [Using exposed includes](#using-exposed-includes)
- [Using read\_terragrunt\_config](#using-read_terragrunt_config)
- [Considerations for CI/CD Pipelines](#considerations-for-cicd-pipelines)

## Motivation

As covered in [Units]({{site.baseurl}}/docs/features/units) and [State Backend]({{site.baseurl}}/docs/features/state-backend),
it quickly becomes important to define base Terragrunt configuration files that are included in units. This is to ensure
that all units have a consistent configuration, and to avoid repeating the same configuration across multiple units.

For example, you might have a **root** Terragrunt configuration that defines the remote state and provider configurations for all your units:

```hcl
remote_state {
  backend = "s3"
  config = {
    bucket         = "my-tofu-state"
    key            = "${path_relative_to_include()}/tofu.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "my-lock-table"
  }
}

generate "provider" {
  path = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents = <<EOF
provider "aws" {
  assume_role {
    role_arn = "arn:aws:iam::0123456789:role/terragrunt"
  }
}
EOF
}
```

You can then include this in each of your **unit** `terragrunt.hcl` files using the `include` block for each
infrastructure module you need to deploy:

```hcl
include "root" {
  path = find_in_parent_folders("root.hcl")
}
```

This pattern is useful for global configuration blocks that need to be included in all of your modules, but what if you
have Terragrunt configurations that are only relevant to subsets of your stack?

For example, consider the following terragrunt file structure, which defines three environments (`prod`, `qa`, and `stage`)
with the same infrastructure in each one (an app, a MySQL database, and a VPC):

```tree
└── live
    ├── root.hcl
    ├── prod
    │   ├── app
    │   │   └── terragrunt.hcl
    │   ├── mysql
    │   │   └── terragrunt.hcl
    │   └── vpc
    │       └── terragrunt.hcl
    ├── qa
    │   ├── app
    │   │   └── terragrunt.hcl
    │   ├── mysql
    │   │   └── terragrunt.hcl
    │   └── vpc
    │       └── terragrunt.hcl
    └── stage
        ├── app
        │   └── terragrunt.hcl
        ├── mysql
        │   └── terragrunt.hcl
        └── vpc
            └── terragrunt.hcl
```

More often than not, each of the services will look similar across the different environments, only requiring small
tweaks.

For example, the `app/terragrunt.hcl` files may be identical across all three environments except for an
adjustment to the `instance_type` parameter for each environment. These identical settings don't belong in the root
`terragrunt.hcl` configuration because they are only relevant to the `app` configurations, and not `mysql` or `vpc`.

To solve this, you can use [multiple include blocks]({{site.baseurl}}/docs/reference/config-blocks-and-attributes#include).

## Using multiple includes

Suppose your `qa/app/terragrunt.hcl` configuration looks like the following:

```hcl
include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "github.com/<org>/modules.git//app?ref=v0.1.0"
}

dependency "vpc" {
  config_path = "../vpc"
}

dependency "mysql" {
  config_path = "../mysql"
}

inputs = {
  env            = "qa"
  basename       = "example-app"
  vpc_id         = dependency.vpc.outputs.vpc_id
  subnet_ids     = dependency.vpc.outputs.subnet_ids
  mysql_endpoint = dependency.mysql.outputs.endpoint
}
```

In this example, the only thing that is different between the environments is the `env` input variable. This means that
except for one line, everything in the config is duplicated across `prod`, `qa`, and `stage`.

To [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself) this up, we will introduce a new folder called `_env`
which will contain the common configurations across the three environments (we prefix with `_` to indicate that this
folder doesn't contain deployable configurations, and so that it is lexically sorted first in the directory listing):

```tree
└── live
    ├── root.hcl
    ├── _env
    │   ├── app.hcl
    │   ├── mysql.hcl
    │   └── vpc.hcl
    ├── prod
    │   ├── app
    │   │   └── terragrunt.hcl
    │   ├── mysql
    │   │   └── terragrunt.hcl
    │   └── vpc
    │       └── terragrunt.hcl
    ├── qa
    │   ├── app
    │   │   └── terragrunt.hcl
    │   ├── mysql
    │   │   └── terragrunt.hcl
    │   └── vpc
    │       └── terragrunt.hcl
    └── stage
        ├── app
        │   └── terragrunt.hcl
        ├── mysql
        │   └── terragrunt.hcl
        └── vpc
            └── terragrunt.hcl
```

In our example, the contents of `_env/app.hcl` would look like the following:

```hcl
terraform {
  source = "github.com/<org>/modules.git//app?ref=v0.1.0"
}

dependency "vpc" {
  config_path = "../vpc"
}

dependency "mysql" {
  config_path = "../mysql"
}

inputs = {
  basename       = "example-app"
  vpc_id         = dependency.vpc.outputs.vpc_id
  subnet_ids     = dependency.vpc.outputs.subnet_ids
  mysql_endpoint = dependency.mysql.outputs.endpoint
}
```

Note that everything is defined except for the `env` input variable. We now modify `qa/app/terragrunt.hcl` to include
this alongside the root configuration by using multiple `include` blocks, significantly reducing our per
environment configuration:

```hcl
include "root" {
  path = find_in_parent_folders("root.hcl")
}

include "env" {
  path = "${get_terragrunt_dir()}/../../_env/app.hcl"
}

inputs = {
  env = "qa"
}
```

## Using exposed includes

In the previous section, we covered using `include` to DRY common component configurations. While powerful, `include` has
a limitation where the included configuration is statically merged into the child configuration.

In our example, note that the `_env/app.hcl` file hardcodes the `app` module version to `v0.1.0` (relevant section
pasted below for convenience):

```hcl
terraform {
  source = "github.com/<org>/modules.git//app?ref=v0.1.0"
}

# ... other blocks omitted for brevity ...
```

What if we want to deploy a different version for each environment? One way you can do this is by redefining the
`terraform` block in the unit. For example, if you want to deploy `v0.2.0` in the `qa` environment, you can do
the following:

```hcl
include "root" {
  path = find_in_parent_folders("root.hcl")
}

include "env" {
  path = "${get_terragrunt_dir()}/../../_env/app.hcl"
}

# Override the terraform.source attribute to v0.2.0
terraform {
  source = "github.com/<org>/modules.git//app?ref=v0.2.0"
}

inputs = {
  env = "qa"
}
```

While this works, we now have duplicated the source URL. To avoid repeating the source URL, we can use exposed includes
to reference data defined in the parent configurations. To do this, we will refactor our parent configuration to expose
the source URL as a local variable instead of defining it into the `terraform` block:

```hcl
locals {
  source_base_url = "github.com/<org>/modules.git//app"
}

# ... other blocks and attributes omitted for brevity ...
```

We then set the `expose` attribute to `true` on the `include` block in the child configuration so that we can reference
the data defined in the included configuration. Using that, we can construct the terraform source URL without having to
repeat the module source:

```hcl
include "root" {
  path = find_in_parent_folders("root.hcl")
}

include "env" {
  path   = "${get_terragrunt_dir()}/../../_env/app.hcl"
  expose = true
}

# Construct the terraform.source attribute using the source_base_url and custom version v0.2.0
terraform {
  source = "${include.env.locals.source_base_url}?ref=v0.2.0"
}

inputs = {
  env = "qa"
}
```

## Using `read_terragrunt_config`

In the previous two sections, we covered using `include` to merge Terragrunt configurations through static merges
with unit configuration. What if you want included configurations to be dynamic in the context of unit where they
are being used?

In our example, the unit configuration defines the `env` input in its configuration (pasted below for convenience):

```hcl
# ... other blocks omitted for brevity ...

inputs = {
  env = "qa"
}
```

What if some inputs depend on this `env` input? For example, what if we want to append the `env` to the `name` input
prior to passing to OpenTofu/Terraform?

One way to do this is to define the override parameters in the child config instead of the parent:

```hcl
# ... other blocks omitted for brevity ...

include "env" {
  path   = "${get_terragrunt_dir()}/../../_env/app.hcl"
  expose = true
}

inputs = {
  env      = "qa"
  basename = "${include.env.locals.basename}-qa"
}
```

While this works, you could lose all the DRY advantages of the include block if you have many configurations that depend
on the `env` input. Instead, you can use `read_terragrunt_config` to load additional context when including configurations
by taking advantage of the folder structure, and define the env based logic in the included configuration.

To show this, let's introduce a new `env.hcl` configuration in each environment:

```tree
└── live
    ├── root.hcl
    ├── _env
    │   ├── app.hcl
    │   ├── mysql.hcl
    │   └── vpc.hcl
    ├── prod
    │   ├── env.hcl
    │   ├── app
    │   │   └── terragrunt.hcl
    │   ├── mysql
    │   │   └── terragrunt.hcl
    │   └── vpc
    │       └── terragrunt.hcl
    ├── qa
    │   ├── env.hcl
    │   ├── app
    │   │   └── terragrunt.hcl
    │   ├── mysql
    │   │   └── terragrunt.hcl
    │   └── vpc
    │       └── terragrunt.hcl
    └── stage
        ├── env.hcl
        ├── app
        │   └── terragrunt.hcl
        ├── mysql
        │   └── terragrunt.hcl
        └── vpc
            └── terragrunt.hcl
```

The `env.hcl` configuration will look like the following:

```hcl
locals {
  env = "qa" # this will be prod in the prod folder, and stage in the stage folder.
}
```

We can then read the `env.hcl` file in the included `_env/app.hcl` file and use the `env` local:

```hcl
locals {
  # Load the relevant env.hcl file based on where the including unit is.
  # This works because find_in_parent_folders always runs in the context of the unit.
  env_vars = read_terragrunt_config(find_in_parent_folders("env.hcl"))
  env_name = local.env_vars.locals.env

  source_base_url = "github.com/<org>/modules.git//app"
}

dependency "vpc" {
  config_path = "../vpc"
}

dependency "mysql" {
  config_path = "../mysql"
}

inputs = {
  env            = local.env_name
  basename       = "example-app-${local.env_name}"
  vpc_id         = dependency.vpc.outputs.vpc_id
  subnet_ids     = dependency.vpc.outputs.subnet_ids
  mysql_endpoint = dependency.mysql.outputs.endpoint
}
```

With this configuration, the `env_vars` local is set based on the location of the unit.

For example, when Terragrunt is run in the context of the `prod/app` unit, `prod/env.hcl` is read,
while `qa/env.hcl` is read when Terragrunt is run in the `qa/app` unit.

Now we can clean up the child config to eliminate the `env` input variable since that is loaded in the `env.hcl` context:

```hcl
include "root" {
  path = find_in_parent_folders("root.hcl")
}

include "env" {
  path   = "${get_terragrunt_dir()}/../../_env/app.hcl"
  expose = true
}

# Construct the terraform.source attribute using the source_base_url and custom version v0.2.0
terraform {
  source = "${include.env.locals.source_base_url}?ref=v0.2.0"
}
```

## Considerations for CI/CD Pipelines

For infrastructure CI/CD pipelines, it is common to only want to run the workflow on the modules that were updated. For
example, if you only changed the unit configuration for the RDS database in the dev account, then you only
want to run `plan` and `apply` on that module, not other components or other accounts.

If you did not take advantage of `include` or `read_terragrunt_config`, then implementing this pipeline is
straightforward: you can use `git diff` to collect all the files that changed, and for those `terragrunt.hcl` files that
were updated, you can run `terragrunt plan` or `terragrunt apply` in that unit.

However, if you use `include` or `read_terragrunt_config`, then a single file change may need to be reflected on
multiple files that were not touched at all in the commit. In our previous example, when a configuration is updated in
the `_env/app.hcl` file, we need to apply the change to all the modules that `include` that common environment
configuration.

The most comprehensive approach to managing this is to use the [--terragrunt-queue-include-units-reading](https://terragrunt.gruntwork.io/docs/reference/cli-options/#terragrunt-queue-include-units-reading)
flag. This flag will automatically add all units that read the file to the queue of units to be run. This includes
both units that include the file, and units that read the file using something like `read_terragrunt_config` (make
to read the documentation on this so that you know the limitations of this flag).

In the previous example, your CI/CD pipeline can run:

```bash
terragrunt run-all plan --terragrunt-queue-include-units-reading _env/app.hcl
```

This will:

- Recursively find all Terragrunt units in the current directory tree.
- Filter out any units that don't include `_env/app.hcl` so that they won't be run.
- Run `plan` on any modules remaining (which will be the set of units in the current tree that include
  `_env/app.hcl`).

Thereby allowing you to only run those modules that need to be updated by the code change.

Alternatively, you can implement a promotion workflow if you have multiple environments that depend on the
`_env/app.hcl` configuration. In the above example, suppose you wanted to progressively roll out the changes through the
environments, `qa`, `stage`, and `prod` in order. In this case, you can use `--terragrunt-working-dir` to scope down the
updates from the common file:

```bash
# Roll out the change to the qa environment first
terragrunt run-all plan --terragrunt-queue-include-units-reading _env/app.hcl --terragrunt-working-dir qa
terragrunt run-all apply --terragrunt-queue-include-units-reading _env/app.hcl --terragrunt-working-dir qa
# If the apply succeeds to qa, move on to the stage environment
terragrunt run-all plan --terragrunt-queue-include-units-reading _env/app.hcl --terragrunt-working-dir stage
terragrunt run-all apply --terragrunt-queue-include-units-reading _env/app.hcl --terragrunt-working-dir stage
# And finally, prod.
terragrunt run-all plan --terragrunt-queue-include-units-reading _env/app.hcl --terragrunt-working-dir prod
terragrunt run-all apply --terragrunt-queue-include-units-reading _env/app.hcl --terragrunt-working-dir prod
```

This allows you to have flexibility in how changes are rolled out. For example, you can add extra validation stages
in-between the roll out to each environment, or add in manual approval between the stages.

**NOTE**: If you identify an issue with rolling out the change in a downstream environment, and want to abort, you will
need to make sure that that environment uses the older version of the common configuration. This is because the common
configuration is now partially rolled out, where some environments need to use the new updated common configuration,
while other environments need the old one. The best way to handle this situation is to create a new copy of the common
configuration at the old version and have the environments that depend on the older version point to that version.
