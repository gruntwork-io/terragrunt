---
question: "What are Terragrunt feature flags?"
title: "What are Terragrunt feature flags? | Terragrunt"
description: "What are Terragrunt feature flags?"
category: "terragrunt"
order: 7
---

Terragrunt has first-class support for feature flags. Feature flags improve the speed and safety with which engineers can integrate their IaC: they let engineers integrate incomplete work without introducing undue risk to the infrastructure they manage, decouple infrastructure release from deployment, and codify important information about the evolution of IaC. They were built for teams operating Terragrunt at large scale, where an error in one unit would otherwise prevent its dependents (and their dependents) from running, so that potentially risky changes can be introduced without slowing down code integration across long dependency chains. See the [Terragrunt documentation](https://docs.terragrunt.com/features/units/runtime-control/) for configuration details and common patterns.
