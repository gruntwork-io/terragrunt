---
question: "How does Terragrunt Scale Patcher work?"
title: "How does Terragrunt Scale Patcher work? | Terragrunt"
description: "How does Terragrunt Scale Patcher work?"
category: "terragrunt-scale"
order: 23
---

Patcher is the automated dependency-update product in [Terragrunt Scale](/terragrunt-scale). It scans your Terragrunt units and stacks, and your OpenTofu/Terraform module references, to find the current versions in use and the updates available. It then opens pull/merge requests with updated version pins. For breaking changes, Patcher applies patches where available, or generates guidance files when manual action is required. This gives teams that use many reusable modules a repeatable process for staying current instead of tracking updates manually.
