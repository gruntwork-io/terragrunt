---
question: "How can I ensure infrastructure changes have proper approval?"
title: "How can I ensure infrastructure changes have proper approval? | Terragrunt"
description: "Pipelines tags each cloud auth session with the change that initiated it and posts plan outputs on PRs, pairing with branch protection for an audit trail."
category: "terragrunt-scale"
order: 10
---

Pipelines creates a clear audit trail of infrastructure changes by tagging each cloud authentication session with unique identifying information of what change initiated that session. In addition, PRs with plan outputs that can be reviewed and approved before merging. Both GitHub and GitLab also provide branch protection rules that require approval before merges. This provides an audit trail of requested changes and approval.
