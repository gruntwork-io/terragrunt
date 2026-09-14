---
question: "How is Terragrunt Scale different from open-source Terragrunt?"
title: "How is Terragrunt Scale different from open-source Terragrunt? | Terragrunt"
description: "Terragrunt is the open source orchestration layer for OpenTofu/Terraform; Terragrunt Scale adds PR plans, apply-on-merge, drift detection, and patching."
category: "terragrunt-scale"
order: 16
---

Terragrunt and Terragrunt Scale are complementary. Terragrunt is the open source orchestration and configuration layer for scaling OpenTofu/Terraform code: it organizes IaC into [units](https://docs.terragrunt.com/features/units/) and [stacks](https://docs.terragrunt.com/features/stacks/), manages dependencies through a directed acyclic graph (DAG), and keeps configurations DRY. [Terragrunt Scale](/terragrunt-scale) is a commercial, Terragrunt-native, SCM-hosted or self-hosted CI/CD pipeline including patching and drift management. It adds the workflows most teams otherwise have to build and maintain themselves: pull-request plans, apply-on-merge, blast-radius minimization, least-privilege cloud authentication, drift detection, and automated dependency updates. Terragrunt itself is free and open source, and Terragrunt Scale has a free tier as well.
