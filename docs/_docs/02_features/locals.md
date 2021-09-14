---
layout: collection-browser-doc
title: Locals
category: features
categories_url: features
excerpt: Learn how to use locals.
tags: ["locals"]
order: 235
nav_title: Documentation
nav_title_link: /docs/
---

## Locals

You can use locals to bind a name to an expression, so you can reuse that expression without having to repeat it multiple times (keeping your Terragrunt configuration DRY).
config. For example, suppose that you need to use the AWS region in multiple inputs. You can bind the name `aws_region`
using locals:

```
locals {
  aws_region = "us-east-1"
}

inputs = {
  aws_region  = local.aws_region
  s3_endpoint = "com.amazonaws.${local.aws_region}.s3"
}
```

You can use any valid terragrunt expression in the `locals` configuration. The `locals` block also supports referencing other `locals`:

```
locals {
  x = 2
  y = 40
  answer = local.x + local.y
}
```

### Including globally defined locals

Currently you can only reference `locals` defined in the same config file. `terragrunt` does not automatically include
`locals` defined in the parent config of an `include` block into the current context. If you wish to reuse variables
globally, consider using `yaml` or `json` files that are included and merged using the `terraform` built in functions
available to `terragrunt`.

For example, suppose you had the following directory tree:

```
.
├── terragrunt.hcl
├── mysql
│   └── terragrunt.hcl
└── vpc
    └── terragrunt.hcl
```

Instead of adding the `locals` block to the parent `terragrunt.hcl` file, you can define a file `common_vars.yaml`
that contains the global variables you wish to pull in:

```
.
├── terragrunt.hcl
├── common_vars.yaml
├── mysql
│   └── terragrunt.hcl
└── vpc
    └── terragrunt.hcl
```

You can then include them into the `locals` block of the child terragrunt config using `yamldecode` and `file`:

```
# child terragrunt.hcl
locals {
  common_vars = yamldecode(file(find_in_parent_folders("common_vars.yaml")))
  region = "us-east-1"
}
```

This configuration will load in the `common_vars.yaml` file and bind it to the attribute `common_vars` so that it is available
in the current context. Note that because `locals` is a block, there is not currently a way to merge the map into the top
level.
