---
layout: collection-browser-doc
title: Custom Commands
category: RFC
categories_url: rfc
excerpt: Ability to define custom commands
tags: ["rfc", "contributing", "community"]
order: 505
nav_title: Documentation
nav_title_link: /docs/
---

# Custom Commands

**STATUS**: In proposal

## Background

Third party tools are available that perform static analysis of Terraform configurations to provide
additional functionality such as cost estimation or security scanning.  Terragrunt currently supports execution
of these tools in concert with a Terraform command using `before_hook` and `after_hook`, but there is no way to run commands without also running a Terraform command.

## Proposed solution

In order to allow seamless execution of third party tools, we propose adding a `custom_command` property to the `terraform` block.  Users could define a custom command that would execute similarly to a `before_hook` without requiring
a Terraform command to be run.  The documentation entry would be:

```
 custom_command (block): Nested blocks used to specify custom
  commands that can be run against terraform stacks.  Like
  `before_hook`, custom commands run from the directory with
  the terraform module.  Supports the following arguments:

    * execute (required) : A list of command and arguments that
      should be run when this command is specified. For example,
      if execute is set as ["echo", "Foo"], the command `echo Foo`
      will be run.
    * working_dir (optional) : The path to set as the working
      directory for execution. Terragrunt will switch directory
      to this path prior to running the command. Defaults to the
      Terraform module directory.
```

A custom block for cost estimates could be defined as:

```hcl
terraform {
  # Cost estimation custom command.  Cost estimates can be generated
  # by invoking this command with `terragrunt run-all infracost` 
custom_command "infracost" {
execute = ["infracost", "breakdown", "--path", "."]
  }
}
```

Generating the cost estimates would be done by calling the command:

```shell
$ terragrunt run-all infracost
```

Implementation would consist of 1) parsing the `custom_command` configuration, 2) modifying `runTerragruntWithConfig` to detect and run the custom command instead of Terraform, and 3)
bypassing the Terraform version check when running a custom command.

## Alternatives

There are a few sub-optimal options for running third party tools with Terragrunt:

### Before/after hooks

Some commands could be executed as a before/after hook. As an example, a proposed work around for
[Checkov's lack of Terragrunt support](https://github.com/bridgecrewio/checkov/issues/1284)
involves executing a security scan in a before `plan` hook:

```hcl
terraform {
source = "git::https://ghe.mycompany.com/repo/my-terraform-repo.git//.?ref=v0.1.0"

before_hook"checkov" {
commands = ["plan"]
execute = [
            "checkov",
            "-d",
            ".",
            "--quiet",
            "--skip-path",
            "/*/examples/*",
            "--framework",
            "terraform",
        ]
    }
}
```

The main disadvantage of this approach is that the third party tool can only be run as a side-effect of running a Terraform command.  In the example, it becomes impossible to run `plan` without running `checkov` (or vice versa).  This can become a serious hurdle in CI environments if the Terraform configuration needs to be analyzed from the `.tf` files without running `terraform` at all.

### Wait for third party tool support of Terragrunt

Some third party tools do support Terragrunt natively, either by wrapping the `terragrunt` binary or by importing source libraries.  For example, Infracost generates cost estimates for Terragrunt projects by executing a `terragrunt run-all terragrunt-info` to determine the location of terragrunt working directories:

```shell
$ infracost breakdown --path .
Detected Terragrunt directory at .
  ✔ Running terragrunt run-all terragrunt-info
  ✔ Running terragrunt run-all plan
  ✔ Running terragrunt show for each project
  ✔ Extracting only cost-related params from terragrunt plan
  ✔ Retrieving cloud prices to calculate costs

  ...
```

Aside from the time and uncertainty involved in waiting for third parties to implement support, the main disadvantage is that it is somewhat confusing to use a third party tool to execute a Terragrunt stack.  It seems more intuitive to tell Terragrunt what command to run the same way I do for Terraform commands.

## References

- [RFC: Custom Commands]()

A sample of people describing issues or work arounds to use third party tools with Terragrunt
- Checkov #1284 [Will Checkov support Terragrunt ? ](https://github.com/bridgecrewio/checkov/issues/1284)
- Rover #21 [[Feature Request] Terragrunt Support](https://github.com/im2nguyen/rover/issues/21)
- Terrascan #251 [Possibility to use with Terragrunt?](https://github.com/accurics/terrascan/issues/251)
- Infracost #224 [Enable to use of terragrunt instead of terraform](https://github.com/infracost/infracost/issues/224)
- [Terraform static code analysis with tfsec and terragrunt](https://medium.com/@twojtun/terraform-static-code-analysis-with-tfsec-and-terragrunt-358aeb0c68dc)
- [How to use driftctl with Terragrunt](https://driftctl.com/how-to-use-driftctl-with-terragrunt/)
