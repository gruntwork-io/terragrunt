---
layout: collection-browser-doc
title: Lock File Handling
category: features
categories_url: features
excerpt: Learn how to Terragrunt handles the Terraform lock file
tags: ["CLI", "DRY"]
order: 270
nav_title: Documentation
nav_title_link: /docs/
---

## The short version: how to use lock files with Terragrunt

To use [Terraform lock files](https://www.terraform.io/docs/configuration/dependency-lock.html) with Terragrunt, you
need to:

1. Run Terragrunt as usual (e.g., run `terragrunt plan`, `terragrunt apply`, etc.).
1. Check the `.terraform.lock.hcl` file, which will end up sitting next to your `terragrunt.hcl`, into version control.

Everything else with Terraform and Terragrunt should work as expected. To learn the details of how this works, read on!


## The long version: details of how Terragrunt handles lock files

### What's a lock file?

[Terraform 0.14 added support for a 
*lock file*](https://www.hashicorp.com/blog/terraform-0-14-introduces-a-dependency-lock-file-for-providers)
which gets created or updated every time you run `terraform init`. The file is typically generated into your working
directory (i.e., the folder in which you ran `terraform init`) and is called `.terraform.lock.hcl`.
It captures the versions of all the Terraform providers you're using. Normally, you want to check this file into 
version control so that when your team members run Terraform, they get the exact same provider versions.

### The problem with mixing remote Terraform configurations in Terragrunt and lock files

Let's say you are using Terragrunt with [remote Terraform 
configurations]({{site.baseurl}}/docs/features/keep-your-terraform-code-dry/) and you have the following folder 
structure:

```
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
1. Run `terraform apply` in the `.terragrunt-cache/xxx/vpc` temp folder.

As a result, the `.terraform.lock.hcl` file will be generated in the `.terragrunt-cache/xxx/vpc` temp folder, rather 
than in `/live/stage/vpc`.     

### How Terragrunt solves this problem

To solve this problem, since version v0.27.0, Terragrunt implements the following logic for lock files:

1. If Terragrunt finds a `.terraform.lock.hcl` file in your working directory (e.g., in `/live/stage/vpc`), before 
   running Terraform, Terragrunt will copy that lock file into the temp folder it uses when running your Terraform code 
   (e.g., `.terragrunt-cache/xxx/vpc`). This way, if you had a lock file checked into version control, Terragrunt will 
   respect and use it with your Terraform code as you'd expect.
1. After running Terraform, if Terragrunt finds a `.terraform.lock.hcl` in the temp folder (e.g., 
   `.terragrunt-cache/xxx/vpc`), it will copy that lock file back to your working directory (e.g., to `/live/stage/vpc`). 
   That way, you can commit the lock file (or the changes to the lock file) to version control as usual.
   
### Check the lock file in!

After running Terragrunt on each of your modules, you should check your lock files in! That means your folder structure
should end up looking something like this:

```
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