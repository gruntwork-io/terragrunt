---
layout: collection-browser-doc
title: Terraform 1.10 Upgrade Guide
category: migrate
categories_url: migrate
excerpt: Migration guide to upgrade to Terraform 1.10
tags: ["migration", "community", "terraform"]
order: 603
nav_title: Documentation
nav_title_link: /docs/
slug: upgrading-to-terraform-1.10
---

## Background

As part of the introduction of ephemeral variables in Terraform `v1.10.0`, Terraform introduced a breaking change in how stored plans are applied.

When applies occur while using `TF_VAR_` prefixed environment variables, Terraform now requires that these variables be explicitly defined as ephemeral when changing during an apply to avoid breaking the apply.

Terragrunt populates `TF_VAR_` prefixed environment variables when using `inputs`, so this change affects Terragrunt users who apply stored plans while using the `inputs` attribute, especially when combined with mock_outputs.

## Migration guide

To work around this breaking change, you can either:

1. Switch to OpenTofu. OpenTofu applies on stored plans continue to work as expected as of `v1.9.0`, regardless of the presence of `TF_VAR_` prefixed environment variables.

   The maintainers of Terragrunt are also coordinating with the OpenTofu team to hopefully prevent this issue from occurring in OpenTofu if ephemeral variables are adopted there.

2. Use Terragrunt configurations to regenerate the plan when necessary.

   The rest of this guide demonstrates how to handle this scenario.

## Dynamic Terragrunt configurations

Say, for example, you have the following configurations in a local directory:

```hcl
# ./dependency/terragrunt.hcl
```

```hcl
# ./dependency/main.tf
output "output" {
  value = "real_output"
}
```

```hcl
# ./dependent/terragrunt.hcl
dependency "dependency" {
  config_path = "../dependency"

  mock_outputs = {
    "output" = "mock_output"
  }
}

inputs = {
	potentially_mocked_input = dependency.dependency.outputs.output
}
```


```hcl
# ./dependent/main.tf
variable "potentially_mocked_input" {
  type = string
}

resource "null_resource" "example" {
  triggers = {
    potentially_mocked_input = var.potentially_mocked_input
  }
}
```

If you ran the following command:

```bash
$ terragrunt run-all plan -out=plan.tfplan
```

You would get an error like the following during your apply:

```bash
$ terragrunt run-all apply plan.tfplan
...
* Failed to execute "terraform apply plan.tfplan" in .
  ╷
  │ Error: Can't change variable when applying a saved plan
  │
  │ The variable input_value cannot be set using the -var and -var-file options
  │ when applying a saved plan file, because a saved plan includes the variable
  │ values that were set when it was created. The saved plan specifies "bar" as
  │ the value whereas during apply the value "foo" was set by an environment
  │ variable. To declare an ephemeral variable which is not saved in the plan
  │ file, use ephemeral = true.
  ╵
```

To workaround this issue, you can include a `before_hook` and `after_hook` like the following:

```hcl
# ./dependent/terragrunt.hcl
dependency "dependency" {
  config_path = "../dependency"

  mock_outputs = {
    "output" = "mock_output"
  }
}

inputs = {
  potentially_mocked_input = dependency.dependency.outputs.output
}

terraform {
  before_hook "before_hook" {
    commands = ["apply"]
    execute  = ["bash", "-c", "[[ -f dirty-plan ]] && $TG_CTX_TF_PATH plan -out plan.tfplan && rm -f dirty-plan || true"]
  }
  after_hook "after_hook" {
    commands = ["plan"]
    execute  = ["bash", "-c", "[[ '${dependency.dependency.outputs.output}' == 'mock_output' ]] && touch dirty-plan || true"]
  }
}
```

This configuration will tell Terragrunt to regenerate the plan when detecting that the plan was previously generated using mocked outputs.

You should now be able to run the following commands without issue:

```bash
$ terragrunt run-all plan -out=plan.tfplan
$ terragrunt run-all apply plan.tfplan
```
