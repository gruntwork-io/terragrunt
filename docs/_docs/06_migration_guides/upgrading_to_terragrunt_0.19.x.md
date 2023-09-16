---
layout: collection-browser-doc
title: Terragrunt 0.19 migration guide
category: upgrade
categories_url: upgrade
excerpt: Migration guide to upgrade to terragrunt 0.19.x
tags: ["migration", "community"]
order: 601
nav_title: Documentation
nav_title_link: /docs/
---

# Upgrading to Terragrunt 0.19.x

## Background

Terraform 0.12 was released in May, 2019, and it included a few major changes:

1. More strict rules around what can go in a `.tfvars` file. In particular, any variable defined in a `.tfvars` file
   that does not match a corresponding `variable` definition in your `.tf` files produces an error.
1. A shift from HCL to HCL2 as the main syntax. This included support for first-class expressions (i.e., using variables
   and functions without having to wrap everything in `${...}`).

Before version 0.19.0, Terragrunt had you define its configuration in a `terragrunt = { ... }` variable in
a `terraform.tfvars` file, but due to item (1) this no longer works with Terraform 0.12 and newer. That means we had to
move to a new file format. This requires a migration, which is unfortunate, but as a nice benefit, item (2)
gives us a nicer syntax and new functionality!




## Migration guide

The following sections outline the steps you may need to take in order to migrate from Terragrunt <= v0.18.x
to Terragrunt 0.19.x and newer:

1. [Move from terraform.tfvars to terragrunt.hcl](#move-from-terraformtfvars-to-terragrunthcl)
1. [Move input variables into inputs](#move-input-variables-into-inputs)
1. [Use first-class expressions](#use-first-class-expressions)
1. [Check attributes vs blocks usage](#check-attributes-vs-blocks-usage)
1. [Rename a few built-in functions ](#rename-a-few-built-in-functions)
1. [Use terraform \<0.12](#use-older-terraform)

Check out [this PR in the terragrunt-infrastructure-live-example
repo](https://github.com/gruntwork-io/terragrunt-infrastructure-live-example/pull/17) for an example of what the code
changes look like.


### Move from terraform.tfvars to terragrunt.hcl

Since Terraform 0.12 has more strict rules about what can go into `terraform.tfvars` files, you now need to move your
Terragrunt configuration from `terraform.tfvars` to a `terragrunt.hcl` file, removing the `terragrunt = { ... }`
wrapping along the way.

For example, if you had the following in `terraform.tfvars`:

```hcl
# terraform.tfvars

terragrunt = {
  terraform {
    source = "git::git@github.com:foo/modules.git//frontend-app?ref=v0.0.3"

    extra_arguments "custom_vars" {
      commands  = ["apply", "plan"]
      arguments = ["-var", "foo=42"]
    }
  }

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
}
```

You would migrate this to `terragrunt.hcl` as follows:

```hcl
# terragrunt.hcl

terraform {
  source = "git::git@github.com:foo/modules.git//frontend-app?ref=v0.0.3"

  extra_arguments "custom_vars" {
    commands  = ["apply", "plan"]
    arguments = ["-var", "foo=42"]
  }
}

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
```


### Move input variables into inputs

When we were using `terraform.tfvars` files for Terragrunt configuration, we were piggybacking on the fact that
Terraform [automatically loads variables from tfvars
files](https://www.terraform.io/docs/configuration/variables.html#variable-definitions-tfvars-files) to set variables
for our modules:

```hcl
# terraform.tfvars

# Terragrunt configuration
terragrunt = {
  terraform {
    # ...
  }

  remote_state {
    # ...
  }
}

# Input variables to set for your Terraform module
instance_type  = "t2.micro"
instance_count = 10
```

With the move to `terragrunt.hcl`, we no longer get this behavior for free. However, Terragrunt can simulate this
behavior for you if you define your input variables by specifying `inputs = { ... }`:

```hcl
# terragrunt.hcl

terraform {
  # ...
}

remote_state {
  # ...
}

# Input variables to set for your Terraform module
inputs = {
  instance_type  = "t2.micro"
  instance_count = 10
}
```

Whenever you run a Terragrunt command, such as `terragrunt apply`, Terragrunt will make these variables available to
your Terraform module as environment variables.

### Use first-class expressions

Terraform 0.11 only allowed special behavior, such as function calls, using "interpolation syntax," where you wrapped
the code with `${...}`. Terragrunt included a handful of functions you could call using interpolation syntax, but
_only_ within the `terragrunt = { ... }` block:

```hcl
# terraform.tfvars

terragrunt = {
  terraform {
    extra_arguments "retry_lock" {
      # Using a function within interpolation syntax
      commands  = "${get_terraform_commands_that_need_locking()}"
      arguments = ["-lock-timeout=20m"]
    }
  }
}

# Using interpolation syntax outside of the terragrunt config did NOT work before
foo = "${get_env("FOO", "default")}"
```

Terraform 0.12 has moved to HCL2, which has first-class support for expressions. That means you can call functions
without having to wrap them in `${...}`. Terragrunt embraces HCL2, and thanks to HCL2's nice parser, that means we not
only get first-class expressions, but we can also use those expressions _everywhere_ in `terragrunt.hcl`!

```hcl
# terragrunt.hcl

terraform {
  extra_arguments "retry_lock" {
    # Using a function within first-class expressions!
    commands  = get_terraform_commands_that_need_locking()
    arguments = ["-lock-timeout=20m"]
  }
}

inputs = {
  # This now works with Terragrunt 0.19.x and newer!
  foo = get_env("FOO", "default")
}
```

### Check attributes vs blocks usage

HCL2 is more strict about the difference between attributes:

```hcl
# Attributes use an equals sign before the curly brace
foo = {
  bar = "baz"
}
```

And blocks:

```hcl
# Blocks do not use equal signs before the curly brace
foo {
  bar = "baz"
}
```

Since Terragrunt uses HCL2, we now have to be more strict with which parts of the Terragrunt configuration are
attributes and which are blocks:

```hcl
# terragrunt.hcl

# terraform is a block, so make sure NOT to include an equals sign
terraform {
  source = "git::git@github.com:foo/modules.git//frontend-app?ref=v0.0.3"

  # extra_arguments is a block, so make sure NOT to include an equals sign
  extra_arguments "custom_vars" {
    commands  = ["apply", "plan"]
    arguments = ["-var", "foo=42"]
  }
}

# remote_state is a block, so make sure NOT to include an equals sign
remote_state {
  backend = "s3"
  # config is an attribute, so an equals sign is REQUIRED
  config = {
    bucket = "foo"

    # s3_bucket_tags is an attribute, so an equals sign is REQUIRED
    s3_bucket_tags = {
      owner = "terragrunt integration test"
      name = "Terraform state storage"
    }

    # dynamodb_table_tags is an attribute, so an equals sign is REQUIRED
    dynamodb_table_tags = {
      owner = "terragrunt integration test"
      name = "Terraform lock table"
    }

    # accesslogging_bucket_tags is an attribute, so an equals sign is REQUIRED
    accesslogging_bucket_tags = {
      owner = "terragrunt integration test"
      name  = "Terraform access log storage"
    }
  }
}

# include is a block, so make sure NOT to include an equals sign
include {
  path = find_in_parent_folders()
}

# dependencies is a block, so make sure NOT to include an equals sign
dependencies {
  paths = ["../vpc", "../mysql", "../redis"]
}

# Inputs is an attribute, so an equals sign is REQUIRED
inputs = {
  instance_type  = "t2.micro"
  instance_count = 10
}
```


### Rename a few built-in functions

Two built-in functions were renamed:

1. `get_tfvars_dir()` is now called `get_terragrunt_dir()`.
1. `get_parent_tfvars_dir()` is now called `get_parent_terragrunt_dir()`.

Make sure to make the corresponding updates in your `terragrunt.hcl` file!

### Use older Terraform

Although it is not officially supported and not tested, it is still possible to use terraform<0.12 with terragrunt >=0.19.

Just install a different version of terraform into a directory of your choice outside of `PATH` and specify path to the binary in `terragrunt.hcl` as `terraform_binary`, plus you need to lower the version check constraint:

```hcl

terraform_binary = "~/bin/terraform-v11/terraform"
terraform_version_constraint = ">= 0.11"


```
