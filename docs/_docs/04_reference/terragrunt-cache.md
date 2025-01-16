---
layout: collection-browser-doc
title: Terragrunt Cache
category: reference
categories_url: reference
excerpt: Learn what the `.terragrunt-cache` directory is and how to manage it.
tags: [ "install" ]
order: 406
nav_title: Documentation
nav_title_link: /docs/
redirect_from:
    - /docs/features/caching/
---

Terragrunt uses a cache directory (`.terragrunt-cache`) to store downloaded modules when using the `source` attribute in the `terraform` block.

This cache directory is created whenever Terragrunt downloads a module from a remote source, and where it runs the OpenTofu/Terraform commands. It also stores any modules and providers that are downloaded as part of these commands by default.

## Clearing the Terragrunt cache

Terragrunt creates a `.terragrunt-cache` folder in the current working directory as its scratch directory. It downloads your remote OpenTofu/Terraform configurations into this folder, runs your OpenTofu/Terraform commands in this folder, and any modules and providers those commands download also get stored in this folder. You can safely delete this folder any time and Terragrunt will recreate it as necessary.

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

If the reason you are clearing out your Terragrunt cache is that you are struggling with running out of disk space, consider using the [Provider Cache](/docs/features/provider-cache-server/#provider-cache-server) feature to store OpenTofu/Terraform provider plugins in a shared location, as those are typically the largest files stored in the `.terragrunt-cache` directory.
