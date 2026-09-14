---
question: "Does Pipelines support both monorepos and polyrepos?"
title: "Does Pipelines support both monorepos and polyrepos? | Terragrunt"
description: "Both are supported. OIDC-based credentials let you split infrastructure across repos or manage it all in one; Pipelines scales to large monorepo structures."
category: "terragrunt-scale"
order: 8
---

Both are supported.

Usage of OIDC-based credential acquisition means that repositories are trusted to assume particular credentials in target environments. You can delegate management of limited infrastructure to separate repositories, or have all your infrastructure managed in one repository. Pipelines and Terragrunt are designed to scale to very large and sophisticated monorepo structures.
