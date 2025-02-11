---
title: OpenTofu Shortcuts
description: Interact with OpenTofu/Terraform backend infrastructure.
slug: docs/reference/cli/commands/opentofu-shortcuts
sidebar:
  order: 99
---

Terragrunt is an orchestration tool for OpenTofu/Terraform, so with a couple exceptions, you can generally use it as a drop-in replacement for OpenTofu/Terraform. Terragrunt has a shortcut for most OpenTofu commands. You can usually just replace `tofu` or `terraform` with `terragrunt` and it will do what you expect.

For example:

```bash
terragrunt apply
```

## Supported Shortcuts

- `apply`
- `destroy`
- `force-unlock`
- `import`
- `init`
- `output`
- `plan`
- `refresh`
- `show`
- `state`
- `test`
- `validate`
