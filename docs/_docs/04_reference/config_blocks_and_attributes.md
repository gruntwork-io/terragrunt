---
layout: collection-browser-doc
title: Configuration Blocks and Attributes
category: reference
excerpt: >-
  Learn about all the blocks and attributes supported in the terragrunt configuration file.
tags: ["config"]
order: 403
nav_title: Documentation
nav_title_link: /docs/
---

## Configuration Blocks and Attributes

The Terragrunt configuration file uses the same HCL syntax as Terraform itself. The following is a reference of all the
supported blocks and attributes in the `terragrunt.hcl` configuration file:

### Blocks

- [terraform](#terraform)
- [remote_state](#remote_state)
- [include](#include)
- [dependency](#dependency)
- [dependencies](#dependencies)

#### terraform

The `terraform` block is used to configure how Terragrunt will interact with Terraform. This includes specifying where
to find the Terraform configuration files, any extra arguments to pass to the `terraform` CLI, and any hooks to run
before or after calling Terraform.

The `terraform` block supports the following arguments:

- `source` (attribute): Specifies where to find Terraform configuration files. This parameter supports the exact same syntax as the
  [module source](https://www.terraform.io/docs/modules/sources.html) parameter for Terraform `module` blocks, including
  local file paths, Git URLs, and Git URLS with `ref` parameters. Terragrunt will download all the code in the repo
  (i.e. the part before the double-slash `//`) so that relative paths work correctly between modules in that repo.
- `extra_arguments` (block): Nested blocks used to specify extra CLI arguments to pass to the `terraform` CLI. Learn more
  about its usage in the [Keep your CLI flags DRY](/use-cases/keep-your-cli-flags-dry/) use case overview. Supports
  the following arguments:
    - `arguments` (required) : A list of CLI arguments to pass to `terraform`.
    - `commands` (required) : A list of `terraform` sub commands that the arguments will be passed to.
    - `env_vars` (optional) : A map of key value pairs to set as environment variables when calling `terraform`.
    - `required_var_files` (optional): A list of file paths to terraform vars files (`.tfvars`) that will be passed in to
      `terraform` as `-var-file=<your file>`.
    - `optional_var_files` (optional): A list of file paths to terraform vars files (`.tfvars`) that will be passed in to
      `terraform` like `required_var_files`, only any files that do not exist are ignored.

- `before_hook` (block): Nested blocks used to specify command hooks that should be run before `terraform` is called.
  Hooks run from the terragrunt configuration directory (the directory where `terragrunt.hcl` lives).
  Supports the following arguments:
    - `commands` (required) : A list of `terraform` sub commands for which the hook should run before.
    - `execute` (required) : A list of command and arguments that should be run as the hook. For example, if `execute` is set as
      `["echo", "Foo"]`, the command `echo Foo` will be run.
    - `run_on_error` (optional) : If set to true, this hook will run even if a previous hook hit an error, or in the case of “after” hooks, if the Terraform command hit an error. Default is false.
    - `init_from_module` and `init`: This is not an argument, but a special name you can use for hooks that run during
      initialization. There are two stages of initialization: one is to download [remote
      configurations](https://terragrunt.gruntwork.io/use-cases/keep-your-terraform-code-dry/) using `go-getter`; the
      other is [Auto-Init](https://terragrunt.gruntwork.io/docs/features/auto-init/), which configures the backend and
      downloads provider plugins and modules. If you wish to execute a hook when Terragrunt is using `go-getter` to
      download remote configurations, name the hook `init_from_module`. If you wish to execute a hook when Terragrunt is
      using terraform `init` for Auto-Init, name the hook `init`.


- `after_hook` (block): Nested blocks used to specify command hooks that should be run after `terraform` is called.
  Hooks run from the terragrunt configuration directory (the directory where `terragrunt.hcl` lives). Supports the same
  arguments as `before_hook`.


#### remote_state
