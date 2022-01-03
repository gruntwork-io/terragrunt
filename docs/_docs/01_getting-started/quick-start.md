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

2.  [Install Terragrunt]({{site.baseurl}}/docs/getting-started/install/).

3.  Put your Terragrunt configuration in a `terragrunt.hcl` file. You’ll see several example configurations shortly.

4.  Now, instead of running `terraform` directly, you run the same commands with `terragrunt`:

<!-- end list -->

    terragrunt plan
    terragrunt apply
    terragrunt output
    terragrunt destroy

Terragrunt will forward almost all commands, arguments, and options directly to Terraform, but based on the settings in your `terragrunt.hcl` file.

## Example

Here is an example configuration you can use to get started. The following configuration can be used to deploy the
[terraform-aws-modules/vpc](https://registry.terraform.io/modules/terraform-aws-modules/vpc/aws/latest) module from the
[Terraform Registry](https://registry.terraform.io/):

_terragrunt.hcl_
```
# Indicate where to source the terraform module from.
# The URL used here is a shorthand for
# "tfr://registry.terraform.io/terraform-aws-modules/vpc/aws?version=3.5.0".
# Note the extra `/` after the protocol is required for the shorthand
# notation.
terraform {
  source = "tfr:///terraform-aws-modules/vpc/aws?version=3.5.0"
}

# Indicate what region to deploy the resources into
generate "provider" {
  path = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents = <<EOF
provider "aws" {
  region = "us-east-1"
}
EOF
}

# Indicate the input values to use for the variables of the module.
inputs = {
  name = "my-vpc"
  cidr = "10.0.0.0/16"

  azs             = ["eu-west-1a", "eu-west-1b", "eu-west-1c"]
  private_subnets = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  public_subnets  = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]

  enable_nat_gateway = true
  enable_vpn_gateway = false

  tags = {
    Terraform = "true"
    Environment = "dev"
  }
}
```

In the configuration, the `terraform` block is used to configure how Terragrunt will interact with Terraform. You can
configure things like before and after hooks for indicating custom commands to run before and after each terraform call,
or what CLI args to pass in for each commands. Here we only use it to indicate where terragrunt should fetch the
terraform code using the `source` attribute. We indicate that terragrunt should fetch the code from the
`terraform-aws-modules/vpc/aws` module hosted in the [Public Terraform Registry](https://registry.terraform.io), version
`3.5.0`. This is indicated by using the `tfr://` protocol in the source URL, which takes the form:

```
tfr://REGISTRY_DOMAIN/MODULE?version=VERSION
```

Note that you can omit the `REGISTRY_DOMAIN` to default to the Public Terraform Registry.

The `generate` block is used to inject the provider configuration into the active Terraform module. This can be used to
customize how Terraform interacts with the cloud APIs, including configuring authentication parameters.

The `inputs` block is used to indicate what variable values should be passed to terraform. This is equivalent to having
the contents of the map in a tfvars file and passing that to terraform.

You can read more about all the supported blocks of the terragrunt configuration in the [reference
documentation](https://terragrunt.gruntwork.io/docs/reference/config-blocks-and-attributes), including additional
sources that terragrunt supports.

You can deploy this example by copy pasting it into a folder and running `terragrunt apply`.

NOTE: Heads up, not all Registry modules can be deployed with Terragrunt, see [A note aboud using modules from the
registry]({{ site.baseurl }}/docs/reference/config-blocks-and-attributes#a-note-about-using-modules-from-the-registry) for details.

## Key features

Terragrunt can help you accomplish the following:

1.  [Keep your backend configuration DRY](#keep-your-backend-configuration-dry)

1.  [Keep your provider configuration DRY](#keep-your-provider-configuration-dry)

1.  [Keep your Terraform CLI arguments DRY](#keep-your-terraform-cli-arguments-dry)

1.  [Promote immutable, versioned Terraform modules across environments](#promote-immutable-versioned-terraform-modules-across-environments)

### Keep your backend configuration DRY

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

Now you can define your `backend` configuration just once in the root `terragrunt.hcl` file:

``` hcl
# stage/terragrunt.hcl
remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
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

The `generate` attribute is used to inform Terragrunt to generate the Terraform code for configuring the backend. When
you run any Terragrunt command, Terragrunt will generate a `backend.tf` file with the contents set to the `terraform`
block that configures the `s3` backend, just like what we had before in each `main.tf` file.

The final step is to update each of the child `terragrunt.hcl` files to tell them to include the configuration from the root `terragrunt.hcl`:

``` hcl
# stage/mysql/terragrunt.hcl
include "root" {
  path = find_in_parent_folders()
}
```

The `find_in_parent_folders()` helper will automatically search up the directory tree to find the root `terragrunt.hcl` and inherit the `remote_state` configuration from it.

Now, [install Terragrunt]({{site.baseurl}}/docs/getting-started/install), and run all the Terraform commands you’re used to, but with `terragrunt` as the command name rather than `terraform` (e.g., `terragrunt apply` instead of `terraform apply`). To deploy the database module, you would run:

``` bash
$ cd stage/mysql
$ terragrunt apply
```

Terragrunt will automatically find the `mysql` module’s `terragrunt.hcl` file, configure the `backend` using the settings from the root `terragrunt.hcl` file, and, thanks to the `path_relative_to_include()` function, will set the `key` to `stage/mysql/terraform.tfstate`. If you run `terragrunt apply` in `stage/frontend-app`, it’ll do the same, except it will set the `key` to `stage/frontend-app/terraform.tfstate`.

You can now add as many child modules as you want, each with a `terragrunt.hcl` with the `include "root" { …​ }` block, and each of those modules will automatically inherit the proper `backend` configuration\!

### Keep your provider configuration DRY

Unifying provider configurations across all your modules can be a pain, especially when you want to customize
authentication credentials. To configure Terraform to assume an IAM role before calling out to AWS, you need to add a
`provider` block with the `assume_role` configuration:

```
# stage/frontend-app/main.tf
provider "aws" {
  assume_role {
    role_arn = "arn:aws:iam::0123456789:role/terragrunt"
  }
}
```

This code tells Terraform to assume the role `arn:aws:iam::0123456789:role/terragrunt` prior to calling out to the AWS
APIs to create the resources. Unlike the `backend` configurations, `provider` configurations support variables, so
typically you will resolve this by making the role configurable in the module:

```
# stage/frontend-app/main.tf
variable "assume_role_arn" {
  description = "Role to assume for AWS API calls"
}

provider "aws" {
  assume_role {
    role_arn = var.assume_role_arn
  }
}
```

You would then copy paste this configuration in every one of your Terraform modules. This isn't a lot of lines of code,
but can be a pain to maintain. For example, if you needed to modify the configuration to expose another parameter (e.g
`session_name`), you would have to then go through each of your modules to make this change.

In addition, what if you wanted to directly deploy a general purpose module, such as that from the [Terraform module
registry](https://registry.terraform.io/) or the [Gruntwork Infrastructure as Code
library](https://gruntwork.io/infrastructure-as-code-library/)? These modules typically do not expose provider
configurations as it is tedious to expose every single provider configuration parameter imaginable through the module
interface.

Terragrunt allows you to refactor common Terraform code to keep your Terraform modules DRY. Just like with the `backend`
configuration, you can define the `provider` configurations once in a root location. In the root `terragrunt.hcl` file,
you would define the `provider` configuration using the `generate` block:

```hcl
# stage/terragrunt.hcl
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

This instructs Terragrunt to create the file `provider.tf` in the working directory (where Terragrunt calls `terraform`)
before it calls any of the Terraform commands (e.g `plan`, `apply`, `validate`, etc). This allows you to inject this
provider configuration in all the modules that includes the root file without having to define them in the underlying
modules.

When you run `terragrunt plan` or `terragrunt apply`, you can see that this file is created in the module working
directory:

``` bash
$ cd stage/mysql
$ terragrunt apply
$ find . -name "provider.tf"
.terragrunt-cache/some-unique-hash/provider.tf
```


### Keep your Terraform CLI arguments DRY

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

### Promote immutable, versioned Terraform modules across environments

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

![Using Terragrunt to promote immutable Terraform code across environments]({{site.baseurl}}/assets/img/collections/documentation/promote-immutable-Terraform-code-across-envs.png)

If at any point you hit a problem, it will only affect the one environment, and you can roll back by deploying a previous version number. That’s immutable infrastructure at work\!

## Next steps

Now that you’ve seen the basics of Terragrunt, here is some further reading to learn more:

1.  [Use cases]({{site.baseurl}}/docs/#features): Learn about the core use cases Terragrunt supports.

2.  [Documentation]({{site.baseurl}}/docs/): Check out the detailed Terragrunt documentation.

3.  [*Terraform: Up & Running*](https://www.terraformupandrunning.com/): This book is the fastest way to get up and running with Terraform\! Terragrunt is a direct implementation of many of the ideas from this book.
