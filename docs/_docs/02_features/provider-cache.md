---
layout: collection-browser-doc
title: Provider Caching
category: features
categories_url: features
excerpt: Learn how to use terragrunt provider cache.
tags: ["cache", "provider"]
order: 280
nav_title: Documentation
nav_title_link: /docs/
---

## Provider Caching

Terragrunt has the ability to cache Terraform providers across all Terraform instances, ensuring that each provider is only ever downloaded and stored on disk exactly once.

#### Why caching is useful

Let's imagine that your project consists of 50 Terragrunt modules (terragrunt.hcl), each of them uses the same provider `aws`. Without caching, each of them will download the provider from the Internet and stored in its own `.terraform` directory. For clarity, the downloadable archive `terraform-provider-aws_5.36.0_darwin_arm64.zip` has a size of ~100MB, and when unzipped it takes up ~450MB of disk space. It’s easy to calculate that initializing such a project with 50 modules will cost you 5GB of traffic and 22.5GB of free space instead of 100MB and 450MB using the cache.

#### Why Terraform's built-in provider caching doesn't work

Terraform has provider caching feature [Provider Plugin Cache](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache), that does the job well until you run multiple Terraform processes simultaneously. Then the Terraform processes begin conflict by overwriting each other’s cache, which causes the error like `Error: Failed to install provider`. By default, to speed up processing, Terragrunt runs multiple Terraform modules simultaneously, which in turn prevents the use of [Provider Plugin Cache](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache) to avoid conflicts or force Terragrunt to run Terraform modules sequentially by specifying the `--terragrunt-parallelism 1` flag, which significantly slows down the work.

### Usage

Terragrunt Provider Cache is currently considered an experimental feature, so it is disabled by default. To enable it you need to use the flag [`terragrunt-provider-cache`](https://terragrunt.gruntwork.io/docs/reference/cli-options/#terragrunt-provider-cache):

``` shell
terragrunt run-all apply --terragrunt-provider-cache
```
or the environment variable `TERRAGRUNT_PROVIDER_CACHE`:

``` shell
TERRAGRUNT_PROVIDER_CACHE=1 terragrunt run-all apply
```

By default, cached providers are stored in `terragrunt/providers` folder, which is located in the user cache directory:

* `$HOME/.cache/terragrunt/providers` on Unix systems
* `$HOME/Library/Caches/terragrunt/providers` on Darwin
* `%LocalAppData%\terragrunt\providers` on Windows

The file structure of the cache directory is identical to the Terraform [plugin_cache_dir](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache) directory.
If you already have cached providers by Terraform [plugin_cache_dir](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache) or want to store them in a different directory, you can override this path with your own using the flag [`terragrunt-provider-cache-dir`](https://terragrunt.gruntwork.io/docs /link/cli-options/#terragrunt-provider-cache-dir):

``` shell
terragrunt plan \
--terragrunt-provider-cache \
--terragrunt-provider-cache-dir /new/path/to/cache/dir
```

or the environment variable `TERRAGRUNT_PROVIDER_CACHE_DIR`:

``` shell
TERRAGRUNT_PROVIDER_CACHE=1 \
TERRAGRUNT_PROVIDER_CACHE_DIR=/new/path/to/cache/dir \
terragrunt plan
```

By default, Terragrunt only caches providers from the following registries: `registry.terraform.io`, `registry.opentofu.org`. You can override this list using the flag [`terragrunt-provider-cache-registry-names`](https://terragrunt.gruntwork.io/docs/reference/cli-options/#terragrunt-provider-cache-registry-names):


``` shell
terragrunt apply \
--terragrunt-provider-cache \
--terragrunt-provider-cache-registry-names example1.com \
--terragrunt-provider-cache-registry-names example2.com
```

or the environment variable `TERRAGRUNT_PROVIDER_CACHE_REGISTRY_NAMES`:

``` shell
TERRAGRUNT_PROVIDER_CACHE=1 \
TERRAGRUNT_PROVIDER_CACHE_REGISTRY_NAMES=example1.com,example2.com \
terragrunt apply
```

Since Terragrunt Cache is essentially a Private Registry server that accepts requests from Terraform, downloads and saves providers to the cache directory, there are a few more flags that are unlikely to be needed, but are useful to know about:

* Server Hostname [`terragrunt-provider-cache-hostname`](https://terragrunt.gruntwork.io/docs/reference/cli-options/#terragrunt-provider-cache-hostname), by default, `localhost`.
* Server Port [`terragrunt-provider-cache-port`](https://terragrunt.gruntwork.io/docs/reference/cli-options/#terragrunt-provider-cache-port), assigned automatically  every time you launch the Terragurnt.
* Server Token [`terragrunt-provider-cache-token`](https://terragrunt.gruntwork.io/docs/reference/cli-options/#terragrunt-provider-cache-token), generated automatically every time you launch  the Terragurnt.

To enhance security, Terragrunt uses token-based authentication between Terragrunt Cache Server and the Terraform to prevent unauthorized connections from third-party applications. You can set your own token using any text, there are no requirements. For example:

``` shell
terragrunt apply \
--terragrunt-provider-cache \
--terragrunt-provider-cache-host 192.168.0.100 \
--terragrunt-provider-cache-port 5758 \
--terragrunt-provider-cache-token my-secret
```
or using environment variables:

``` shell
TERRAGRUNT_PROVIDER_CACHE=1 \
TERRAGRUNT_PROVIDER_CACHE_HOST=192.168.0.100 \
TERRAGRUNT_PROVIDER_CACHE_PORT=5758 \
TERRAGRUNT_PROVIDER_CACHE_TOKEN=my-secret \
terragrunt apply
```

### How Terragrunt Provider Caching works

##### Description of the step-by-step:

* Run Terragurn Provider Cache server on localhost.
* Configure Terraform instances to use the cache server as a remote registry:
  * Create local CLI config file `.terragrunt-cache/.terraformrc` that inherits a default Terraform [CLI config file](https://developer.hashicorp.com/terraform/cli/config/config-file) with additional sections:
     * [provider-installation](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-installation) forces first, try to find the required provider in the cache directory and create symlinks to them, otherwise, request it from the remote registry.
     * [host](https://github.com/hashicorp/terraform/issues/28309) forces to make all requests through the Terragrunt Cache Server, which is somewhat of a proxy.
  * Set environment variables:
     * [TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE](https://developer.hashicorp.com/terraform/cli/config/config-file#allowing-the-provider-plugin-cache-to-break-the-dependency-lock-file) allows to generate `.terraform.lock.hcl` files based only on providers from the cache directory.
     * [TF_CLI_CONFIG_FILE](https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_plugin_cache_dir) sets to use just created local CLI config `.terragrunt-cache/.terraformrc`
     * [TF_TOKEN_*](https://developer.hashicorp.com/terraform/cli/config/config-file#environment-variable-credentials) sets per-remote-registry tokens for authentication to Terragrunt Cache Server.
* Call `terraform providers lock -platform=cache_provider` before each `terragrunt init`.
* When Terragurn Cache Server receives a `providers lock` request with the `-platform=cache_provider` arg, it starts caching that provider and returns HTTP status _429 Locked_.
* Terragrunt waits until the requested providers appear in the cache directory.
* Terragrunt runs `terragrunt init`, which in turn finds providers in the cache directory and creates symlinks to them.


##### What a working directory with cached providers looks like

```
├── $HOME/.cache/terragrunt
│   └── providers
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
│   │               │       └── darwin_arm64 -> $HOME/.cache/terragrunt/providers/registry.terraform.io/hashicorp/aws/5.36.0/darwin
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
│   │               │       └── darwin_arm64 -> $HOME/.cache/terragrunt/providers/registry.terraform.io/hashicorp/aws/5.36.0/darwin
│   ├── .terraform.lock.hcl
│   ├── main.tf
│   └── terragrunt.hcl

```
