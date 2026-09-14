---
question: "What is the workflow for getting infrastructure changes approved?"
title: "What is the workflow for getting infrastructure changes approved? | Terragrunt"
description: "On each PR, Pipelines plans the affected units and posts a clear summary plus full plan output in comments; merging kicks off the CI/CD pipeline to apply."
category: "terragrunt-scale"
order: 6
---

When you create a PR, Pipelines will run a plan on units affected by the code change. The plan output shows in the PR comments with: (1) a clear plan summary of what specific infrastructure resources were impacted, and (2) a full plan output of what's changing. When you merge the PR, it kicks off the CI/CD pipeline to apply it.
