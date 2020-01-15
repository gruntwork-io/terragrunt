---
layout: collection-browser-doc
title: Quick start
category: getting-started
excerpt: Learn how to start with Terragrunt.
tags: ["Quick Start", "DRY", "backend", "CLI"]
order: 100
nav_title: Documentation
nav_title_link: /docs/
---

## Introduction

Terragrunt is a thin wrapper that provides extra tools for keeping your configurations DRY, working with multiple Terraform modules, and managing remote state.

To use it, you:

1.  [Install Terraform](https://learn.hashicorp.com/terraform/getting-started/install).

2.  [Install Terragrunt](../../_docs/01_getting-started/install.md).

3.  Put your Terragrunt configuration in a `terragrunt.hcl` file. You’ll see several example configurations shortly.

4.  Now, instead of running `terraform` directly, you run the same commands with `terragrunt`:

<!-- end list -->

    terragrunt plan
    terragrunt apply
    terragrunt output
    terragrunt destroy

Terragrunt will forward almost all commands, arguments, and options directly to Terraform, but based on the settings in your `terragrunt.hcl` file, Terragrunt can help you accomplish the following:

1.  [Keep your backend configuration DRY](#keep-your-backend-configuration-dry)

2.  [Keep your Terraform CLI arguments DRY](#keep-your-terraform-cli-arguments-dry)

3.  [Promote immutable, versioned Terraform modules across environments](#promote-immutable-versioned-terraform-modules-across-environments)

## Keep your backend configuration DRY

*Terraform* backends allow you to store Terraform state in a shared location that everyone on your team can access, such as an S3 bucket, and provide locking around your state files to protect against race conditions. To use a Terraform backend, you add a `backend` configuration to your Terraform code:

``` hcl
# stage/frontend-app/main.tf
terraform {
  backend "s3" {
    bucket         = "my-terraform-state"
    key            = "stage/frontend-app/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "my-lock-table"
  }
}
```

The code above tells Terraform to store the state for a `frontend-app` module in an S3 bucket called `my-terraform-state` under the path `stage/frontend-app/terraform.tfstate`, and to use a DynamoDB table called `my-lock-table` for locking. This is a great feature that every single Terraform team uses to collaborate, but it comes with one major gotcha: the `backend` configuration does not support variables or expressions of any sort. That is, the following will NOT work:

``` hcl
# stage/frontend-app/main.tf
terraform {
  backend "s3" {
    # Using variables does NOT work here!
    bucket         = var.terraform_state_bucket
    key            = var.terraform_state_key
    region         = var.terraform_state_region
    encrypt        = var.terraform_state_encrypt
    dynamodb_table = var.terraform_state_dynamodb_table
  }
}
```

That means you have to copy/paste the same `backend` configuration into every one of your Terraform modules. Not only do you have to copy/paste, but you also have to very carefully *not* copy/paste the `key` value so that you don’t have two modules overwriting each other’s state files\! E.g., The `backend` configuration for a `database` module would look nearly identical to the `backend` configuration of the `frontend-app` module, except for a different `key` value:

``` hcl
# stage/mysql/main.tf
terraform {
  backend "s3" {
    bucket         = "my-terraform-state"
    key            = "stage/mysql/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "my-lock-table"
  }
}
```

Terragrunt allows you to keep your `backend` configuration DRY (“Don’t Repeat Yourself”) by defining it once in a root location and inheriting that configuration in all child modules. Let’s say your Terraform code has the following folder layout:

    stage
    ├── frontend-app
    │   └── main.tf
    └── mysql
        └── main.tf

To use Terragrunt, add a single `terragrunt.hcl` file to the root of your repo, in the `stage` folder, and one `terragrunt.hcl` file in each module folder:

    stage
    ├── terragrunt.hcl
    ├── frontend-app
    │   ├── main.tf
    │   └── terragrunt.hcl
    └── mysql
        ├── main.tf
        └── terragrunt.hcl

Next, in each of your Terraform modules, remove the contents of the `backend` configuration:

``` hcl
# stage/mysql/main.tf
terraform {
  # Intentionally empty. Will be filled by Terragrunt.
  backend "s3" {}
}
```

Now you can define your `backend` configuration just once in the root `terragrunt.hcl` file:

``` hcl
# stage/terragrunt.hcl
remote_state {
  backend = "s3"
  config = {
    bucket = "my-terraform-state"

    key = "${path_relative_to_include()}/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "my-lock-table"
  }
}
```

The `terragrunt.hcl` files use the same configuration language as Terraform (HCL) and the configuration is more or less the same as the `backend` configuration you had in each module, except that the `key` value is now using the `path_relative_to_include()` built-in function, which will automatically set `key` to the relative path between the root `terragrunt.hcl` and the child module (so your Terraform state folder structure will match your Terraform code folder structure, which makes it easy to go from one to the other).

The final step is to update each of the child `terragrunt.hcl` files to tell them to include the configuration from the root `terragrunt.hcl`:

``` hcl
# stage/mysql/terragrunt.hcl
include {
  path = find_in_parent_folders()
}
```

The `find_in_parent_folders()` helper will automatically search up the directory tree to find the root `terragrunt.hcl` and inherit the `remote_state` configuration from it.

Now, [install Terragrunt](../../_docs/01_getting-started/install.md), and run all the Terraform commands you’re used to, but with `terragrunt` as the command name rather than `terraform` (e.g., `terragrunt apply` instead of `terraform apply`). To deploy the database module, you would run:

``` bash
$ cd stage/mysql
$ terragrunt apply
```

Terragrunt will automatically find the `mysql` module’s `terragrunt.hcl` file, configure the `backend` using the settings from the root `terragrunt.hcl` file, and, thanks to the `path_relative_to_include()` function, will set the `key` to `stage/mysql/terraform.tfstate`. If you run `terragrunt apply` in `stage/frontend-app`, it’ll do the same, except it will set the `key` to `stage/frontend-app/terraform.tfstate`.

You can now add as many child modules as you want, each with a `terragrunt.hcl` with the `include { …​ }` block, and each of those modules will automatically inherit the proper `backend` configuration\!

## Keep your Terraform CLI arguments DRY

CLI flags are another common source of copy/paste in the Terraform world. For example, a typical pattern with Terraform is to define common account-level variables in an `account.tfvars` file:

``` hcl
# account.tfvars
account_id     = "123456789012"
account_bucket = "my-terraform-bucket"
```

And to define common region-level variables in a `region.tfvars` file:

``` hcl
# region.tfvars
aws_region = "us-east-2"
foo        = "bar"
```

You can tell Terraform to use these variables using the `-var-file` argument:

``` bash
$ terraform apply \
    -var-file=../../common.tfvars \
    -var-file=../region.tfvars
```

Having to remember these `-var-file` arguments every time can be tedious and error prone. Terragrunt allows you to keep your CLI arguments DRY by defining those arguments as code in your `terragrunt.hcl` configuration:

``` hcl
# terragrunt.hcl
terraform {
  extra_arguments "common_vars" {
    commands = ["plan", "apply"]

    arguments = [
      "-var-file=../../common.tfvars",
      "-var-file=../region.tfvars"
    ]
  }
}
```

Now, when you run the `plan` or `apply` commands, Terragrunt will automatically add those arguments:

``` bash
$ terragrunt apply

Running command: terraform with arguments
[apply -var-file=../../common.tfvars -var-file=../region.tfvars]
```

You can even use the `get_terraform_commands_that_need_vars()` built-in function to automatically get the list of all commands that accept `-var-file` and `-var` arguments:

``` hcl
# terragrunt.hcl
terraform {
  extra_arguments "common_vars" {
    commands = get_terraform_commands_that_need_vars()

    arguments = [
      "-var-file=../../common.tfvars",
      "-var-file=../region.tfvars"
    ]
  }
}
```

## Promote immutable, versioned Terraform modules across environments

One of the most important [lessons we’ve learned from writing hundreds of thousands of lines of infrastructure code](https://blog.gruntwork.io/5-lessons-learned-from-writing-over-300-000-lines-of-infrastructure-code-36ba7fadeac1) is that large modules should be considered harmful. That is, it is a Bad Idea to define all of your environments (dev, stage, prod, etc), or even a large amount of infrastructure (servers, databases, load balancers, DNS, etc), in a single Terraform module. Large modules are slow, insecure, hard to update, hard to code review, hard to test, and brittle (i.e., you have all your eggs in one basket).

Therefore, you typically want to break up your infrastructure across multiple modules:

    ├── prod
    │   ├── app
    │   │   ├── main.tf
    │   │   └── outputs.tf
    │   ├── mysql
    │   │   ├── main.tf
    │   │   └── outputs.tf
    │   └── vpc
    │       ├── main.tf
    │       └── outputs.tf
    ├── qa
    │   ├── app
    │   │   ├── main.tf
    │   │   └── outputs.tf
    │   ├── mysql
    │   │   ├── main.tf
    │   │   └── outputs.tf
    │   └── vpc
    │       ├── main.tf
    │       └── outputs.tf
    └── stage
        ├── app
        │   ├── main.tf
        │   └── outputs.tf
        ├── mysql
        │   ├── main.tf
        │   └── outputs.tf
        └── vpc
            ├── main.tf
            └── outputs.tf

The folder structure above shows how to separate the code for each environment (`prod`, `qa`, `stage`) and for each type of infrastructure (apps, databases, VPCs). However, the downside is that it isn’t DRY. The `.tf` files will contain a LOT of duplication. You can reduce it somewhat by defining all the infrastructure in [reusable Terraform modules](https://blog.gruntwork.io/how-to-create-reusable-infrastructure-with-terraform-modules-25526d65f73d), but even the code to instantiate a module—including configuring the `provider`, `backend`, the module’s input variables, and `output` variables—means you still end up with dozens or hundreds of lines of copy/paste for every module in every environment:

``` hcl
# prod/app/main.tf
provider "aws" {
  region = "us-east-1"
  # ... other provider settings ...
}
terraform {
  backend "s3" {}
}
module "app" {
  source = "../../../app"
  instance_type  = "m4.large"
  instance_count = 10
  # ... other app settings ...
}
# prod/app/outputs.tf
output "url" {
  value = module.app.url
}
# ... and so on!
```

Terragrunt allows you to define your Terraform code *once* and to promote a versioned, immutable “artifact” of that exact same code from environment to environment. Here’s a quick overview of how.

First, create a Git repo called `infrastructure-modules` that has your Terraform code (`.tf` files). This is the exact same Terraform code you just saw above, except that any variables that will differ between environments should be exposed as input variables:

``` hcl
# infrastructure-modules/app/main.tf
provider "aws" {
  region = "us-east-1"
  # ... other provider settings ...
}
terraform {
  backend "s3" {}
}
module "app" {
  source = "../../../app"
  instance_type  = var.instance_type
  instance_count = var.instance_count
  # ... other app settings ...
}
# infrastructure-modules/app/outputs.tf
output "url" {
  value = module.app.url
}
# infrastructure-modules/app/variables.tf

variable "instance_type" {}
variable "instance_count" {}
```

Once this is in place, you can release a new version of this module by creating a Git tag:

``` bash
$ git tag -a "v0.0.1" -m "First release of app module"
$ git push --follow-tags
```

Now, in another Git repo called `infrastructure-live`, you create the same folder structure you had before for all of your environments, but instead of lots of copy/pasted `.tf` files for each module, you have just a single `terragrunt.hcl` file:

    # infrastructure-live
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

The contents of each `terragrunt.hcl` file look something like this:

``` hcl
# infrastructure-live/prod/app/terragrunt.hcl
terraform {
  source =
    "github.com:foo/infrastructure-modules.git//app?ref=v0.0.1"
}
inputs = {
  instance_count = 10
  instance_type  = "m4.large"
}
```

The `terragrunt.hcl` file above sets the `source` parameter to point at the `app` module you just created in your `infrastructure-modules` repo, using the `ref` parameter to specify version `v0.0.1` of that repo. It also configures the variables for this module for the `prod` environment in the `inputs = {…​}` block.

The `terragrunt.hcl` file in the `stage` environment will look similar, but it will configure smaller/fewer instances in the `inputs = {…​}` block to save money:

``` hcl
# infrastructure-live/stage/app/terragrunt.hcl
terraform {
  source =
    "github.com:foo/infrastructure-modules.git//app?ref=v0.0.1"
}
inputs = {
  instance_count = 3
  instance_type  = "t2.micro"
}
```

When you run `terragrunt apply`, Terragrunt will download your `app` module into a temporary folder, run `terraform apply` in that folder, passing the module the input variables you specified in the `inputs = {…​}` block:

``` bash
$ terragrunt apply
Downloading Terraform configurations from github.com:foo/infrastructure-modules.git...
Running command: terraform with arguments [apply]...
```

This way, each module in each environment is defined by a single `terragrunt.hcl` file that solely specifies the Terraform module to deploy and the input variables specific to that environment. This is about as DRY as you can get\!

Moreover, you can specify a different version of the module to deploy in each environment\! For example, after making some changes to the `app` module in the `infrastructure-modules` repo, you could create a `v0.0.2` tag, and update just the `qa` environment to run this new version:

``` bash
# infrastructure-live/qa/app/terragrunt.hcl
terraform {
  source =
    "github.com:foo/infrastructure-modules.git//app?ref=v0.0.2"
}
inputs = {
  instance_count = 3
  instance_type  = "t2.micro"
}
```

If it works well in the `qa` environment, you could promote the exact same code to the `stage` environment by updating its `terragrunt.hcl` file to run `v0.0.2`. And finally, if that code works well in `stage`, you could again promote the exact same code to `prod` by updating that `terragrunt.hcl` file to use `v0.0.2` as well.

![Using Terragrunt to promote immutable Terraform code across environments]({{site.baseurl}}/assets/img/collections/docs/promote-immutable-Terraform-code-across-envs.png)

If at any point you hit a problem, it will only affect the one environment, and you can roll back by deploying a previous version number. That’s immutable infrastructure at work\!

## Next steps

Now that you’ve seen the basics of Terragrunt, here is some further reading to learn more:

1.  [Use cases](../../_docs/01_getting-started/use-cases.md): Learn about the core use cases Terragrunt supports.

2.  [Documentation](../../README.md): Check out the detailed Terragrunt documentation.

3.  [*Terraform: Up & Running*](https://www.terraformupandrunning.com/): This book is the fastest way to get up and running with Terraform\! Terragrunt is a direct implementation of many of the ideas from this book.
