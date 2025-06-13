---
layout: collection-browser-doc
title: Terragrunt Stacks
category: migrate
categories_url: migrate
excerpt: Migration guide to rewrite configurations to use Terragrunt Stacks
tags: ["migration", "community"]
order: 604
nav_title: Documentation
nav_title_link: /docs/
slug: terragrunt-stacks
---

## Migrating from the `terragrunt-infrastructure-live-example` repository

If you have an existing repository that was started using the [terragrunt-infrastructure-live-example](https://github.com/gruntwork-io/terragrunt-infrastructure-live-example) repository as a starting point, follow the steps below to migrate your existing configurations to take advantage of the patterns available using Terragrunt Stacks.

### Step 1: Assess your current infrastructure

Before you get started adjusting any of your existing configurations, it's important to understand the current state of your infrastructure.

How much of it do you regularly update? Does any of it result in frustration or difficulty? Why?

Determine whether it's a good time to be migrating your infrastructure to new patterns, and if so, how much of it you're willing to migrate. If you are happy, and successful with your current patterns, you may not need to migrate any existing configuration, and that's great! Consider this a best practice that you can adopt when you start to introduce new infrastructure, and that you may want to adjust your existing infrastructure configurations over time to take advantage of new patterns.

The advantages of using the new paradigm with Terragrunt Stacks are:

- You can more easily manage your infrastructure at scale.
- You can more easily manage your infrastructure in different environments.
- You can more easily manage your infrastructure across multiple accounts and regions.
- You can more easily manage your infrastructure across multiple teams and organizations.

We, at Gruntwork, generally consider this paradigm to be the best practice for managing Infrastructure as Code (IaC) at scale, which is why we've created this migration guide to help you transition to it.

If you get overwhelmed at any point, read the [Support docs](/docs/community/support/) to learn how you can get help.

### Step 2: Update your Terragrunt version

Now that you've determined that you want to migrate some or all of your infrastructure to new patterns, the next step is to ensure that you have a version of Terragrunt that supports the `terragrunt.stack.hcl` file.

You can do this by updating the version of Terragrunt you use to the latest available version. If you would like more information on how to update your Terragrunt version, see the [Installation](/docs/getting-started/install/) guide.

### Step 3: Add `.terragrunt-stack` directories to your repository `.gitignore` file

Now that you're adopting Terragrunt Stacks, you'll want to add the `.terragrunt-stack` directories to your repository `.gitignore` file.

```bash
echo ".terragrunt-stack" >> .gitignore
git add .gitignore
git commit -m "Add .terragrunt-stack to .gitignore"
```

This will prevent you from accidentally committing `.terragrunt-stack` directories to your repository, which is good because you can always regenerate them on demand using the `terragrunt stack generate` command.

All other `terragrunt stack` commands also automatically generate the `.terragrunt-stack` directory on demand, so you can safely ignore it.

### Step 4: Re-define existing infrastructure using `terragrunt.stack.hcl` files

The infrastructure that you already have can be re-defined using `terragrunt.stack.hcl` files, reducing the amount of code that you need to maintain in your repository.

To do this, you'll need to:

<!-- Note to maintainers:
    We have a repository here: [infrastructure-catalog](https://github.com/gruntwork-io/terragrunt-infrastructure-catalog-example), but we have not yet published it, so we'll just be vague about this for now.

    Units example link: [units](https://github.com/gruntwork-io/terragrunt-infrastructure-catalog-example/tree/main/units)
    Usage example link: [terragrunt.stack.hcl examples](https://github.com/gruntwork-io/terragrunt-infrastructure-catalog-example/tree/main/examples/terragrunt/stacks)
-->

1. Create an `infrastructure-catalog` repository if you don't already have one to store your infrastructure configurations.
2. Define the units that you want to reproduce from your `infrastructure-catalog` repository in your `infrastructure-live` repository via `terragrunt.stack.hcl` files.
3. Find a collection of units that you want to abstract into a stack, and define a `terragrunt.stack.hcl` file for them.

   For example, say you have a collection of units like this, that you want to abstract into a stack:

   ```tree
   non-prod
   └── us-east-1
       └── stateful-ec2-asg-service
           ├── service
           │   └── terragrunt.hcl
           ├── db
           │   └── terragrunt.hcl
           └── sgs
               └── asg
                   └── terragrunt.hcl
   ```

   This collection of units can be abstracted into a single stack by creating a `terragrunt.stack.hcl` file in the `stateful-ec2-asg-service` directory that references each unit configuration, as defined in your `infrastructure-catalog` repository (in this example, the `infrastructure-catalog` repository is hosted at `git@github.com:acme/infrastructure-catalog.git`):

   ```hcl
    ## non-prod/us-east-1/stateful-ec2-asg-service/terragrunt.stack.hcl

   unit "service" {
     source = "git::git@github.com:acme/infrastructure-catalog.git//units/ec2-asg-stateful-service"
     path   = "service"

     no_dot_terragrunt_stack = true

     ## Add any additional configuration for the service unit here
   }

   unit "db" {
     source = "git::git@github.com:acme/infrastructure-catalog.git//units/mysql"
     path   = "db"

     no_dot_terragrunt_stack = true

     ## Add any additional configuration for the db unit here
   }

   unit "asg-sg" {
     source = "git::git@github.com:acme/infrastructure-catalog.git//units/security-group"
     path   = "sgs/asg"

     no_dot_terragrunt_stack = true

     ## Add any additional configuration for the asg-sg unit here
   }
   ```

   **Note the use of the `no_dot_terragrunt_stack` attribute.** This is used to prevent Terragrunt from automatically generating the units into a `.terragrunt-stack` directory. This is important, because you are probably using `path_relative_to_include()` in the `key` attribute of the `remote_state` block of the root `root.hcl` file, which is included in every unit. By specifying `no_dot_terragrunt_stack = true`, the generated units will be generated into the same directory as they were before, and the `path_relative_to_include()` function will resolve to the same path as before. Migrating to a `terragrunt.stack.hcl` file in this way allows you to migrate your infrastructure to the new patterns outlined here at your own pace, and to migrate state between the old and new patterns if you want to.

   Now, you can remove the existing unit configurations, and regenerate them on demand using the `terragrunt stack generate` command.

   ```bash
   cd non-prod/us-east-1/stateful-ec2-asg-service
   rm -rf service db sgs
   ```

   If you have identical unit configurations after performing the following, you can remove the unit configurations again, add them to a `.gitignore` file, and commit the new `terragrunt.stack.hcl` file.

   ```bash
   terragrunt stack generate
   ```

   ```tree
   non-prod
   └── us-east-1
       └── stateful-ec2-asg-service
           ├── terragrunt.stack.hcl
           ├── service
           │   └── terragrunt.hcl ## This should be identical to the unit configuration before
           ├── db
           │   └── terragrunt.hcl ## This should be identical to the unit configuration before
           └── sgs
               └── asg/
                   └── terragrunt.hcl ## This should be identical to the unit configuration before
   ```

   Now that you've confirmed generation is working, you can remove the unit configurations again, add them to a `.gitignore` file, and commit the new `terragrunt.stack.hcl` file.

   ```bash
   rm -rf service db sgs
   git add terragrunt.stack.hcl service db sgs
   git commit -m "Remove unit configurations and add terragrunt.stack.hcl"
   echo "service" >> .gitignore
   echo "db" >> .gitignore
   echo "sgs" >> .gitignore
   git add .gitignore
   git commit -m "Add unit configurations to .gitignore"
   ```

   Your repository should now look like this:

   ```tree
   non-prod
   └── us-east-1
       └── stateful-ec2-asg-service
           ├── .gitignore
           └── terragrunt.stack.hcl
   ```

   You can repeat this process as much as you want, abstracting more and more of your infrastructure into Terragrunt Stacks.

### Step 5: Remove reliance on the `_envcommon` directory

The `_envcommon` directory is no longer needed to create "Don't Repeat Yourself" (DRY) configurations with Terragrunt, and is no longer recommended as a best practice.

If you would like to remove usage of the `_envcommon` directory, you can do so by replacing usage of the `include` block referencing the `_envcommon` directory with content directly committed to `terragrunt.hcl` files.

For example, say you have a `terragrunt.hcl` file that looks like this:

```hcl
## non-prod/us-east-1/mysql/terragrunt.hcl

include "root" {
  path = find_in_parent_folders("root.hcl")
}

include "envcommon" {
  path = "${dirname(find_in_parent_folders("root.hcl"))}/_envcommon/mysql.hcl"
  expose = true
}

terraform {
  source = "${include.envcommon.locals.base_source_url}?ref=v0.8.0"
}

inputs = {
  instance_class    = "db.t2.medium"
  allocated_storage = 100
}
```

and an `_envcommon/mysql.hcl` file that looks like this:

```hcl
## _envcommon/mysql.hcl

locals {
  environment_vars = read_terragrunt_config(find_in_parent_folders("env.hcl"))

  env = local.environment_vars.locals.environment

  base_source_url = "git::git@github.com:acme/infrastructure-catalog.git//modules/mysql"
}

inputs = {
  name              = "mysql_${local.env}"
  instance_class    = "db.t2.micro"
  allocated_storage = 20
  storage_type      = "standard"
  master_username   = "admin"
}
```

This pattern was previously used to create "Don't Repeat Yourself" (DRY) configurations with Terragrunt. However, this pattern is no longer recommended as a best practice, and is no longer needed to create DRY configurations with Terragrunt.

Instead, you can create a `terragrunt.hcl` file in your `infrastructure-catalog` repository that looks like this:

```hcl
## units/mysql/terragrunt.hcl

include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "git::git@github.com:acme/infrastructure-catalog.git//modules/mysql?ref=${values.version}"
}

inputs = {
  ## Required inputs
  name              = values.name
  instance_class    = values.instance_class
  allocated_storage = values.allocated_storage
  storage_type      = values.storage_type
  master_username   = values.master_username
  master_password   = values.master_password

  ## Optional inputs
  skip_final_snapshot = try(values.skip_final_snapshot, null)
  engine_version      = try(values.engine_version, null)
}
```

Then reference that `terragrunt.hcl` file in your `terragrunt.stack.hcl` files, like so:

```hcl
## non-prod/us-east-1/terragrunt.stack.hcl

unit "mysql" {
  source = "git::git@github.com:acme/infrastructure-catalog.git//units/mysql"
  path   = "mysql"

  ## As discussed above, this prevents Terragrunt from automatically generating the units into a `.terragrunt-stack` directory.
  no_dot_terragrunt_stack = true

  values = {
    version = "v0.8.0"
    name = "mysql_dev"
    instance_class = "db.t2.micro"
    allocated_storage = 20
    storage_type = "standard"
    master_username = "admin"
  }
}
```

Now, all your unit configurations can be found directly in the `terragrunt.hcl` file in the `infrastructure-catalog` repository, without having to bounce around between different included or referenced files, and you have an explicit interface for the values that can be set externally, via the `values` attribute.

Different environments can pin different versions of the unit, and that allows for easy atomic updates (and rollbacks) of both OpenTofu/Terraform module versions and Terragrunt unit configurations if needed.

### Step 6: Update your CI/CD pipeline

Chances are, if you're currently performing Terragrunt updates via a CI/CD pipeline (and you aren't using [Gruntwork Pipelines](https://www.gruntwork.io/platform/pipelines)), your CI/CD pipeline doesn't yet have integration with Terragrunt Stacks.

There are a few options for how to proceed here.

<!--
    Note to maintainers: This is basically ready to go, but we're not announcing it yet, so we'll leave this commented out for now.

1. You can start using [Gruntwork Pipelines](https://www.gruntwork.io/platform/pipelines).

   This is a little self-serving, as it's a paid product that we offer at Gruntwork, but we are constantly refining it as the best way to manage IaC at scale via GitOps, and we've built first-class support for Terragrunt Stacks into it, so it's the most straightforward way to get Terragrunt Stacks support out of the box.
-->

1. You can simply commit the generated `.terragrunt-stack` directories to your repository.

   This is the easiest option when managing CI/CD yourself, but it also means that you won't gain some of the benefits that come from using Terragrunt Stacks. When getting started, however, this is a good way to avoid the additional technical debt that comes from having to update your CI/CD pipeline to support Terragrunt Stacks, while learning how to use `terragrunt.stack.hcl` files, and reorganizing your infrastructure configurations.

   To do this, remove the `.terragrunt-stack` entry from your `.gitignore` file, and commit the changes to your repository. You can then manually run the `terragrunt stack generate` command to generate the `.terragrunt-stack` directories on demand, and commit them to your repository, allowing your CI/CD pipeline to completely ignore the fact that you're using Terragrunt Stacks. The units generated by the `terragrunt stack generate` command are completely compatible with units that you can author manually, so you don't have to worry about any incompatibility issues that might arise from this approach.

1. You can configure your CI/CD pipeline to run the `terragrunt stack generate` command whenever your pipeline runs, and leverage the generated `.terragrunt-stack` directories in your pipeline.

   Depending on the complexity of your CI/CD pipeline, this might be as simple as performing the following:

   ```bash
   terragrunt stack generate
   terragrunt run --all plan/apply --non-interactive
   ```

   This doesn't account for destroys or the reduction of blast radius for changes by carefully inspecting Git diffs, but it's a good start for users that don't have CI/CD pipelines that are too complex.

   There is an open RFC in GitHub ([Filter Flag](https://github.com/gruntwork-io/terragrunt/issues/4060)) that would allow for this kind of complex filtering out of the box with Terragrunt, but at the moment, it's still an open RFC.
