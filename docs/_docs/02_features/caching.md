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
