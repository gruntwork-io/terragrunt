---
question: "Does Terragrunt Scale support Terragrunt Stacks?"
title: "Does Terragrunt Scale support Terragrunt Stacks? | Terragrunt"
description: "Yes. Terragrunt Scale understands units, stacks, and DAGs: Pipelines runs updates in dependency order, and Drift Detection and Patcher support stacks too."
category: "terragrunt-scale"
order: 18
---

Yes. [Terragrunt Scale](/terragrunt-scale) understands Terragrunt [units](https://docs.terragrunt.com/features/units/), [stacks](https://docs.terragrunt.com/features/stacks/), and DAGs. Pipelines executes updates through the dependency graph so adds, changes, and destroys trigger in the right order, and it plans and applies only the affected units to reduce blast radius. [Drift Detection](https://docs.terragrunt.com/terragrunt-scale/drift-detection/) supports stacks too: it opens a PR/MR for drifted stacks when deployed cloud resources no longer match your IaC. Patcher scans Terragrunt units and stacks, and OpenTofu/Terraform module references, to find current versions and available updates, and opens PRs/MRs with updated version pins.
