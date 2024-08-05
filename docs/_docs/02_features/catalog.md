---
layout: collection-browser-doc
title: Catalog
category: features
categories_url: features
excerpt: Learn how to use catalog function in  Terragrunt.
tags: ["catalog"]
order: 212
nav_title: Documentation
nav_title_link: /docs/
---

## Catalog

Launch the user interface for searching and managing your module catalog.

Example:

```bash
terragrunt catalog <repo-url>
```

[![screenshot](/assets/img/screenshots/catalog-screenshot.png){: width="50%" }](https://terragrunt.gruntwork.io/assets/img/screenshots/catalog-screenshot.png)

If `<repo-url>` is provided, the repository will be cloned into a temporary directory, otherwise:

1. The repository list are searched in the config file `terragrunt.hcl`. if `terragrunt.hcl` does not exist in the current directory, the config are searched in the parent directories.
1. If the repository list is not found in the configuration file, the modules are looked for in the current directory.

An example of how to define the list of repositories for the `catalog` command in the `terragrunt.hcl` configuration file:

``` hcl
catalog {
  urls = [
    "relative/path/to/repo", # will be converted to the absolute path, relative to the path of the configuration file.
    "/absolute/path/to/repo",
    "github.com/gruntwork-io/terraform-aws-lambda", # url to remote repository
    "http://github.com/gruntwork-io/terraform-aws-lambda", # same as above
  ]
}
```

This will recursively search for OpenTofu/Terraform modules in the root of the repo and the `modules` directory and show a table with all the modules. You can then:

1. Search and filter the table: `/` and start typing.
1. Select a module in the table: use the arrow keys to go up and down and next/previous page.
1. See the docs for a selected module: `ENTER`.
1. Use [`terragrunt scaffold`](https://terragrunt.gruntwork.io/docs/features/scaffold/) to render a `terragrunt.hcl` for using the module: `S`.
