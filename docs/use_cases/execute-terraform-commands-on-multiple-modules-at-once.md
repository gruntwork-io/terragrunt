---
title: Execute Terraform commands on multiple modules at once
layout: single
author_profile: true
sidebar:
  nav: "working-with-multiple-modules-at-once"
---

## Motivation

Let's say your infrastructure is defined across multiple Terraform modules:

```
root
├── backend-app
│   └── main.tf
├── frontend-app
│   └── main.tf
├── mysql
│   └── main.tf
├── redis
│   └── main.tf
└── vpc
    └── main.tf
```

There is one module to deploy a frontend-app, another to deploy a backend-app, another for the MySQL database, and so
on. To deploy such an environment, you'd have to manually run `terraform apply` in each of the subfolder, wait for it
to complete, and then run `terraform apply` in the next subfolder. How do you avoid this tedious and time-consuming
process?


## Commands

Terragrunt provides some commands to work with multiple modules at once: `apply-all`, `destroy-all`, `output-all` and `plan-all`.
To be able to deploy multiple Terraform modules in a single command, add a `terraform.tfvars` file to each module:

```
root
├── backend-app
│   ├── main.tf
│   └── terraform.tfvars
├── frontend-app
│   ├── main.tf
│   └── terraform.tfvars
├── mysql
│   ├── main.tf
│   └── terraform.tfvars
├── redis
│   ├── main.tf
│   └── terraform.tfvars
└── vpc
    ├── main.tf
    └── terraform.tfvars
```

Inside each `terraform.tfvars` file, add a `terragrunt = { ... }` block to identify this as a module managed by
Terragrunt (the block can be empty or include any of the configs described in this documentation):

```json
terragrunt = {
  # Put your Terragrunt configuration here
}
```

Now you can go into the `root` folder and deploy all the modules within it by using the `apply-all` command:

```
cd root
terragrunt apply-all
```

When you run this command, Terragrunt will recursively look through all the subfolders of the current working
directory, find all `terraform.tfvars` files with a `terragrunt = { ... }` block, and run `terragrunt apply` in each
one concurrently.

Similarly, to undeploy all the Terraform modules, you can use the `destroy-all` command:

```
cd root
terragrunt destroy-all
```

To see the currently applied outputs of all of the subfolders, you can use the `output-all` command:

```
cd root
terragrunt output-all
```

Finally, if you make some changes to your project, you could evaluate the impact by using `plan-all` command:

Note: It is important to realize that you could get errors running `plan-all` if you have dependencies between your projects
and some of those dependencies haven't been applied yet.

_Ex: If module A depends on module B and module B hasn't been applied yet, then plan-all will show the plan for B,
but exit with an error when trying to show the plan for A._

```
cd root
terragrunt plan-all
```

If your modules have dependencies between them—for example, you can't deploy the backend-app until MySQL and redis are
deployed—you'll need to express those dependencies in your Terragrunt configuration as explained in the next section.


## Dependencies between modules

Consider the following file structure:

```
root
├── backend-app
│   ├── main.tf
│   └── terraform.tfvars
├── frontend-app
│   ├── main.tf
│   └── terraform.tfvars
├── mysql
│   ├── main.tf
│   └── terraform.tfvars
├── redis
│   ├── main.tf
│   └── terraform.tfvars
└── vpc
    ├── main.tf
    └── terraform.tfvars
```

Let's assume you have the following dependencies between Terraform modules:

* `backend-app` depends on `mysql`, `redis`, and `vpc`
* `frontend-app` depends on `backend-app` and `vpc`
* `mysql` depends on `vpc`
* `redis` depends on `vpc`
* `vpc` has no dependencies

You can express these dependencies in your `terraform.tfvars` config files using a `dependencies` block. For example,
in `backend-app/terraform.tfvars` you would specify:

```json
terragrunt = {
  dependencies {
    paths = ["../vpc", "../mysql", "../redis"]
  }
}
```

Similarly, in `frontend-app/terraform.tfvars`, you would specify:

```json
terragrunt = {
  dependencies {
    paths = ["../vpc", "../backend-app"]
  }
}
```

Once you've specified the dependencies in each `terraform.tfvars` file, when you run the `terragrunt apply-all` or
`terragrunt destroy-all`, Terragrunt will ensure that the dependencies are applied or destroyed, respectively, in the
correct order. For the example at the start of this section, the order for the `apply-all` command would be:

1. Deploy the VPC
1. Deploy MySQL and Redis in parallel
1. Deploy the backend-app
1. Deploy the frontend-app

If any of the modules fail to deploy, then Terragrunt will not attempt to deploy the modules that depend on them. Once
you've fixed the error, it's usually safe to re-run the `apply-all` or `destroy-all` command again, since it'll be a
no-op for the modules that already deployed successfully, and should only affect the ones that had an error the last
time around.

To check all of your dependencies and validate the code in them, you can use the `validate-all` command.


## Testing multiple modules locally

If you are using Terragrunt to configure [Remote Terraform configurations](keep-your-remote-state-configuration-dry) and all
of your modules have the `source` parameter set to a Git URL, but you want to test with a local checkout of the code,
you can use the `--terragrunt-source` parameter:


```
cd root
terragrunt plan-all --terragrunt-source /source/modules
```

If you set the `--terragrunt-source` parameter, the `xxx-all` commands will assume that parameter is pointing to a
folder on your local file system that has a local checkout of all of your Terraform modules. For each module that is
being processed via a `xxx-all` command, Terragrunt will read in the `source` parameter in that module's `.tfvars`
file, parse out the path (the portion after the double-slash), and append the path to the `--terragrunt-source`
parameter to create the final local path for that module.

For example, consider the following `.tfvars` file:

```json
terragrunt = {
  terraform {
    source = "git::git@github.com:acme/infrastructure-modules.git//networking/vpc?ref=v0.0.1"
  }
}
```

If you run `terragrunt apply-all --terragrunt-source: /source/infrastructure-modules`, then the local path Terragrunt
will compute for the module above will be `/source/infrastructure-modules//networking/vpc`.
