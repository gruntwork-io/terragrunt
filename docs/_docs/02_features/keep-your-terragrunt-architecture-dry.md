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

  - [Using import to DRY common Terragrunt config](#using-import-to-dry-common-terragrunt-config)

  - [Using read\_terragrunt\_config to DRY parent configurations](#using-read_terragrunt_config-to-dry-parent-configurations)


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
include {
  path = find_in_parent_folders()
}
```

This pattern is useful for global configuration blocks that need to be imported in all of your modules, but what if you
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

To solve this, you can use [import instead of
include]({{site.baseurl}}/docs/reference/config-blocks-and-attributes#import-include), which supports merging multiple
included configurations unlike `include` blocks.

### Using import to DRY common Terragrunt config

Suppose your `qa/app/terragrunt.hcl` configuration looks like the following:

```hcl
include {
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

Note that everything is defined except for the `env` input variable. We now modify `qa/app/terragrunt.hcl` to import
this alongside the root configuration by using `import` blocks instead of `include`, significantly reducing our per
environment configuration:

```hcl
import "root" {
  path = find_in_parent_folders()
}

import "env" {
  path = "${get_terragrunt_dir()}/../../_env/app.hcl"
}

inputs = {
  env = "qa"
}
```

### Using read\_terragrunt\_config to DRY parent configurations

In the previous section, we covered using `import` to DRY common component configurations. While powerful, `import` has
a limitation where the imported configuration is statically merged into the child configuration.

In our example, note that the `_env/app.hcl` file hardcodes the `app `module version to `v0.1.0` (relevant section
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
import "root" {
  path = find_in_parent_folders()
}

import "env" {
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

While this works, we now have duplicated the source URL. To avoid repeating the source URL, we can use
`read_terragrunt_config` to load additional context into the the parent configuration by taking advantage of the folder
structure.

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

We can then load the `env.hcl` file in the `_env/app.hcl` file to change the version based on which environment is
loaded:

```hcl
locals {
  # Load the relevant env.hcl file based on where terragrunt was invoked. This works because find_in_parent_folders
  # always works at the context of the child configuration.
  env_vars = read_terragrunt_config(find_in_parent_folders("env.hcl"))
  env_name = local.env_vars.locals.env

  # Centrally manage what version of the app module is used in each environment. This makes it easier to promote
  # a version from dev -> stage -> prod.
  module_version = {
    qa    = "v0.2.0"
    stage = "v0.1.0"
    prod  = "v0.1.0"
  }
}

terraform {
  source = "github.com/<org>/modules.git//app?ref=${local.module_version[local.env_name]}"
}

dependency "vpc" {
  config_path = "../vpc"
}

dependency "mysql" {
  config_path = "../mysql"
}

inputs = {
  env            = local.env_name
  basename       = "example-app"
  vpc_id         = dependency.vpc.outputs.vpc_id
  subnet_ids     = dependency.vpc.outputs.subnet_ids
  mysql_endpoint = dependency.mysql.outputs.endpoint
}
```

With this configuration, `env_vars` is loaded based on which folder is being invoked. For example, when Terragrunt is
invoked in the `prod/app/terragrunt.hcl` folder, `prod/env.hcl` is loaded, while `qa/env.hcl` is loaded when
Terragrunt is invoked in the `qa/app/terragrunt.hcl` folder.

Now we can keep the same child config even if we have different versions to deploy per environment. As a bonus, we can
further reduce our child config to eliminate the `env` input variable since that is loaded in the `env.hcl` context:

```hcl
import "root" {
  path = find_in_parent_folders()
}

import "env" {
  path = "${get_terragrunt_dir()}/../../_env/app.hcl"
}
```
