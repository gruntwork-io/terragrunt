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

Terraform has provider caching feature [Provider Plugin Cache](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache), that does the job well until you run multiple Terraform processes simultaneously, such as when you use `terragrunt run-all`. Then the Terraform processes begin conflict by overwriting each other’s cache, which causes the error like `Error: Failed to install provider`. As a result, Terragrunt previously had to disable concurrency for `init` steps in `run-all`, which is significantly slower. If you enable Terragrunt Provider Caching, as described in this section, that will no longer be necessary, and you should see significant performance improvements with `init`, as well as significant savings in terms of bandwidth and disk space usage.

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

### How Terragrunt Provider Caching works

* Start a server on localhost. This is the _Terragrunt Provider Cache server_.
* Configure Terraform instances to use the Terragrunt Provider Cache server as a remote registry:
  * Create local CLI config file `.terraformrc` for each module that concatenates the user configuration from the Terraform [CLI config file](https://developer.hashicorp.com/terraform/cli/config/config-file) with additional sections:
     * [provider-installation](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-installation) forces Terraform to look for for the required providers in the cache directory and create symbolic links to them, if not found, then request them from the remote registry.
     * [host](https://github.com/hashicorp/terraform/issues/28309) forces Terraform to [forward](#how-forwarding-terraform-requests-through-the-terragrunt-Provider-cache-works) all provider requests through the Terragrunt Provider Cache server. The address link contains [UUID](https://en.wikipedia.org/wiki/Universally_unique_identifier) and is unique for each module, used by Terragrunt Provider Cache server to associate modules with the requested providers.
  * Set environment variables:
     * [TF_CLI_CONFIG_FILE](https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_plugin_cache_dir) sets to use just created local CLI config `.terragrunt-cache/.terraformrc`
     * [TF_TOKEN_*](https://developer.hashicorp.com/terraform/cli/config/config-file#environment-variable-credentials) sets per-remote-registry tokens for authentication to Terragrunt Provider Cache server.
* Any time Terragrunt is going to run `init`:
    * Call `terraform init`. This gets Terraform to request all the providers it needs from the Terragrunt Provider Cache server.
    * The Terragrunt Provider Cache server will download the provider from the remote registry, unpack and store it into the cache directory or [create a symlink](#reusing-providers-from-the-user-plugins-directory) if the required provider exists in the user plugins directory. Note that the Terragrunt Provider Cache server will ensure that each unique provider is only ever downloaded and stored on disk once, handling concurrency (from multiple Terraform and Terragrunt instances) correctly. Along with the provider, the cache server downloads hashes and signatures of the providers to check that the files are not corrupted.
    * The Terragrunt Provider Cache server returns the HTTP status _429 Locked_ to Terraform. This is because we do _not_ want Terraform to actually download any providers as a result of calling `terraform init`; we only use that command to request the Terragrunt Provider Cache Server to start caching providers.
    * At this point, all providers are downloaded and cached, so finally, we run `terragrunt init` a second time, which will find all the providers it needs in the cache, and it'll create symlinks to them nearly instantly, with no additional downloading.
    * Note that if a Terraform module doesn't have a lock file, Terraform does _not_ use the cache, so it would end up downloading all the providers from scratch. To work around this, we generate `.terraform.lock.hcl` based on the request made by `terrafrom init` to the Terragrunt Provider Cache server. Since `terraform init` only requestes the providers that need to be added/updated, we can keep track of them using the Terragrunt Provider Cache server and update the Terraform lock file with the appropriate hashes without having to parse `tf` configs.

#### Reusing providers from the user plugins directory

Some plugins for some operating systems may not be available in the remote registries. Thus, the cache server will not be able to download the requested provider. As an example, plugin `template v2.2.0` for `darwin-arm64`, see [Template v2.2.0 does not have a package available - Mac M1](https://discuss.hashicorp.com/t/template-v2-2-0-does-not-have-a-package-available-mac-m1/35099). The workaround is to compile the plugin from source code and put it into the user plugins directory or use the automated solution [https://github.com/kreuzwerker/m1-terraform-provider-helper](https://github.com/kreuzwerker/m1-terraform-provider-helper). For this reason, the cache server first tries to create a symlink from the user's plugin directory if the required provider already exists there:

* %APPDATA%\terraform.d\plugins on Windows
* ~/.terraform.d/plugins on other systems


#### How forwarding Terraform requests through the Terragrunt Provider Cache works

Terraform has an official documented setting [network_mirror](https://developer.hashicorp.com/terraform/cli/config/config-file#network_mirror), that works great, but has one major drawback for the local cache server - the need to use https connection with a trusted certificate. Fortunately, there is another way - using the undocumented [host](https://github.com/hashicorp/terraform/issues/28309) setting, which allows Terraform to create connections to the caching server over HTTP.


### Configure the Terragrunt Cache Provider

Since Terragrunt Provider Cache is essentially a Private Registry server that accepts requests from Terraform, downloads and saves providers to the cache directory, there are a few more flags that are unlikely to be needed, but are useful to know about:

* Server Hostname [`terragrunt-provider-cache-hostname`](https://terragrunt.gruntwork.io/docs/reference/cli-options/#terragrunt-provider-cache-hostname), by default, `localhost`.
* Server Port [`terragrunt-provider-cache-port`](https://terragrunt.gruntwork.io/docs/reference/cli-options/#terragrunt-provider-cache-port), assigned automatically  every time you launch the Terragurnt.
* Server Token [`terragrunt-provider-cache-token`](https://terragrunt.gruntwork.io/docs/reference/cli-options/#terragrunt-provider-cache-token), generated automatically every time you launch  the Terragurnt.

To enhance security, the Terragrunt Provider Cache has authentication to prevent unauthorized connections from third-party applications. You can set your own token using any character set.

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
