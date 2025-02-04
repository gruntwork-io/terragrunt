---
title: OpenTofu shortcuts
description: Terragrunt shortcuts for OpenTofu/Terraform
slug: docs/reference/cli/commands/opentofu-shortcuts
sidebar:
  order: 1
---

Terragrunt is an orchestration tool for OpenTofu/Terraform, so you can generally expect that a command you can run with `tofu`/ `terraform` you can also run with `terragrunt`.

Terragrunt will pass the command to `tofu`/ `terraform` with the same arguments.

There are some exceptions to this rule.

For example, when you run `terragrunt apply`, Terragrunt executes `tofu apply`/ `terraform apply`.

Examples:

```bash
terragrunt plan
terragrunt apply
terragrunt output
terragrunt destroy
# etc
```

Run `tofu --help` to get the full list.
