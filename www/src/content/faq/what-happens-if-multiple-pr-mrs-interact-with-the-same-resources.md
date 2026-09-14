---
question: "What happens if multiple PR/MRs interact with the same resources?"
title: "What happens if multiple PR/MRs interact with the same resources? | Terragrunt"
description: "Pipelines applies after merge, so IaC on the deploy branch stays the single source of truth; branch protection keeps plans recent and applies predictable."
category: "terragrunt-scale"
order: 1
---

Pipelines uses an "apply after merge" approach. This means that while multiple PRs are proposing changes to the same resources, there’s only ever a single source of truth for the infrastructure that will be provisioned (the Infrastructure as Code on the deploy branch e.g. main). During initial implementation we guide teams to using features of their SCM platform to enforce branch protection rules that ensure that plans are recent and applies are predictable.
