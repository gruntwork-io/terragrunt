---
layout: collection-browser-doc
title: Terragrunt settings
category: Features
categories_url: features
excerpt: Learn more about Terragrunt settings.
tags: ["AWS", "Settings"]
order: 214
nav_title: Documentation
nav_title_link: /docs/
---
## prevent\_destroy

Terragrunt `prevent_destroy` boolean flag allows you to protect selected Terraform module. It will prevent `destroy` or `destroy-all` command to actually destroy resources of the protected module. This is useful for modules you want to carefully protect, such as a database, or a module that provides auth.

Example:

``` hcl
terraform {
  source = "git::git@github.com:foo/modules.git//app?ref=v0.0.3"
}

prevent_destroy = true
```

## skip

The terragrunt `skip` boolean flag can be used to protect modules you don’t want any changes to or just to skip modules that don’t define any infrastructure by themselves. When set to true, all terragrunt commands will skip the selected module.

Consider the following file structure:

    root
    ├── terragrunt.hcl
    ├── prod
    │   └── terragrunt.hcl
    ├── dev
    │   └── terragrunt.hcl
    └── qa
        └── terragrunt.hcl

In some cases, the root level `terragrunt.hcl` file is solely used to DRY up your Terraform configuration by being included in the other `terragrunt.hcl` files. In this case, you do not want the `xxx-all` commands to process the root level `terragrunt.hcl` since it does not define any infrastructure by itself. To make the `xxx-all` commands skip the root level `terragrunt.hcl` file, you can set `skip = true`:

``` hcl
skip = true
```

The `skip` flag must be set explicitly in terragrunt modules that should be skipped. If you set `skip = true` in a `terragrunt.hcl` file that is included by another `terragrunt.hcl` file, only the `terragrunt.hcl` file that explicitly set `skip = true` will be skipped.

## terraform\_binary

The terragrunt `terraform_binary` string option can be used to override the default terraform binary path (which is `terraform`).

The precedence is as follows: `--terragrunt-tfpath` command line option → `TERRAGRUNT_TFPATH` env variable → `terragrunt.hcl` in the module directory → included `terragrunt.hcl`

## download\_dir

The terragrunt `download_dir` string option can be used to override the default download directory.

The precedence is as follows: `--terragrunt-download-dir` command line option → `TERRAGRUNT_DOWNLOAD` env variable → `terragrunt.hcl` in the module directory → included `terragrunt.hcl`

It supports all terragrunt functions, i.e. `path_relative_from_include()`.

## terraform\_version\_constraint

The terragrunt `terraform_version_constraint` string overrides the default minimum supported version of terraform. Terragrunt only officially supports the latest version of terraform, however in some cases an old terraform is needed.

For example:

``` hcl
terraform_version_constraint = ">= 0.11"
```
