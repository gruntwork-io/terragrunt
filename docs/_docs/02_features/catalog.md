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

If `<repo-url>` is not specified, the modules are searched in the current directory. If a URL is provided, the repository will be copied to a temporary directory and deleted upon complete.

This will recursively search for Terraform modules in the root of the repo and the `modules` directory and show a table with all the modules. You can then:
1. Search and filter the table: `/` and start typing.
1. Select a module in the table: use the arrow keys to go up and down and next/previous page.
1. See the docs for a selected module: `ENTER`.
1. Use [`terragrunt scaffold`](https://terragrunt.gruntwork.io/docs/features/scaffold/) to render a `terragrunt.hcl` for using the module: `S`.
