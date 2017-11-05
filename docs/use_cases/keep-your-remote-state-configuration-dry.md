---
title: Keep Your Remote State Configuration DRY
layout: single
author_profile: true
sidebar:
  nav: "keep-your-remote-configuration-code-dry"
---

## Motivation

Terraform supports [remote state storage](https://www.terraform.io/docs/state/remote.html) via a variety of
[backends](https://www.terraform.io/docs/backends) that you configure as follows:

```json
terraform {
  backend "s3" {
    bucket         = "my-terraform-state"
    key            = "frontend-app/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "my-lock-table"
  }
}
```

Unfortunately, the `backend` configuration does not support interpolation. This makes it hard to keep your code
[DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself) if you have multiple Terraform modules. For example,
consider the following folder structure, which uses different Terraform modules to deploy a backend app, frontend app,
MySQL database, and a VPC:

```
├── backend-app
│   └── main.tf
├── frontend-app
│   └── main.tf
├── mysql
│   └── main.tf
└── vpc
    └── main.tf
```

To use remote state with each of these modules, you would have to copy/paste the exact same `backend` configuration
into each of the `main.tf` files. The only thing that would differ between the configurations would be the `key`
parameter: e.g., the `key` for `mysql/main.tf` might be `mysql/terraform.tfstate` and the `key` for
`frontend-app/main.tf` might be `frontend-app/terraform.tfstate`.

To keep your remote state configuration DRY, you can use Terragrunt. You still have to specify the `backend` you want
to use in each module, but instead of copying and pasting the configuration settings over and over again into each
`main.tf` file, you can leave them blank:

```json
terraform {
  # The configuration for this backend will be filled in by Terragrunt
  backend "s3" {}
}
```


## Filling in remote state settings with Terragrunt

To fill in the settings via Terragrunt, create a `terraform.tfvars` file in the root folder and in each of the
Terraform modules:

```
├── terraform.tfvars
├── backend-app
│   ├── main.tf
│   └── terraform.tfvars
├── frontend-app
│   ├── main.tf
│   └── terraform.tfvars
├── mysql
│   ├── main.tf
│   └── terraform.tfvars
└── vpc
    ├── main.tf
    └── terraform.tfvars
```

In your **root** `terraform.tfvars` file, you can define your entire remote state configuration just once in a
`remote_state` block, as follows:

```json
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      bucket         = "my-terraform-state"
      key            = "${path_relative_to_include()}/terraform.tfstate"
      region         = "us-east-1"
      encrypt        = true
      dynamodb_table = "my-lock-table"
    }
  }
}
```

The `remote_state` block supports all the same [backend types](https://www.terraform.io/docs/backends/types/index.html)
as Terraform. The next time you run `terragrunt`, it will automatically configure all the settings in the
`remote_state.config` block, if they aren't configured already, by calling [terraform
init](https://www.terraform.io/docs/commands/init.html).

In each of the **child** `terraform.tfvars` files, such as `mysql/terraform.tfvars`, you can tell Terragrunt to
automatically include all the settings from the root `terraform.tfvars` file as follows:

```json
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }
}
```

The `include` block tells Terragrunt to use the exact same Terragrunt configuration from the `terraform.tfvars` file
specified via the `path` parameter. It behaves exactly as if you had copy/pasted the Terraform configuration from 
the root `terraform.tfvars` file into `mysql/terraform.tfvars`, but this approach is much easier to maintain!

The child `.tfvars` file's `terragrunt.terraform` settings will be merged into the parent file's `terragrunt.terraform`
settings as follows:

* If an `extra_arguments` block in the child has the same name as an `extra_arguments` block in the parent,
  then the child's block will override the parent's.
  * Specifying an empty `extra_arguments` block in a child with the same name will effectively remove the parent's block.
* If an `extra_arguments` block in the child has a different name than `extra_arguments` blocks in the parent,
  then both the parent and child's `extra_arguments` will be effective.
  * The child's `extra_arguments` will be placed _after_ the parent's `extra_arguments` on the terraform command line.
  * Therefore, if a child's and parent's `extra_arguments` include `.tfvars` files with the same variable defined,
    the value from the `.tfvars` file from the child's `extra_arguments` will be used by terraform.
* The `source` field in the child will override `source` field in the parent

Other settings in the child `.tfvars` file's `terragrunt` block (e.g. `remote_state`) override the respective
settings in the parent.

The `terraform.tfvars` files above use two Terragrunt built-in functions:

* `find_in_parent_folders()`: This function returns the path to the first `terraform.tfvars` file it finds in the parent
  folders above the current `terraform.tfvars` file. In the example above, the call to `find_in_parent_folders()` in
  `mysql/terraform.tfvars` will return `../terraform.tfvars`. This way, you don't have to hard code the `path`
  parameter in every module.

* `path_relative_to_include()`: This function returns the relative path between the current `terraform.tfvars`
  file and the path specified in its `include` block. We typically use this in a root `terraform.tfvars` file so that
  each Terraform child module stores its Terraform state at a different `key`. For example, the `mysql` module will
  have its `key` parameter resolve to `mysql/terraform.tfstate` and the `frontend-app` module will have its `key`
  parameter resolve to `frontend-app/terraform.tfstate`.

See [the Interpolation Syntax docs](#interpolation-syntax) for more info.

Check out the [terragrunt-infrastructure-modules-example](https://github.com/gruntwork-io/terragrunt-infrastructure-modules-example)
and [terragrunt-infrastructure-live-example](https://github.com/gruntwork-io/terragrunt-infrastructure-live-example) 
repos for fully-working sample code that demonstrates how to use Terragrunt to manage remote state.




## Create remote state and locking resources automatically

When you run `terragrunt` with `remote_state` configuration, it will automatically create the following resources if
they don't already exist:

* **S3 bucket**: If you are using the [S3 backend](https://www.terraform.io/docs/backends/types/s3.html) for remote
  state storage and the `bucket` you specify in `remote_state.config` doesn't already exist, Terragrunt will create it
  automatically, with [versioning enabled](http://docs.aws.amazon.com/AmazonS3/latest/dev/Versioning.html).

* **DynamoDB table**: If you are using the [S3 backend](https://www.terraform.io/docs/backends/types/s3.html) for
  remote state storage and you specify a `dynamodb_table` (a [DynamoDB table used for
  locking](https://www.terraform.io/docs/backends/types/s3.html#dynamodb_table)) in `remote_state.config`, if that table
  doesn't already exist, Terragrunt will create it automatically, including a primary key called `LockID`.

**Note**: If you specify a `profile` key in `remote_state.config`, Terragrunt will automatically use this AWS profile
when creating the S3 bucket or DynamoDB table.
