---
question: "How does authentication work with Pipelines?"
title: "How does authentication work with Pipelines? | Terragrunt"
description: "Pipelines authenticates via a GitHub App or GitLab machine user, then uses an OIDC handshake to acquire temporary least-privilege cloud credentials per run."
category: "terragrunt-scale"
order: 9
---

Pipelines uses a GitHub app in GitHub and a GitLab machine user in GitLab to authenticate with the respective Source Code Management (SCM) provider.

When authenticating with cloud providers, Pipelines will perform an OIDC handshake between the repository in the SCM and a principal in a given environment to acquire temporary, least privilege credentials for the actions it needs to perform in that environment. Pipelines also provides an escape-hatch mechanism which allows developers to implement a per-environment custom authentication mechanism to authenticate with arbitrary APIs and pass those credentials into Terragrunt at runtime.
