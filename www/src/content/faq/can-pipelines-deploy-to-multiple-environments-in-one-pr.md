---
question: "Can Pipelines deploy to multiple environments in one PR?"
title: "Can Pipelines deploy to multiple environments in one PR? | Terragrunt"
description: "Yes. A single PR can change infrastructure in any number of environments (e.g. AWS accounts), and the pipeline updates them all concurrently on merge."
category: "terragrunt-scale"
order: 3
---

Yes, you can make changes to infrastructure components within any number of different environments (e.g. AWS accounts) at the same time in the same PR, and the pipeline will update them concurrently on merge.
