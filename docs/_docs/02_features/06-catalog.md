---
layout: collection-browser-doc
title: Catalog
category: features
categories_url: features
excerpt: Learn how to search and use your module catalog with Terragrunt.
tags: ["catalog"]
order: 206
nav_title: Documentation
nav_title_link: /docs/
slug: catalog
---

Launch the user interface for searching and managing your module catalog.

Example:

```bash
terragrunt catalog <repo-url> [--no-include-root] [--root-file-name] [--output-folder]
```

[![screenshot](/assets/img/screenshots/catalog-screenshot.png){: width="50%" }](https://terragrunt.gruntwork.io/assets/img/screenshots/catalog-screenshot.png)

If `<repo-url>` is provided, the repository will be cloned into a temporary directory, otherwise:

1. The `urls` listed in the `catalog` configuration of your [parent configuration file](#configuration) are used. If the root configuration file does not exist in the current directory, parent directories are recursively searched.
1. If the repository list is not found in the configuration file, the modules are looked for in the current directory.

For each of the provided repositories, Terragrunt will recursively search for OpenTofu/Terraform modules from the root of the repo and the `modules` directory. A table with all the discovered OpenTofu/Terraform modules will subsequently be displayed.

You can then:

1. Search and filter the table: `/` and start typing.
1. Select a module in the table: use the arrow keys to go up and down and next/previous page.
1. See the docs for a selected module: `ENTER`.
1. Use [`terragrunt scaffold`]({{site.baseurl}}/docs/features/scaffold/) to render a `terragrunt.hcl` file that uses the module: `S`.

## Custom templates for scaffolding

Terragrunt has a basic template built-in for rendering `terragrunt.hcl` files, but you can provide your own templates to customize how code is generated! Scaffolding is done via an integration with [boilerplate](https://github.com/gruntwork-io/boilerplate), and Terragrunt allows you to specify custom boilerplate templates via two mechanisms while using catalog:

1. You can define a custom Boilerplate template in a `.boilerplate` sub-directory of any OpenTofu/Terraform module.
1. You can specify a custom Boilerplate template in the [catalog config](#configuration).

## Configuration

An example of how to define the optional default template, and the list of repositories for the `catalog` command in a `root.hcl` configuration file:

``` hcl
catalog {
  default_template = "git@github.com/acme/example.git//path/to/template"
  urls = [
    "relative/path/to/repo", # will be converted to the absolute path, relative to the path of the configuration file.
    "/absolute/path/to/repo",
    "github.com/gruntwork-io/terraform-aws-lambda", # url to remote repository
    "http://github.com/gruntwork-io/terraform-aws-lambda", # same as above
  ]
}
```

## Scaffolding Flags

The following `catalog` flags control behavior of the underlying `scaffold` command when the `S` key is pressed in a catalog entry:

- `--no-include-root` - Do not include the root configuration file in any generated `terragrunt.hcl` during scaffolding.
- `--root-file-name` - The name of the root configuration file to include in any generated `terragrunt.hcl` during scaffolding. This value also controls the name of the root configuration file to search for when trying to determine Catalog urls.
- `--output-folder` - Location for the scaffolded configurations. If flag is not provided current working directory is selected.
