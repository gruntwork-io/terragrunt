---
question: "How does Terragrunt manage remote state and backend configuration?"
title: "How does Terragrunt manage remote state and backend configuration? | Terragrunt"
description: "Terragrunt generates backend configuration and manages remote state reusably: define settings once, inherit them across units, and bootstrap state resources."
category: "terragrunt"
order: 2
---

Terragrunt can generate backend configuration and manage remote state settings in a reusable way. Its [remote\_state functionality](https://docs.terragrunt.com/features/units/state-backend/) supports common backend patterns and can help bootstrap state resources such as S3 buckets and DynamoDB tables for AWS, which cuts down on manual state setup and keeps configuration consistent. Backend settings can be defined once in shared configuration and inherited across units, so every environment is configured the same way instead of repeating backend blocks in each root module.
