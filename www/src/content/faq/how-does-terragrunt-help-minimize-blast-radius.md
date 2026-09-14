---
question: "How does Terragrunt help minimize blast radius?"
title: "How does Terragrunt help minimize blast radius? | Terragrunt"
description: "Terragrunt isolates changes into independently operable units with their own state, grouped into stacks, so plans and applies touch smaller parts of the system."
category: "terragrunt"
order: 1
---

Terragrunt organizes infrastructure into [units](https://docs.terragrunt.com/features/units/) (directories containing a terragrunt.hcl file that are independently operable, atomic, and reproducible), so changes are isolated to smaller portions of the system instead of applying to one large monolithic OpenTofu/Terraform root module. Because each unit has its own state and deployment boundary, teams can plan, apply, and troubleshoot smaller parts of the system independently. Related units are grouped into [stacks](https://docs.terragrunt.com/features/stacks/), which help control the blast radius of changes and manage dependencies between units. Terragrunt's [Run Queue](https://docs.terragrunt.com/features/stacks/run-queue/) runs operations in a safe order across the dependency graph.
