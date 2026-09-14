---
question: "How does Terragrunt keep configuration DRY across environments?"
title: "How does Terragrunt keep configuration DRY across environments? | Terragrunt"
description: "Terragrunt lets you define shared configuration once and inherit it across units, so backend settings, inputs, and conventions are not repeated per environment."
category: "terragrunt"
order: 3
---

Terragrunt allows shared configuration to be defined once and [inherited across units](https://docs.terragrunt.com/features/units/includes/). Common backend settings, provider configuration, module inputs, and environment conventions can be centralized so the same code is not repeated across development, staging, and production. This is especially useful when the same infrastructure pattern, such as a VPC, cluster, database, and application stack, must be deployed across multiple environments. You define the common configuration once and override only the values that differ by environment.
