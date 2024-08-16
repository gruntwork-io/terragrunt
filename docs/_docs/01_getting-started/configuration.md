---
layout: collection-browser-doc
title: Configuration
category: getting-started
excerpt: >-
  Learn how to configure Terragrunt.
tags: ["config", "formatting"]
order: 104
nav_title: Documentation
nav_title_link: /docs/
---

## Terragrunt configuration file

Terragrunt configuration is defined in a `terragrunt.hcl` file. This uses the same HCL syntax as OpenTofu/Terraform itself.

Here’s an example:

``` hcl
include "root" {
  path = find_in_parent_folders()
}

dependencies {
  paths = ["../vpc", "../mysql", "../redis"]
}
```

Terragrunt also supports [JSON-serialized HCL](https://github.com/hashicorp/hcl/blob/hcl2/json/spec.md) defined in a `terragrunt.hcl.json` file: where `terragrunt.hcl` is mentioned you can always use `terragrunt.hcl.json` instead.

Terragrunt figures out the path to its config file according to the following rules:

1. The value of the `--terragrunt-config` command-line option, if specified.

2. The value of the `TERRAGRUNT_CONFIG` environment variable, if defined.

3. A `terragrunt.hcl` file in the current working directory, if it exists.

4. A `terragrunt.hcl.json` file in the current working directory, if it exists.

5. If none of these are found, exit with an error.

Refer to the following pages for a complete reference of supported features in the terragrunt configuration file:

- [Config blocks and attributes]({{site.baseurl}}/docs/reference/config-blocks-and-attributes/)
- [Built-in functions]({{site.baseurl}}/docs/reference/built-in-functions/)

## Configuration parsing order

It is important to be aware of the terragrunt configuration parsing order when using features like [locals]({{site.baseurl}}/docs/features/locals/#locals) and [dependency outputs]({{site.baseurl}}/docs/features/execute-terraform-commands-on-multiple-modules-at-once/#passing-outputs-between-modules), where you can reference attributes of other blocks in the config in your `inputs`. For example, because `locals` are evaluated before `dependency` blocks, you can not bind outputs from `dependency` into `locals`. On the other hand, for the same reason, you can use `locals` in the `dependency` blocks.

Currently terragrunt parses the config in the following order:

1. `include` block

2. `locals` block

3. Evaluation of values for `iam_role`, `iam_assume_role_duration`, `iam_assume_role_session_name`, and `iam_web_identity_token` attributes, if defined

4. `dependencies` block

5. `dependency` blocks, including calling `terragrunt output` on the dependent modules to retrieve the outputs

6. Everything else

7. The config referenced by `include`

8. A merge operation between the config referenced by `include` and the current config.

Blocks that are parsed earlier in the process will be made available for use in the parsing of later blocks. Similarly, you cannot use blocks that are parsed later earlier in the process (e.g you can’t reference `dependency` in `locals`, `include`, or `dependencies` blocks).

Note that the parsing order is slightly different when using the `-all` flavors of the command. In the `-all` flavors of the command, Terragrunt parses the configuration twice. In the first pass, it follows the following parsing order:

1. `include` block of all configurations in the tree

2. `locals` block of all configurations in the tree

3. `dependency` blocks of all configurations in the tree, but does NOT retrieve the outputs

4. `terraform` block of all configurations in the tree

5. `dependencies` block of all configurations in the tree

The results of this pass are then used to build the dependency graph of the modules in the tree. Once the graph is constructed, Terragrunt will loop through the modules and run the specified command. It will then revert to the single configuration parsing order specified above for each module as it runs the command.

This allows Terragrunt to avoid resolving `dependency` on modules that haven’t been applied yet when doing a clean deployment from scratch with `run-all apply`.

## Formatting hcl files

You can rewrite the hcl files to a canonical format using the `hclfmt` command built into `terragrunt`. Similar to `terraform fmt`, this command applies a subset of [the OpenTofu/Terraform language style conventions](https://www.terraform.io/docs/configuration/style.html), along with other minor adjustments for readability.

This command will recursively search for hcl files and format all of them under a given directory tree. Consider the following file structure:

```tree
root
├── terragrunt.hcl
├── prod
│   └── terragrunt.hcl
├── dev
│   └── terragrunt.hcl
└── qa
    ├── terragrunt.hcl
    └── services
        ├── services.hcl
        └── service01
            └── terragrunt.hcl
```

If you run `terragrunt hclfmt` at the `root`, this will update:

- `root/terragrunt.hcl`

- `root/prod/terragrunt.hcl`

- `root/dev/terragrunt.hcl`

- `root/qa/terragrunt.hcl`

- `root/qa/services/services.hcl`

- `root/qa/services/service01/terragrunt.hcl`

You can set `--terragrunt-diff` option. `terragrunt hclfmt --terragrunt-check` will output diff in unified format which can be redirected to your favourite diff tool. `diff` utility must be presented in PATH.

Additionally, there’s a flag `--terragrunt-check`. `terragrunt hclfmt --terragrunt-check` will only verify if the files are correctly formatted **without rewriting** them. The command will return exit status 1 if any matching files are improperly formatted, or 0 if all matching .hcl files are correctly formatted.
