---
layout: collection-browser-doc
title: Keep your Terragrunt Architecture DRY
category: features
categories_url: features
excerpt: Learn how to use multiple terragrunt configurations to DRY up your architecture.
tags: ["DRY", "Use cases", "backend"]
order: 210
nav_title: Documentation
nav_title_link: /docs/
---

## Keep your Terragrunt Architecture DRY

  - [Motivation](#motivation)

  - [Using include to DRY common Terragrunt config](#using-include-to-dry-common-terragrunt-config)

  - [Using exposed includes to override common configurations](#using-exposed-includes-to-override-common-configurations)

  - [Using read\_terragrunt\_config to DRY parent configurations](#using-read_terragrunt_config-to-dry-parent-configurations)

  - [Considerations for CI/CD pipelines](#considerations-for-ci-cd-pipelines)


### Motivation

As covered in [Keep your Terraform code DRY]({{site.baseurl}}/docs/features/keep-your-terraform-code-dry) and [Keep your
remote state configuration DRY]({{site.baseurl}}/docs/features/keep-your-remote-state-configuration-dry), it becomes
important to define base Terragrunt configuration files that are included in the child config. For example, you might
have a **root** Terragrunt configuration that defines the remote state and provider configurations:

```hcl
remote_state {
  backend = "s3"
  config = {
    bucket         = "my-terraform-state"
    key            = "${path_relative_to_include()}/terraform.tfstate"
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

You can then include this in each of your **child** `terragrunt.hcl` files using the `include` block for each
infrastructure module you need to deploy:

```hcl
include "root" {
  path = find_in_parent_folders()
}
```

This pattern is useful for global configuration blocks that need to be included in all of your modules, but what if you
have Terragrunt configurations that are only relevant to subsets of your module? For example, consider the following
terragrunt file structure, which defines three environments (`prod`, `qa`, and `stage`) with the same infrastructure in
each one (an app, a MySQL database, and a VPC):

    └── live
        ├── terragrunt.hcl
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

More often than not, each of the services will look similar across the different environments, only requiring small
tweaks. For example, the `app/terragrunt.hcl` files may be identical across all three environments except for an
adjustment to the `instance_type` parameter for each environment. These identical settings don't belong in the root
`terragrunt.hcl` configuration because they are only relevant to the `app` configurations, and not `mysql` or `vpc`.
However, it is cumbersome to copy paste these settings across all three environments.

To solve this, you can use [multiple include blocks]({{site.baseurl}}/docs/reference/config-blocks-and-attributes#include).

### Using include to DRY common Terragrunt config

Suppose your `qa/app/terragrunt.hcl` configuration looks like the following:

```hcl
include "root" {
  path = find_in_parent_folders()
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

To DRY this up, we will introduce a new folder called `_env` which will contain the common configurations across the
three environments (we prefix with `_` to indicate that this folder doesn't contain deployable configurations):

    └── live
        ├── terragrunt.hcl
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
  path = find_in_parent_folders()
}

include "env" {
  path = "${get_terragrunt_dir()}/../../_env/app.hcl"
}

inputs = {
  env = "qa"
}
```

### Using exposed includes to override common configurations

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
`terraform` block in the child config. For example, if you want to deploy `v0.2.0` in the `qa` environment, you can do
the following:

```hcl
include "root" {
  path = find_in_parent_folders()
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
the defined data in the parent configuration. Using that, we can construct the terraform source URL without having to
repeat the module source:

```hcl
include "root" {
  path = find_in_parent_folders()
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


### Using read\_terragrunt\_config to DRY parent configurations

In the previous two sections, we covered using `include` to DRY common component configurations through static merges
with the child configuration. What if you want to dynamically update the parent configuration without having to define
the override blocks in the child config?

In our example, the child configuration defines the `env` input in its configuration (pasted below for convenience):

```hcl
# ... other blocks omitted for brevity ...

inputs = {
  env = "qa"
}
```

What if some inputs depend on this `env` input? For example, what if we want to append the `env` to the `name` input
prior to passing to terraform? One way is to define the override parameters in the child config instead of the parent:

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
on the `env` input. Instead, you can use `read_terragrunt_config` to load additional context into the the parent
configuration by taking advantage of the folder structure, and define the env based logic in the parent configuration.

To do this, we will introduce a new `env.hcl` configuration in each environment:

    └── live
        ├── terragrunt.hcl
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


The `env.hcl` configuration will look like the following:

```hcl
locals {
  env = "qa" # this will be prod in the prod folder, and stage in the stage folder.
}
```

We can then load the `env.hcl` file in the `_env/app.hcl` file to load the `env` string:

```hcl
locals {
  # Load the relevant env.hcl file based on where terragrunt was invoked. This works because find_in_parent_folders
  # always works at the context of the child configuration.
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

With this configuration, `env_vars` is loaded based on which folder is being invoked. For example, when Terragrunt is
invoked in the `prod/app/terragrunt.hcl` folder, `prod/env.hcl` is loaded, while `qa/env.hcl` is loaded when
Terragrunt is invoked in the `qa/app/terragrunt.hcl` folder.

Now we can clean up the child config to eliminate the `env` input variable since that is loaded in the `env.hcl` context:

```hcl
include "root" {
  path = find_in_parent_folders()
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

### Considerations for CI/CD Pipelines

For infrastructure CI/CD pipelines, it is common to only want to run the workflow on the modules that were updated. For
example, if you only changed the `terragrunt.hcl` configuration for the RDS database in the dev account, then you only
want to run `plan` and `apply` on that module, not other components or other accounts.

If you did not take advantage of `include` or `read_terragrunt_config`, then implementing this pipeline is
straightforward: you can use `git diff` to collect all the files that changed, and for those `terragrunt.hcl` files that
were updated, you can run `terragrunt plan` or `terragrunt apply` by passing in the updated file with
`--terragrunt-config`.

However, if you use `include` or `read_terragrunt_config`, then a single file change may need to be reflected on
multiple files that were not touched at all in the commit. In our previous example, when a configuration is updated in
the `_env/app.hcl` file, we need to apply the change to all the modules that `include` that common environment
configuration.

Terragrunt currently does not have any features for supporting this use case when `read_terragrunt_config` is
used. However, for `include` blocks, you can use the
[--terragrunt-modules-that-include]({{site.baseurl}}/docs/reference/cli-options/#terragrunt-modules-that-include) CLI
option for the `run-all` command.

In the previous example, your CI/CD pipeline can run `terragrunt run-all plan --terragrunt-modules-that-include
_env/app.hcl`. This will:

- Recursively find all Terragrunt modules in the current directory tree.
- Filter out any modules that don't include `_env/app.hcl` so that they won't be touched.
- Run `plan` on any modules remaining (which will be the set of modules in the current tree that include
  `_env/app.hcl`).

Thereby allowing you to only touch those modules that need to be updated by the code change.

Alternatively, you can implement a promotion workflow if you have multiple environments that depend on the
`_env/app.hcl` configuration. In the above example, suppose you wanted to progressively roll out the changes through the
environments, `qa`, `stage`, and `prod` in order. In this case, you can use `--terragrunt-working-dir` to scope down the
updates from the common file:

```
# Roll out the change to the qa environment first
terragrunt run-all plan --terragrunt-modules-that-include _env/app.hcl --terragrunt-working-dir qa
terragrunt run-all apply --terragrunt-modules-that-include _env/app.hcl --terragrunt-working-dir qa
# If the apply succeeds to qa, move on to the stage environment
terragrunt run-all plan --terragrunt-modules-that-include _env/app.hcl --terragrunt-working-dir stage
terragrunt run-all apply --terragrunt-modules-that-include _env/app.hcl --terragrunt-working-dir stage
# And finally, prod.
terragrunt run-all plan --terragrunt-modules-that-include _env/app.hcl --terragrunt-working-dir prod
terragrunt run-all apply --terragrunt-modules-that-include _env/app.hcl --terragrunt-working-dir prod
```

This allows you to have flexibility in how changes are rolled out. For example, you can add extra validation stages
inbetween the roll out to each environment, or add in manual approval between the stages.

**NOTE**: If you identify an issue with rolling out the change in a downstream environment, and want to abort, you will
need to make sure that that environment uses the older version of the common configuration. This is because the common
configuration is now partially rolled out, where some environments need to use the new updated common configuration,
while other environments need the old one. The best way to handle this situation is to create a new copy of the common
configuration at the old version and have the environments that depend on the older version point to that version.
