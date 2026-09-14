---
question: "How does Terragrunt manage dependencies between infrastructure units?"
title: "How does Terragrunt manage dependencies between infrastructure units? | Terragrunt"
description: "Terragrunt's Run Queue orders runs across units with a DAG: dependencies run first for plan and apply, destroys run in safe reverse order, with concurrency."
category: "terragrunt"
order: 4
---

Terragrunt's [Run Queue](https://docs.terragrunt.com/features/stacks/run-queue/) controls ordering and concurrency when running OpenTofu/Terraform commands across multiple units. It uses a directed acyclic graph (DAG) built from the dependencies between units, so dependencies run before dependent units for operations such as plan or apply, and destroy operations occur in a safe reverse order, while still allowing safe concurrency where possible. This automates common ordering requirements such as networks before clusters, IAM before services, and databases before applications.
