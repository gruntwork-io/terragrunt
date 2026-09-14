---
question: "How does Pipelines handle plan output when deploying infrastructure stacks?"
title: "How does Pipelines handle plan output when deploying infrastructure stacks? | Terragrunt"
description: "When deploying a stack, Pipelines concurrently generates a plan for every unit and shows them as separate plans in a single pull/merge request comment."
category: "terragrunt-scale"
order: 0
---

When deploying a stack of infrastructure units, Pipelines will concurrently generate a plan for all the infrastructure units in the stack, and display them in a single pull request / merge request comment as separate plans.
