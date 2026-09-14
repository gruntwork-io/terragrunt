---
question: "How do Terragrunt units and stacks work, and when should I use each?"
title: "How do Terragrunt units and stacks work, and when should I use each? | Terragrunt"
description: "How do Terragrunt units and stacks work, and when should I use each?"
category: "terragrunt"
order: 6
---

A Terragrunt [unit](https://docs.terragrunt.com/features/units/) is a directory containing a terragrunt.hcl file. It is the smallest deployable entity in Terragrunt and is intended to be independently operable, atomic, and reproducible, so teams can isolate infrastructure changes to smaller portions of the system instead of applying changes to a large monolithic OpenTofu/Terraform root module. A [stack](https://docs.terragrunt.com/features/stacks/) is a collection of related units managed together: stacks let teams deploy multiple infrastructure components with a single command, manage dependencies between units, control the blast radius of changes, and organize infrastructure into logical groups. Terragrunt supports both implicit stacks, based on directory structure, and explicit stacks, defined with terragrunt.stack.hcl files. Use units to keep each piece of infrastructure independently deployable; use stacks when you need to operate a group of related units as one.
