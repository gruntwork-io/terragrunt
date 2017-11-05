---
title: Keep your Terraform code DRY
layout: single
author_profile: true
sidebar:
  nav: "keep-your-terraform-code-dry"
---

## Motivation

Consider the following file structure, which defines three environments (prod, qa, stage) with the same infrastructure
in each one (an app, a MySQL database, and a VPC):

```
└── live
    ├── prod
    │   ├── app
    │   │   └── main.tf
    │   ├── mysql
    │   │   └── main.tf
    │   └── vpc
    │       └── main.tf
    ├── qa
    │   ├── app
    │   │   └── main.tf
    │   ├── mysql
    │   │   └── main.tf
    │   └── vpc
    │       └── main.tf
    └── stage
        ├── app
        │   └── main.tf
        ├── mysql
        │   └── main.tf
        └── vpc
            └── main.tf
```

The contents of each environment will be more or less identical, except perhaps for a few settings (e.g. the prod
environment may run bigger or more servers). As the size of the infrastructure grows, having to maintain all of this
duplicated code between environments becomes more error prone. You can reduce the amount of copy paste using
[Terraform modules](https://blog.gruntwork.io/how-to-create-reusable-infrastructure-with-terraform-modules-25526d65f73d),
but even the code to instantiate a module and set up input variables, output variables, providers, and remote state
can still create a lot of maintenance overhead.

How can you keep your Terraform code [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself) so that you only
have to define it once, no matter how many environments you have?


## Remote Terraform configurations

Terragrunt has the ability to download remote Terraform configurations. The idea is that you define the Terraform code
for your infrastructure just once, in a single repo, called, for example, `modules`:

```
└── modules
    ├── app
    │   └── main.tf
    ├── mysql
    │   └── main.tf
    └── vpc
        └── main.tf
```

This repo contains typical Terraform code, with one difference: anything in your code that should be different between
environments should be exposed as an input variable. For example, the `app` module might expose the following
variables:

```json
variable "instance_count" {
  description = "How many servers to run"
}

variable "instance_type" {
  description = "What kind of servers to run (e.g. t2.large)"
}
```

These variables allow you to run smaller/fewer servers in qa and stage to save money and larger/more servers in prod to
ensure availability and scalability.

In a separate repo, called, for example, `live`, you define the code for all of your environments, which now consists
of just one `.tfvars` file per component (e.g. `app/terraform.tfvars`, `mysql/terraform.tfvars`, etc). This gives you
the following file layout:

```
└── live
    ├── prod
    │   ├── app
    │   │   └── terraform.tfvars
    │   ├── mysql
    │   │   └── terraform.tfvars
    │   └── vpc
    │       └── terraform.tfvars
    ├── qa
    │   ├── app
    │   │   └── terraform.tfvars
    │   ├── mysql
    │   │   └── terraform.tfvars
    │   └── vpc
    │       └── terraform.tfvars
    └── stage
        ├── app
        │   └── terraform.tfvars
        ├── mysql
        │   └── terraform.tfvars
        └── vpc
            └── terraform.tfvars
```

Notice how there are no Terraform configurations (`.tf` files) in any of the folders. Instead, each `.tfvars` file
specifies a `terraform { ... }` block that specifies from where to download the Terraform code, as well as the
environment-specific values for the input variables in that Terraform code. For example,
`stage/app/terraform.tfvars` may look like this:

```json
terragrunt = {
  terraform {
    source = "git::git@github.com:foo/modules.git//app?ref=v0.0.3"
  }
}

instance_count = 3
instance_type = "t2.micro"
```

*(Note: the double slash (`//`) is intentional and required. It's part of Terraform's Git syntax for [module
sources](https://www.terraform.io/docs/modules/sources.html). Terraform may display a "Terraform initialized in an empty 
directory" warning, but you can safely ignore it.)*

And `prod/app/terraform.tfvars` may look like this:

```json
terragrunt = {
  terraform {
    source = "git::git@github.com:foo/modules.git//app?ref=v0.0.1"
  }
}

instance_count = 10
instance_type = "m2.large"
```

Notice how the two `terraform.tfvars` files set the `source` URL to the same `app` module, but at different
versions (i.e. `stage` is testing out a newer version of the module). They also set the parameters for the
`app` module to different values that are appropriate for the environment: smaller/fewer servers in `stage`
to save money, larger/more instances in `prod` for scalability and high availability.

Check out the [terragrunt-infrastructure-modules-example](https://github.com/gruntwork-io/terragrunt-infrastructure-modules-example)
and [terragrunt-infrastructure-live-example](https://github.com/gruntwork-io/terragrunt-infrastructure-live-example) 
repos for fully-working sample code that demonstrates this new folder structure.




## How to use remote configurations

Once you've set up your `live` and `modules` repositories, all you need to do is run `terragrunt` commands in the
`live` repository. For example, to deploy the `app` module in qa, you would do the following:

```
cd live/qa/app
terragrunt apply
```

When Terragrunt finds the `terraform` block with a `source` parameter in `live/qa/app/terraform.tfvars` file, it will:

1. Download the configurations specified via the `source` parameter into a temporary folder. This downloading is done
   by using the [terraform init command](https://www.terraform.io/docs/commands/init.html), so the `source` parameter
   supports the exact same syntax as the [module source](https://www.terraform.io/docs/modules/sources.html) parameter,
   including local file paths, Git URLs, and Git URLs with `ref` parameters (useful for checking out a specific tag,
   commit, or branch of Git repo). Terragrunt will download all the code in the repo (i.e. the part before the
   double-slash `//`) so that relative paths work correctly between modules in that repo.

1. Copy all files from the current working directory into the temporary folder. This way, Terraform will automatically
   read in the variables defined in the `terraform.tfvars` file.

1. Execute whatever Terraform command you specified in that temporary folder.


## Achieve DRY Terraform code and immutable infrastructure

With this new approach, copy/paste between environments is minimized. The `.tfvars` files contain solely the variables
that are different between environments. To create a new environment, you copy an old one and update just the
environment-specific values in the `.tfvars` files, which is about as close to the "essential complexity" of the
problem as you can get.

Just as importantly, since the Terraform module code is now defined in a single repo, you can version it (e.g., using Git
tags and referencing them using the `ref` parameter in the `source` URL, as in the `stage/app/terraform.tfvars` and
`prod/app/terraform.tfvars` examples above), and promote a single, immutable version through each environment (e.g.,
qa -> stage -> prod). This idea is inspired by Kief Morris' blog post [Using Pipelines to Manage Environments with
Infrastructure as Code](https://medium.com/@kief/https-medium-com-kief-using-pipelines-to-manage-environments-with-infrastructure-as-code-b37285a1cbf5).


## Working locally

If you're testing changes to a local copy of the `modules` repo, you you can use the `--terragrunt-source` command-line
option or the `TERRAGRUNT_SOURCE` environment variable to override the `source` parameter. This is useful to point
Terragrunt at a local checkout of your code so you can do rapid, iterative, make-a-change-and-rerun development:

```
cd live/stage/app
terragrunt apply --terragrunt-source ../../../modules//app
```

*(Note: the double slash (`//`) here too is intentional and required. Terragrunt downloads all the code in the folder
before the double-slash into the temporary folder so that relative paths between modules work correctly. Terraform may
display a "Terraform initialized in an empty directory" warning, but you can safely ignore it.)*


## Important gotcha: Terragrunt caching

The first time you set the `source` parameter to a remote URL, Terragrunt will download the code from that URL into a tmp folder. 
It will *NOT* download it again afterwords unless you change that URL. That's because downloading code—and more importantly,
reinitializing remote state, redownloading provider plugins, and redownloading modules—can take a long time. To avoid adding 10-90
seconds of overhead to every Terragrunt command, Terragrunt assumes all remote URLs are immutable, and only downloads them once.

Therefore, when working locally, you should use the `--terragrunt-source` parameter and point it at a local file path as described 
in the previous section. Terragrunt will copy the local files every time you run it, which is nearly instantaneous, and doesn't
require reinitializing everything, so you'll be able to iterate quickly.

If you need to force Terragrunt to redownload something from a remote URL, run Terragrunt with the `--terragrunt-source-update` flag
and it'll delete the tmp folder, download the files from scratch, and reinitialize everything. This can take a while, so avoid it
and use `--terragrunt-source` when you can!

## Important gotcha: working with relative file paths

One of the gotchas with downloading Terraform configurations is that when you run `terragrunt apply` in folder `foo`,
Terraform will actually execute in some temporary folder such as `/tmp/foo`. That means you have to be especially
careful with relative file paths, as they will be relative to that temporary folder and not the folder where you ran
Terragrunt!

In particular:

* **Command line**: When using file paths on the command line, such as passing an extra `-var-file` argument, you
  should use absolute paths:

```bash
# Use absolute file paths on the CLI!
terragrunt apply -var-file /foo/bar/extra.tfvars
```

* **Terragrunt configuration**: When using file paths directly in your Terragrunt configuration (`terraform.tfvars`),
  such as in an `extra_arguments` block, you can't use hard-coded absolute file paths, or it won't work on your
  teammates' computers. Therefore, you should utilize the Terragrunt built-in function `get_tfvars_dir()` to use
  a relative file path:

```json
terragrunt = {
  terraform {
    source = "git::git@github.com:foo/modules.git//frontend-app?ref=v0.0.3"

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
  }
}
```

  See the [get_tfvars_dir()](#get_tfvars_dir) documentation for more details.


## Using Terragrunt with private Git repos

The easiest way to use Terragrunt with private Git repos is to use SSH authentication. 
Configure your Git account so you can use it with SSH 
(see the [guide for GitHub here](https://help.github.com/articles/connecting-to-github-with-ssh/))
and use the SSH URL for your repo, prepended with `git::ssh://`: 

```json
terragrunt = {
  terraform {
    source = "git::ssh://git@github.com/foo/modules.git//path/to/module?ref=v0.0.1"
  }
}
```
Look up the Git repo for your repository to find the proper format. 

Note: In automated pipelines, you may need to run the following command for your 
Git repository prior to calling `terragrunt` to ensure that the ssh host is registered 
locally, e.g.:

```bash
ssh -T -oStrictHostKeyChecking=no git@github.com || true
```
