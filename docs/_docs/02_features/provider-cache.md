---
layout: collection-browser-doc
title: Provider Cache
category: features
categories_url: features
excerpt: Learn how to use provider cache with run-all commands.
tags: ["cache"]
order: 280
nav_title: Documentation
nav_title_link: /docs/
---

## Provider Cache

Terraform has provider caching feature [Provider Plugin Cache](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache), that does the job well until you run multiple Terraform processes simultaneously. Then the Terraform processes begin conflict by overwriting each other’s cache, which causes the error `Error: Failed to install provider`. By default, to speed up processing, Terragrunt with the [`run-all`](https://terragrunt.gruntwork.io/docs/reference/cli-options/#run-all) command processes modules simultaneously, which in turn prevents the use of [Provider Plugin Cache](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache) to avoid conflicts or force Terragrunt to process modules sequentially by specifying the `--terragrunt-parallelism 1` flag, which significantly slows down the work.

#### Why using cache so necessary

Let's imagine that your project consists of 50 Terragrunt modules (terragrunt.hcl), each of them uses the same provider `aws`. Without caching, each of them will download the provider from the Internet and stored in its own `.terraform` directory. For clarity, the downloadable archive `terraform-provider-aws_5.36.0_darwin_arm64.zip` has a size of ~100MB, and when unzipped it takes up ~450MB of disk space. It’s easy to calculate that initializing such a project with 50 modules will cost you 5GB of traffic and 22.5GB of free space instead of 100MB and 450MB using the cache.

#### How Terragrunt Porvider` Cache works

Terragrant has a built-in Private Register. Before running Terraform processes, Terragrunt configures the shell environment to force Terraform to query for providers through the built-in registry, which in turn creates a shared providers cache, by downloading the same provider only once and storing them on disk in a single instance. Then using [Provider Installation](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-installation) we force Terraform processes to create symlinks to providers from the shared cache instead storing large binary files. To create `.terraform.lock.hcl` files super fast, Terragrunt enables Terraform [_plugin_cache_may_break_dependency_lock_file_](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-installation) feature, which allows Terraform to generate `.terraform.lock.hcl` files by relying only on provider hashes from the shared cache. Below is what a working directory with cached providers looks like:

```
├── .terragrunt-cache
│   ├── .terraformrc
│   └── provider-cache
│       └── registry.terraform.io
│           └── hashicorp
│               ├── aws
│               │   └── 5.36.0
│               │       └── darwin_arm64
│               │           └── terraform-provider-aws_v5.36.0_x5
├── app1
│   ├── .terraform
│   │   └── providers
│   │       └── registry.terraform.io
│   │           └── hashicorp
│   │               ├── aws
│   │               │   └── 5.36.0
│   │               │       └── darwin_arm64 -> /.terragrunt-cache/provider-cache/registry.terraform.io/hashicorp/aws/5.36.0/darwin
│   ├── .terraform.lock.hcl
│   ├── main.tf
│   └── terragrunt.hcl
├── app2
│   ├── .terraform
│   │   └── providers
│   │       └── registry.terraform.io
│   │           └── hashicorp
│   │               ├── aws
│   │               │   └── 5.36.0
│   │               │       └── darwin_arm64 -> /.terragrunt-cache/provider-cache/registry.terraform.io/hashicorp/aws/5.36.0/darwin
│   ├── .terraform.lock.hcl
│   ├── main.tf
│   └── terragrunt.hcl

```

#### Usage

Terragrunt Provider Cache is currently considered an experimental feature, so it is disabled by default. To enable it you need to use the [`terragrunt-provider-cache`](https://terragrunt.gruntwork.io/docs/reference/cli-options/#terragrunt-provider-cache) flag. By default, the shared cached is stored in `.terragrunt-cache/provider-cache` relative to working directory. You can override this path with your own by using the [`terragrunt-provider-cache-dir`](https://terragrunt.gruntwork.io/docs/reference/cli-options/#terragrunt-provider-cache-dir) flag, which in turn can be very effective by creating one shared cache for all your terragrunt projects. By default, Terragrunt only caches providers from  the following registries: 'registry.terraform.io', 'registry.opentofu.org'. You can be override it by using the [`terragrunt-registry-names`](https://terragrunt.gruntwork.io/docs/reference/cli-options/#terragrunt-registry-names) flag, for example `--terragrunt-registry-names example1.com --terragrunt-registry-names example2.com`.
