# Upgrading to Terragrunt 0.12.x

## Background

Terragrunt was originally created to support two features that were not available in Terraform: defining remote state
configuration in a file (rather than via CLI commands) and locking. As of version 0.9.0, Terraform now supports both of
these features natively, so we have made some changes to Terragrunt:

1. Terragrunt still supports remote state configuration, so you can take advantage of Terragrunt's interpolation
   functions.
1. Terragrunt no longer supports locking.


## Migration guide

The following sections outlines the steps you may need to take in order to migrate from Terragrunt <= v0.11.x
to Terragrunt 0.12.x. If you are using Terraform <= 0.8.x, see the upgrade guide on the Terraform website
[Upgrading to Terraform v0.9](https://www.terraform.io/upgrade-guides/0-9.html).

Migration steps include:

* [Define a backend in your Terraform files](#define-a-backend-in-your-terraform-files)
* [Switch from Terragrunt locking to Terraform locking](#switch-from-terragrunt-locking-to-terraform-locking)
* [Migrate from .terragrunt to terraform.tfvars](#migrate-from-terragrunt-to-terraformtfvars)


### Define a backend in your Terraform files

In your Terraform code (the `.tf` files), you must now define a `backend`. The Terraform guides 
[Upgrading to Terraform v0.9](https://www.terraform.io/upgrade-guides/0-9.html) and 
[Backends: Migrating From 0.8.x and Earlier](https://www.terraform.io/docs/backends/legacy-0-8.html)
may be useful to understand backends.

For example, to use S3 as a remote state
backend, you will need to add the following to your Terraform code:

```hcl
# main.tf
terraform {
  # The configuration for this backend will be filled in by Terragrunt
  backend "s3" {}
}
```

Note that you can leave the configuration of the `backend` empty and allow Terragrunt to provide that configuration
instead. This allows you to keep your remote state configuration more DRY by taking advantage of Terragrunt's
interpolation functions:

```hcl
# terraform.tfvars
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      bucket  = "my-terraform-state"
      key     = "${path_relative_to_include()}/terraform.tfstate"
      region  = "us-east-1"
      encrypt = true
    }
  }
}
```

### Switch from Terragrunt locking to Terraform locking

Remove any `lock { ... }` blocks from your Terragrunt configurations, as these are no longer supported.

If you were storing remote state in S3 and relying on DynamoDB as a locking mechanism, Terraform now supports that
natively. To enable it, simply add the `lock_table` parameter to your S3 backend configuration. If you configure
your S3 backend using Terragrunt, then Terragrunt will automatically create the `lock_table` for you if that table
doesn't already exist:

```hcl
# terraform.tfvars
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      bucket  = "my-terraform-state"
      key     = "${path_relative_to_include()}/terraform.tfstate"
      region  = "us-east-1"
      encrypt = true

      # Tell Terraform to do locking using DynamoDB. Terragrunt will automatically
      # create this table for you if it doesn't already exist.
      lock_table = "my-lock-table"
    }
  }
}
```

**NOTE**: We recommend using a completely new lock table name and NOT reusing the lock table from older versions of
Terragrunt, as that older table had a different structure than what Terraform expects, and Terragrunt will not
automatically recreate it.

If you would like Terraform to automatically retry locks like Terragrunt did (this is particularly useful when
running Terraform as part of an automated script, such as a CI build), you use an `extra_arguments` block:

```hcl
# terraform.tfvars
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      bucket  = "my-terraform-state"
      key     = "${path_relative_to_include()}/terraform.tfstate"
      region  = "us-east-1"
      encrypt = true

      # Tell Terraform to do locking using DynamoDB. Terragrunt will automatically
      # create this table for you if it doesn't already exist.
      lock_table = "my-lock-table"
    }
  }

  terraform {
    # Force Terraform to keep trying to acquire a lock for up to 20 minutes
    # if someone else already has the lock.
    extra_arguments "retry_lock" {
      commands = [
        "init",
        "apply",
        "refresh",
        "import",
        "plan",
        "taint",
        "untaint"
      ]

      arguments = [
        "-lock-timeout=20m"
      ]
    }
  }
}
```


### Migrate from .terragrunt to terraform.tfvars

The configuration in a `.terragrunt` file is identical to that of the `terraform.tfvars` file, except the
`terraform.tfvars` file requires you to wrap that configuration in a `terragrunt = { ... }` block.

For example, if this is your `.terragrunt` file:

```hcl
include {
  path = "${find_in_parent_folders()}"
}

dependencies {
  paths = ["../vpc", "../mysql", "../redis"]
}
```

The equivalent `terraform.tfvars` file is:

```hcl
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }

  dependencies {
    paths = ["../vpc", "../mysql", "../redis"]
  }
}
```

To migrate, all you need to do is:

1. Copy all the contents of the `.terragrunt` file.
1. Paste those contents into a `terragrunt = { ... }` block in a `terraform.tfvars` file.
1. Delete the `.terragrunt` file.
