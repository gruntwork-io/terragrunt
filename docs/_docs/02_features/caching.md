---
layout: collection-browser-doc
title: Caching
category: features
categories_url: features
excerpt: Learn more about caching in Terragrunt.
tags: ["caching"]
order: 255
nav_title: Documentation
nav_title_link: /docs/
---

## Clearing the Terragrunt cache

Terragrunt creates a `.terragrunt-cache` folder in the current working directory as its scratch directory. It downloads your remote Terraform configurations into this folder, runs your Terraform commands in this folder, and any modules and providers those commands download also get stored in this folder. You can safely delete this folder any time and Terragrunt will recreate it as necessary.

If you need to clean up a lot of these folders (e.g., after `terragrunt run-all apply`), you can use the following commands on Mac and Linux:

Recursively find all the `.terragrunt-cache` folders that are children of the current folder:

``` bash
find . -type d -name ".terragrunt-cache"
```

If you are ABSOLUTELY SURE you want to delete all the folders that come up in the previous command, you can recursively delete all of them as follows:

``` bash
find . -type d -name ".terragrunt-cache" -prune -exec rm -rf {} \;
```

Also consider setting the `TERRAGRUNT_DOWNLOAD` environment variable if you wish to place the cache directories somewhere else.

## Terraform/OpenTofu plugins cache

By default, Terraform/OpenTofu downloads plugins into a subdirectory of the working directory in `.terragrunt-cache`. As a consequence, each Terragrunt module that use the same provider then a separate copy of its plugin will be downloaded.

Given that provider plugins can be quite large (hundreds of megabytes), this default behavior can be inconvenient for those with slow or metered Internet connections. Therefore Terraform/OpenTofu optionally allows the use of a local directory as a shared plugin cache, which then allows each distinct plugin binary to be downloaded only once.

To enable the plugin cache, use the `plugin_cache_dir` setting. One of the best solutions is to configure this cache globally using Terraform/OpenTofu config file.

Create a `~/.terraformrc` file with the folliwing content:

```
plugin_cache_dir = "$HOME/.terraform.d/plugins-cache"
```

This directory must already exist before Terraform/OpenTofu will cache plugins; Terraform/OpenTofu will not create the directory itself.

Please note that on Windows it is necessary to use forward slash separators (`/`) rather than the conventional backslash (`\`) since the configuration file parser considers a backslash to begin an escape sequence.

When a plugin cache directory is enabled, the init command will still use the configured or implied installation methods to obtain metadata about which plugins are available, but once a suitable version has been selected it will first check to see if the chosen plugin is already available in the cache directory. If so, Terraform/OpenTofu will use the previously-downloaded copy.

If the selected plugin is not already in the cache, Terraform/OpenTofu will download it into the cache first and then copy it from there into the correct location under your current working directory. When possible Terraform/OpenTofu will use symbolic links to avoid storing a separate copy of a cached plugin in multiple directories.

Terraform/OpenTofu will never itself delete a plugin from the plugin cache once it has been placed there. Over time, as plugins are upgraded, the cache directory may grow to contain several unused versions which you must delete manually.
