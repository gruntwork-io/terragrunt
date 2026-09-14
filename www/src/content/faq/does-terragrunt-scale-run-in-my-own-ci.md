---
question: "Does Terragrunt Scale run in my own CI, or is it a hosted SaaS?"
title: "Does Terragrunt Scale run in my own CI, or is it a hosted SaaS? | Terragrunt"
description: "Terragrunt Scale runs in your existing CI/CD: plans and applies execute in your own GitHub Actions or GitLab CI runners, never on Gruntwork servers."
category: "terragrunt-scale"
order: 17
---

[Terragrunt Scale](/terragrunt-scale) runs in your existing CI/CD. All operations run inside your own runners and repositories using standard GitHub Actions or GitLab CI pipelines. Plans and applies execute in your runners, not on Gruntwork servers. Terragrunt Scale never holds direct access to your cloud accounts or state files, and OIDC handshakes provide temporary, least-privilege credentials instead of long-lived secrets. It supports GitHub, GitLab, GitHub Enterprise, and GitLab Self-Managed for version control.
