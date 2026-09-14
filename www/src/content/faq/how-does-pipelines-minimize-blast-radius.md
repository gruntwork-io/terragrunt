---
question: "How does Pipelines minimize blast radius?"
title: "How does Pipelines minimize blast radius? | Terragrunt"
description: "Pipelines uses Terragrunt state isolation so infrastructure is only updated when a corresponding code change actually impacts that specific resource."
category: "terragrunt-scale"
order: 5
---

Pipelines takes full advantage of state isolation using Terragrunt to ensure that infrastructure is only updated if a corresponding code-change would impact that resource.
