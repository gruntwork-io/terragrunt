---
question: "Does Terragrunt Scale integrate with secret managers?"
title: "Does Terragrunt Scale integrate with secret managers? | Terragrunt"
description: "Yes. Workflows use your CI/CD platform's OIDC credentials to reach any secret manager; Terragrunt Scale never holds access to your cloud accounts or state."
category: "terragrunt-scale"
order: 22
---

Yes. [Terragrunt Scale](/terragrunt-scale) can integrate with any secret manager you want: your workflows use the OIDC credentials provided by your CI/CD platform to call cloud APIs and access your own secret manager. For cloud authentication, OIDC handshakes provide temporary, least-privilege credentials instead of long-lived secrets, and Terragrunt Scale never holds direct access to your cloud accounts or state files.
