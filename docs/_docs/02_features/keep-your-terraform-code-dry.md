---
layout: collection-browser-doc
title: Keep your OpenTofu/Terraform code DRY
category: features
categories_url: features
excerpt: Learn how to achieve DRY OpenTofu/Terraform code and immutable infrastructure.
tags: ["DRY", "Use cases", "backend"]
order: 200
nav_title: Documentation
nav_title_link: /docs/
---

## Keep your OpenTofu/Terraform code DRY

- [Keep your OpenTofu/Terraform code DRY](#keep-your-opentofuterraform-code-dry)
  - [Motivation](#motivation)
  - [Remote OpenTofu/Terraform configurations](#remote-opentofuterraform-configurations)
  - [Achieve DRY OpenTofu/Terraform code and immutable infrastructure](#achieve-dry-opentofuterraform-code-and-immutable-infrastructure)
  - [Working locally](#working-locally)
  - [Working with lock files](#working-with-lock-files)
  - [Important gotcha: Terragrunt caching](#important-gotcha-terragrunt-caching)
  - [Important gotcha: working with relative file paths](#important-gotcha-working-with-relative-file-paths)
  - [Using Terragrunt with private Git repos](#using-terragrunt-with-private-git-repos)
  - [DRY common OpenTofu/Terraform code with Terragrunt generate blocks](#dry-common-opentofuterraform-code-with-terragrunt-generate-blocks)

### Motivation

Consider the following file structure, which defines three environments (prod, qa, stage) with the same infrastructure in each one (an app, a MySQL database, and a VPC):

```tree
└── live
    ├── prod
    │   ├── app
    │   │   └── main.tf
    │   ├── mysql
    │   │   └── main.tf
    │   └── vpc
    │       └── main.tf
    ├── qa
    │   ├── app
    │   │   └── main.tf
    │   ├── mysql
    │   │   └── main.tf
    │   └── vpc
    │       └── main.tf
    └── stage
        ├── app
        │   └── main.tf
        ├── mysql
        │   └── main.tf
        └── vpc
            └── main.tf
```

The contents of each environment will be more or less identical, except perhaps for a few settings (e.g. the prod environment may run bigger or more servers). As the size of the infrastructure grows, having to maintain all of this duplicated code between environments becomes more error prone. You can reduce the amount of copy paste using [OpenTofu/Terraform modules](https://blog.gruntwork.io/how-to-create-reusable-infrastructure-with-terraform-modules-25526d65f73d), but even the code to instantiate a module and set up input variables, output variables, providers, and remote state can still create a lot of maintenance overhead.

How can you keep your OpenTofu/Terraform code [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself) so that you only have to define it once, no matter how many environments you have?

### Remote OpenTofu/Terraform configurations

Terragrunt has the ability to download remote OpenTofu/Terraform configurations. The idea is that you define the OpenTofu/Terraform code for your infrastructure just once, in a single repo, called, for example, `modules`:

```tree
└── modules
    ├── app
    │   └── main.tf
    ├── mysql
    │   └── main.tf
    └── vpc
        └── main.tf
```

This repo contains typical OpenTofu/Terraform code, with one difference: anything in your code that should be different between environments should be exposed as an input variable. For example, the `app` module might expose the following variables:

``` hcl
variable "instance_count" {
  description = "How many servers to run"
}

variable "instance_type" {
  description = "What kind of servers to run (e.g. t2.large)"
}
```

These variables allow you to run smaller/fewer servers in qa and stage to save money and larger/more servers in prod to ensure availability and scalability.

In a separate repo, called, for example, `live`, you define the code for all of your environments, which now consists of just one `terragrunt.hcl` file per component (e.g. `app/terragrunt.hcl`, `mysql/terragrunt.hcl`, etc). This gives you the following file layout:

```tree
└── live
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
```

Notice how there are no OpenTofu/Terraform configurations (`.tf` files) in any of the folders. Instead, each `terragrunt.hcl` file specifies a `terraform { …​ }` block that specifies from where to download the OpenTofu/Terraform code, as well as the environment-specific values for the input variables in that OpenTofu/Terraform code. For example, `stage/app/terragrunt.hcl` may look like this:

``` hcl
terraform {
  # Deploy version v0.0.3 in stage
  source = "git::git@github.com:foo/modules.git//app?ref=v0.0.3"
}

inputs = {
  instance_count = 3
  instance_type  = "t2.micro"
}
```

*(Note: the double slash (`//`) in the `source` parameter is intentional and required. It’s part of OpenTofu/Terraform’s Git syntax for [module sources](https://opentofu.org/docs/language/modules/sources/). OpenTofu/Terraform may display a "OpenTofu/Terraform initialized in an empty directory" warning, but you can safely ignore it.)*

And `prod/app/terragrunt.hcl` may look like this:

``` hcl
terraform {
  # Deploy version v0.0.1 in prod
  source = "git::git@github.com:foo/modules.git//app?ref=v0.0.1"
}

inputs = {
  instance_count = 10
  instance_type  = "m2.large"
}
```

You can now deploy the modules in your `live` repo. For example, to deploy the `app` module in stage, you would do the following:

```bash
cd live/stage/app
terragrunt apply
```

When Terragrunt finds the `terraform` block with a `source` parameter in `live/stage/app/terragrunt.hcl` file, it will:

1. Download the configurations specified via the `source` parameter into the `--terragrunt-download-dir` folder (by default `.terragrunt-cache` in the working directory, which we recommend adding to `.gitignore`). This downloading is done by using the same [go-getter library](https://github.com/hashicorp/go-getter) OpenTofu/Terraform uses, so the `source` parameter supports the exact same syntax as the [module source](https://opentofu.org/docs/language/modules/sources/) parameter, including local file paths, Git URLs, and Git URLs with `ref` parameters (useful for checking out a specific tag, commit, or branch of Git repo). Terragrunt will download all the code in the repo (i.e. the part before the double-slash `//`) so that relative paths work correctly between modules in that repo.

2. Copy all files from the current working directory into the temporary folder.

3. Execute whatever OpenTofu/Terraform command you specified in that temporary folder.

4. Pass any variables defined in the `inputs = { …​ }` block as environment variables (prefixed with `TF_VAR_` to your OpenTofu/Terraform code. Notice how the `inputs` block in `stage/app/terragrunt.hcl` deploys fewer and smaller instances than prod.

Check out the [terragrunt-infrastructure-modules-example](https://github.com/gruntwork-io/terragrunt-infrastructure-modules-example) and [terragrunt-infrastructure-live-example](https://github.com/gruntwork-io/terragrunt-infrastructure-live-example) repos for fully-working sample code that demonstrates this new folder structure.

### Achieve DRY OpenTofu/Terraform code and immutable infrastructure

With this new approach, copy/paste between environments is minimized. The `terragrunt.hcl` files contain solely the `source` URL of the module to deploy and the `inputs` to set for that module in the current environment. To create a new environment, you copy an old one and update just the environment-specific `inputs` in the `terragrunt.hcl` files, which is about as close to the "essential complexity" of the problem as you can get.

Just as importantly, since the OpenTofu/Terraform module code is now defined in a single repo, you can version it (e.g., using Git tags and referencing them using the `ref` parameter in the `source` URL, as in the `stage/app/terragrunt.hcl` and `prod/app/terragrunt.hcl` examples above), and promote a single, immutable version through each environment (e.g., qa → stage → prod). This idea is inspired by Kief Morris' blog post [Using Pipelines to Manage Environments with Infrastructure as Code](https://medium.com/@kief/https-medium-com-kief-using-pipelines-to-manage-environments-with-infrastructure-as-code-b37285a1cbf5).

### Working locally

If you’re testing changes to a local copy of the `modules` repo, you can use the `--terragrunt-source` command-line option or the `TERRAGRUNT_SOURCE` environment variable to override the `source` parameter. This is useful to point Terragrunt at a local checkout of your code so you can do rapid, iterative, make-a-change-and-rerun development:

```bash
cd live/stage/app
terragrunt apply --terragrunt-source ../../../modules//app
```

*(Note: the double slash (`//`) here too is intentional and required. Terragrunt downloads all the code in the folder before the double-slash into the temporary folder so that relative paths between modules work correctly. OpenTofu/Terraform may display a "OpenTofu/Terraform initialized in an empty directory" warning, but you can safely ignore it.)*

### Working with lock files

Terraform 0.14 introduced lock files. These should mostly "just work" with Terragrunt version v0.27.0 and above: that
is, the lock file (`.terraform.lock.hcl`) will be generated next to your `terragrunt.hcl`, and you should check it into
version control. See the [Lock File Handling docs]({{site.baseurl}}/docs/features/lock-file-handling/) for more details.

### Important gotcha: Terragrunt caching

The first time you set the `source` parameter to a remote URL, Terragrunt will download the code from that URL into a tmp folder. It will *NOT* download it again afterwards unless you change that URL. That’s because downloading code—and more importantly, reinitializing remote state, redownloading provider plugins, and redownloading modules—can take a long time. To avoid adding 10-90 seconds of overhead to every Terragrunt command, Terragrunt assumes all remote URLs are immutable, and only downloads them once.

Therefore, when working locally, you should use the `--terragrunt-source` parameter and point it at a local file path as described in the previous section. Terragrunt will copy the local files every time you run it, which is nearly instantaneous, and doesn’t require reinitializing everything, so you’ll be able to iterate quickly.

If you need to force Terragrunt to redownload something from a remote URL, run Terragrunt with the `--terragrunt-source-update` flag and it’ll delete the tmp folder, download the files from scratch, and reinitialize everything. This can take a while, so avoid it and use `--terragrunt-source` when you can\!

### Important gotcha: working with relative file paths

One of the gotchas with downloading OpenTofu/Terraform configurations is that when you run `terragrunt apply` in folder `foo`, OpenTofu/Terraform will actually execute in some temporary folder such as `.terragrunt-cache/foo`. That means you have to be especially careful with relative file paths, as they will be relative to that temporary folder and not the folder where you ran Terragrunt\!

In particular:

- **Command line**: When using file paths on the command line, such as passing an extra `-var-file` argument, you should use absolute paths:

    ``` bash
    # Use absolute file paths on the CLI!
    terragrunt apply -var-file /foo/bar/extra.tfvars
    ```

- **Terragrunt configuration**: When using file paths directly in your Terragrunt configuration (`terragrunt.hcl`), such as in an `extra_arguments` block, you can’t use hard-coded absolute file paths, or it won’t work on your teammates' computers. Therefore, you should utilize the Terragrunt built-in function `get_terragrunt_dir()` to use a relative file path:

    ``` hcl
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

        # With the get_terragrunt_dir() function, you can use relative paths!
        arguments = [
          "-var-file=${get_terragrunt_dir()}/../common.tfvars",
          "-var-file=example.tfvars"
        ]
      }
    }
    ```

    See the [get\_terragrunt\_dir()]({{site.baseurl}}/docs/reference/built-in-functions/#get_terragrunt_dir) documentation for more details.

### Using Terragrunt with private Git repos

The easiest way to use Terragrunt with private Git repos is to use SSH authentication. Configure your Git account so you can use it with SSH (see the [guide for GitHub here](https://help.github.com/articles/connecting-to-github-with-ssh/)) and use the SSH URL for your repo:

``` hcl
terraform {
  source = "git@github.com:foo/modules.git//path/to/module?ref=v0.0.1"
}
```

Look up the Git repo for your repository to find the proper format.

Note: In automated pipelines, you may need to run the following command for your Git repository prior to calling `terragrunt` to ensure that the ssh host is registered locally, e.g.:

```bash
ssh -T -oStrictHostKeyChecking=accept-new git@github.com || true
```

### DRY common OpenTofu/Terraform code with Terragrunt generate blocks

Terragrunt has the ability to generate code in to the downloaded remote OpenTofu/Terraform modules before calling out to
`tofu`/`terraform` using the [generate block](/docs/reference/config-blocks-and-attributes#generate). This can be used to
inject common OpenTofu/Terraform configurations into all the modules that you use.

For example, it is common to have custom provider configurations in your code to customize authentication. Consider a
setup where you want to always assume a specific role when calling out to the OpenTofu/Terraform module. However, not all modules
expose the right variables for configuring the `aws` provider so that you can assume the role through OpenTofu/Terraform.

In this situation, you can use Terragrunt `generate` blocks to generate a tf file called `provider.tf` that includes the
provider configuration. Add a root `terragrunt.hcl` file for each of the environments in the file layout for the live
repo:

```bash
└── live
    ├── prod
    │   ├── terragrunt.hcl
    │   ├── app
    │   │   └── terragrunt.hcl
    │   ├── mysql
    │   │   └── terragrunt.hcl
    │   └── vpc
    │       └── terragrunt.hcl
    ├── qa
    │   ├── terragrunt.hcl
    │   ├── app
    │   │   └── terragrunt.hcl
    │   ├── mysql
    │   │   └── terragrunt.hcl
    │   └── vpc
    │       └── terragrunt.hcl
    └── stage
        ├── terragrunt.hcl
        ├── app
        │   └── terragrunt.hcl
        ├── mysql
        │   └── terragrunt.hcl
        └── vpc
            └── terragrunt.hcl
```

Each **root** `terragrunt.hcl` file (the one at the environment level, e.g `prod/terragrunt.hcl`) should define a
`generate` block to generate the AWS provider configuration to assume the role for that environment. For example,
if you wanted to assume the role `arn:aws:iam::0123456789:role/terragrunt` in all the modules for the prod account, you
would put the following in `prod/terragrunt.hcl`:

```hcl
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

This instructs Terragrunt to create the file `provider.tf` in the working directory (where Terragrunt calls `tofu`/`terraform`)
before it calls any of the OpenTofu/Terraform commands (e.g `plan`, `apply`, `validate`, etc). This allows you to inject this
provider configuration in all the modules that includes the root file.

To include this in the child configurations (e.g `mysql/terragrunt.hcl`), you would update all the child modules to
include this configuration using the `include` block:

```hcl
include "root" {
  path = find_in_parent_folders()
}
```

The `include` block tells Terragrunt to use the exact same Terragrunt configuration from the `terragrunt.hcl` file
specified via the `path` parameter. It behaves exactly as if you had copy/pasted the OpenTofu/Terraform configuration from the
included file `generate` configuration into the child, but this approach is much easier to maintain\!
