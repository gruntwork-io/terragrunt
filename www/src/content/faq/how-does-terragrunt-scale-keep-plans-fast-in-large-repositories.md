---
question: "How does Terragrunt Scale keep plans fast in large repositories?"
title: "How does Terragrunt Scale keep plans fast in large repositories? | Terragrunt"
description: "Pipelines plans only the smallest set of units affected by a change, runs independent units concurrently, and applies on merge in dependency order."
category: "terragrunt-scale"
order: 19
---

[Pipelines](/terragrunt-scale) identifies the smallest set of units affected by a change and runs plans only for those, rather than the whole repository, to minimize blast radius. It supports concurrent runs for independent units, posts results back to the PR/MR, and applies on merge in dependency order, including multi-environment changes. Plan summaries and logs appear directly in PR/MR comments.
