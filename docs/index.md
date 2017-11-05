---
title: "Quick Start"
excerpt: "A thin wrapper for Terraform that provides extra tools for keeping your Terraform configurations DRY, working with multiple Terraform modules, and managing remote state."
sidebar:
  nav: "docs"
---

1. [Install Terraform](https://www.terraform.io/intro/getting-started/install.html).

1. Install Terragrunt by going to the [Releases Page](https://github.com/gruntwork-io/terragrunt/releases),
   downloading the binary for your OS, renaming it to `terragrunt`, and adding it to your PATH.
     * See the [Install Terragrunt](#install-terragrunt) docs for other installation options.

1. Go into a folder with your Terraform configurations (`.tf` files) and create a `terraform.tfvars` file with a
   `terragrunt = { ... }` block that contains the configuration for Terragrunt (check out the [Use cases](/use_cases)
   section for the types of configuration Terragrunt supports):

```json
terragrunt = {
  # (put your Terragrunt configuration here)
}
```

1. Now, instead of running `terraform` directly, run all the standard Terraform commands using `terragrunt`:

```bash
terragrunt get
terragrunt plan
terragrunt apply
terragrunt output
terragrunt destroy
```

   Terragrunt forwards almost all commands, arguments, and options directly to Terraform, using whatever version of
   Terraform you already have installed. However, based on the settings in your `terraform.tfvars` file, Terragrunt can
   configure remote state, locking, extra arguments, and lots more.

1. Terragrunt is a direct implementation of the ideas expressed in
   [Terraform: Up & Running](http://www.terraformupandrunning.com). Additional background reading that will help
   explain the motivation for Terragrunt includes the Gruntwork.io blog posts
   [How to create reusable infrastructure with Terraform modules](https://blog.gruntwork.io/how-to-create-reusable-infrastructure-with-terraform-modules-25526d65f73d)
   and [How to use Terraform as a team](https://blog.gruntwork.io/how-to-use-terraform-as-a-team-251bc1104973).

1. Check out the [terragrunt-infrastructure-modules-example](https://github.com/gruntwork-io/terragrunt-infrastructure-modules-example)
   and [terragrunt-infrastructure-live-example](https://github.com/gruntwork-io/terragrunt-infrastructure-live-example)
   repos for fully-working sample code that demonstrates how to use Terragrunt.

----

## Basic Usage

Here is a basic setup for a Staging and Production setup, it will show off some of the advantages of using Terragrunt.

```
root
├── prod
│   └── app
│       └── terraform.tfvars
├── stage
│   └── app
│       └── terraform.tfvars
```

`stage/app/terraform.tfvars` may look like this:

```json
terragrunt = {
  terraform {
    source = "git::git@github.com:foo/modules.git//app?ref=v0.0.2"

    extra_arguments "custom_vars" {
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]

      # With the get_tfvars_dir() function, you can use relative paths!
      arguments = [
        "-var-file=${get_tfvars_dir()}/../common.tfvars",
        "-var-file=terraform.tfvars"
      ]
    }

    backend "s3" {
      bucket         = "stage-terraform"
      key            = "app/terraform.tfstate"
      region         = "us-east-1"
      encrypt        = false
      dynamodb_table = "stage-terraform-lock-table"
    }
  }
}

instance_count = 3
instance_type = "t2.micro"
```

And `prod/app/terraform.tfvars` may look like this:

```json
terragrunt = {
  terraform {
    source = "git::git@github.com:foo/modules.git//app?ref=v0.0.1"

    extra_arguments "custom_vars" {
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]

      # With the get_tfvars_dir() function, you can use relative paths!
      arguments = [
        "-var-file=${get_tfvars_dir()}/../common.tfvars",
        "-var-file=terraform.tfvars"
      ]
    }

    backend "s3" {
      bucket         = "production-terraform"
      key            = "app/terraform.tfstate"
      region         = "us-east-1"
      encrypt        = true
      dynamodb_table = "prod-terraform-lock-table"
    }
  }
}

instance_count = 10
instance_type = "m2.large"
```

Terragrunt can help [Keep your Terraform code DRY](/use_cases/keep-your-terraform-code-dry.md) by allow you to use a collection of Terraform modules, that are versioned, and you just provide the unique variables for that environment. Like `instance_count` and `instance_type` in this case.


```json
terragrunt = {
  terraform {
    source = "git::git@github.com:foo/modules.git//app?ref=v0.0.1"
  }
}

instance_count = 10
instance_type = "m2.large"
```


Terragrunt helps you [Keep your remote state configuration DRY](use_cases/keep-your-remote-state-configuration-dry.md) by providing a mechanism for defining your backend and locking state. For example:

```json
backend "s3" {
  bucket         = "production-terraform"
  key            = "app/terraform.tfstate"
  region         = "us-east-1"
  encrypt        = true
  dynamodb_table = "prod-terraform-lock-table"
}
```

Terragrunt helps [Keep your CLI flags DRY](use_cases/keep-your-cli-flags-dry.md) and removes the need to always remember what flags should be passed, by providing the ability
to define them in code. For Example:

```json
extra_arguments "custom_vars" {
  commands = [
    "apply",
    "plan",
    "import",
    "push",
    "refresh"
  ]

  # With the get_tfvars_dir() function, you can use relative paths!
  arguments = [
    "-var-file=${get_tfvars_dir()}/../common.tfvars",
    "-var-file=terraform.tfvars"
  ]
}
```

Terragrunt also helps [Execute Terraform commands on multiple modules at once](use_cases/execute-terraform-commands-on-multiple-modules-at-once.md). The following commands can all be fun on multiple Terragrunt defined environments at once.

```bash
terragrunt apply-all
terragrunt destroy-all
terragrunt output-all
terragrunt plan-all
```

Terragrunt finally makes it easier to [Work with multiple AWS accounts](use_cases/work-with-multiple-aws-accounts.md).
To tell Terragrunt to assume an IAM role, just set the `--terragrunt-iam-role` command line argument:

```bash
terragrunt --terragrunt-iam-role "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME" apply
```

Alternatively, you can set the `TERRAGRUNT_IAM_ROLE` environment variable:

```bash
export TERRAGRUNT_IAM_ROLE="arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
terragrunt apply
```

-----

## Install Terragrunt

Note that third-party Terragrunt packages may not be updated with the latest version, but are often close.
Please check your version against the latest available on the
[Releases Page](https://github.com/gruntwork-io/terragrunt/releases).

### OSX
You can install Terragrunt on OSX using [Homebrew](https://brew.sh/): `brew install terragrunt`.

### Linux

**WARNING**: the snap installer seems to have a bug where it does not allow Terragrunt to work with Terraform and Git dependencies, so we currently do not recommend using it. See the manual install instructions below, instead.

You can install Terragrunt on Linux systems using [snap](https://snapcraft.io/docs/core/install): `snap install terragrunt`.

### Manual
You can install Terragrunt manually by going to the [Releases Page](https://github.com/gruntwork-io/terragrunt/releases),
downloading the binary for your OS, renaming it to `terragrunt`, and adding it to your PATH.

## License

This code is released under the MIT License. See LICENSE.txt.
