---
layout: collection-browser-doc
title: Inputs
category: features
categories_url: features
excerpt: Learn how to use inputs.
tags: ["inputs"]
order: 205
nav_title: Documentation
nav_title_link: /docs/
---
## Inputs

You can set values for your module’s input parameters by specifying an `inputs` block in `terragrunt.hcl`:

``` hcl
inputs = {
  instance_type  = "t2.micro"
  instance_count = 10

  tags = {
    Name = "example-app"
  }
}
```

Whenever you run a Terragrunt command, Terragrunt will convert these inputs into [a tfvars json
file](https://www.terraform.io/docs/configuration/variables.html#variable-definitions-tfvars-files) named
`terragrunt-generated.auto.tfvars.json`. This file will be autoloaded by terraform when it is invoked.

For example, with the `terragrunt.hcl` file above, running `terragrunt apply` will generate the following
`terragrunt-generated.auto.tfvars.json` file in the terraform module directory:

    {
        "instance_type": "t2.micro",
        "instance_count": 10,
        "tags": {
            "Name": "example-app"
        }
    }

Note that Terragrunt will respect any `TF_VAR_xxx` variables you’ve manually set in your environment, ensuring that anything in `inputs` will NOT be override anything you’ve already set in your environment. For example, if you set the environment variable `TF_VAR_instance_type`, then the resulting generated tfvars file will omit that variable:

    {
        "instance_count": 10,
        "tags": {
            "Name": "example-app"
        }
    }


### Variable precedence

Terragrunt follows the same variable precedence as [terraform](https://www.terraform.io/docs/configuration/variables.html#variable-definition-precedence).

If the same variable is assigned multiple values, Terraform will use the **last** value it finds overriding any previous values.

Variables are loaded in the following order:

  - Environment variables.

  - `terraform.tfvars` file, if present.

  - `terraform.tfvars.json` file, if present.

  - Any `.auto.tfvars`/`</emphasis>.auto.tfvars.json` files, processed in order of their filenames.

  - Any `-var`/`-var-file` options on the command line, in the order they are provided.
