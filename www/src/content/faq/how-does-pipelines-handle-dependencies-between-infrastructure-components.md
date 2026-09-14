---
question: "How does Pipelines handle dependencies between infrastructure components?"
title: "How does Pipelines handle dependencies between infrastructure components? | Terragrunt"
description: "Pipelines drives updates through Terragrunt's dependency graph (DAG): dependencies update before dependents, and destroys run in reverse order across units."
category: "terragrunt-scale"
order: 2
---

Pipelines natively integrates with the Terragrunt CLI to drive infrastructure updates through the Directed Acyclic Graph (DAG). When creating or updating multiple infrastructure components, dependencies will be created or updated before dependents (e.g. VPCs before servers), and when deleting infrastructure components, dependents will be destroyed before dependencies (e.g. servers before VPCs). This ordering is applied even if infrastructure is deployed through different units.
