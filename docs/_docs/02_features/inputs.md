---
layout: collection-browser-doc
title: Inputs
category: features
categories_url: features
excerpt: Learn how to use inputs.
tags: ["inputs"]
order: 230
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

Whenever you run a Terragrunt command, Terragrunt will set any inputs you pass in as environment variables. For example, with the `terragrunt.hcl` file above, running `terragrunt apply` is roughly equivalent to:

    $ terragrunt apply

    # Roughly equivalent to:

    TF_VAR_instance_type="t2.micro" \
    TF_VAR_instance_count=10 \
    TF_VAR_tags='{"Name":"example-app"}' \
    terraform apply

Note that Terragrunt will respect any `TF_VAR_xxx` variables you’ve manually set in your environment, ensuring that anything in `inputs` will NOT override anything you’ve already set in your environment.

### Variable precedence

Terragrunt follows the same variable precedence as [terraform](https://www.terraform.io/docs/configuration/variables.html#variable-definition-precedence).

If the same variable is assigned multiple values, Terraform will use the **last** value it finds overriding any previous values.

Variables are loaded in the following order:

  - Environment variables.

  - `terraform.tfvars` file, if present.

  - `terraform.tfvars.json` file, if present.

  - Any `.auto.tfvars`/`</emphasis>.auto.tfvars.json` files, processed in order of their filenames.

  - Any `-var`/`-var-file` options on the command line, in the order they are provided.
