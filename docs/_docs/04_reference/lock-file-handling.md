---
layout: collection-browser-doc
title: Lock File Handling
category: reference
categories_url: reference
excerpt: Learn how to Terragrunt handles the OpenTofu/Terraform lock file
tags: ["CLI", "DRY"]
order: 407
nav_title: Documentation
nav_title_link: /docs/
redirect_from:
    - /docs/features/lock-file-handling/
---

## How to use lock files with Terragrunt

To use [OpenTofu/Terraform lock files](https://opentofu.org/docs/language/files/dependency-lock/) with Terragrunt, you
need to:

1. Run Terragrunt as usual (e.g., run `terragrunt plan`, `terragrunt apply`, etc.).
1. Check the `.terraform.lock.hcl` file, which will end up sitting next to your `terragrunt.hcl`, into version control.

Everything else with OpenTofu/Terraform and Terragrunt should work as expected. To learn the details of how this works, read on!

## How Terragrunt handles lock files

### What's a lock file?

[Terraform 0.14 added support for a
*lock file*](https://www.hashicorp.com/blog/terraform-0-14-introduces-a-dependency-lock-file-for-providers)
which gets created or updated every time you run `tofu init`/`terraform init`. The file is typically generated into your working
directory (i.e., the folder in which you ran `tofu init`/`terraform init`) and is called `.terraform.lock.hcl`.
It captures the versions of all the OpenTofu/Terraform providers you're using. Normally, you want to check this file into
version control so that when your team members run OpenTofu/Terraform, they get the exact same provider versions.

### The problem with mixing remote OpenTofu/Terraform configurations in Terragrunt and lock files

Let's say you are using Terragrunt with [remote OpenTofu/Terraform
configurations]({{site.baseurl}}/docs/features/units/) and you have the following folder
structure:

```tree
└── live
    ├── prod
    │   └── vpc
    │       └── terragrunt.hcl
    └── stage
        └── vpc
            └── terragrunt.hcl
```

Imagine that in `/live/stage/vpc/terragrunt.hcl`, you have the following contents:

```hcl
terraform {
  source = "git::git@github.com:acme/infrastructure-modules.git//networking/vpc?ref=v0.0.1"
}
```

If you ran `terragrunt apply` in the `/live/stage/vpc` folder, Terragrunt will:

1. `git clone` the VPC module in the `source` URL into a temp folder in `.terragrunt-cache/xxx/vpc`, where `xxx` is
   dynamically determined based on the URL.
1. Run `tofu apply`/`terraform apply` in the `.terragrunt-cache/xxx/vpc` temp folder.

As a result, the `.terraform.lock.hcl` file will be generated in the `.terragrunt-cache/xxx/vpc` temp folder, rather
than in `/live/stage/vpc`.

### How Terragrunt solves this problem

To solve this problem, since version v0.27.0, Terragrunt implements the following logic for lock files:

1. If Terragrunt finds a `.terraform.lock.hcl` file in your working directory (e.g., in `/live/stage/vpc`), before
   running OpenTofu/Terraform, Terragrunt will copy that lock file into the temp folder it uses when running your OpenTofu/Terraform code
   (e.g., `.terragrunt-cache/xxx/vpc`). This way, if you had a lock file checked into version control, Terragrunt will
   respect and use it with your OpenTofu/Terraform code as you'd expect.
1. After running OpenTofu/Terraform, if Terragrunt finds a `.terraform.lock.hcl` in the temp folder (e.g.,
   `.terragrunt-cache/xxx/vpc`), it will copy that lock file back to your working directory (e.g., to `/live/stage/vpc`).
   That way, you can commit the lock file (or the changes to the lock file) to version control as usual.

<!-- markdownlint-disable MD026 -->
### Check the lock file in!

After running Terragrunt on each of your modules, you should check your lock files in! That means your folder structure
should end up looking something like this:

```tree
└── live
    ├── prod
    │   └── vpc
    │       ├── .terraform.lock.hcl
    │       └── terragrunt.hcl
    └── stage
        └── vpc
            ├── .terraform.lock.hcl
            └── terragrunt.hcl
```

Also, any time you change the providers you're using, and re-run `init`, the lock file will be updated, so make sure
to check the updates into version control too.

### Disabling the copy of the generated lock file

In certain use cases, like when using a remote module containing a lock file within it, you probably
don't want Terragrunt to also copy the lock file into your working directory. In these scenarios you, can opt-out of copying
the `.terraform.lock.hcl` file by using `copy_terraform_lock_file = false` in the `terraform` configuration block as follows:

```hcl
terraform {
  ...
  copy_terraform_lock_file = false
}
```
