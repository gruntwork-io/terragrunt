---
layout: collection-browser-doc
title: Before and after hooks
category: features
categories_url: features
excerpt: Learn how to execute custom code before or after running Terraform.
tags: ["hooks"]
order: 208
nav_title: Documentation
nav_title_link: /docs/
---

## Before and After Hooks

*Before Hooks* or *After Hooks* are a feature of terragrunt that make it possible to define custom actions that will be called either before or after execution of the `terraform` command.

Here’s an example:

``` hcl
terraform {
  before_hook "before_hook_1" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Foo"]
    run_on_error = true
  }

  before_hook "before_hook_2" {
    commands     = ["apply"]
    execute      = ["echo", "Bar"]
    run_on_error = false
  }

  before_hook "interpolation_hook_1" {
    commands     = ["apply", "plan"]
    execute      = ["echo", get_env("HOME", "HelloWorld")]
    run_on_error = false
  }

  after_hook "after_hook_1" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Baz"]
    run_on_error = true
  }

  after_hook "init_from_module" {
    commands = ["init-from-module"]
    execute  = ["cp", "${get_parent_terragrunt_dir()}/foo.tf", "."]
  }
}
```

Hooks support the following arguments:

  - `commands` (required): the `terraform` commands that will trigger the execution of the hook.

  - `execute` (required): the shell command to execute.

  - `run_on_error` (optional): if set to true, this hook will run even if a previous hook hit an error, or in the case of "after" hooks, if the Terraform command hit an error. Default is false.

  - `init_from_module` and `init`: This is not an argument, but a special name you can use for hooks that run during initialization. There are two stages of initialization: one is to download [remote configurations]({{site.baseurl}}/use-cases/keep-your-terraform-code-dry) using `go-getter`; the other is [Auto-Init]({{site.baseurl}}/docs/features/auto-init), which configures the backend and downloads provider plugins and modules. If you wish to execute a hook when Terragrunt is using `go-getter` to download remote configurations, name the hook `init_from_module`. If you wish to execute a hook when Terragrunt is using `terraform init` for Auto-Init, name the hook `init`.
