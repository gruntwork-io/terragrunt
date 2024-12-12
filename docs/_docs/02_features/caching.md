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

## Global Caching Directory

Terragrunt now supports a global caching directory for modules supporting different versions. This feature helps save space and improve performance by avoiding multiple caches for the same module and version.

### Benefits of Global Caching Directory

- **Space Efficiency**: By storing modules supporting different versions in a global caching directory, you can avoid redundant copies of the same module, saving disk space.
- **Performance Improvement**: With a global caching directory, Terragrunt can quickly access cached modules, reducing the time required to download and set up modules.

### Setting Up Global Caching Directory

To enable the global caching directory feature, you need to set the `TERRAGRUNT_GLOBAL_CACHE` environment variable to the desired path for the global cache directory. For example:

```bash
export TERRAGRUNT_GLOBAL_CACHE=/path/to/global/cache
```

Once set, Terragrunt will use the specified global caching directory for storing modules supporting different versions.
