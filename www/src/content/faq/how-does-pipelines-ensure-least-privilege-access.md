---
question: "How does Pipelines ensure least privilege access?"
title: "How does Pipelines ensure least privilege access? | Terragrunt"
description: "Access is segmented by least-privilege principals per environment, with read-only roles for open PRs and read-write roles used only after review, on merge."
category: "terragrunt-scale"
order: 4
---

Access control to infrastructure is segmented by least privilege principals (e.g. AWS IAM roles) that only have access to individual environments, further segmented by a different principal used for read-only access during pull requests / merge requests open and a principal used for read-write access during pull request merge.

As a consequence, updates to a particular environment will only be done by a principal that only has access to that environment, and can only be used after review by your team.
