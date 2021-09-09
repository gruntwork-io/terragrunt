---
layout: collection-browser-doc
title: Before and After Hooks
category: features
categories_url: features
excerpt: Learn how to execute custom code before or after running Terraform.
tags: ["hooks"]
order: 240
nav_title: Documentation
nav_title_link: /docs/
---

## Before and After Hooks

*Before Hooks* or *After Hooks* are a feature of terragrunt that make it possible to define custom actions that will be called either before or after execution of the `terraform` command.

Here’s an example:

``` hcl
terraform {
  before_hook "before_hook" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Running Terraform"]
  }

  after_hook "after_hook" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Finished running Terraform"]
    run_on_error = true
  }
}
```

In this example configuration, whenever Terragrunt runs `terraform apply` or `terraform plan`, two things will happen:

- Before Terragrunt runs `terraform`, it will output `Running Terraform` to the console.
- After Terragrunt runs `terraform`, it will output `Finished running Terraform`, regardless of whether or not the
  command failed.

You can have multiple before and after hooks. Each hook will execute in the order they are defined. For example:

``` hcl
terraform {
  before_hook "before_hook_1" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Will run Terraform"]
  }

  before_hook "before_hook_2" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Running Terraform"]
  }
}
```

This configuration will cause Terragrunt to output `Will run Terraform` and then `Running Terraform` before the call
to Terraform.

You can learn more about all the various configuration options supported in [the reference docs for the terraform
block](/docs/reference/config-blocks-and-attributes/#terraform).
