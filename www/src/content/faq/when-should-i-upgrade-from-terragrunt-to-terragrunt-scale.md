---
question: "When should I upgrade from open source Terragrunt to Terragrunt Scale?"
title: "When should I upgrade from open source Terragrunt to Terragrunt Scale? | Terragrunt"
description: "When should I upgrade from open source Terragrunt to Terragrunt Scale?"
category: "terragrunt-scale"
order: 25
---

Open source Terragrunt is the orchestration and configuration layer for running OpenTofu/Terraform at scale. It is and always will be free to use. [Terragrunt Scale](/terragrunt-scale) is a commercial, Terragrunt-native, SCM-hosted or self-hosted CI/CD pipeline including patching and drift management, built by the creators of Terragrunt, that automates Terragrunt workflows across teams, repositories, environments, and cloud accounts. Consider Terragrunt Scale when you want plans to run automatically on pull/merge requests and applies to run on merge (Pipelines), scheduled and on-demand drift scans that open PRs/MRs when deployed resources no longer match your IaC (Drift Detection), and automated dependency updates for your modules and units/stacks (Patcher). That applies whether you have not built this automation yet, you built it in-house and its upkeep now consumes significant engineering time, or you adopted another solution and are hitting its scaling limits. There is a free tier, so you can adopt it incrementally: the Free and Team tiers are limited by the number of Terragrunt Units being deployed, with no meter and no resources-under-management (RUM) pricing. Enterprises receive custom pricing.
