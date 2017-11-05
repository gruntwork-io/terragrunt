---
title: Configuration
layout: single
author_profile: true
sidebar:
  nav: "docs"
---

Terragrunt configuration is defined in a `terraform.tfvars` file in a `terragrunt = { ... }` block.

For example:

```json
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }

  dependencies {
    paths = ["../vpc", "../mysql", "../redis"]
  }
}
```

Terragrunt figures out the path to its config file according to the following rules:

 1. The value of the `--terragrunt-config` command-line option, if specified.
 1. The value of the `TERRAGRUNT_CONFIG` environment variable, if defined.
 1. A `terraform.tfvars` file in the current working directory, if it exists.
 1. If none of these are found, exit with an error.

 The `--terragrunt-config` parameter is only used by Terragrunt and has no effect on which variable files are loaded
 by Terraform. Terraform will automatically read variables from a file named `terraform.tfvars`, but if you want it
 to read variables from some other .tfvars file, you must pass it in using the `--var-file` argument:

```bash
terragrunt plan --terragrunt-config example.tfvars --var-file example.tfvars
```

