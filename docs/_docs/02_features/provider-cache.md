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

The file structure of the cache directory is identical to the Terraform [plugin_cache_dir](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache) directory. If you already have a directory with providers cached by Terraform [plugin_cache_dir](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache), you can set this path using the flag [`terragrunt-provider-cache-dir`](https://terragrunt.gruntwork.io/docs /link/cli-options/#terragrunt-provider-cache-dir), to make cache server reuse them.

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

To enhance security, the cache server has authentication to prevent unauthorized connections from third-party applications. You can set your own token using any character set.

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

* Run Terragurn Provider Cache server on localhost.
* Configure Terraform instances to use the cache server as a remote registry:
  * Create local CLI config file `.terragrunt-cache/.terraformrc` that concatenates the user configuration from the Terraform [CLI config file](https://developer.hashicorp.com/terraform/cli/config/config-file) with additional sections:
     * [provider-installation](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-installation) forces Terraform to look for for the required providers in the cache directory and create symbolic links to them, if not found, then request them from the remote registry.
     * [host](https://github.com/hashicorp/terraform/issues/28309) forces Terraform to [forward](#How forwarding Terraform requests through the cache server works) all provider requests through the cache server.
  * Set environment variables:
     * [TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE](https://developer.hashicorp.com/terraform/cli/config/config-file#allowing-the-provider-plugin-cache-to-break-the-dependency-lock-file) allows to generate `.terraform.lock.hcl` files based only on provider hashes from the cache directory.
     * [TF_CLI_CONFIG_FILE](https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_plugin_cache_dir) sets to use just created local CLI config `.terragrunt-cache/.terraformrc`
     * [TF_TOKEN_*](https://developer.hashicorp.com/terraform/cli/config/config-file#environment-variable-credentials) sets per-remote-registry tokens for authentication to cache server.
* Call `terraform providers lock -platform=cache_provider`. When the cache server receives this request, it returns HTTP status _429 Locked_ and starts caching the provider (for any other requests, cache server acts as a proxy):
  * Create the [lock file](https://en.wikipedia.org/wiki/File_locking) to prevent multiple cache servers from overwriting the same provider cache.
  * Download the provider from the remote registry, unpack and store into the cache directory or [create a symlink](#Why Cache Server creates symlinks to providers from the user plugins directory) if the required provider exists in the user plugins directory.
  * Remove the [lock file](https://en.wikipedia.org/wiki/File_locking).
* Receive the response _429 Locked_ from the cache server, wait until all providers are cached.
* If the [`terragrunt-provider-cache-complete-lock`](https://terragrunt.gruntwork.io/docs/reference/cli-options/#terragrunt-provider-cache-complete-lock) flag is set, call `terraform providers lock` to generate a complete `.terraform.lock.hcl` file.
* Run `terragrunt init`, which in turn finds providers in the cache directory and creates symlinks to them.

#### Reusing providers from the user plugins directory

In official registers, some plugins for some operating systems may not exist. Thus, the cache server will not be able to download the requested plugin. A workaround could be to compile the plugin from source code and save it into the user plugins directory:
* %APPDATA%\terraform.d\plugins on Windows
* ~/.terraform.d/plugins on other systems

As an example, plugin `template v2.2.0` for `darwin-arm64`, see [Template v2.2.0 does not have a package available - Mac M1](https://discuss.hashicorp.com/t/template-v2-2-0-does-not-have-a-package-available-mac-m1/35099), and the solution for it [https://github.com/kreuzwerker/m1-terraform-provider-helper](https://github.com/kreuzwerker/m1-terraform-provider-helper)

#### How forwarding Terraform requests through the cache server works

Terraform has the official documented setting [network_mirror](https://developer.hashicorp.com/terraform/cli/config/config-file#network_mirror), that works great, but has one significant drawback for the local cache server - the need to use https connections with a trusted certificate. Luckily, there is another way - using the undocumented setting [host](https://github.com/hashicorp/terraform/issues/28309).
