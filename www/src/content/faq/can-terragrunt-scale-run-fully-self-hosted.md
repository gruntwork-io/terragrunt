---
question: "Can Terragrunt Scale run fully self-hosted?"
title: "Can Terragrunt Scale run fully self-hosted? | Terragrunt"
description: "Yes. Terragrunt Scale runs in your own runners and repositories via GitHub Actions or GitLab CI, including GitHub Enterprise Server and self-hosted GitLab."
category: "terragrunt-scale"
order: 21
---

Yes. [Terragrunt Scale](/terragrunt-scale) runs in your existing CI/CD: all operations run inside your own runners and repositories using standard GitHub Actions or GitLab CI pipelines. Plans and applies execute in your runners, not on Gruntwork servers. It supports GitHub, GitLab, GitHub Enterprise, and GitLab Self-Managed for version control, and it works with self-hosted GitLab and GitHub Enterprise Server.
