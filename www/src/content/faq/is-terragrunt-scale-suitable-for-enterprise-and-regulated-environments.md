---
question: "Is Terragrunt Scale suitable for enterprise and regulated environments?"
title: "Is Terragrunt Scale suitable for enterprise and regulated environments? | Terragrunt"
description: "Is Terragrunt Scale suitable for enterprise and regulated environments?"
category: "terragrunt-scale"
order: 24
---

Yes. [Terragrunt Scale](/terragrunt-scale) was designed with a strong security posture from day one, which makes it a fit for enterprise and regulated environments. There is no SaaS control plane: all operations run inside your own runners and repositories using standard GitHub Actions or GitLab CI pipelines, and it can run fully self-hosted on self-hosted GitLab or GitHub Enterprise Server. Your state lives in your own cloud (for example, an S3 bucket), and authentication uses OIDC to obtain temporary, short-lived credentials at runtime, so no cloud credentials are stored in the pipeline. Access follows the principle of least privilege, changes go through pull/merge-request review with branch-protection-enforced approvals, and you get a complete audit trail of who changed what, when, and why. Terragrunt Scale does not hold compliance certifications of its own: your code and data live in your SCM, so the security model is inherited from GitHub or GitLab.
